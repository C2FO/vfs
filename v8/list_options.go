package vfs

import (
	"regexp"
)

// ListConfig holds resolved settings for [Lister.List]. Embed or copy fields in
// backends when applying options.
type ListConfig struct {
	// Prefix restricts listing to names beginning with this path segment (backend-defined).
	Prefix string
	// Matcher filters names; if nil, all names pass.
	Matcher func(string) bool
	// Recursive requests depth-first listing when supported.
	Recursive bool
	// PageSize hints maximum keys per backend round trip (0 = backend default).
	PageSize int
}

// ListOption configures [ListConfig]. Implementations apply options in order.
type ListOption func(*ListConfig)

// ApplyListOptions merges opts into a copy of base and returns the result.
func ApplyListOptions(base ListConfig, opts ...ListOption) ListConfig {
	cfg := base
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	return cfg
}

// WithPrefix narrows listing to entries under a relative prefix, subsuming v7's ListByPrefix.
func WithPrefix(prefix string) ListOption {
	return func(c *ListConfig) {
		c.Prefix = prefix
	}
}

// WithNameMatcher filters listed names. Subsumes v7's ListByRegex when used with
// regexp-based matchers.
func WithNameMatcher(fn func(string) bool) ListOption {
	return func(c *ListConfig) {
		c.Matcher = fn
	}
}

// WithRegexp filters names matching re. Convenience over [WithNameMatcher].
func WithRegexp(re *regexp.Regexp) ListOption {
	if re == nil {
		return func(_ *ListConfig) {}
	}
	return WithNameMatcher(re.MatchString)
}

// WithRecursive requests recursive listing when the backend supports it.
func WithRecursive(recursive bool) ListOption {
	return func(c *ListConfig) {
		c.Recursive = recursive
	}
}

// WithPageSize sets a soft page size for object listing APIs (S3, GCS, etc.).
func WithPageSize(n int) ListOption {
	return func(c *ListConfig) {
		c.PageSize = n
	}
}
