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
)

const (
	maxTxAttempts   = 3
	cleanupBatch    = 100
	tableNameFormat = `^[A-Za-z0-9_]{1,32}$`
)

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
	case fsys.opTimeout < 0, fsys.stagingRetention < 0, fsys.revisionRetention < 0:
		return fmt.Errorf("%w: durations must not be negative", ErrInvalidConfig)
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
func (fsys *FileSystem) inTx(op string, fn func(ctx context.Context, tx txn) error) error {
	var err error
	for attempt := range maxTxAttempts {
		if attempt > 0 {
			jitter := time.Duration(rand.IntN(10)) * time.Millisecond //nolint:gosec // jitter does not need a secure source
			time.Sleep(time.Duration(attempt)*10*time.Millisecond + jitter)
		}
		err = fsys.runTx(fn)
		if err == nil {
			return nil
		}
		if !errors.Is(err, errRetry) && fsys.dialect.Classify(err) != KindRetryable {
			break
		}
	}
	return fsys.wrapErr(op, err)
}

func (fsys *FileSystem) runTx(fn func(ctx context.Context, tx txn) error) error {
	ctx, cancel := fsys.opCtx(context.Background())
	defer cancel()

	tx, err := fsys.q.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(ctx, tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
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
	ctx, cancel := fsys.opCtx(ctx)
	defer cancel()
	var r fileRow
	err := q.QueryRowContext(ctx, fsys.qs.selFile, ns, dir, name).
		Scan(&r.id, &r.revisionID, &r.size, &r.modifiedAt, &r.generation, &r.chunkSize)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil //nolint:nilnil // nil row means "no such file"
	}
	if err != nil {
		return nil, err
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
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) ||
			fsys.dialect.Classify(err) == KindUnavailable {
			return fsys.wrapErr("validate schema", err)
		}
		return fmt.Errorf("sqlfs: validate schema: %w: schema not found (run EnsureSchema): %w", ErrInvalidSchema, err)
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
	return nil
}

// EnsureSchema creates or upgrades the schema to the version supported by this module. It is idempotent, safe to
// run concurrently, and never performs destructive changes.
func (fsys *FileSystem) EnsureSchema(_ context.Context) error {
	latest := fsys.latestVersion()
	return fsys.inTx("ensure schema", func(ctx context.Context, tx txn) error {
		if err := fsys.dialect.LockSchema(ctx, tx); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, fsys.qs.createSchemaTable); err != nil {
			return err
		}
		var v sql.NullInt64
		if err := tx.QueryRowContext(ctx, fsys.qs.selVersion).Scan(&v); err != nil {
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
			for _, stmt := range renderMigration(m, fsys.prefix) {
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
