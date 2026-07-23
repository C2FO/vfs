//go:build vfsintegration

package testcontainers

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/azure/azurite"
	"github.com/testcontainers/testcontainers-go/modules/minio"
	"github.com/testcontainers/testcontainers-go/wait"
	"golang.org/x/crypto/ssh"
	"google.golang.org/api/option"

	"github.com/c2fo/vfs/v7/backend"
	"github.com/c2fo/vfs/v7/backend/azure"
	"github.com/c2fo/vfs/v7/backend/ftp"
	"github.com/c2fo/vfs/v7/backend/gs"
	"github.com/c2fo/vfs/v7/backend/s3"
	"github.com/c2fo/vfs/v7/backend/sftp"
)

// The container-provisioning approach in this file is derived from the
// community contribution in C2FO/vfs#294 by Nathan Baulch
// (https://github.com/NathanBaulch), reworked here and split alongside the
// backend bug fixes (C2FO/vfs#346) and conformance-suite improvements
// (C2FO/vfs#347) that it depended on.

// Container images are pinned for reproducibility. minio, azurite, and
// fake-gcs-server expose immutable version tags. atmoz/sftp and fauria/vsftpd
// only publish rolling tags (:alpine and :latest respectively), so they are
// additionally pinned by digest. Bump these deliberately.
const (
	minioImage     = "minio/minio:RELEASE.2025-09-07T16-13-09Z"
	azuriteImage   = "mcr.microsoft.com/azure-storage/azurite:3.33.0"
	gcsServerImage = "fsouza/fake-gcs-server:1.52.2"
	atmozImage     = "atmoz/sftp:alpine@sha256:a6cb3eb29202ca7f57e73bb7e527286e66e0e822fff65609207c7e0ef2d135a3"
	ftpImage       = "fauria/vsftpd:latest@sha256:6d71d7c7f1b0ab2844ec7dc7999a30aef6d758b6d8179cf5967513f87c79c177"
)

// registerFunc provisions a container (if needed) and registers one or more
// backends, returning their authorities (VFS URIs) for use with vfssimple. It
// returns a slice so a single container can expose multiple test targets (e.g.
// minio serves both the SSE-on and SSE-off configurations of the s3 backend).
// It returns an error rather than asserting so that the caller can start
// containers concurrently and fail the test from the test goroutine.
type registerFunc func(t *testing.T) ([]string, error)

// allRegisters lists every backend the container suite exercises. registerMem
// and registerOS need no container and always run; the rest require Docker.
func allRegisters() []registerFunc {
	return []registerFunc{
		registerMem,
		registerOS,
		registerAtmoz,
		registerAzurite,
		registerGCSServer,
		registerMinio,
		registerFTP,
	}
}

func registerMem(*testing.T) ([]string, error) { return []string{"mem://test/"}, nil }

func registerOS(t *testing.T) ([]string, error) {
	return []string{fmt.Sprintf("file://%s/", filepath.ToSlash(t.TempDir()))}, nil
}

const minioRegion = "dummy"

// minioKMSKey enables minio's built-in KMS so that SSE-S3 (AES256) auto
// encryption works. Without a configured KMS, minio rejects the s3 backend's
// default server-side-encryption headers with "501 NotImplemented". This is a
// deliberately non-secret, all-zero key in minio's "<name>:<base64-256-bit-key>"
// format, used only by throwaway test containers.
const minioKMSKey = "minio-test-key:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

