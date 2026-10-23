package gs

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/stretchr/testify/suite"
	htransport "google.golang.org/api/transport/http"
)

const (
	testServiceAccount   = "target@test-project.iam.gserviceaccount.com"
	testSubjectToken     = "test-subject-token"
	testWorkloadAudience = "//iam.googleapis.com/projects/123456/locations/global/workloadIdentityPools/test-pool/providers/test-provider"
	testBucketURL        = "https://storage.googleapis.com/storage/v1/b/test-bucket"

	fakeSTSToken          = "sts-token"
	fakeUserToken         = "user-token"
	fakeImpersonatedToken = "impersonated-token"
)

// writeAuthorizedUserCredentials writes a throwaway authorized_user credential file. Refreshing it only works against
// fakeGoogleAPIs.
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
// subject token file it points at. Exchanging the subject token only works against fakeGoogleAPIs.
func writeExternalAccountCredentials(dir string) (string, error) {
	tokenPath := filepath.Join(dir, "subject_token")
	if err := os.WriteFile(tokenPath, []byte(testSubjectToken), 0o600); err != nil {
		return "", err
	}

	path := filepath.Join(dir, "external_account.json")
	contents, err := json.Marshal(map[string]any{
		"type":               "external_account",
		"audience":           testWorkloadAudience,
		"subject_token_type": "urn:ietf:params:oauth:token-type:jwt",
		"token_url":          "https://sts.googleapis.com/v1/token",
		"credential_source":  map[string]string{"file": tokenPath},
	})
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, contents, 0o600)
}

// fakeRequests is what fakeGoogleAPIs received. Fields stay empty for endpoints that weren't called.
type fakeRequests struct {
	stsForm    url.Values
	iamAuth    string
	iamPath    string
	iamScopes  []string
	gcsAuth    string
	unexpected []string
}

// fakeGoogleAPIs stands in for the Google endpoints used while impersonating, routed on the request host: STS for
// external_account base credentials, OAuth2 for authorized_user base credentials, IAM Credentials, and GCS. A non-zero
// stsStatus or iamStatus makes that endpoint fail with the status.
type fakeGoogleAPIs struct {
	stsStatus int
	iamStatus int

	mu   sync.Mutex
	seen fakeRequests
}

