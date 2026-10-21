package types

import "testing"

func TestItemIsNative(t *testing.T) {
	tests := []struct {
		name     string
		item     Item
		expected bool
	}{
		{name: "doc", item: Item{MimeType: "application/vnd.google-apps.document"}, expected: true},
		{name: "shortcut", item: Item{MimeType: "application/vnd.google-apps.shortcut"}, expected: true},
		{name: "folder", item: Item{MimeType: FolderMimeType, IsFolder: true}},
		{name: "regular file", item: Item{MimeType: "text/plain"}},
		{name: "short mime", item: Item{MimeType: "a"}},
		{name: "empty mime", item: Item{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.item.IsNative(); got != tt.expected {
				t.Errorf("IsNative() = %v, want %v", got, tt.expected)
			}
		})
	}
}
