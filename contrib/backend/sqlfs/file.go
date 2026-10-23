package sqlfs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"path"
	"time"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/utils"
)

// fileToken records what a handle observed about its file, used to detect conflicting changes at publish time.
type fileToken struct {
	exists bool
	row    fileRow
}

type readBuf struct {
	rev   string
	idx   int64
	data  []byte
	valid bool
}

type writeBuf struct {
	idx   int64
	data  []byte
	valid bool
	dirty bool
}

// File implements vfs.File.
//
// A File is not safe for concurrent use by multiple goroutines. Content written through a File is staged in the
// database and becomes visible only when Close succeeds. Call Abort to discard staged content instead.
type File struct {
	location *Location
	path     string
	opts     []options.NewFileOption

	cursor     int64
	readCalled bool
	seekCalled bool

	pinned bool
	tok    fileToken
	rbuf   readBuf

	writing    bool
	staged     string
	stagedSize int64
	stagedCS   int
	lastBeat   int64 // nanoseconds of the last staged-revision heartbeat
	wbuf       writeBuf
}

var _ vfs.File = (*File)(nil)

func (f *File) fsys() *FileSystem { return f.location.fileSystem }
func (f *File) ns() string        { return f.location.authority.String() }
func (f *File) dir() string       { return utils.EnsureTrailingSlash(path.Dir(f.path)) }
func (f *File) base() string      { return path.Base(f.path) }

func (f *File) notFound() error { return fmt.Errorf("%w: %s", ErrNotFound, f.URI()) }

func (f *File) reset() {
	f.cursor = 0
	f.readCalled, f.seekCalled = false, false
	f.pinned, f.tok = false, fileToken{}
	f.rbuf, f.wbuf = readBuf{}, writeBuf{}
	f.writing, f.staged, f.stagedSize, f.stagedCS = false, "", 0, 0
}

// view returns the revision the handle currently reads from: its staged content if it has written, otherwise the
// published revision it pinned.
func (f *File) view() (rev string, size int64, chunkSize int) {
	if f.writing {
		return f.staged, f.stagedSize, f.stagedCS
	}
	return f.tok.row.revisionID, f.tok.row.size, f.tok.row.chunkSize
}

// pin fixes the published revision the handle reads from. It fails if the file does not exist.
func (f *File) pin() error {
	if f.writing || f.pinned {
		return nil
	}
	row, err := f.fsys().lookupFile(context.Background(), f.fsys().q, f.ns(), f.dir(), f.base())
	if err != nil {
		return f.fsys().wrapErr("read "+f.URI(), err)
	}
	if row == nil {
		return f.notFound()
	}
	f.tok, f.pinned = fileToken{exists: true, row: *row}, true
	return nil
}

// Read implements io.Reader. Content is read chunk by chunk from the revision pinned when reading began, so a
// concurrent replacement never mixes old and new bytes.
func (f *File) Read(p []byte) (int, error) {
	if f.writing {
		if err := f.flush(); err != nil {
			return 0, utils.WrapReadError(f.fsys().wrapErr("read "+f.URI(), err))
		}
	} else if err := f.pin(); err != nil {
		return 0, utils.WrapReadError(err)
	}
	f.readCalled = true

	rev, size, cs := f.view()
	n := 0
	for n < len(p) && f.cursor < size {
		chunk, err := f.readChunk(rev, f.cursor/int64(cs), size, cs)
		if err != nil {
			return n, utils.WrapReadError(f.fsys().wrapErr("read "+f.URI(), err))
		}
		c := copy(p[n:], chunk[f.cursor%int64(cs):])
		n += c
		f.cursor += int64(c)
	}
	if n == 0 && len(p) > 0 {
		return 0, io.EOF
	}
	return n, nil
}

