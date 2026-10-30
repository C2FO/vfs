package list_test

import (
	"context"
	"fmt"
	"log"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/mem"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/options"
)

// reportsLocation returns an in-memory location holding a few files.
func reportsLocation() vfs.Location {
	loc, err := mem.NewFileSystem().NewLocation("", "/reports/")
	if err != nil {
		log.Fatal(err)
	}
	for _, name := range []string{"2026-01.csv", "2026-02.csv", "2026-03.csv", "summary.txt", "notes.md"} {
		f, err := loc.NewFile(name)
		if err != nil {
			log.Fatal(err)
		}
		if _, err := f.Write([]byte("data")); err != nil {
			log.Fatal(err)
		}
		if err := f.Close(); err != nil {
			log.Fatal(err)
		}
	}
	return loc
}

// Entries streams a location's contents in byte order. A listing never loads more than it needs, so stopping the loop
// stops the requests.
func ExampleEntries() {
	ctx := context.Background()
	loc := reportsLocation()

	for e, err := range list.Entries(ctx, loc) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(e.RelPath())
	}
	// Output:
	// 2026-01.csv
	// 2026-02.csv
	// 2026-03.csv
	// notes.md
	// summary.txt
}

// Glob selects entries by pattern, using the same rules as path.Match and io/fs.
func ExampleGlob() {
	ctx := context.Background()
	loc := reportsLocation()

	for e, err := range list.Entries(ctx, loc, list.Glob("2026-0[12].csv")) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(e.RelPath())
	}
	// Output:
	// 2026-01.csv
	// 2026-02.csv
}

// Limit and After page through a listing. The cursor is the last entry of the previous page, so no page needs to
// remember anything but that entry.
func ExampleAfter() {
	ctx := context.Background()
	loc := reportsLocation()

	var cursor vfs.Entry
	for page := 1; ; page++ {
		opts := []options.ListOption{list.Limit(2)}
		if page > 1 {
			opts = append(opts, list.After(cursor))
		}

		n := 0
		for e, err := range list.Entries(ctx, loc, opts...) {
			if err != nil {
				log.Fatal(err)
			}
			fmt.Printf("page %d: %s\n", page, e.RelPath())
			cursor = e
			n++
		}
		if n < 2 {
			break
		}
	}
	// Output:
	// page 1: 2026-01.csv
	// page 1: 2026-02.csv
	// page 2: 2026-03.csv
	// page 2: notes.md
	// page 3: summary.txt
}

// Collect gathers a listing into a slice, stopping at the first error.
func ExampleCollect() {
	ctx := context.Background()
	loc := reportsLocation()

	entries, err := list.Collect(list.Entries(ctx, loc, list.Prefix("2026-"), list.Limit(2)))
	if err != nil {
		log.Fatal(err)
	}
	for _, e := range entries {
		fmt.Println(e.Name())
	}
	// Output:
	// 2026-01.csv
	// 2026-02.csv
}

func archiveLocation() vfs.Location {
	loc, err := mem.NewFileSystem().NewLocation("", "/archive/")
	if err != nil {
		log.Fatal(err)
	}
	files := []struct {
		path string
		size int
	}{
		{"readme.txt", 10},
		{"2025/q1/sales.csv", 400},
		{"2025/q2/sales.csv", 250},
		{"2026/q1/sales.csv", 900},
		{"2026/q1/notes.md", 30},
	}
	for _, file := range files {
		f, err := loc.NewFile(file.path)
		if err != nil {
			log.Fatal(err)
		}
		if _, err := f.Write(make([]byte, file.size)); err != nil {
			log.Fatal(err)
		}
		if err := f.Close(); err != nil {
			log.Fatal(err)
		}
	}
	return loc
}

// Recursive and FilesOnly together list every file under a location, however deep.
func ExampleEntries_recursive() {
	ctx := context.Background()

	for e, err := range list.Entries(ctx, archiveLocation(), list.Recursive(), list.FilesOnly()) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(e.RelPath())
	}
	// Output:
	// 2025/q1/sales.csv
	// 2025/q2/sales.csv
	// 2026/q1/notes.md
	// 2026/q1/sales.csv
	// readme.txt
}

// TopN keeps only the best n entries, so finding the largest files uses memory for n entries however many are listed.
func ExampleTopN() {
	ctx := context.Background()

	largestFirst := func(a, b vfs.Entry) int { return list.BySize(b, a) }
	largest, err := list.TopN(list.Entries(ctx, archiveLocation(), list.Recursive(), list.FilesOnly()), 2, largestFirst)
	if err != nil {
		log.Fatal(err)
	}
	for _, e := range largest {
		fmt.Println(e.RelPath(), e.Size())
	}
	// Output:
	// 2026/q1/sales.csv 900
	// 2025/q1/sales.csv 400
}
