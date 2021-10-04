package gs

import (
	"google.golang.org/api/option"

	"github.com/c2fo/vfs/v6"
)

// Options holds Google Cloud Storage -specific options.  Currently only client options are used.
type Options struct {
	APIKey         string   `json:"apiKey,omitempty"`
	CredentialFile string   `json:"credentialFilePath,omitempty"`
	Endpoint       string   `json:"endpoint,omitempty"`
	Scopes         []string `json:"WithoutAuthentication,omitempty"`
	Retry          vfs.Retry
	FileBufferSize int // Buffer Size In Bytes Used with utils.TouchCopyBuffered
}

func parseClientOptions(opts vfs.Options) []option.ClientOption {
	var googleClientOpts []option.ClientOption

	// we only care about 'gs.Options' types, skip anything else
	if opts, ok := opts.(Options); ok {
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
	}
	return googleClientOpts
}
