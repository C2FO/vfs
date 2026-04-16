package vfs

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyListOptions(t *testing.T) {
	t.Parallel()

	re := regexp.MustCompile(`^foo`)

	cfg := ApplyListOptions(ListConfig{}, WithPrefix("p/"), WithRegexp(re), WithRecursive(true), WithPageSize(100))

	assert.Equal(t, "p/", cfg.Prefix)
	require.NotNil(t, cfg.Matcher)
	assert.True(t, cfg.Matcher("foo"))
	assert.False(t, cfg.Matcher("bar"))
	assert.True(t, cfg.Recursive)
	assert.Equal(t, 100, cfg.PageSize)
}

func TestWithNameMatcher(t *testing.T) {
	t.Parallel()

	cfg := ApplyListOptions(ListConfig{}, WithNameMatcher(func(s string) bool { return s == "x" }))
	require.NotNil(t, cfg.Matcher)
	assert.True(t, cfg.Matcher("x"))
	assert.False(t, cfg.Matcher("y"))
}

func TestNilListOptionIgnored(t *testing.T) {
	t.Parallel()

	cfg := ApplyListOptions(ListConfig{Prefix: "a"}, nil, WithPrefix("b"))
	assert.Equal(t, "b", cfg.Prefix)
}
