package sqlfs

import "fmt"

// queries holds every SQL statement the engine runs, already rebound for the dialect.
type queries struct {
	createSchemaTable string
	selVersion        string
	selFile           string
	insRevision       string
	copyChunks        string
	upsertChunk       string
	heartbeat         string
	selChunk          string
	selRevExists      string
	publishRevision   string
	supersede         string
	updFilePublish    string
	insFile           string
	delFile           string
	claimRevision     string
	touchFile         string
	moveFile          string
	list              string
	selCleanup        string
	delChunks         string
	delRevision       string
}

func newQueries(d Dialect, prefix string) queries {
	q := d.QuoteIdentifier
	schema, revs, files, chunks := q(prefix+"schema"), q(prefix+"revisions"), q(prefix+"files"), q(prefix+"chunks")
	rb := d.Rebind
	return queries{
		createSchemaTable: fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (version INTEGER PRIMARY KEY)", schema),
		selVersion:        "SELECT MAX(version) FROM " + schema,
		selFile: rb(fmt.Sprintf(`SELECT f.id, f.revision_id, f.size, f.modified_at, f.generation, r.chunk_size
FROM %s f JOIN %s r ON r.id = f.revision_id
WHERE f.namespace = ? AND f.dir = ? AND f.name = ?`, files, revs)),
		insRevision: rb(fmt.Sprintf(`INSERT INTO %s (id, state, size, chunk_size, created_at, expires_at, superseded_at)
VALUES (?, ?, ?, ?, ?, ?, NULL)`, revs)),
		copyChunks: rb(fmt.Sprintf(`INSERT INTO %[1]s (revision_id, chunk_index, data)
SELECT ?, chunk_index, data FROM %[1]s WHERE revision_id = ?`, chunks)),
		upsertChunk:  rb(d.UpsertChunk(chunks)),
		heartbeat:    rb(fmt.Sprintf("UPDATE %s SET expires_at = ? WHERE id = ? AND state = %d", revs, stateStaged)),
		selChunk:     rb(fmt.Sprintf("SELECT data FROM %s WHERE revision_id = ? AND chunk_index = ?", chunks)),
		selRevExists: rb(fmt.Sprintf("SELECT 1 FROM %s WHERE id = ?", revs)),
		publishRevision: rb(fmt.Sprintf("UPDATE %s SET state = %d, size = ?, expires_at = NULL WHERE id = ? AND state = %d",
			revs, statePublished, stateStaged)),
		supersede: rb(fmt.Sprintf("UPDATE %s SET state = %d, superseded_at = ? WHERE id = ?", revs, stateSuperseded)),
		updFilePublish: rb(fmt.Sprintf(`UPDATE %s SET revision_id = ?, size = ?, modified_at = ?, generation = generation + 1
WHERE id = ? AND generation = ?`, files)),
		insFile: rb(fmt.Sprintf(`INSERT INTO %s (id, namespace, dir, name, revision_id, size, created_at, modified_at, generation)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1)`, files)),
		delFile:   rb(fmt.Sprintf("DELETE FROM %s WHERE id = ? AND generation = ?", files)),
		touchFile: rb(fmt.Sprintf("UPDATE %s SET modified_at = ?, generation = generation + 1 WHERE id = ?", files)),
		moveFile: rb(fmt.Sprintf("UPDATE %s SET namespace = ?, dir = ?, name = ?, generation = generation + 1 WHERE id = ? AND generation = ?",
			files)),
		list: rb(fmt.Sprintf("SELECT name FROM %s WHERE namespace = ? AND dir = ? ORDER BY name", files)),
		selCleanup: rb(fmt.Sprintf(`SELECT id, state FROM %s
WHERE (state = %d AND expires_at < ?) OR (state = %d AND superseded_at < ?) LIMIT ?`, revs, stateStaged, stateSuperseded)),
		claimRevision: rb(fmt.Sprintf(`UPDATE %s SET state = %d
WHERE id = ? AND ((state = %d AND expires_at < ?) OR (state = %d AND superseded_at < ?))`,
			revs, stateReclaiming, stateStaged, stateSuperseded)),
		delChunks:   rb(fmt.Sprintf("DELETE FROM %s WHERE revision_id = ?", chunks)),
		delRevision: rb(fmt.Sprintf("DELETE FROM %s WHERE id = ?", revs)),
	}
}
