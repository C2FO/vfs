CREATE TABLE IF NOT EXISTS vfs_schema (
    version INTEGER PRIMARY KEY
);
CREATE TABLE IF NOT EXISTS vfs_revisions (
    id            VARCHAR(32) PRIMARY KEY,
    state         SMALLINT NOT NULL,
    size          BIGINT   NOT NULL,
    chunk_size    INTEGER  NOT NULL,
    created_at    BIGINT   NOT NULL,
    expires_at    BIGINT,
    superseded_at BIGINT
);
CREATE INDEX IF NOT EXISTS vfs_revisions_state_idx ON vfs_revisions (state);
CREATE TABLE IF NOT EXISTS vfs_files (
    id          VARCHAR(32) PRIMARY KEY,
    namespace   TEXT        NOT NULL,
    dir         TEXT        NOT NULL,
    name        TEXT        NOT NULL,
    revision_id VARCHAR(32) NOT NULL REFERENCES vfs_revisions (id),
    size        BIGINT      NOT NULL,
    created_at  BIGINT      NOT NULL,
    modified_at BIGINT      NOT NULL,
    generation  BIGINT      NOT NULL,
    UNIQUE (namespace, dir, name)
);
CREATE INDEX IF NOT EXISTS vfs_files_revision_idx ON vfs_files (revision_id);
CREATE TABLE IF NOT EXISTS vfs_chunks (
    revision_id VARCHAR(32) NOT NULL REFERENCES vfs_revisions (id),
    chunk_index BIGINT      NOT NULL,
    data        BYTEA       NOT NULL,
    PRIMARY KEY (revision_id, chunk_index)
);
INSERT INTO vfs_schema (version) VALUES (1);
