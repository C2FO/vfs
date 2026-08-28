module github.com/c2fo/vfs/contrib/backend/dropbox

go 1.26.7

require (
	github.com/c2fo/vfs/v7 v7.27.0
	github.com/dropbox/dropbox-sdk-go-unofficial/v6 v6.6.1
	github.com/stretchr/testify v1.12.1
)

require (
	github.com/stretchr/objx v0.5.3 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
)

// Exclude old monolithic go-control-plane to avoid ambiguous import with split modules
exclude github.com/envoyproxy/go-control-plane v0.9.4
