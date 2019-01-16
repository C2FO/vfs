package gs

import (
	"google.golang.org/api/option"

	"github.com/c2fo/vfs"
)

// Options holds Google Cloud Storage -specific options.  Currently only client options are used.
type Options struct {
	APIKey         string   `json:"apiKey,omitempty"`
	CredentialFile string   `json:"credentialFilePath,omitempty"`
	Endpoint       string   `json:"endpoint,omitempty"`
	Scopes         []string `json:"WithoutAuthentication,omitempty"`
}

func parseClientOptions(opts vfs.Options) []option.ClientOption {
	googleClientOpts := []option.ClientOption{}

	// we only care about 'gs.Options' types, skip anything else
	if opts, ok := opts.(Options); ok {
		switch {
		case opts.APIKey != "":
			googleClientOpts = append(googleClientOpts, option.WithAPIKey(opts.APIKey))
		case opts.CredentialFile != "":
			//TODO: this is Deprecated: Use WithCredentialsFile instead (once we update google cloud sdk)
			googleClientOpts = append(googleClientOpts, option.WithServiceAccountFile(opts.CredentialFile))
		case opts.Endpoint != "":
			googleClientOpts = append(googleClientOpts, option.WithEndpoint(opts.Endpoint))
		case len(opts.Scopes) > 0:
			googleClientOpts = append(googleClientOpts, option.WithScopes(opts.Scopes...))
		}
	}
	return googleClientOpts
}
