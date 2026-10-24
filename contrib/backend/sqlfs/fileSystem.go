package sqlfs

import (
	"context"
	cryptorand "crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"math/rand/v2"
	"path"
	"regexp"
	"time"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
)

// Scheme is the URI scheme served by this backend.
const Scheme = "sqlfs"

const name = "SQL FS"

// Revision states.
const (
	stateStaged     = 0
	statePublished  = 1
	stateSuperseded = 2
	// stateReclaiming marks a revision that Cleanup has claimed for deletion; it never persists past the transaction.
	stateReclaiming = 3
)

const (
	maxTxAttempts   = 3
	cleanupBatch    = 100
	tableNameFormat = `^[A-Za-z0-9_]{1,32}$`
)

// statementBytes bounds the chunk data a single bulk copy or delete statement touches, so that each statement of an
// operation on a large file fits within the operation timeout. It is a variable so tests can lower it.
var statementBytes = 64 << 20

var prefixPattern = regexp.MustCompile(tableNameFormat)

var (
	errFileSystemRequired = errors.New("non-nil sqlfs.FileSystem pointer is required")
	errPathRequired       = errors.New("non-empty string for path is required")
)

// FileSystem implements vfs.FileSystem on top of a SQL database.
type FileSystem struct {
	q       querier
	dialect Dialect
	qs      queries

	prefix            string
	chunkSize         int
	maxFileSize       int64
	opTimeout         time.Duration
	stagingRetention  time.Duration
	revisionRetention time.Duration
}

var _ vfs.FileSystem = (*FileSystem)(nil)

// NewFileSystem validates the options and returns a FileSystem. It performs no database I/O: use EnsureSchema or
// ValidateSchema to prepare or check the database. The caller owns the *sql.DB and must close it.
//
// SQL FS does not register itself with the backend registry; call backend.Register(sqlfs.Scheme, fs) to make
// sqlfs:// URIs resolve to the returned instance.
func NewFileSystem(opts ...options.NewFileSystemOption[FileSystem]) (*FileSystem, error) {
	fsys := &FileSystem{
		prefix:            DefaultTablePrefix,
		chunkSize:         DefaultChunkSize,
		opTimeout:         DefaultOperationTimeout,
		stagingRetention:  DefaultStagingRetention,
		revisionRetention: DefaultRevisionRetention,
	}
	options.ApplyOptions(fsys, opts...)

	if err := fsys.validate(); err != nil {
		return nil, err
	}
	fsys.qs = newQueries(fsys.dialect, fsys.prefix)
	return fsys, nil
}

func (fsys *FileSystem) validate() error {
	switch {
	case fsys.q == nil:
		return fmt.Errorf("%w: a database is required (WithDB)", ErrInvalidConfig)
	case fsys.dialect == nil:
		return fmt.Errorf("%w: a dialect is required (WithDialect)", ErrInvalidConfig)
	case !prefixPattern.MatchString(fsys.prefix):
		return fmt.Errorf("%w: table prefix must match %s", ErrInvalidConfig, tableNameFormat)
	case fsys.chunkSize < MinChunkSize || fsys.chunkSize > MaxChunkSize:
		return fmt.Errorf("%w: chunk size must be between %d and %d bytes", ErrInvalidConfig, MinChunkSize, MaxChunkSize)
	case fsys.maxFileSize < 0:
		return fmt.Errorf("%w: maximum file size must not be negative", ErrInvalidConfig)
	case fsys.opTimeout < 0:
		return fmt.Errorf("%w: durations must not be negative", ErrInvalidConfig)
	case fsys.stagingRetention <= 0, fsys.revisionRetention <= 0:
		// Zero would let Cleanup reclaim in-progress writes and reads immediately.
		return fmt.Errorf("%w: retention periods must be positive", ErrInvalidConfig)
	}
	return nil
}

// Retry returns the default no-op retrier.
//
// Deprecated: This method is deprecated and will be removed in a future release.
func (fsys *FileSystem) Retry() vfs.Retry { return vfs.DefaultRetryer() }

