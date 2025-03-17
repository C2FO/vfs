package gs

import (
	"google.golang.org/api/option"
)

// Options holds Google Cloud Storage -specific options.  Currently only client options are used.
type Options struct {
	APIKey         string   `json:"apiKey,omitempty"`
	CredentialFile string   `json:"credentialFilePath,omitempty"`
	Endpoint       string   `json:"endpoint,omitempty"`
	Scopes         []string `json:"WithoutAuthentication,omitempty"`
	FileBufferSize int      `json:"fileBufferSize,omitempty"` // Buffer Size In Bytes Used with utils.TouchCopyBuffered
}

func parseClientOptions(opts Options) []option.ClientOption {
	var googleClientOpts []option.ClientOption
	switch {
	case opts.APIKey != "":
		googleClientOpts = append(googleClientOpts, option.WithAPIKey(opts.APIKey))
	case opts.CredentialFile != "":
		googleClientOpts = append(googleClientOpts, option.WithCredentialsFile(opts.CredentialFile))
	case opts.Endpoint != "":
		googleClientOpts = append(googleClientOpts, option.WithEndpoint(opts.Endpoint))
	case len(opts.Scopes) > 0:
		googleClientOpts = append(googleClientOpts, option.WithScopes(opts.Scopes...))
	}

	return googleClientOpts
}
