# Part 9 — End-to-End Tests with Testcontainers

## 9.1 What this approach looks like

Your proposed setup has three containers, all started and torn down by Go test code — no Docker Compose file:

```
┌──────────────────────────────────────────────────────────────────┐
│  Go test process (runs on your laptop / CI agent)                │
│                                                                  │
│  test assertions → [app container] → [WireMock container]       │
│                          │                                       │
│                          └──────── [PostgreSQL container] (future)
│                                                                  │
│  All three containers share a private Docker network.           │
│  The test process reaches the app via a mapped host port.       │
└──────────────────────────────────────────────────────────────────┘
```

- **App container** — your Go binary, built from your `Dockerfile`, configured entirely via env vars.
- **WireMock container** — a ready-made HTTP mock server with YAML stub configuration. No Go code needed to define what it returns.
- **PostgreSQL container** (future) — a real Postgres instance, created from scratch every run, seeded from SQL files you control.

The Go test code only needs to start the containers, wait for them to be ready, and then send HTTP requests. All mock configuration lives in YAML files.

---

## 9.2 Can you do this without Docker Compose?

Yes, completely. `testcontainers-go` replaces Docker Compose for test environments. It starts containers, creates networks, mounts files, and tears everything down — all from Go. The result is that container orchestration lives in the same language and the same repo as your tests, rather than in a separate YAML file that has to be kept in sync.

**Why not Docker Compose then?**

| | Testcontainers-go | Docker Compose |
|---|---|---|
| Container lifecycle | Automatic — Go code starts/stops them | Manual — `docker compose up/down` in shell scripts |
| Readiness waiting | Built-in strategies (HTTP, log line, port) | You write shell loops or use `healthcheck:` |
| Port mapping | Go gets the mapped port automatically | You hardcode ports or grep `docker compose port` |
| Test coordination | Go variables pass config between setup and tests | Shell environment variables, fragile |
| CI | Just needs Docker — no extra scripting | Same, but you need a shell wrapper around `go test` |
| Cleanup on crash | Ryuk (Testcontainers daemon) cleans up orphans | Orphan containers accumulate until `docker compose down` |

Testcontainers-go is the industry standard for container-based tests in Go. Docker Compose belongs in the application delivery workflow (running the app for development), not in the test suite.

---

## 9.3 WireMock — what it is and why

WireMock is a dedicated HTTP mock server. You drop YAML files into a `mappings/` directory, WireMock reads them on startup, and it serves the configured responses for any matching request. It is not a Go library — it is a self-contained server you run as a Docker container.

This is the key difference from Part 8's approach:

| | `httptest.Server` (Part 8) | WireMock |
|---|---|---|
| Language | Go | YAML / JSON |
| Lives in | Test code | Files under `e2e/testdata/wiremock/` |
| Changing a mock response | Edit Go code | Edit a YAML file |
| Runs in | Same process as tests | Separate Docker container |
| Suitable for | Simple, code-defined scenarios | Config-driven, YAML-readable scenarios |

WireMock is the most widely used HTTP mock server in the industry (originally from the Java world, but its Docker image makes it language-agnostic). The pattern of keeping mock definitions in config files, separate from test code, scales well as the number of scenarios grows.

---

## 9.4 Prerequisites

### 9.4.1 A Dockerfile

The app container needs a Docker image. Here is a minimal production-quality two-stage `Dockerfile` for the project root:

```dockerfile
# ── Stage 1: build ───────────────────────────────────────────────
FROM golang:1.26-trixie AS builder

LABEL authors="github.com/davidsilvasanmartin"
# The app's version. Use as `docker build --build-arg VERSION=1.4.2 -t playlists:1.4.2`
ARG VERSION=dev

WORKDIR /app

# Download dependencies first — Docker caches this layer until go.mod changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux \
    go build \
    -ldflags="-X main.version=${VERSION}" \
    -o /app/bin/server ./cmd/server

# ── Stage 2: run ─────────────────────────────────────────────────
FROM debian:trixie-slim

COPY --from=builder /app/bin/server /server

ENTRYPOINT ["/server"]
```

This is a two-stage build. The first stage compiles the binary inside a Go image. The second stage copies only the binary into a slim runtime image with no compiler or build tools. The `VERSION` build argument is wired up here too — see section 9.8 for how that works.

`CGO_ENABLED=0` produces a fully static binary that has no C library dependencies.

### 9.4.2 Go dependencies

