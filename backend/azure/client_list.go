package azure

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"

	"github.com/c2fo/vfs/v7/backend/azure/types"
)

var _ types.BlobLister = (*DefaultClient)(nil)

// BlobPager returns a pager over the blobs of a container. A delimited request uses a hierarchy listing, which rolls
// virtual directories up, and any other request uses a flat listing.
func (a *DefaultClient) BlobPager(_ context.Context, containerName string, req types.BlobListRequest) (types.BlobPager, error) {
	cli, err := a.newContainerClient(containerName)
	if err != nil {
		return nil, err
	}

	var maxResults *int32
	if req.PageSize > 0 {
		maxResults = to.Ptr(req.PageSize)
	}
	var prefix *string
	if req.Prefix != "" {
		prefix = to.Ptr(req.Prefix)
	}

	if req.Delimited {
		return &hierarchyPager{pager: cli.NewListBlobsHierarchyPager("/", &container.ListBlobsHierarchyOptions{
			Prefix: prefix, MaxResults: maxResults,
		})}, nil
	}
	return &flatPager{pager: cli.NewListBlobsFlatPager(&container.ListBlobsFlatOptions{
		Prefix: prefix, MaxResults: maxResults,
	})}, nil
}

type hierarchyPager struct {
	pager *runtime.Pager[container.ListBlobsHierarchyResponse]
}

func (p *hierarchyPager) NextPage(ctx context.Context) (types.BlobPage, error) {
	if !p.pager.More() {
		return types.BlobPage{Done: true}, nil
	}
	resp, err := p.pager.NextPage(ctx)
	if err != nil {
		return types.BlobPage{}, err
	}

	page := types.BlobPage{Done: !p.pager.More()}
	for _, item := range resp.Segment.BlobItems {
		page.Blobs = append(page.Blobs, blobItem(item))
	}
	for _, prefix := range resp.Segment.BlobPrefixes {
		if prefix != nil && prefix.Name != nil {
			page.Prefixes = append(page.Prefixes, *prefix.Name)
		}
	}
	return page, nil
}

type flatPager struct {
	pager *runtime.Pager[container.ListBlobsFlatResponse]
}

func (p *flatPager) NextPage(ctx context.Context) (types.BlobPage, error) {
	if !p.pager.More() {
		return types.BlobPage{Done: true}, nil
	}
	resp, err := p.pager.NextPage(ctx)
	if err != nil {
		return types.BlobPage{}, err
	}

	page := types.BlobPage{Done: !p.pager.More()}
	for _, item := range resp.Segment.BlobItems {
		page.Blobs = append(page.Blobs, blobItem(item))
	}
	return page, nil
}

// blobItem converts a listed blob, which the SDK returns by pointer and may leave partly unset.
func blobItem(item *container.BlobItem) types.BlobItem {
	if item == nil {
		return types.BlobItem{}
	}
	out := types.BlobItem{Native: item}
	if item.Name != nil {
		out.Name = *item.Name
	}
	if item.Properties != nil {
		if item.Properties.ContentLength != nil {
			out.Size = *item.Properties.ContentLength
		}
		if item.Properties.LastModified != nil {
			out.LastModified = *item.Properties.LastModified
		}
	}
	return out
}