// readChunk returns chunk idx of rev, zero-padded to the length the revision's size implies.
func (f *File) readChunk(rev string, idx, size int64, cs int) ([]byte, error) {
	if f.rbuf.valid && f.rbuf.rev == rev && f.rbuf.idx == idx {
		return f.rbuf.data, nil
	}
	fsys := f.fsys()
	ctx, cancel := fsys.opCtx(context.Background())
	defer cancel()

	var data []byte
	err := fsys.q.QueryRowContext(ctx, fsys.qs.selChunk, rev, idx).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		var one int
		switch err := fsys.q.QueryRowContext(ctx, fsys.qs.selRevExists, rev).Scan(&one); {
		case errors.Is(err, sql.ErrNoRows):
			return nil, fmt.Errorf("%w: content was reclaimed while open", ErrConflict)
		case err != nil:
			return nil, err
		}
		data = nil
	} else if err != nil {
		return nil, err
	}

	expected := int(min(int64(cs), size-idx*int64(cs)))
	if len(data) > expected {
		return nil, fmt.Errorf("%w: chunk %d of %s holds %d bytes, expected at most %d", ErrCorrupt, idx, f.URI(), len(data), expected)
	}
	if len(data) < expected {
		data = append(data, make([]byte, expected-len(data))...)
	}
	f.rbuf = readBuf{rev: rev, idx: idx, data: data, valid: true}
	return data, nil
}

// Seek implements io.Seeker. Seeking a file that does not exist and has not been written returns an error.
func (f *File) Seek(offset int64, whence int) (int64, error) {
	if err := f.pin(); err != nil {
		return 0, utils.WrapSeekError(err)
	}
	_, size, _ := f.view()
	pos, err := utils.SeekTo(size, f.cursor, offset, whence)
	if err != nil {
		return 0, utils.WrapSeekError(err)
	}
	f.cursor, f.seekCalled = pos, true
	return pos, nil
}

// beginWrite creates the staged revision for the first Write. A write that follows a Read or Seek on an existing
// file modifies a copy of its content; any other write replaces the content.
func (f *File) beginWrite() error {
	fsys := f.fsys()
	overlay := f.pinned && f.tok.exists && (f.readCalled || f.seekCalled)
	if !f.pinned {
		row, err := fsys.lookupFile(context.Background(), fsys.q, f.ns(), f.dir(), f.base())
		if err != nil {
			return fsys.wrapErr("write "+f.URI(), err)
		}
		f.tok = fileToken{}
		if row != nil {
			f.tok = fileToken{exists: true, row: *row}
		}
	}

	id, cs, size := newID(), fsys.chunkSize, int64(0)
	if overlay {
		cs, size = f.tok.row.chunkSize, f.tok.row.size
	}
	now := nowNanos()
	err := fsys.inTx(context.Background(), "begin write "+f.URI(), func(ctx context.Context, tx txn) error {
		if overlay {
			var one int
			err := tx.QueryRowContext(ctx, fsys.qs.selRevExists, f.tok.row.revisionID).Scan(&one)
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: content was reclaimed while open", ErrConflict)
			}
			if err != nil {
				return err
			}
		}
		expires := now + fsys.stagingRetention.Nanoseconds()
		if _, err := tx.ExecContext(ctx, fsys.qs.insRevision, id, stateStaged, size, cs, now, expires); err != nil {
			return err
		}
		if overlay {
			if _, err := tx.ExecContext(ctx, fsys.qs.copyChunks, id, f.tok.row.revisionID); err != nil {
				return err
			}
			// Cleanup removes a revision's chunks and row together, so if the source row still exists after the copy,
			// the copy saw all of its chunks.
			return revisionStillExists(ctx, tx, fsys, f.tok.row.revisionID, ErrConflict)
		}
		return nil
	})
	if err != nil {
		return err
	}
	f.writing, f.staged, f.stagedSize, f.stagedCS, f.lastBeat = true, id, size, cs, now
	f.wbuf, f.rbuf = writeBuf{}, readBuf{}
	return nil
}