```bash
go get github.com/testcontainers/testcontainers-go
go get github.com/testcontainers/testcontainers-go/modules/postgres
go mod tidy
```

There is no separate Testcontainers module for WireMock that you need — you use the generic container API, which is actually easier to understand.

---

## 9.5 Project structure

```
playlists/
├── cmd/server/main.go
├── internal/
├── Dockerfile                          ← new
├── e2e/                                ← already exists from Part 8
│   ├── testdata/
│   │   └── wiremock/
│   │       ├── mappings/               ← YAML stub definitions
│   │       │   ├── mb-search-found.yaml
│   │       │   └── mb-search-unavailable.yaml
│   │       └── __files/                ← response body files referenced by stubs
│   │           └── mb-search-found.json
│   ├── setup_test.go                   ← TestMain: starts all containers
│   ├── search_test.go                  ← test cases
│   └── network_test.go                 ← Docker network helper
├── go.mod
└── Makefile
```

> **`mappings/` and `__files/`** are WireMock conventions. `mappings/` contains the routing rules. `__files/` contains response bodies that are too long to inline in YAML. WireMock looks for both directories relative to its working directory, which is `/home/wiremock/` inside its container.

---

## 9.6 WireMock YAML stubs

### What a stub file looks like
File e2e/testdata/wiremock/mappings/mb-search-found.json:
```json
{
  "request": {
    "method": "GET",
    "urlPathPattern": "/ws/2/recording",
    "queryParameters": {
      "query": {
        "contains": "Bohemian"
      }
    }
  },
  "response": {
    "status": 200,
    "headers": {
      "Content-Type": "application/json"
    },
    "bodyFileName": "mb-search-found.json"
  }
}
```

When WireMock receives `GET /ws/2/recording?query=...Bohemian+Rhapsody...`, it returns HTTP 200 with the contents of `__files/mb-search-found.json` as the body.

File e2e/testdata/wiremock/mappings/mb-search-unavailable.yaml:
```json
{
  "request": {
    "method": "GET",
    "urlPathPattern": "/ws/2/recording",
    "queryParameters": {
      "query": {
        "contains": "TRIGGER_503"
      }
    }
  },
  "response": {
    "status": 503
  }
}
```

For the "service unavailable" scenario, the test sends a request containing `TRIGGER_503` in the title. WireMock matches on that string and returns a 503 with no body. The app sees a 503 from its "MusicBrainz client" and maps it to its own 503 response — which is exactly what the test asserts.

> **Why use special trigger strings?** WireMock matches on request content, not on "which test is calling." Since all tests share one running WireMock instance, the only way for a test to reliably get a specific response is to send a request that uniquely matches one stub. Using a recognisable trigger string like `TRIGGER_503` makes this intention obvious. The alternative — resetting and reconfiguring WireMock stubs between tests via its admin API — is possible but more complex.

File e2e/testdata/wiremock/__files/mb-search-found.json:
```json
{
  "recordings": [
    {
      "id": "b1a9c0e2-0000-0000-0000-000000000001",
      "title": "Bohemian Rhapsody",
      "length": 354000,
      "disambiguation": "studio recording",
      "artist-credit": [
        {
          "artist": {
            "id": "0383dadf-2a4e-4d10-a46a-e9e041da8eb3",
            "name": "Queen"
          }
        }
      ],
      "releases": [
        {
          "id": "1dc4c347-a1db-32aa-b14f-bc9cc507b843",
          "title": "A Night at the Opera",
          "date": "1975-11-21"
        }
      ]
    }
  ]
}
```

---

## 9.7 IDE schema support for WireMock YAML

Modern IDEs can validate your stub files against a JSON Schema and show documentation on hover for every property — which methods are valid, what `urlPathPattern` means, which response fields exist, and so on. This is especially useful while you are learning WireMock's syntax.

### How it works

A JSON Schema is a standard file format that describes the shape of a JSON (or YAML) document — what keys are allowed, what types they expect, what values are valid. IDEs that support the YAML language server protocol can download a schema from a URL and apply it to any matching file.

WireMock publishes its stub-mapping schema on SchemaStore, a community-maintained registry of schemas for popular tools. The URL is:

```
https://json.schemastore.org/wiremock-stub.json
```

> Verify this is still current at **https://www.schemastore.org/json/** — search for "wiremock". If the URL has changed, SchemaStore will show the current one.

### JetBrains (GoLand / IntelliJ IDEA)

