package testsuite

import (
	"context"
	"sort"

	vfs "github.com/c2fo/vfs/v8"
)

// CollectList materializes all entry names from [vfs.Location.List] into a sorted slice.
func CollectList(ctx context.Context, loc vfs.Location, opts ...vfs.ListOption) ([]string, error) {
	names := make([]string, 0)
	for e, err := range loc.List(ctx, opts...) {
		if err != nil {
			return names, err
		}
		names = append(names, e.Name)
	}
	sort.Strings(names)
	return names, nil
}
