//go:build vfsintegration

package onedrive

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c2fo/vfs/v7/backend/testsuite"
	"github.com/c2fo/vfs/v7/options"
)

// TestConformance runs the shared VFS conformance and IO suites against a real OneDrive. It is
// skipped unless credentials are configured.
//
// Personal Microsoft account (device code flow):
//
//	VFS_ONEDRIVE_CLIENT_ID=your-app-id go test -v -tags=vfsintegration -run TestConformance ./...
//
// Work/school account (client credentials):
//
//	VFS_ONEDRIVE_TENANT_ID=your-tenant \
//	VFS_ONEDRIVE_CLIENT_ID=your-client-id \
//	VFS_ONEDRIVE_CLIENT_SECRET=your-secret \
//	go test -v -tags=vfsintegration -run TestConformance ./...
//
// Environment variables:
//
//   - VFS_ONEDRIVE_AUTH_TYPE: "devicecode", "browser", "token", or "credentials" (default: auto-detect)
//   - VFS_ONEDRIVE_CLIENT_ID: Azure AD application (client) ID
//   - VFS_ONEDRIVE_TENANT_ID: "consumers" (default), "organizations", or a specific tenant
//   - VFS_ONEDRIVE_CLIENT_SECRET: client secret (credentials auth only)
//   - VFS_ONEDRIVE_ACCESS_TOKEN: pre-obtained access token (token auth)
//   - VFS_ONEDRIVE_TEST_PATH: base path for tests (default: /vfs-test/)
func TestConformance(t *testing.T) {
	fs := NewFileSystem(realDriveAuth(t))

	loc, err := fs.NewLocation("", cmp.Or(os.Getenv("VFS_ONEDRIVE_TEST_PATH"), "/vfs-test/"))
	require.NoError(t, err)

	t.Run("conformance", func(t *testing.T) { testsuite.RunConformanceTests(t, loc) })
	t.Run("io", func(t *testing.T) { testsuite.RunIOTests(t, loc) })
}

// realDriveAuth returns the authentication option selected by the VFS_ONEDRIVE_* environment.
func realDriveAuth(t *testing.T) options.NewFileSystemOption[FileSystem] {
	t.Helper()

	clientID := os.Getenv("VFS_ONEDRIVE_CLIENT_ID")
	clientSecret := os.Getenv("VFS_ONEDRIVE_CLIENT_SECRET")
	accessToken := os.Getenv("VFS_ONEDRIVE_ACCESS_TOKEN")
	rawTenantID := os.Getenv("VFS_ONEDRIVE_TENANT_ID")
	tenantID := cmp.Or(rawTenantID, "consumers")

	authType := os.Getenv("VFS_ONEDRIVE_AUTH_TYPE")
	if authType == "" {
		switch {
		case accessToken != "":
			authType = "token"
		case clientID != "" && clientSecret != "" && rawTenantID != "":
			authType = "credentials"
		case clientID != "":
			authType = "devicecode"
		default:
			t.Skip("no OneDrive credentials configured; set VFS_ONEDRIVE_CLIENT_ID for device code flow")
		}
	}

	switch authType {
	case "token":
		return WithAccessToken(accessToken)
	case "credentials":
		return WithClientCredentials(tenantID, clientID, clientSecret)
	case "devicecode":
		return WithDeviceCode(clientID, tenantID, func(_ context.Context, msg string) error {
			_, err := fmt.Fprintln(os.Stderr, msg)
			return err
		})
	case "browser":
		return WithInteractiveBrowser(clientID, tenantID, "")
	}

	t.Fatalf("unknown VFS_ONEDRIVE_AUTH_TYPE %q (use devicecode, browser, token, or credentials)", authType)
	return nil
}
