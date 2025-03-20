package newfile_test

import (
	"testing"

	"github.com/c2fo/vfs/v7/options/newfile"
)

func TestWithContentType(t *testing.T) {
	opt := newfile.WithContentType("application/json")

	ct, ok := opt.(*newfile.ContentType)
	if !ok {
		t.Fatalf("expected `*newfile.ContentType`, got %T", opt)
	}
	if *ct != "application/json" {
		t.Errorf("expected `application/json`, got %v", *ct)
	}
	if ct.NewFileOptionName() != "newFileContentType" {
		t.Errorf("expected `newFileContentType`, got %v", ct.NewFileOptionName())
	}
}