// registerMinio provisions a single minio server and registers the s3 backend
// against it twice: once with server-side encryption disabled and once with the
// default (SSE-on) configuration. This exercises both branches of the s3
// backend's encryption handling against a real S3-compatible object store
// without running a second container.
func registerMinio(t *testing.T) ([]string, error) {
	ctx := context.Background()

	ctr, err := minio.Run(ctx, minioImage,
		testcontainers.WithEnv(map[string]string{"MINIO_KMS_SECRET_KEY": minioKMSKey}),
	)
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		return nil, err
	}

	ep, err := ctr.ConnectionString(ctx)
	if err != nil {
		return nil, err
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	cli := awss3.NewFromConfig(cfg, func(opts *awss3.Options) {
		opts.Region = minioRegion
		opts.UsePathStyle = true
		opts.BaseEndpoint = aws.String("http://" + ep)
		opts.Credentials = credentials.NewStaticCredentialsProvider(ctr.Username, ctr.Password, "")
	})

	for _, bucket := range []string{"miniobucket", "miniobucket-sse"} {
		if _, err := cli.CreateBucket(ctx, &awss3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
			return nil, err
		}
	}

	// SSE off.
	backend.Register("s3://miniobucket/", s3.NewFileSystem(s3.WithClient(cli), s3.WithOptions(s3.Options{DisableServerSideEncryption: true})))
	// SSE on (default).
	backend.Register("s3://miniobucket-sse/", s3.NewFileSystem(s3.WithClient(cli)))

	return []string{"s3://miniobucket/", "s3://miniobucket-sse/"}, nil
}

func registerAzurite(t *testing.T) ([]string, error) {
	ctx := context.Background()

	ctr, err := azurite.Run(ctx, azuriteImage,
		azurite.WithEnabledServices(azurite.BlobService),
		testcontainers.WithCmdArgs("--skipApiVersionCheck"),
	)
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		return nil, err
	}

	ep, err := ctr.BlobServiceURL(ctx)
	if err != nil {
		return nil, err
	}

	cred, err := azblob.NewSharedKeyCredential(azurite.AccountName, azurite.AccountKey)
	if err != nil {
		return nil, err
	}

	u, err := url.JoinPath(ep, azurite.AccountName)
	if err != nil {
		return nil, err
	}

	cli, err := azblob.NewClientWithSharedKeyCredential(u, cred, nil)
	if err != nil {
		return nil, err
	}

	if _, err := cli.CreateContainer(ctx, "azurite", nil); err != nil {
		return nil, err
	}

	c, err := azure.NewClient(&azure.Options{
		ServiceURL:  u,
		AccountName: azurite.AccountName,
		AccountKey:  azurite.AccountKey,
	})
	if err != nil {
		return nil, err
	}

	backend.Register("https://azurite/", azure.NewFileSystem(azure.WithClient(c)))
	return []string{"https://azurite/"}, nil
}

const gcsServerPort = "4443/tcp"

func registerGCSServer(t *testing.T) ([]string, error) {
	ctx := context.Background()

	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:      gcsServerImage,
			Entrypoint: []string{"/bin/fake-gcs-server", "-backend", "memory"},
			WaitingFor: wait.ForHTTP("/_internal/healthcheck").WithTLS(true).WithAllowInsecure(true).WithPort(gcsServerPort),
		},
		Started: true,
	}
	ctr, err := testcontainers.GenericContainer(ctx, req)
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		return nil, err
	}

	host, err := ctr.Host(ctx)
	if err != nil {
		return nil, err
	}
	port, err := ctr.MappedPort(ctx, gcsServerPort)
	if err != nil {
		return nil, err
	}
	ep := fmt.Sprintf("https://%s:%s", host, port.Port())

	hc := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // fake-gcs-server uses a self-signed cert
	}}
	configJSON := strings.NewReader(fmt.Sprintf(`{"publicHost":"%s:%s"}`, host, port.Port()))
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPut, ep+"/_internal/config", configJSON)
	if err != nil {
		return nil, err
	}
	res, err := hc.Do(hreq)
	if err != nil {
		return nil, err
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fake-gcs-server config returned status %d", res.StatusCode)
	}

	cli, err := storage.NewClient(ctx,
		option.WithHTTPClient(hc),
		option.WithEndpoint(ep+"/storage/v1/"),
		option.WithoutAuthentication(),
	)
	if err != nil {
		return nil, err
	}

	if err := cli.Bucket("gcsserver").Create(ctx, "", &storage.BucketAttrs{VersioningEnabled: true}); err != nil {
		return nil, err
	}

	backend.Register("gs://gcsserver/", gs.NewFileSystem(gs.WithClient(cli)))
	return []string{"gs://gcsserver/"}, nil
}

const (
	atmozPort     = "22/tcp"
	atmozUsername = "dummy"
	atmozPassword = "dummy"
)

