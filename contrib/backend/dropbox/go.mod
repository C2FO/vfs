module github.com/c2fo/vfs/contrib/backend/dropbox

go 1.24.11

require (
	github.com/c2fo/vfs/v7 v7.13.0
	github.com/dropbox/dropbox-sdk-go-unofficial/v6 v6.0.5
	github.com/stretchr/testify v1.11.1
)

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/stretchr/objx v0.5.3 // indirect
	golang.org/x/oauth2 v0.34.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// Exclude old monolithic go-control-plane to avoid ambiguous import with split modules
exclude github.com/envoyproxy/go-control-plane v0.9.4
