package testcontainers

import (
	"context"
	"net/url"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/azure/azurite"

	"github.com/c2fo/vfs/v7/backend"
	"github.com/c2fo/vfs/v7/backend/azure"
)

func registerAzurite(t *testing.T) string {
	ctx := context.Background()
	is := require.New(t)

	ctr, err := azurite.Run(ctx, "mcr.microsoft.com/azure-storage/azurite:latest",
		testcontainers.WithName("vfs-azurite"),
		azurite.WithEnabledServices(azurite.BlobService),
	)
	testcontainers.CleanupContainer(t, ctr)
	is.NoError(err)

	ep, err := ctr.BlobServiceURL(ctx)
	is.NoError(err)

	cred, err := azblob.NewSharedKeyCredential(azurite.AccountName, azurite.AccountKey)
	is.NoError(err)

	u, err := url.JoinPath(ep, azurite.AccountName)
	is.NoError(err)

	cli, err := azblob.NewClientWithSharedKeyCredential(u, cred, nil)
	is.NoError(err)

	_, err = cli.CreateContainer(ctx, "azurite", nil)
	is.NoError(err)

	c, err := azure.NewClient(&azure.Options{
		ServiceURL:  u,
		AccountName: azurite.AccountName,
		AccountKey:  azurite.AccountKey,
	})
	is.NoError(err)

	backend.Register("https://azurite/", azure.NewFileSystem(azure.WithClient(c)))
	return "https://azurite/"
}
