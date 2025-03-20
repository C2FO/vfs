package delete

import (
	"testing"
)

func TestWithAllVersions(t *testing.T) {
	opt := WithAllVersions()
	if opt.DeleteOptionName() != "deleteAllVersions" {
		t.Errorf("expected `deleteAllVersions`, got %s", opt.DeleteOptionName())
	}
}

func TestAllVersionsName(t *testing.T) {
	var opt AllVersions
	if opt.DeleteOptionName() != "deleteAllVersions" {
		t.Errorf("expected `deleteAllVersions`, got %s", opt.DeleteOptionName())
	}
}

func TestWithDeleteAllVersions(t *testing.T) {
	opt := WithDeleteAllVersions()
	if opt.DeleteOptionName() != "deleteAllVersions" {
		t.Errorf("expected `deleteAllVersions`, got %s", opt.DeleteOptionName())
	}
}

func TestDeleteAllVersionsName(t *testing.T) {
	var opt DeleteAllVersions
	if opt.DeleteOptionName() != "deleteAllVersions" {
		t.Errorf("expected `deleteAllVersions`, got %s", opt.DeleteOptionName())
	}
}
