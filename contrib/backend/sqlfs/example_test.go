package sqlfs_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"github.com/c2fo/vfs/contrib/backend/sqlfs"
	"github.com/c2fo/vfs/v7/backend"
)

func newExampleFS() (*sqlfs.FileSystem, func()) {
	dir, err := os.MkdirTemp("", "sqlfs-example")
	if err != nil {
		panic(err)
	}
	dsn := "file:" + filepath.Join(dir, "files.db") +
		"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		panic(err)
	}
	fsys, err := sqlfs.NewFileSystem(sqlfs.WithDB(db), sqlfs.WithDialect(sqlfs.SQLite()))
	if err != nil {
		panic(err)
	}
	if err := fsys.EnsureSchema(context.Background()); err != nil {
		panic(err)
	}
	return fsys, func() { _ = db.Close(); _ = os.RemoveAll(dir) }
}

// Open a database with a driver of your choice, create a FileSystem, and make sure the schema exists.
func ExampleNewFileSystem() {
	fsys, cleanup := newExampleFS()
	defer cleanup()

	f, err := fsys.NewFile("documents", "/reports/q1.txt")
	if err != nil {
		panic(err)
	}
	if _, err := f.Write([]byte("hello")); err != nil {
		panic(err)
	}
	if err := f.Close(); err != nil { // Close publishes the content atomically.
		panic(err)
	}
	fmt.Println(f.URI())
	// Output: sqlfs://documents/reports/q1.txt
}

// Unlike other backends, sqlfs does not register itself; register your configured instance to use it with vfssimple.
func ExampleFileSystem_register() {
	fsys, cleanup := newExampleFS()
	defer cleanup()

	backend.Register(sqlfs.Scheme, fsys)
	fmt.Println(fsys.Scheme())
	// Output: sqlfs
}

// Concurrent replacement of the same file is detected on Close with ErrConflict. Re-read and retry.
func ExampleErrConflict() {
	fsys, cleanup := newExampleFS()
	defer cleanup()

	update := func() error {
		f, err := fsys.NewFile("documents", "/counter.txt")
		if err != nil {
			return err
		}
		if _, err := f.Write([]byte("new value")); err != nil {
			return err
		}
		return f.Close()
	}

	var err error
	for range 3 {
		if err = update(); !errors.Is(err, sqlfs.ErrConflict) {
			break
		}
	}
	fmt.Println(err)
	// Output: <nil>
}

// Cleanup removes abandoned staged writes and replaced content past its retention. Run it periodically.
func ExampleFileSystem_Cleanup() {
	fsys, cleanup := newExampleFS()
	defer cleanup()

	stats, err := fsys.Cleanup(context.Background())
	if err != nil {
		panic(err)
	}
	fmt.Println(stats.StagedRemoved, stats.SupersededRemoved)
	// Output: 0 0
}

// Read a file back.
func ExampleFile_Read() {
	fsys, cleanup := newExampleFS()
	defer cleanup()

	w, _ := fsys.NewFile("documents", "/a.txt")
	_, _ = w.Write([]byte("contents"))
	_ = w.Close()

	r, _ := fsys.NewFile("documents", "/a.txt")
	b, err := io.ReadAll(r)
	if err != nil {
		panic(err)
	}
	_ = r.Close()
	fmt.Println(string(b))
	// Output: contents
}
