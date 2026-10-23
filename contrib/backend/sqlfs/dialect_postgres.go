package sqlfs

import (
	"context"
	"database/sql/driver"
	"errors"
	"net"
	"strconv"
	"strings"
)

// PostgreSQL returns the dialect for PostgreSQL databases. It works with any database/sql driver whose errors
// expose SQLState(), such as pgx's stdlib driver.
func PostgreSQL() Dialect { return postgresDialect{} }

type postgresDialect struct{}

func (postgresDialect) Name() string                    { return "postgres" }
func (postgresDialect) QuoteIdentifier(n string) string { return `"` + n + `"` }
func (postgresDialect) Migrations() []Migration {
	return []Migration{mustMigration("postgres", 1, "0001_init.sql")}
}

func (postgresDialect) Rebind(q string) string {
	var b strings.Builder
	b.Grow(len(q) + 8)
	n := 0
	for i := range len(q) {
		if q[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(q[i])
	}
	return b.String()
}

func (postgresDialect) UpsertChunk(t string) string {
	return "INSERT INTO " + t + " (revision_id, chunk_index, data) VALUES (?, ?, ?) " +
		"ON CONFLICT (revision_id, chunk_index) DO UPDATE SET data = EXCLUDED.data"
}

// schemaLockKey is an arbitrary constant identifying the schema-initialization advisory lock.
const schemaLockKey = 0x7366_6673 // "sffs"

func (postgresDialect) LockSchema(ctx context.Context, tx Execer) error {
	_, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", schemaLockKey)
	return err
}

func (postgresDialect) Classify(err error) ErrorKind {
	if err == nil {
		return KindOther
	}
	var state interface{ SQLState() string }
	if errors.As(err, &state) {
		code := state.SQLState()
		switch {
		case code == "23505":
			return KindUniqueViolation
		case code == "40001", code == "40P01", code == "55P03": // serialization, deadlock, lock timeout
			return KindRetryable
		case strings.HasPrefix(code, "08"), code == "57P01", code == "57P02", code == "57P03":
			return KindUnavailable
		}
		return KindOther
	}
	var netErr net.Error
	if errors.Is(err, driver.ErrBadConn) || errors.As(err, &netErr) {
		return KindUnavailable
	}
	return KindOther
}