See the bottom-right of the IDE, where it says "No JSON Schema".
Click on there and select the schema `WireMock stub mapping`.

---

## 9.8 The version endpoint

The e2e test suite uses `GET /api/v1/version` as its readiness probe — the condition Testcontainers polls before handing control to the tests. This section explains how to implement it and why it belongs in every production service.

### Why a version endpoint?

A readiness probe needs an endpoint that:

- always returns 200 when the server is up,
- touches no external dependencies (no database, no MusicBrainz),
- is free to call thousands of times (load balancer health checks, k8s probes).

Beyond tests, a version endpoint is useful in practice:

- **Deployments.** After a rollout you can `curl /api/v1/version` on each instance to verify the new binary is running, without grepping logs or checking image tags.
- **Debugging.** When a bug is reported you immediately know which build the reporter was on, without asking them to find a log line.
- **Load balancer / k8s health checks.** Most platforms let you configure an HTTP path as the liveness or readiness probe. `/api/v1/version` (or a dedicated `/health`) is the right target — it answers in microseconds and never fails due to a downstream outage.
- **Canary releases.** With multiple versions running simultaneously you can confirm traffic is routing to the right instances.

### Where to add it

The router lives in `internal/api/router.go`. `NewRouter` currently takes a `*search.Handler` and a `*zap.Logger`. The cleanest change is to also accept a `version string` and register an inline handler for it — the endpoint has no dependencies and no logic, so it does not need its own package or struct.

```go
// internal/api/router.go

func NewRouter(logger *zap.Logger, searchHandler *search.Handler, version string) http.Handler {
    type versionResponse struct {
        Version string `json:"version"`
    }
    body, _ := json.Marshal(versionResponse{Version: version})

    mux := http.NewServeMux()

    mux.HandleFunc("GET /api/v1/version", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.Write(body)
    })
    mux.HandleFunc("GET /api/v1/songs/search", searchHandler.Search)

    return LoggingMiddleware(logger)(mux)
}
```

The response body is marshalled **once at startup**, not on every request. `json.Marshal` on a one-field struct is safe to call at init time and produces a fixed `[]byte` that all requests share.

Add `"encoding/json"` to the import block in `router.go`.

### Injecting the version string

The version value should come from the build, not from the running binary's environment. The standard Go way is `-ldflags`:

```bash
go build -ldflags="-X main.version=1.4.2" ./cmd/server
```

This sets the package-level variable `version` in `package main` at link time, without touching source code. Declare it in `cmd/server/main.go`:

```go
// cmd/server/main.go

// version is set at build time via -ldflags="-X main.version=<value>".
// It defaults to "dev" for local builds that omit the flag.
var version = "dev"
```

Then pass it through to `NewRouter`:

```go
// cmd/server/main.go — inside main()

mux := api.NewRouter(logger, searchHandler, version)
```

In production, your Makefile or CI pipeline sets the flag. A common pattern is to use the Git tag or commit SHA:

```makefile
VERSION ?= $(shell git describe --tags --always --dirty)

build:
    go build -ldflags="-X main.version=$(VERSION)" -o bin/server ./cmd/server
```

`git describe --tags --always --dirty` produces values like `v1.4.2`, `v1.4.2-3-gabcdef0`, or `v1.4.2-dirty` depending on whether you are on a tag, ahead of one, or have uncommitted changes. `--always` ensures it falls back to a plain commit SHA if no tags exist yet.

In the `Dockerfile`, the `ARG VERSION=dev` and the `-ldflags` flag are already wired up in section 9.4.1. You only need to pass the argument at build time:

```bash
docker build --build-arg VERSION=1.4.2 -t playlists:1.4.2 .
```

For the e2e tests you do not need to pass `VERSION` — the default `"dev"` is fine. The probe only checks that the server responds with 200; it does not assert on the version string.

---

## 9.9 The Go test code

### `e2e/network_test.go`

```go
//go:build e2e

package e2e_test

import (
    "context"

    "github.com/testcontainers/testcontainers-go"
    "github.com/testcontainers/testcontainers-go/network"
)

// createNetwork creates a Docker bridge network shared by all containers.
// Containers on the same network can reach each other by their alias
// (e.g. "http://wiremock:8080") without exposing ports to the host.
func createNetwork(ctx context.Context) (*testcontainers.DockerNetwork, string, error) {
    net, err := network.New(ctx)
    if err != nil {
        return nil, "", err
    }
    return net, net.Name, nil
}
```

