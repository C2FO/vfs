package azure

import (
	"os"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultTokenCredentialFactory(t *testing.T) {
	tests := []struct {
		name            string
		tenantID        string
		clientID        string
		clientSecret    string
		expectError     bool
		envTenantID     string
		envClientID     string
		envClientSecret string
	}{
		{
			name:         "ClientSecretCredential",
			tenantID:     "test-tenant-id",
			clientID:     "test-client-id",
			clientSecret: "test-client-secret",
			expectError:  false,
		},
		{
			name:         "tenant not set",
			tenantID:     "",
			clientID:     "test-client-id",
			clientSecret: "test-client-secret",
			expectError:  true,
		},
		{
			name:         "clientSecret not set",
			tenantID:     "test-tenant-id",
			clientID:     "test-client-id",
			clientSecret: "",
			expectError:  true,
		},
		{
			name:            "EnvironmentCredential",
			tenantID:        "",
			clientID:        "",
			clientSecret:    "",
			expectError:     false,
			envTenantID:     "test-tenant-id",
			envClientID:     "test-client-id",
			envClientSecret: "test-client-secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NoError(t, os.Setenv("AZURE_TENANT_ID", tt.envTenantID))
			assert.NoError(t, os.Setenv("AZURE_CLIENT_ID", tt.envClientID))
			assert.NoError(t, os.Setenv("AZURE_CLIENT_SECRET", tt.envClientSecret))
			cred, err := DefaultTokenCredentialFactory(tt.tenantID, tt.clientID, tt.clientSecret)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, cred)
				switch tt.name {
				case "ClientSecretCredential":
					_, ok := cred.(*azidentity.ClientSecretCredential)
					assert.True(t, ok, "Expected ClientSecretCredential")
				case "EnvironmentCredential":
					_, ok := cred.(*azidentity.EnvironmentCredential)
					assert.True(t, ok, "Expected EnvironmentCredential")
				}
			}
		})
	}
}
