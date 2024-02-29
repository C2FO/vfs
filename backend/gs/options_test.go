package gs

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

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
			result := parseClientOptions(tc.input)
			s.Len(result, tc.expectedOptions, "unexpected number of client options returned")
		})
	}
}