// Name returns "SQL FS".
func (fsys *FileSystem) Name() string { return name }

// Scheme returns "sqlfs".
func (fsys *FileSystem) Scheme() string { return Scheme }

// NewFile returns a vfs.File for the absolute path in the namespace identified by authority. No I/O is performed.
func (fsys *FileSystem) NewFile(authorityStr, absFilePath string, opts ...options.NewFileOption) (vfs.File, error) {
	if fsys == nil {
		return nil, errFileSystemRequired
	}
	if absFilePath == "" {
		return nil, errPathRequired
	}
	if err := utils.ValidateAbsoluteFilePath(absFilePath); err != nil {
		return nil, err
	}
	loc, err := fsys.NewLocation(authorityStr, utils.EnsureTrailingSlash(path.Dir(absFilePath)))
	if err != nil {
		return nil, err
	}
	return loc.NewFile(path.Base(absFilePath), opts...)
}

// NewLocation returns a vfs.Location for the absolute path in the namespace identified by authority. No I/O is
// performed.
func (fsys *FileSystem) NewLocation(authorityStr, absLocPath string) (vfs.Location, error) {
	if fsys == nil {
		return nil, errFileSystemRequired
	}
	if absLocPath == "" {
		return nil, errPathRequired
	}
	if err := utils.ValidateAbsoluteLocationPath(absLocPath); err != nil {
		return nil, err
	}
	auth, err := authority.NewAuthority(utils.RemoveTrailingSlash(authorityStr))
	if err != nil {
		return nil, err
	}
	return &Location{
		fileSystem: fsys,
		path:       utils.EnsureTrailingSlash(path.Clean(absLocPath)),
		authority:  auth,
	}, nil
}

// opCtx returns a context bounded by the operation timeout.
func (fsys *FileSystem) opCtx(parent context.Context) (context.Context, context.CancelFunc) {
	if fsys.opTimeout <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeoutCause(parent, fsys.opTimeout, ErrTimeout)
}

// inTx runs fn in a transaction, retrying the whole transaction on transient database errors or when fn returns
// errRetry. fn must be safe to run more than once.
func (fsys *FileSystem) inTx(parent context.Context, op string, fn func(ctx context.Context, tx txn) error) error {
	var err error
	for attempt := range maxTxAttempts {
		if attempt > 0 {
			jitter := time.Duration(rand.IntN(10)) * time.Millisecond //nolint:gosec // jitter does not need a secure source
			timer := time.NewTimer(time.Duration(attempt)*10*time.Millisecond + jitter)
			select {
			case <-parent.Done():
				timer.Stop()
				return fsys.wrapErr(op, context.Cause(parent))
			case <-timer.C:
			}
		}
		err = fsys.runTx(parent, fn)
		if err == nil {
			return nil
		}
		if !errors.Is(err, errRetry) && fsys.dialect.Classify(err) != KindRetryable {
			break
		}
	}
	return fsys.wrapErr(op, err)
}

