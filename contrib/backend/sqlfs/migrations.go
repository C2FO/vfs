package sqlfs

import (
	"embed"
	"fmt"
	"regexp"
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

var migrationIdent = regexp.MustCompile(`\bvfs_\w+`)

// renderMigration replaces every literal vfs_ identifier with the quoted, prefixed name (quoting matches the runtime
// queries, so prefixes with uppercase letters or a leading digit work) and splits the migration into statements.
func renderMigration(m Migration, prefix string, quote func(string) string) []string {
	sqlText := migrationIdent.ReplaceAllStringFunc(m.SQL, func(ident string) string {
		return quote(prefix + strings.TrimPrefix(ident, "vfs_"))
	})
	var stmts []string
	for part := range strings.SplitSeq(sqlText, ";\n") {
		if s := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(part), ";")); s != "" {
			stmts = append(stmts, s)
		}
	}
	return stmts
}
