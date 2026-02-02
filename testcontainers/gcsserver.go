package testcontainers

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/api/option"

	"github.com/c2fo/vfs/v7/backend"
	"github.com/c2fo/vfs/v7/backend/gs"
)

const gcsServerPort = "4443/tcp"

func registerGCSServer(t *testing.T) string {
	ctx := context.Background()
	is := require.New(t)

	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Name:       "vfs-fake-gcs-server",
			Image:      "fsouza/fake-gcs-server:latest",
			Entrypoint: []string{"/bin/fake-gcs-server", "-backend", "memory"},
			WaitingFor: wait.ForHTTP("/_internal/healthcheck").WithTLS(true).WithAllowInsecure(true).WithPort(gcsServerPort),
		},
		Started: true,
	}
	ctr, err := testcontainers.GenericContainer(ctx, req)
	testcontainers.CleanupContainer(t, ctr)
	is.NoError(err)

	host, err := ctr.Host(ctx)
	is.NoError(err)
	port, err := ctr.MappedPort(ctx, gcsServerPort)
	is.NoError(err)
	ep := fmt.Sprintf("https://%s:%s", host, port.Port())

	hc := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}}
	configJSON := strings.NewReader(fmt.Sprintf(`{"publicHost":"%s:%s"}`, host, port.Port()))
	hreq, err := http.NewRequest(http.MethodPut, ep+"/_internal/config", configJSON)
	is.NoError(err)
	res, err := hc.Do(hreq)
	is.NoError(err)
	_ = res.Body.Close()
	is.Equal(http.StatusOK, res.StatusCode)

	cli, err := storage.NewClient(ctx,
		option.WithHTTPClient(hc),
		option.WithEndpoint(ep+"/storage/v1/"),
		option.WithoutAuthentication(),
	)
	is.NoError(err)

	err = cli.Bucket("gcsserver").Create(ctx, "", &storage.BucketAttrs{VersioningEnabled: true})
	is.NoError(err)

	backend.Register("gs://gcsserver/", gs.NewFileSystem(gs.WithClient(cli)))
	return "gs://gcsserver/"
}
