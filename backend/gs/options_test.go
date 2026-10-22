package gs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
)

const testServiceAccount = "target@test-project.iam.gserviceaccount.com"

// writeAuthorizedUserCredentials writes a throwaway authorized_user credential file. It's only parsed, never used to
// fetch a token.
func writeAuthorizedUserCredentials(dir string) (string, error) {
	path := filepath.Join(dir, "authorized_user.json")
	contents, err := json.Marshal(map[string]string{
		"type":          "authorized_user",
		"client_id":     "test-client-id",
		"client_secret": "test-client-secret",
		"refresh_token": "test-refresh-token",
	})
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, contents, 0o600)
}

// writeExternalAccountCredentials writes a throwaway external_account (Workload Identity Federation) config and the
// subject token file it points at. It's only parsed, never used to fetch a token.
func writeExternalAccountCredentials(dir string) (string, error) {
	tokenPath := filepath.Join(dir, "subject_token")
	if err := os.WriteFile(tokenPath, []byte("test-subject-token"), 0o600); err != nil {
		return "", err
	}

	path := filepath.Join(dir, "external_account.json")
	contents, err := json.Marshal(map[string]any{
		"type":               "external_account",
		"audience":           "//iam.googleapis.com/projects/123456/locations/global/workloadIdentityPools/test-pool/providers/test-provider",
		"subject_token_type": "urn:ietf:params:oauth:token-type:jwt",
		"token_url":          "https://sts.googleapis.com/v1/token",
		"credential_source":  map[string]string{"file": tokenPath},
	})
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, contents, 0o600)
}

type optionsSuite struct {
	suite.Suite
}

func TestOptionsSuite(t *testing.T) {
	suite.Run(t, new(optionsSuite))
}

func (s *optionsSuite) TestParseClientOptions() {
	testCases := []struct {
		name            string
		input           Options
		expectedOptions int // Count of expectedErrString Google client options
	}{
		{
			name: "API Key only",
			input: Options{
				APIKey: "test-api-key",
			},
			expectedOptions: 1,
		},
		{
			name: "Credential File only",
			input: Options{
				CredentialFile: "path/to/credential/file.json",
			},
			expectedOptions: 1,
		},
		{
			name: "Endpoint only",
			input: Options{
				Endpoint: "custom-endpoint",
			},
			expectedOptions: 1,
		},
		{
			name: "Scopes only",
			input: Options{
				Scopes: []string{"scope1", "scope2"},
			},
			expectedOptions: 1,
		},
		{
			name: "Multiple options",
			input: Options{
				APIKey:         "test-api-key",
				CredentialFile: "path/to/credential/file.json",
				Endpoint:       "custom-endpoint",
				Scopes:         []string{"scope1", "scope2"},
			},
			expectedOptions: 1, // The function currently selects the first matching case
		},
		{
			name:            "No options",
			input:           Options{},
			expectedOptions: 0,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			result, err := parseClientOptions(s.T().Context(), tc.input)
			s.Require().NoError(err)
			s.Len(result, tc.expectedOptions, "unexpected number of client options returned")
		})
	}
}

func (s *optionsSuite) TestParseClientOptionsWithImpersonation() {
	dir := s.T().TempDir()
	authorizedUserFile, err := writeAuthorizedUserCredentials(dir)
	s.Require().NoError(err)
	externalAccountFile, err := writeExternalAccountCredentials(dir)
	s.Require().NoError(err)
	missingFile := filepath.Join(dir, "missing.json")

	tests := []struct {
		name              string
		adcFile           string
		input             Options
		expectedErr       error
		expectedErrString string
	}{
		{
			name:    "ADC base from an authorized_user file",
			adcFile: authorizedUserFile,
			input:   Options{ImpersonateServiceAccount: testServiceAccount},
		},
		{
			name:    "ADC base from an external_account config",
			adcFile: externalAccountFile,
			input:   Options{ImpersonateServiceAccount: testServiceAccount},
		},
		{
			name:    "Credential file base is used instead of ADC",
			adcFile: missingFile,
			input:   Options{ImpersonateServiceAccount: testServiceAccount, CredentialFile: authorizedUserFile},
		},
		{
			name:    "Endpoint and Scopes are ignored",
			adcFile: authorizedUserFile,
			input: Options{
				ImpersonateServiceAccount: testServiceAccount,
				Endpoint:                  "custom-endpoint",
				Scopes:                    []string{"scope1"},
			},
		},
		{
			name:              "Missing ADC file returns an error",
			adcFile:           missingFile,
			input:             Options{ImpersonateServiceAccount: testServiceAccount},
			expectedErrString: "unable to impersonate service account " + testServiceAccount,
		},
		{
			name:              "Missing credential file returns an error",
			adcFile:           authorizedUserFile,
			input:             Options{ImpersonateServiceAccount: testServiceAccount, CredentialFile: missingFile},
			expectedErrString: "unable to impersonate service account " + testServiceAccount,
		},
		{
			name:        "APIKey can't be combined with impersonation",
			adcFile:     authorizedUserFile,
			input:       Options{ImpersonateServiceAccount: testServiceAccount, APIKey: "test-api-key"},
			expectedErr: errAPIKeyWithImpersonation,
		},
	}

	for _, tt := range tests { //nolint:gocritic // rangeValCopy
		s.Run(tt.name, func() {
			s.T().Setenv("GOOGLE_APPLICATION_CREDENTIALS", tt.adcFile)

			result, err := parseClientOptions(s.T().Context(), tt.input)
			switch {
			case tt.expectedErr != nil:
				s.Require().ErrorIs(err, tt.expectedErr)
				s.Nil(result)
			case tt.expectedErrString != "":
				s.Require().ErrorContains(err, tt.expectedErrString)
				s.Nil(result)
			default:
				s.Require().NoError(err)
				s.Len(result, 1, "expected only the impersonated token source option")
			}
		})
	}
}

func (s *optionsSuite) TestOptionsJSONDecoding() {
	tests := []struct {
		name     string
		input    string
		expected Options
	}{
		{
			name:     "Impersonation only",
			input:    `{"impersonateServiceAccount": "target@test-project.iam.gserviceaccount.com"}`,
			expected: Options{ImpersonateServiceAccount: testServiceAccount},
		},
		{
			name:     "Impersonation with a credential file",
			input:    `{"impersonateServiceAccount": "target@test-project.iam.gserviceaccount.com", "credentialFilePath": "/path/to/creds.json"}`,
			expected: Options{ImpersonateServiceAccount: testServiceAccount, CredentialFile: "/path/to/creds.json"},
		},
		{
			name:     "No impersonation",
			input:    `{"credentialFilePath": "/path/to/creds.json"}`,
			expected: Options{CredentialFile: "/path/to/creds.json"},
		},
	}

	for _, tt := range tests { //nolint:gocritic // rangeValCopy
		s.Run(tt.name, func() {
			var opts Options
			s.Require().NoError(json.Unmarshal([]byte(tt.input), &opts))
			s.Equal(tt.expected, opts)
		})
	}
}