### `e2e/setup_test.go`

This file contains `TestMain`, which is Go's hook that runs once before all tests in the package. Think of it as the constructor and destructor of the entire test suite.

```go
//go:build e2e

package e2e_test

import (
    "context"
    "fmt"
    "io"
    "net/http"
    "os"
    "path/filepath"
    "runtime"
    "testing"
    "time"

    "github.com/testcontainers/testcontainers-go"
    "github.com/testcontainers/testcontainers-go/wait"
)

// appURL is the base URL of the app container. Set once in TestMain, read by all tests.
var appURL string

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
        Image:        "wiremock/wiremock:3.5.4",
        ExposedPorts: []string{"8080/tcp"},
        Networks:     []string{networkName},
        NetworkAliases: map[string][]string{
            // "wiremock" is the hostname other containers use to reach this one.
            networkName: {"wiremock"},
        },
        // Copy the local YAML stubs and response files into the container before it starts.
        // WireMock reads /home/wiremock/mappings/ on startup.
        // CopyFileToContainer handles directories automatically.
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

// stdoutLogConsumer streams container log lines to stderr so they appear
// in `go test -v` output. Attach it via LogConsumerCfg when debugging.
type stdoutLogConsumer struct{}

func (s *stdoutLogConsumer) Accept(l testcontainers.Log) {
    fmt.Fprint(os.Stderr, string(l.Content))
}

// startApp builds the app Docker image from the project root Dockerfile and
// starts it, pointed at the WireMock container for outbound MusicBrainz calls.
func startApp(ctx context.Context, networkName, wiremockURL string) (testcontainers.Container, error) {
    // Walk up from e2e/ to the project root where the Dockerfile lives.
    _, thisFile, _, _ := runtime.Caller(0)
    projectRoot := filepath.Join(filepath.Dir(thisFile), "..")

    req := testcontainers.ContainerRequest{
        // Build the image from the project Dockerfile.
        // Testcontainers rebuilds it every run; see section 9.11 for caching.
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
            "PLAYLISTS_LOG_LEVEL":     "debug", // change to "error" to suppress logs
            "PLAYLISTS_LOG_FORMAT":    "dev",   // change to "json" for structured output
        },
        // Stream container logs to stderr so they appear in `go test -v` output.
        // Remove LogConsumerCfg (or set Consumers to nil) once debugging is done.
        LogConsumerCfg: &testcontainers.LogConsumerConfig{
            Consumers: []testcontainers.LogConsumer{&stdoutLogConsumer{}},
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
```

> **`MappedPort`** — Docker does not expose container ports directly on the host. Instead it maps them to random high ports (e.g. `8080` in the container → `54321` on the host). `app.MappedPort(ctx, "8080")` returns that random host port. This is why you cannot hardcode `localhost:8080` in the test — the actual port is only known after the container starts.

> **`NetworkAliases`** — within the Docker network, `wiremock` resolves to the WireMock container's IP. When the app container calls `http://wiremock:8080/ws/2/recording`, it reaches WireMock. This is the DNS resolution inside a Docker network, not the public internet.

> **`WaitingFor`** — Testcontainers polls this condition before handing control back to your test code. For WireMock we wait for its `/__admin/health` endpoint to return 200. For the app we poll `/api/v1/version`, which is a dependency-free endpoint that always returns 200 — no WireMock interaction, no ambiguity about what status code to expect. This is why a `/version` (or `/health`) endpoint is worth adding to every service: it gives infrastructure tooling a clean, always-cheap liveness signal.

> **`LogConsumerCfg` / debugging container logs** — Containers are short-lived: once `TestMain` exits they are torn down, and their logs are gone. `LogConsumerCfg` tells Testcontainers to stream log lines to your `Accept` method in real time, while the container is running. The `stdoutLogConsumer` above writes to `os.Stderr`, which `go test -v` prints to the terminal. Combined with `PLAYLISTS_LOG_LEVEL=debug` and `PLAYLISTS_LOG_FORMAT=dev`, you get coloured, human-readable log lines from inside the container interleaved with your test output — exactly what you need when a test fails and you cannot attach a debugger to a container. Once debugging is done, set the env vars back to `error` / `json` and remove `LogConsumerCfg` to keep test output clean.

### `e2e/search_test.go`

