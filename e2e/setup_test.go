package e2e

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TODO PIN IMAGES TO A SHA, so that Docker does not download too many of them into my laptop
// TODO find out how to debug

// appURL is the base URL of the app container. Set once in TestMain, read by all tests
var appURL string

// TestMain is Go's hook that runs one before all tests in the package. Acts
// as the constructor and destructor of the entire test suite
func TestMain(m *testing.M) {
	ctx := context.Background()

	// Resolve the absolute path to this file's directory.
	// Tests in Go run with the working directory set to the package directory,
	// so we could also use os.Getwd(). runtime.Caller is more explicit.
	_, thisFile, _, _ := runtime.Caller(0)
	testdataDir := filepath.Join(filepath.Dir(thisFile), "testdata", "wiremock")

	// ── 1. Shared Docker network ──────────────────────────────────────────────
	network, networkName, err := createNetwork(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create network: %v\n", err)
		os.Exit(1)
	}
	defer network.Remove(ctx)

	// ── 2. WireMock container ─────────────────────────────────────────────────
	wiremock, wiremockInternalURL, err := startWireMock(ctx, networkName, testdataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start wiremock: %v\n", err)
		os.Exit(1)
	}
	defer wiremock.Terminate(ctx)

	// ── 3. App container ──────────────────────────────────────────────────────
	app, err := startApp(ctx, networkName, wiremockInternalURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start app: %v\n", err)
		os.Exit(1)
	}
	defer app.Terminate(ctx)

	// ── 4. Resolve the app's mapped port on the host ──────────────────────────
	host, err := app.Host(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "get app host: %v\n", err)
		os.Exit(1)
	}
	port, err := app.MappedPort(ctx, "8080")
	if err != nil {
		fmt.Fprintf(os.Stderr, "get app port: %v\n", err)
		os.Exit(1)
	}
	appURL = fmt.Sprintf("http://%s:%s", host, port.Port())

	// ── 5. Run all tests ──────────────────────────────────────────────────────
	os.Exit(m.Run())
}

// startWireMock starts a WireMock container and returns:
//   - the container handle (for Terminate)
//   - the internal URL other containers use to reach it (e.g. "http://wiremock:8080")
func startWireMock(ctx context.Context, networkName, testdataDir string) (testcontainers.Container, string, error) {
	mappingsDir := filepath.Join(testdataDir, "mappings")
	filesDir := filepath.Join(testdataDir, "__files")

	req := testcontainers.ContainerRequest{
		Image:        "wiremock/wiremock:latest",
		ExposedPorts: []string{"8080/tcp"},
		Networks:     []string{networkName},
		NetworkAliases: map[string][]string{
			// "wiremock" is the hostname other containers use to reach this one.
			networkName: {"wiremock"},
		},
		// Copy the local YAML stubs and response files into the container before it starts
		Files: []testcontainers.ContainerFile{
			{
				HostFilePath:      mappingsDir,
				ContainerFilePath: "/home/wiremock/mappings",
				FileMode:          0o755,
			},
			{
				HostFilePath:      filesDir,
				ContainerFilePath: "/home/wiremock/__files",
				FileMode:          0o755,
			},
		},
		WaitingFor: wait.ForHTTP("/__admin/health").
			WithPort("8080").
			WithStartupTimeout(30 * time.Second),
	}

	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, "", err
	}

	internalURL := "http://wiremock:8080"
	return c, internalURL, nil
}

// startApp builds the app Docker image from the project root Dockerfile and
// starts it, pointed at the WireMock container for outbound MusicBrainz calls.
func startApp(ctx context.Context, networkName, wiremockURL string) (testcontainers.Container, error) {
	// Walk up from e2e/ to the project root where the Dockerfile lives.
	_, thisFile, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(thisFile), "..")

	req := testcontainers.ContainerRequest{
		// Build the image from the project Dockerfile.
		// Testcontainers rebuilds it every run; see section 9.10 for caching.
		FromDockerfile: testcontainers.FromDockerfile{
			Context:        projectRoot,
			Dockerfile:     "Dockerfile",
			BuildLogWriter: io.Discard, // set to os.Stderr to debug image build failures
			KeepImage:      false,      // do not reuse between runs
		},
		ExposedPorts: []string{"8080/tcp"},
		Networks:     []string{networkName},
		Env: map[string]string{
			"PLAYLISTS_MB_BASE_URL":   wiremockURL,
			"PLAYLISTS_MB_USER_AGENT": "playlists-e2e/0.0.1 ( test@example.com )",
			"PLAYLISTS_LOG_LEVEL":     "error", // suppress logs during tests
			"PLAYLISTS_LOG_FORMAT":    "json",
		},
		WaitingFor: wait.ForHTTP("/api/v1/version").
			WithPort("8080").
			WithStartupTimeout(60 * time.Second),
	}

	return testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
}

// createNetwork creates a Docker bridge network shared by all containers.
// Containers on the same network can reach each other by their alias
// (e.g. "http://wiremock:8080") without exposing ports to the host.
func createNetwork(ctx context.Context) (*testcontainers.DockerNetwork, string, error) {
	net, err := network.New(ctx)
	if err != nil {
		return nil, "", err
	}
	return net, net.Name, err
}
