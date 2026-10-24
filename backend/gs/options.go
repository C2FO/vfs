package gs

import (
	"context"
	"errors"
	"fmt"

	"cloud.google.com/go/storage"
	"google.golang.org/api/impersonate"
	"google.golang.org/api/option"
)

const cloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"

var errAPIKeyWithImpersonation = errors.New(
	"cannot combine APIKey with ImpersonateServiceAccount: an API key can't impersonate a service account",
)

// Options holds Google Cloud Storage -specific options.  Currently only client options are used.
type Options struct {
	APIKey         string   `json:"apiKey,omitempty"`
	CredentialFile string   `json:"credentialFilePath,omitempty"`
	Endpoint       string   `json:"endpoint,omitempty"`
	Scopes         []string `json:"withoutAuthentication,omitempty"`
	FileBufferSize int      `json:"fileBufferSize,omitempty"` // Buffer Size In Bytes Used with utils.TouchCopyBuffered
	// ImpersonateServiceAccount is the email of a service account to impersonate. The base identity is CredentialFile
	// if set, otherwise Application Default Credentials. It can't be combined with APIKey, and Endpoint and Scopes are
	// ignored when it's set.
	ImpersonateServiceAccount string `json:"impersonateServiceAccount,omitempty"`
}

func parseClientOptions(ctx context.Context, opts Options) ([]option.ClientOption, error) {
	if opts.ImpersonateServiceAccount != "" {
		return impersonationClientOptions(ctx, opts)
	}

	var googleClientOpts []option.ClientOption
	switch {
	case opts.APIKey != "":
		googleClientOpts = append(googleClientOpts, option.WithAPIKey(opts.APIKey))
	case opts.CredentialFile != "":
		//nolint:staticcheck // Deprecated but functional
		googleClientOpts = append(googleClientOpts, option.WithCredentialsFile(opts.CredentialFile))
	case opts.Endpoint != "":
		googleClientOpts = append(googleClientOpts, option.WithEndpoint(opts.Endpoint))
	case len(opts.Scopes) > 0:
		googleClientOpts = append(googleClientOpts, option.WithScopes(opts.Scopes...))
	}

	return googleClientOpts, nil
}

func impersonationClientOptions(ctx context.Context, opts Options) ([]option.ClientOption, error) {
	if opts.APIKey != "" {
		return nil, errAPIKeyWithImpersonation
	}

	var baseOpts []option.ClientOption
	if opts.CredentialFile != "" {
		//nolint:staticcheck // Deprecated but functional
		baseOpts = append(baseOpts, option.WithCredentialsFile(opts.CredentialFile))
	}

	// The token source is cached on the client and refreshes tokens long after this call returns, and the Google
	// libraries reuse the construction context for those refreshes, so it must not be tied to ctx's cancellation.
	ts, err := impersonate.CredentialsTokenSource(context.WithoutCancel(ctx), impersonate.CredentialsConfig{
		TargetPrincipal: opts.ImpersonateServiceAccount,
		Scopes:          []string{storage.ScopeFullControl, cloudPlatformScope},
	}, baseOpts...)
	if err != nil {
		return nil, fmt.Errorf("unable to impersonate service account %s: %w", opts.ImpersonateServiceAccount, err)
	}

	return []option.ClientOption{option.WithTokenSource(ts)}, nil
}