// Write implements io.Writer. Writes are staged in the database and are not visible until Close.
func (f *File) Write(p []byte) (int, error) {
	if !f.writing {
		if err := f.beginWrite(); err != nil {
			return 0, utils.WrapWriteError(err)
		}
	}
	fsys := f.fsys()
	if limit := fsys.maxFileSize; limit > 0 && f.cursor+int64(len(p)) > limit {
		return 0, utils.WrapWriteError(fmt.Errorf("sqlfs: write to %s would exceed the maximum file size of %d bytes", f.URI(), limit))
	}

	if f.stagedCS < MinChunkSize || f.stagedCS > MaxChunkSize {
		return 0, utils.WrapWriteError(fmt.Errorf("%w: invalid chunk size %d for %s", ErrCorrupt, f.stagedCS, f.URI()))
	}
	cs := int64(f.stagedCS)
	written := 0
	for written < len(p) {
		idx, off := f.cursor/cs, int(f.cursor%cs)
		if err := f.loadWriteChunk(idx); err != nil {
			return written, utils.WrapWriteError(fsys.wrapErr("write "+f.URI(), err))
		}
		n := min(len(p)-written, f.stagedCS-off)
		if need := off + n; len(f.wbuf.data) < need {
			f.wbuf.data = append(f.wbuf.data, make([]byte, need-len(f.wbuf.data))...)
		}
		copy(f.wbuf.data[off:off+n], p[written:written+n])
		f.wbuf.dirty = true
		written += n
		f.cursor += int64(n)
		f.stagedSize = max(f.stagedSize, f.cursor)
	}
	f.rbuf.valid = false
	// Renew the staged revision's expiry on a cadence tied to the retention period, even when writes only hit the
	// buffered chunk, so that retention measures idle time rather than the interval between flushes.
	if now := nowNanos(); now-f.lastBeat >= fsys.stagingRetention.Nanoseconds()/4 {
		if err := f.heartbeat(now); err != nil {
			return written, utils.WrapWriteError(fsys.wrapErr("write "+f.URI(), err))
		}
	}
	return written, nil
}

// heartbeat extends the staged revision's expiry. It fails with ErrConflict if Cleanup already reclaimed it.
func (f *File) heartbeat(now int64) error {
	fsys := f.fsys()
	n, err := fsys.exec(context.Background(), fsys.qs.heartbeat, now+fsys.stagingRetention.Nanoseconds(), f.staged)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: staged content was reclaimed", ErrConflict)
	}
	f.lastBeat = now
	return nil
}

// revisionStillExists fails with err (wrapped with context) if the revision row is gone.
func revisionStillExists(ctx context.Context, tx txn, fsys *FileSystem, id string, sentinel error) error {
	var one int
	err := tx.QueryRowContext(ctx, fsys.qs.selRevExists, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: source content was reclaimed during the operation", sentinel)
	}
	return err
}

// loadWriteChunk makes chunk idx of the staged revision the write buffer, flushing the previous one if needed.
func (f *File) loadWriteChunk(idx int64) error {
	if f.wbuf.valid && f.wbuf.idx == idx {
		return nil
	}
	if err := f.flush(); err != nil {
		return err
	}
	fsys := f.fsys()
	ctx, cancel := fsys.opCtx(context.Background())
	defer cancel()

	var data []byte
	err := fsys.q.QueryRowContext(ctx, fsys.qs.selChunk, f.staged, idx).Scan(&data)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	f.wbuf = writeBuf{idx: idx, data: data, valid: true}
	return nil
}

// flush persists the dirty write buffer and extends the staged revision's expiry, which acts as the writer's
// heartbeat for Cleanup.
func (f *File) flush() error {
	if !f.wbuf.valid || !f.wbuf.dirty {
		return nil
	}
	fsys := f.fsys()
	if err := f.heartbeat(nowNanos()); err != nil {
		return err
	}
	if _, err := fsys.exec(context.Background(), fsys.qs.upsertChunk, f.staged, f.wbuf.idx, f.wbuf.data); err != nil {
		return err
	}
	f.wbuf.dirty = false
	f.rbuf.valid = false
	return nil
}

// Close publishes any staged writes atomically and resets the handle. If publication fails, the previously
// published content is unchanged and the handle keeps its staged writes; call Abort to discard them.
func (f *File) Close() error {
	if f.writing {
		if err := f.publish(); err != nil {
			return utils.WrapCloseError(err)
		}
	}
	f.reset()
	return nil
}

