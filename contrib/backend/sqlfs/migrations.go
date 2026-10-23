package sqlfs

import (
	"embed"
	"fmt"
	"strings"
)

//go:embed migrations
var migrationFiles embed.FS

// mustMigration loads an embedded migration file for a dialect. The files are part of the module, so a failure is a
// programming error.
func mustMigration(dialect string, version int, file string) Migration {
	b, err := migrationFiles.ReadFile("migrations/" + dialect + "/" + file)
	if err != nil {
		panic(fmt.Sprintf("sqlfs: missing embedded migration %s/%s: %v", dialect, file, err))
	}
	return Migration{Version: version, SQL: string(b)}
}

// renderMigration substitutes the table prefix and splits the migration into individual statements.
func renderMigration(m Migration, prefix string) []string {
	sqlText := strings.ReplaceAll(m.SQL, "vfs_", prefix)
	var stmts []string
	for part := range strings.SplitSeq(sqlText, ";\n") {
		if s := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(part), ";")); s != "" {
			stmts = append(stmts, s)
		}
	}
	return stmts
}