func (f *fakeGoogleAPIs) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch r.Host {
	case "sts.googleapis.com":
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.seen.stsForm = r.PostForm
		if f.stsStatus != 0 {
			writeFakeJSON(w, f.stsStatus, map[string]string{"error": "invalid_grant"})
			return
		}
		writeFakeJSON(w, http.StatusOK, map[string]any{
			"access_token":      fakeSTSToken,
			"issued_token_type": "urn:ietf:params:oauth:token-type:access_token",
			"token_type":        "Bearer",
			"expires_in":        3600,
		})
	case "oauth2.googleapis.com":
		writeFakeJSON(w, http.StatusOK, map[string]any{"access_token": fakeUserToken, "token_type": "Bearer", "expires_in": 3600})
	case "iamcredentials.googleapis.com":
		var body struct {
			Scope []string `json:"scope"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.seen.iamAuth = r.Header.Get("Authorization")
		f.seen.iamPath = r.URL.Path
		f.seen.iamScopes = body.Scope
		if f.iamStatus != 0 {
			writeFakeJSON(w, f.iamStatus, map[string]any{"error": map[string]string{"status": "PERMISSION_DENIED"}})
			return
		}
		writeFakeJSON(w, http.StatusOK, map[string]string{
			"accessToken": fakeImpersonatedToken,
			"expireTime":  time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		})
	case "storage.googleapis.com":
		f.seen.gcsAuth = r.Header.Get("Authorization")
		writeFakeJSON(w, http.StatusOK, map[string]string{"name": "test-bucket"})
	default:
		f.seen.unexpected = append(f.seen.unexpected, r.Method+" "+r.Host+r.URL.Path)
		http.NotFound(w, r)
	}
}

func (f *fakeGoogleAPIs) requests() fakeRequests {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.seen
}

func writeFakeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
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

// routeDefaultTransportTo sends every request made through clones of http.DefaultTransport to srv until the test ends.
// The Google client libraries clone http.DefaultTransport for each client they build, and the IAM Credentials endpoint
// is hardcoded, so this is how the STS, IAM, and GCS calls reach the fake.
func (s *optionsSuite) routeDefaultTransportTo(srv *httptest.Server) {
	original, ok := http.DefaultTransport.(*http.Transport)
	s.Require().True(ok, "http.DefaultTransport must be an *http.Transport")
	srvTransport, ok := srv.Client().Transport.(*http.Transport)
	s.Require().True(ok, "httptest client transport must be an *http.Transport")

	routed := original.Clone()
	routed.Proxy = nil
	routed.TLSClientConfig = srvTransport.TLSClientConfig.Clone()
	routed.TLSClientConfig.ServerName = "example.com" // the name httptest's certificate is issued for
	addr := srv.Listener.Addr().String()
	routed.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, addr)
	}

	http.DefaultTransport = routed
	s.T().Cleanup(func() { http.DefaultTransport = original })
}

func (s *optionsSuite) TestParseClientOptionsTokenExchange() {
	dir := s.T().TempDir()
	authorizedUserFile, err := writeAuthorizedUserCredentials(dir)
	s.Require().NoError(err)
	externalAccountFile, err := writeExternalAccountCredentials(dir)
	s.Require().NoError(err)

	tests := []struct {
		name            string
		adcFile         string
		stsStatus       int
		iamStatus       int
		expectSTS       bool
		expectedIAMAuth string
		expectedGCSAuth string
		expectedErr     string
	}{
		{
			name:            "external_account base exchanges its subject token with STS, then impersonates",
			adcFile:         externalAccountFile,
			expectSTS:       true,
			expectedIAMAuth: "Bearer " + fakeSTSToken,
			expectedGCSAuth: "Bearer " + fakeImpersonatedToken,
		},
		{
			name:            "authorized_user base refreshes its token, then impersonates",
			adcFile:         authorizedUserFile,
			expectedIAMAuth: "Bearer " + fakeUserToken,
			expectedGCSAuth: "Bearer " + fakeImpersonatedToken,
		},
		{
			name:        "STS rejecting the subject token fails before IAM is called",
			adcFile:     externalAccountFile,
			stsStatus:   http.StatusBadRequest,
			expectSTS:   true,
			expectedErr: "impersonate: unable to generate access token",
		},
		{
			name:            "IAM denying impersonation fails before GCS is called",
			adcFile:         externalAccountFile,
			iamStatus:       http.StatusForbidden,
			expectSTS:       true,
			expectedIAMAuth: "Bearer " + fakeSTSToken,
			expectedErr:     "impersonate: status code 403",
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.T().Setenv("GOOGLE_APPLICATION_CREDENTIALS", tt.adcFile)
			// Keep the libraries from adding lookups or client certificates that the fake can't serve.
			s.T().Setenv("GOOGLE_AUTH_TRUST_BOUNDARY_ENABLED", "")
			s.T().Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", "false")

			fake := &fakeGoogleAPIs{stsStatus: tt.stsStatus, iamStatus: tt.iamStatus}
			srv := httptest.NewTLSServer(fake)
			s.T().Cleanup(srv.Close)
			s.routeDefaultTransportTo(srv)

			opts, err := parseClientOptions(s.T().Context(), Options{ImpersonateServiceAccount: testServiceAccount})
			s.Require().NoError(err)
			client, _, err := htransport.NewClient(s.T().Context(), opts...)
			s.Require().NoError(err)
			req, err := http.NewRequestWithContext(s.T().Context(), http.MethodGet, testBucketURL, http.NoBody)
			s.Require().NoError(err)

			resp, err := client.Do(req) //nolint:gosec // G704: testBucketURL is a constant served by the fake
			if tt.expectedErr != "" {
				s.Require().ErrorContains(err, tt.expectedErr)
			} else {
				s.Require().NoError(err)
				s.Require().NoError(resp.Body.Close())
				s.Equal(http.StatusOK, resp.StatusCode)
			}

			got := fake.requests()
			s.Empty(got.unexpected, "unexpected requests")
			if tt.expectSTS {
				s.Equal("urn:ietf:params:oauth:grant-type:token-exchange", got.stsForm.Get("grant_type"))
				s.Equal(testSubjectToken, got.stsForm.Get("subject_token"))
				s.Equal("urn:ietf:params:oauth:token-type:jwt", got.stsForm.Get("subject_token_type"))
				s.Equal(testWorkloadAudience, got.stsForm.Get("audience"))
			} else {
				s.Nil(got.stsForm, "STS should not be called")
			}
			s.Equal(tt.expectedIAMAuth, got.iamAuth)
			if tt.expectedIAMAuth != "" {
				s.Equal("/v1/projects/-/serviceAccounts/"+testServiceAccount+":generateAccessToken", got.iamPath)
				s.ElementsMatch([]string{storage.ScopeFullControl, cloudPlatformScope}, got.iamScopes)
			}
			s.Equal(tt.expectedGCSAuth, got.gcsAuth)
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