func (f *File) publish() error {
	fsys := f.fsys()
	if err := f.flush(); err != nil {
		return fsys.wrapErr("close "+f.URI(), err)
	}
	size, fileID := f.stagedSize, newID()
	return fsys.inTx(context.Background(), "close "+f.URI(), func(ctx context.Context, tx txn) error {
		now := nowNanos()
		n, err := execRows(ctx, tx, fsys.qs.publishRevision, size, f.staged)
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("%w: staged content for %s was reclaimed", ErrConflict, f.URI())
		}
		if f.tok.exists {
			n, err = execRows(ctx, tx, fsys.qs.updFilePublish, f.staged, size, now, f.tok.row.id, f.tok.row.generation)
			if err != nil {
				return err
			}
			if n == 0 {
				return fmt.Errorf("%w: %s was changed or deleted since it was opened", ErrConflict, f.URI())
			}
			if _, err := tx.ExecContext(ctx, fsys.qs.supersede, now, f.tok.row.revisionID); err != nil {
				return err
			}
		} else if _, err := tx.ExecContext(ctx, fsys.qs.insFile, fileID, f.ns(), f.dir(), f.base(), f.staged, size, now, now); err != nil {
			if fsys.isUnique(err) {
				return fmt.Errorf("%w: %w: %s", ErrConflict, ErrAlreadyExists, f.URI())
			}
			return err
		}
		return nil
	})
}

// Abort discards staged writes without publishing them and resets the handle. It is safe to call when nothing has
// been written.
func (f *File) Abort() error {
	var err error
	if f.writing {
		fsys := f.fsys()
		staged := f.staged
		err = fsys.inTx(context.Background(), "abort "+f.URI(), func(ctx context.Context, tx txn) error {
			if _, err := tx.ExecContext(ctx, fsys.qs.delChunks, staged); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, fsys.qs.delRevision, staged)
			return err
		})
	}
	f.reset()
	return err
}

func execRows(ctx context.Context, e Execer, query string, args ...any) (int64, error) {
	res, err := e.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Exists reports whether published content exists at the path.
func (f *File) Exists() (bool, error) {
	row, err := f.fsys().lookupFile(context.Background(), f.fsys().q, f.ns(), f.dir(), f.base())
	if err != nil {
		return false, utils.WrapExistsError(f.fsys().wrapErr("exists "+f.URI(), err))
	}
	return row != nil, nil
}

// Location returns the file's location.
func (f *File) Location() vfs.Location { return f.location }

// Size returns the published size in bytes.
func (f *File) Size() (uint64, error) {
	row, err := f.published()
	if err != nil {
		return 0, utils.WrapSizeError(err)
	}
	return uint64(row.size), nil
}

// LastModified returns the published modification time.
func (f *File) LastModified() (*time.Time, error) {
	row, err := f.published()
	if err != nil {
		return nil, utils.WrapLastModifiedError(err)
	}
	t := time.Unix(0, row.modifiedAt)
	return &t, nil
}

func (f *File) published() (*fileRow, error) {
	row, err := f.fsys().lookupFile(context.Background(), f.fsys().q, f.ns(), f.dir(), f.base())
	if err != nil {
		return nil, f.fsys().wrapErr("stat "+f.URI(), err)
	}
	if row == nil {
		return nil, f.notFound()
	}
	return row, nil
}

// Touch creates an empty file if none exists, otherwise updates its modification time.
func (f *File) Touch() error {
	fsys := f.fsys()
	err := fsys.inTx(context.Background(), "touch "+f.URI(), func(ctx context.Context, tx txn) error {
		row, err := fsys.lookupFile(ctx, tx, f.ns(), f.dir(), f.base())
		if err != nil {
			return err
		}
		now := nowNanos()
		if row != nil {
			_, err := tx.ExecContext(ctx, fsys.qs.touchFile, now, row.id)
			return err
		}
		rev := newID()
		if _, err := tx.ExecContext(ctx, fsys.qs.insRevision, rev, statePublished, 0, fsys.chunkSize, now, nil); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, fsys.qs.insFile, newID(), f.ns(), f.dir(), f.base(), rev, 0, now, now); err != nil {
			if fsys.isUnique(err) {
				return errRetry
			}
			return err
		}
		return nil
	})
	if err != nil {
		return utils.WrapTouchError(err)
	}
	return nil
}

