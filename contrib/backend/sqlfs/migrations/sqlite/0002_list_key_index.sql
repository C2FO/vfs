CREATE INDEX IF NOT EXISTS vfs_files_key_idx ON vfs_files (namespace, (dir || name));
INSERT INTO vfs_schema (version) VALUES (2);
