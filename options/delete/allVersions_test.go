package delete

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithAllVersions(t *testing.T) {
	opt := WithAllVersions()
	assert.Equal(t, optionNameDeleteAllVersions, opt.DeleteOptionName())
}

func TestAllVersionsName(t *testing.T) {
	var opt AllVersions
	assert.Equal(t, optionNameDeleteAllVersions, opt.DeleteOptionName())
}

func TestWithDeleteAllVersions(t *testing.T) {
	opt := WithDeleteAllVersions()
	assert.Equal(t, optionNameDeleteAllVersions, opt.DeleteOptionName())
}

func TestDeleteAllVersionsName(t *testing.T) {
	var opt DeleteAllVersions
	assert.Equal(t, optionNameDeleteAllVersions, opt.DeleteOptionName())
}