// Delete removes the file. Deleting a file that does not exist returns an error wrapping fs.ErrNotExist.
func (f *File) Delete(_ ...options.DeleteOption) error {
	fsys := f.fsys()
	err := fsys.inTx(context.Background(), "delete "+f.URI(), func(ctx context.Context, tx txn) error {
		row, err := fsys.lookupFile(ctx, tx, f.ns(), f.dir(), f.base())
		if err != nil {
			return err
		}
		if row == nil {
			return f.notFound()
		}
		n, err := execRows(ctx, tx, fsys.qs.delFile, row.id, row.generation)
		if err != nil {
			return err
		}
		if n == 0 {
			return errRetry
		}
		_, err = tx.ExecContext(ctx, fsys.qs.supersede, nowNanos(), row.revisionID)
		return err
	})
	if err != nil {
		return utils.WrapDeleteError(err)
	}
	f.reset()
	return nil
}

// Path returns the absolute file path.
func (f *File) Path() string { return f.path }

// Name returns the file's base name.
func (f *File) Name() string { return f.base() }

// URI returns the file URI.
func (f *File) URI() string { return utils.GetFileURI(f) }

// String implements fmt.Stringer.
func (f *File) String() string { return f.URI() }

// sameInstance returns other as a *File when it belongs to the same FileSystem instance as f.
func (f *File) sameInstance(other vfs.File) (*File, bool) {
	o, ok := other.(*File)
	return o, ok && o.location.fileSystem == f.location.fileSystem
}

// prepareTransfer readies f as the source of a copy or move: pending writes are published and, if requested, the
// cursor must be at the start.
func (f *File) prepareTransfer(requireStart bool) error {
	if f.writing {
		return f.Close()
	}
	if requireStart {
		return backend.ValidateCopySeekPosition(f)
	}
	return nil
}

// CopyToFile copies the file's content to file, overwriting it, and closes both. The cursor must be at offset 0.
func (f *File) CopyToFile(file vfs.File) error {
	if err := f.prepareTransfer(true); err != nil {
		return utils.WrapCopyToFileError(err)
	}
	if dst, ok := f.sameInstance(file); ok {
		err := f.copyNative(dst)
		f.reset()
		dst.reset()
		if err != nil {
			return utils.WrapCopyToFileError(err)
		}
		return nil
	}
	if err := f.streamTo(file); err != nil {
		return utils.WrapCopyToFileError(err)
	}
	return nil
}

// CopyToLocation copies the file into location, keeping its name, and returns the new file.
func (f *File) CopyToLocation(location vfs.Location) (vfs.File, error) {
	dst, err := location.NewFile(f.Name())
	if err != nil {
		return nil, utils.WrapCopyToLocationError(err)
	}
	if err := f.CopyToFile(dst); err != nil {
		return nil, utils.WrapCopyToLocationError(err)
	}
	return dst, nil
}

// MoveToFile moves the file's content to file, overwriting it, and removes the file. Within one FileSystem the
// move is a single atomic metadata change.
func (f *File) MoveToFile(file vfs.File) error {
	if dst, ok := f.sameInstance(file); ok {
		if err := f.prepareTransfer(true); err != nil {
			return utils.WrapMoveToFileError(err)
		}
		err := f.moveNative(dst)
		f.reset()
		dst.reset()
		if err != nil {
			return utils.WrapMoveToFileError(err)
		}
		return nil
	}
	if err := f.prepareTransfer(true); err != nil {
		return utils.WrapMoveToFileError(err)
	}
	// Remember the exact version being copied so that only that version is removed afterwards.
	if err := f.pin(); err != nil {
		return utils.WrapMoveToFileError(err)
	}
	copied := f.tok
	if err := f.streamTo(file); err != nil {
		return utils.WrapMoveToFileError(err)
	}
	if err := f.deleteVersion(copied.row); err != nil {
		return utils.WrapMoveToFileError(err)
	}
	return nil
}