```go
//go:build e2e

package e2e_test

import (
    "encoding/json"
    "net/http"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestE2E_Search_HappyPath(t *testing.T) {
    resp, err := http.Get(appURL + "/api/v1/songs/search?title=Bohemian+Rhapsody&artist=Queen")
    require.NoError(t, err)
    defer resp.Body.Close()

    assert.Equal(t, http.StatusOK, resp.StatusCode)
    assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

    var body struct {
        Results []struct {
            MBID        string `json:"mbid"`
            Title       string `json:"title"`
            Artist      string `json:"artist"`
            Album       string `json:"album"`
            ReleaseDate string `json:"releaseDate"`
            DurationMs  int    `json:"durationMs"`
        } `json:"results"`
    }
    require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
    require.Len(t, body.Results, 1)

    first := body.Results[0]
    assert.Equal(t, "Bohemian Rhapsody", first.Title)
    assert.Equal(t, "Queen", first.Artist)
    assert.Equal(t, "A Night at the Opera", first.Album)
    assert.Equal(t, "1975-11-21", first.ReleaseDate)
    assert.Equal(t, 354000, first.DurationMs)
}

func TestE2E_Search_ValidationError_TitleTooShort(t *testing.T) {
    resp, err := http.Get(appURL + "/api/v1/songs/search?title=X&artist=Queen")
    require.NoError(t, err)
    defer resp.Body.Close()

    assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

    var body map[string]any
    require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
    assert.Equal(t, float64(400), body["status"])
    assert.Contains(t, body["message"], "title")
}

func TestE2E_Search_MusicBrainzUnavailable(t *testing.T) {
    // The "TRIGGER_503" stub in mappings/mb-search-unavailable.yaml matches
    // any query containing this string and returns 503.
    resp, err := http.Get(appURL + "/api/v1/songs/search?title=TRIGGER_503&artist=test")
    require.NoError(t, err)
    defer resp.Body.Close()

    assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}
```

Notice how clean the tests are. There is no mock setup code at all — the routing logic is in the YAML files, and the tests just make requests and check responses.

---

## 9.10 Adding PostgreSQL (when you need it)

When the app gains a database, add a third container to `TestMain`. You do not need to change the test files at all.

```go
// In setup_test.go, add after startWireMock:

import "github.com/testcontainers/testcontainers-go/modules/postgres"

db, err := startPostgres(ctx, networkName)
if err != nil {
    fmt.Fprintf(os.Stderr, "start postgres: %v\n", err)
    os.Exit(1)
}
defer db.Terminate(ctx)

// Get the connection string and pass it to the app via env.
dbURL, err := db.ConnectionString(ctx, "sslmode=disable")
// ... add dbURL to the app's Env map
```

```go
func startPostgres(ctx context.Context, networkName string) (*postgres.PostgresContainer, error) {
    _, thisFile, _, _ := runtime.Caller(0)
    sqlDir := filepath.Join(filepath.Dir(thisFile), "testdata", "sql")

    return postgres.RunContainer(ctx,
        testcontainers.WithImage("postgres:16-alpine"),
        postgres.WithDatabase("playlists_test"),
        postgres.WithUsername("test"),
        postgres.WithPassword("test"),
        postgres.WithInitScripts(
            // These run in filename order on container first start.
            filepath.Join(sqlDir, "01-schema.sql"),   // CREATE TABLE ...
            filepath.Join(sqlDir, "02-fixtures.sql"),  // INSERT INTO ...
        ),
        testcontainers.WithNetworks(networkName),
        testcontainers.WithNetworkAliases(networkName, "postgres"),
        // Wait until Postgres is actually accepting connections, not just listening.
        testcontainers.WithWaitStrategy(
            wait.ForLog("database system is ready to accept connections").
                WithOccurrence(2). // Postgres logs this twice on first start
                WithStartupTimeout(30 * time.Second),
        ),
    )
}
```

The database is created from scratch and seeded from your SQL files every time `make teste2e` runs. Because the container is torn down at the end of the test run, there is no leftover state.

---

## 9.11 Makefile

```makefile
.PHONY: teste2e

teste2e:
	go test -v -count=1 -tags=e2e -timeout=120s ./e2e/...
```

`-count=1` disables Go's test result cache, ensuring tests always re-run even if no Go files changed (important when you only change YAML stub files). `-timeout=120s` gives enough headroom for Docker image builds.

