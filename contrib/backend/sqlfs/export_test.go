package sqlfs

// ListKeysQuery returns the listing query of fsys, so the integration tests can check its plan.
func ListKeysQuery(fsys *FileSystem) string { return fsys.qs.listKeys }

// SetListPageSize sets how many files one listing query returns, so tests can make a small fixture span many pages.
func SetListPageSize(fsys *FileSystem, n int) { fsys.listPageSize = n }