// deleteVersion removes the file only if it is still the given version. If it changed since, nothing is deleted and
// ErrConflict is returned.
func (f *File) deleteVersion(row fileRow) error {
	fsys := f.fsys()
	return fsys.inTx(context.Background(), "delete "+f.URI(), func(ctx context.Context, tx txn) error {
		n, err := execRows(ctx, tx, fsys.qs.delFile, row.id, row.generation)
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("%w: %s changed while it was being moved", ErrConflict, f.URI())
		}
		_, err = tx.ExecContext(ctx, fsys.qs.supersede, nowNanos(), row.revisionID)
		return err
	})
}

// MoveToLocation moves the file into location, keeping its name, and returns the new file.
func (f *File) MoveToLocation(location vfs.Location) (vfs.File, error) {
	dst, err := location.NewFile(f.Name())
	if err != nil {
		return nil, utils.WrapMoveToLocationError(err)
	}
	if err := f.MoveToFile(dst); err != nil {
		return nil, utils.WrapMoveToLocationError(err)
	}
	return dst, nil
}

// streamTo copies content through the application for targets that are not in the same FileSystem instance.
func (f *File) streamTo(file vfs.File) error {
	if err := utils.TouchCopyBuffered(file, f, f.fsys().chunkSize); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return file.Close()
}

// copyNative copies the source's published content to dst entirely inside the database.
func (f *File) copyNative(dst *File) error {
	fsys := f.fsys()
	return fsys.inTx(context.Background(), "copy "+f.URI(), func(ctx context.Context, tx txn) error {
		src, err := fsys.lookupFile(ctx, tx, f.ns(), f.dir(), f.base())
		if err != nil {
			return err
		}
		if src == nil {
			return f.notFound()
		}
		now, rev := nowNanos(), newID()
		if _, err := tx.ExecContext(ctx, fsys.qs.insRevision, rev, statePublished, src.size, src.chunkSize, now, nil); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, fsys.qs.copyChunks, rev, src.revisionID); err != nil {
			return err
		}
		// If Cleanup reclaimed the source between the lookup and the copy, the copy is empty; start over so the
		// lookup finds the current revision (or reports the file gone).
		if err := revisionStillExists(ctx, tx, fsys, src.revisionID, errRetry); err != nil {
			return err
		}
		tgt, err := fsys.lookupFile(ctx, tx, dst.ns(), dst.dir(), dst.base())
		if err != nil {
			return err
		}
		if tgt == nil {
			if _, err := tx.ExecContext(ctx, fsys.qs.insFile, newID(), dst.ns(), dst.dir(), dst.base(), rev, src.size, now, now); err != nil {
				if fsys.isUnique(err) {
					return errRetry
				}
				return err
			}
			return nil
		}
		n, err := execRows(ctx, tx, fsys.qs.updFilePublish, rev, src.size, now, tgt.id, tgt.generation)
		if err != nil {
			return err
		}
		if n == 0 {
			return errRetry
		}
		_, err = tx.ExecContext(ctx, fsys.qs.supersede, now, tgt.revisionID)
		return err
	})
}

// moveNative moves the source's metadata row to dst's path in one transaction.
func (f *File) moveNative(dst *File) error {
	fsys := f.fsys()
	return fsys.inTx(context.Background(), "move "+f.URI(), func(ctx context.Context, tx txn) error {
		src, err := fsys.lookupFile(ctx, tx, f.ns(), f.dir(), f.base())
		if err != nil {
			return err
		}
		if src == nil {
			return f.notFound()
		}
		tgt, err := fsys.lookupFile(ctx, tx, dst.ns(), dst.dir(), dst.base())
		if err != nil {
			return err
		}
		if tgt != nil {
			if tgt.id == src.id {
				return nil
			}
			n, err := execRows(ctx, tx, fsys.qs.delFile, tgt.id, tgt.generation)
			if err != nil {
				return err
			}
			if n == 0 {
				return errRetry
			}
			if _, err := tx.ExecContext(ctx, fsys.qs.supersede, nowNanos(), tgt.revisionID); err != nil {
				return err
			}
		}
		n, err := execRows(ctx, tx, fsys.qs.moveFile, dst.ns(), dst.dir(), dst.base(), src.id, src.generation)
		if err != nil {
			if fsys.isUnique(err) {
				return errRetry
			}
			return err
		}
		if n == 0 {
			return errRetry
		}
		return nil
	})
}