func registerAtmoz(t *testing.T) ([]string, error) {
	ctx := context.Background()

	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:      atmozImage,
			Env:        map[string]string{"SFTP_USERS": fmt.Sprintf("%s:%s:::upload", atmozUsername, atmozPassword)},
			WaitingFor: wait.ForListeningPort(atmozPort),
		},
		Started: true,
	}
	ctr, err := testcontainers.GenericContainer(ctx, req)
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		return nil, err
	}

	host, err := ctr.Host(ctx)
	if err != nil {
		return nil, err
	}

	port, err := ctr.MappedPort(ctx, atmozPort)
	if err != nil {
		return nil, err
	}

	authority := fmt.Sprintf("sftp://%s@%s:%s/upload/", atmozUsername, host, port.Port())
	backend.Register(authority, sftp.NewFileSystem(sftp.WithOptions(sftp.Options{
		Password:           atmozPassword,
		KnownHostsCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // ephemeral test container host key
	})))
	return []string{authority}, nil
}

const (
	ftpPort     = "21/tcp"
	ftpUsername = "admin"
	ftpPassword = "dummy"
	// Passive data-connection port range (fauria/vsftpd defaults). Passive FTP
	// requires the server to advertise concrete port numbers that the client then
	// dials, so unlike the other backends this container cannot use random port
	// mapping: each passive port is published on an identical host port. The
	// pasv_address is set to 127.0.0.1 because that is where the Docker host is
	// reachable on GitHub-hosted runners and typical local daemons; a remote
	// Docker host would need a different address.
	ftpPasvMinPort = 21100
	ftpPasvMaxPort = 21110
	ftpPasvAddress = "127.0.0.1"
)

// registerFTP provisions an FTP server (fauria/vsftpd). REVERSE_LOOKUP_ENABLE is
// turned off to avoid the ~50s reverse-DNS stall vsftpd otherwise incurs at login
// when the client IP (the Docker gateway) has no PTR record.
func registerFTP(t *testing.T) ([]string, error) {
	ctx := context.Background()

	// Expose the control port plus every passive port (as plain container ports;
	// the docker port parser rejects host:container mappings, and the fixed host
	// binding is applied via HostConfigModifier below).
	exposedPorts := []string{ftpPort}
	for p := ftpPasvMinPort; p <= ftpPasvMaxPort; p++ {
		exposedPorts = append(exposedPorts, fmt.Sprintf("%d/tcp", p))
	}

	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        ftpImage,
			ExposedPorts: exposedPorts,
			Env: map[string]string{
				"FTP_USER":              ftpUsername,
				"FTP_PASS":              ftpPassword,
				"PASV_ADDRESS":          ftpPasvAddress,
				"PASV_MIN_PORT":         strconv.Itoa(ftpPasvMinPort),
				"PASV_MAX_PORT":         strconv.Itoa(ftpPasvMaxPort),
				"REVERSE_LOOKUP_ENABLE": "NO",
			},
			WaitingFor: wait.ForListeningPort(ftpPort),
			// Passive FTP needs fixed, predictable host ports: the server advertises
			// concrete port numbers the client then dials. testcontainers maps
			// exposed ports to random host ports, so override the passive ports with
			// an identical host:container binding here.
			HostConfigModifier: func(hc *container.HostConfig) {
				if hc.PortBindings == nil {
					hc.PortBindings = network.PortMap{}
				}
				for p := ftpPasvMinPort; p <= ftpPasvMaxPort; p++ {
					cp, _ := network.PortFrom(uint16(p), network.TCP)
					hc.PortBindings[cp] = []network.PortBinding{{
						HostIP:   netip.IPv4Unspecified(),
						HostPort: strconv.Itoa(p),
					}}
				}
			},
		},
		Started: true,
	}
	ctr, err := testcontainers.GenericContainer(ctx, req)
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		return nil, err
	}

	host, err := ctr.Host(ctx)
	if err != nil {
		return nil, err
	}

	port, err := ctr.MappedPort(ctx, ftpPort)
	if err != nil {
		return nil, err
	}

	authority := fmt.Sprintf("ftp://%s@%s:%s/", ftpUsername, host, port.Port())
	backend.Register(authority, ftp.NewFileSystem(ftp.WithOptions(ftp.Options{Password: ftpPassword})))
	return []string{authority}, nil
}
