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

var (
	migrationIdent = regexp.MustCompile(`\bvfs_\w+`)
	// statementEnd matches a semicolon that ends a line, tolerating trailing blanks.
	statementEnd = regexp.MustCompile(`;[ \t]*\n`)
)

// renderMigration replaces every literal vfs_ identifier with the quoted, prefixed name (quoting matches the runtime
// queries, so prefixes with uppercase letters or a leading digit work) and splits the migration into statements.
func renderMigration(m Migration, prefix string, quote func(string) string) []string {
	// Normalize line endings first so a checkout with CRLF endings renders the same statements.
	sqlText := strings.ReplaceAll(m.SQL, "\r\n", "\n")
	sqlText = migrationIdent.ReplaceAllStringFunc(sqlText, func(ident string) string {
		return quote(prefix + strings.TrimPrefix(ident, "vfs_"))
	})
	var stmts []string
	for _, part := range statementEnd.Split(sqlText+"\n", -1) {
		if s := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(part), ";")); s != "" {
			stmts = append(stmts, s)
		}
	}
	return stmts
}