func (fsys *FileSystem) runTx(parent context.Context, fn func(ctx context.Context, tx txn) error) error {
	ctx, tx, cancel, err := fsys.beginTx(parent)
	if err != nil {
		return err
	}
	defer cancel()
	if err := fn(ctx, tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// beginTx starts a transaction whose statements are each bounded by the operation timeout. The transaction itself is
// bound to parent rather than to a single timeout, so a transaction made of many bounded statements is not cut short.
// Waiting to begin (for a pooled connection or a database lock) is bounded by the operation timeout too.
func (fsys *FileSystem) beginTx(parent context.Context) (context.Context, txn, context.CancelFunc, error) {
	ctx, cancel := context.WithCancel(parent)
	var timer *time.Timer
	if fsys.opTimeout > 0 {
		timer = time.AfterFunc(fsys.opTimeout, cancel)
	}
	tx, err := fsys.q.BeginTx(ctx, nil)
	if timer != nil && !timer.Stop() {
		// The timer fired, so the transaction context is canceled or about to be.
		if err == nil {
			_ = tx.Rollback()
		}
		cancel()
		return nil, nil, nil, fmt.Errorf("begin transaction: %w", ErrTimeout)
	}
	if err != nil {
		cancel()
		return nil, nil, nil, err
	}
	return ctx, timedTxn{txn: tx, fsys: fsys}, cancel, nil
}

// timedTxn bounds each statement executed in a transaction by the operation timeout. Single-row queries are bounded
// by scanRow, which can keep its context alive until the row is scanned.
type timedTxn struct {
	txn
	fsys *FileSystem
}

func (t timedTxn) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	ctx, cancel := t.fsys.opCtx(ctx)
	defer cancel()
	return t.txn.ExecContext(ctx, query, args...)
}

// scanRow runs a single-row query bounded by the operation timeout and scans the result into dest.
func (fsys *FileSystem) scanRow(ctx context.Context, q Execer, query string, args []any, dest ...any) error {
	ctx, cancel := fsys.opCtx(ctx)
	defer cancel()
	return q.QueryRowContext(ctx, query, args...).Scan(dest...)
}

// chunksPerStatement returns how many chunks of size cs one bulk statement may touch.
func chunksPerStatement(cs int) int64 {
	return int64(max(1, statementBytes/max(cs, 1)))
}

// copyChunks copies every chunk of revision src into revision dst, in statements of bounded size. Sparse revisions
// are handled by jumping to the next stored chunk rather than scanning empty index ranges.
func (fsys *FileSystem) copyChunks(ctx context.Context, tx txn, dst, src string, cs int) error {
	step := chunksPerStatement(cs)
	for from := int64(0); ; {
		var next sql.NullInt64
		if err := fsys.scanRow(ctx, tx, fsys.qs.minChunkFrom, []any{src, from}, &next); err != nil {
			return err
		}
		if !next.Valid {
			return nil
		}
		if _, err := tx.ExecContext(ctx, fsys.qs.copyChunksRange, dst, src, next.Int64, next.Int64+step); err != nil {
			return err
		}
		from = next.Int64 + step
	}
}

// deleteRevision removes a revision's chunks, in statements of bounded size, and then the revision row. The caller
// must already have claimed the revision so that no writer or reader can be using it.
func (fsys *FileSystem) deleteRevision(ctx context.Context, tx txn, id string, cs int) error {
	step := chunksPerStatement(cs)
	for {
		var next sql.NullInt64
		if err := fsys.scanRow(ctx, tx, fsys.qs.minChunk, []any{id}, &next); err != nil {
			return err
		}
		if !next.Valid {
			break
		}
		if _, err := tx.ExecContext(ctx, fsys.qs.delChunksRange, id, next.Int64, next.Int64+step); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, fsys.qs.delRevision, id)
	return err
}

// exec runs a single statement outside a transaction and returns the rows affected.
func (fsys *FileSystem) exec(ctx context.Context, query string, args ...any) (int64, error) {
	ctx, cancel := fsys.opCtx(ctx)
	defer cancel()
	res, err := fsys.q.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// newID returns a new random 26-character identifier.
func newID() string { return cryptorand.Text() }

func nowNanos() int64 { return time.Now().UnixNano() }

// isUnique reports whether err is a unique-constraint violation.
func (fsys *FileSystem) isUnique(err error) bool {
	return err != nil && fsys.dialect.Classify(err) == KindUniqueViolation
}

// fileRow is the published state of a file.
type fileRow struct {
	id         string
	revisionID string
	size       int64
	modifiedAt int64
	generation int64
	chunkSize  int
}

// lookupFile returns the published row for a path, or nil if no file exists.
func (fsys *FileSystem) lookupFile(ctx context.Context, q Execer, ns, dir, name string) (*fileRow, error) {
	var r fileRow
	err := fsys.scanRow(ctx, q, fsys.qs.selFile, []any{ns, dir, name},
		&r.id, &r.revisionID, &r.size, &r.modifiedAt, &r.generation, &r.chunkSize)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil //nolint:nilnil // nil row means "no such file"
	}
	if err != nil {
		return nil, err
	}
	if r.chunkSize < MinChunkSize || r.chunkSize > MaxChunkSize {
		return nil, fmt.Errorf("%w: file %s/%s has invalid chunk size %d", ErrCorrupt, dir, name, r.chunkSize)
	}
	return &r, nil
}

// ValidateSchema checks, without modifying the database, that the schema exists and is at a version this module
// supports.
func (fsys *FileSystem) ValidateSchema(ctx context.Context) error {
	latest := fsys.latestVersion()
	ctx, cancel := fsys.opCtx(ctx)
	defer cancel()

	var v sql.NullInt64
	if err := fsys.q.QueryRowContext(ctx, fsys.qs.selVersion).Scan(&v); err != nil {
		if fsys.dialect.Classify(err) == KindUndefinedTable {
			return fmt.Errorf("sqlfs: validate schema: %w: schema not found (run EnsureSchema): %w", ErrInvalidSchema, err)
		}
		// Anything else (permissions, connectivity, timeouts) is reported as itself rather than as a missing schema.
		return fsys.wrapErr("validate schema", err)
	}
	switch {
	case !v.Valid || v.Int64 == 0:
		return fmt.Errorf("sqlfs: validate schema: %w: schema not initialized (run EnsureSchema)", ErrInvalidSchema)
	case int(v.Int64) > latest:
		return fmt.Errorf("sqlfs: validate schema: %w: database schema version %d is newer than supported version %d; upgrade this module",
			ErrInvalidSchema, v.Int64, latest)
	case int(v.Int64) < latest:
		return fmt.Errorf("sqlfs: validate schema: %w: database schema version %d is older than %d (run EnsureSchema)",
			ErrInvalidSchema, v.Int64, latest)
	}
	if err := fsys.validateSession(ctx, fsys.q); err != nil {
		return fsys.wrapErr("validate schema", err)
	}
	return nil
}

// validateSession runs the dialect's session checks, if it has any.
func (fsys *FileSystem) validateSession(ctx context.Context, q Execer) error {
	v, ok := fsys.dialect.(SessionValidator)
	if !ok {
		return nil
	}
	return v.ValidateSession(ctx, q)
}

// EnsureSchema creates or upgrades the schema to the version supported by this module. It is idempotent, safe to
// run concurrently, and never performs destructive changes.
func (fsys *FileSystem) EnsureSchema(ctx context.Context) error {
	latest := fsys.latestVersion()
	return fsys.inTx(ctx, "ensure schema", func(ctx context.Context, tx txn) error {
		if err := fsys.validateSession(ctx, tx); err != nil {
			return err
		}
		if err := fsys.dialect.LockSchema(ctx, tx, fsys.prefix); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, fsys.qs.createSchemaTable); err != nil {
			return err
		}
		var v sql.NullInt64
		if err := fsys.scanRow(ctx, tx, fsys.qs.selVersion, nil, &v); err != nil {
			return err
		}
		if int(v.Int64) > latest {
			return fmt.Errorf("%w: database schema version %d is newer than supported version %d; upgrade this module",
				ErrInvalidSchema, v.Int64, latest)
		}
		for _, m := range fsys.dialect.Migrations() {
			if int64(m.Version) <= v.Int64 {
				continue
			}
			for _, stmt := range renderMigration(m, fsys.prefix, fsys.dialect.QuoteIdentifier) {
				if _, err := tx.ExecContext(ctx, stmt); err != nil {
					return fmt.Errorf("migration %d: %w", m.Version, err)
				}
			}
		}
		return nil
	})
}

func (fsys *FileSystem) latestVersion() int {
	latest := 0
	for _, m := range fsys.dialect.Migrations() {
		latest = max(latest, m.Version)
	}
	return latest
}