> **Pre-building the image (optional speed-up):** Testcontainers rebuilds the Docker image from the `Dockerfile` on every `teste2e` run. For a small app this takes about 10–30 seconds. If this is too slow you can pre-build the image and tell the test to use it by name:
> ```go
> // Instead of FromDockerfile, use a pre-built image:
> Image: "playlists:e2e-test",
> ```
> ```makefile
> teste2e: docker-build
>     go test -v -count=1 -tags=e2e -timeout=120s ./e2e/...
>
> docker-build:
>     docker build -t playlists:e2e-test .
> ```

---

## 9.12 CI pipeline

Because Testcontainers only needs Docker (not Docker Compose, not a running database service), your CI config is simple. Example for GitHub Actions:

```yaml
# .github/workflows/e2e.yml
name: E2E tests

on: [push, pull_request]

jobs:
  e2e:
    runs-on: ubuntu-latest   # Docker is available on all GitHub-hosted runners
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      - name: Run E2E tests
        run: make teste2e
```

That is the entire CI config. No `services:` block, no `docker-compose up`, no waiting scripts. Testcontainers handles everything inside the `go test` call.

---

## 9.13 Honest assessment: pros and cons

### What this approach does well

- **True production parity.** The Docker image, the environment variables, the container networking — it is the same shape as what you deploy. The in-process tests in Part 5 can never catch a bug in the `Dockerfile` or a missing env var in the container config.
- **YAML mock configuration.** Adding or changing a mock scenario means editing a YAML file, not a Go source file. Non-engineers (QA, product) can read and update stubs.
- **Future-proof.** Adding the PostgreSQL container is an incremental change. The test assertions do not change at all.
- **CI is trivial.** One `go test` call. No shell scripts. Same command locally and in the pipeline.
- **Automatic cleanup.** Testcontainers ships with a background daemon called Ryuk that monitors for orphaned containers and removes them — even if your test process crashes.

### What this approach costs

- **Docker is required.** You cannot run `make teste2e` on a machine without Docker. This matters for contributors or CI environments where Docker is unavailable or restricted.
- **Slower than all other options.** Building the Docker image takes 10–30 seconds. Starting three containers and waiting for readiness takes another 10–20 seconds. A full e2e run on a cold machine can take 60–90 seconds before the first test runs.
- **WireMock's trigger-string pattern is a workaround.** Because all tests share one WireMock instance, different error scenarios need different request patterns to route to different stubs. This is workable but less elegant than Part 8's approach where each test sets its own handler directly. The alternative — resetting WireMock between tests via its `/__admin/mappings/reset` API — is possible but adds complexity.
- **`FromDockerfile` rebuilds every run.** Without the pre-build step in section 9.11, the image is rebuilt from scratch on each `go test` call. This is safe but slow.
- **More moving parts.** Three containers, a Docker network, a Testcontainers version to manage, a WireMock image version to pin. Each is a dependency that can break.

---

## 9.14 Comparison of all approaches

| | Unit + in-process integration (Part 5) | Option A: binary + Go mock (Part 8) | Option B: binary + Testcontainers (Part 8) | **This approach (Part 9)** |
|---|---|---|---|---|
| Tests the compiled binary | No — wired in Go code | Yes | Yes | Yes |
| Tests the Docker image | No | No | Yes | Yes |
| Mock config language | Go | Go | Go | **YAML** |
| Requires Docker | No | No | Yes | Yes |
| Speed (time to first test) | < 1s | < 1s | 10–20s | 30–90s |
| Future PostgreSQL | No | Hard | Possible | **Built-in** |
| CI complexity | Trivial | Simple | Simple | **Simple** |
| Good for | Daily development loop | Quick full-stack smoke test | Verifying the Docker image | **Pre-release / CI gate** |

### Recommended strategy

Run all four layers:

1. **`make test`** — unit tests. Run on every save. Milliseconds.
2. **`make testint`** — in-process integration tests (Part 5). Run before every commit. A few seconds.
3. **`make teste2e-local`** — Option A from Part 8 (binary + Go mock, no Docker). Run locally when you change wiring or config. Under a second once the binary is built.
4. **`make teste2e`** — this approach (Part 9, Docker containers). Run in CI on every push, and locally before opening a pull request.

Layers 1–3 give fast feedback during development. Layer 4 gives you confidence that the thing you ship actually works. They are complementary, not alternatives.

If you only have bandwidth to implement one e2e layer right now, **start with Option A (Part 8)**. It gives you 80% of the value with 20% of the setup. Migrate to this approach (Part 9) when you have a `Dockerfile` ready and want CI parity.
