# Part 8 — End-to-End Testing

## 8.1 Clearing up terminology

Part 5 calls certain tests "integration tests." That name is defensible — they do test multiple layers integrating together — but they are not what you are picturing, and your instinct is correct.

Here is the full picture:

| Layer | What runs | What it tests |
|---|---|---|
| Unit | Pure functions, mocked dependencies | Logic in isolation |
| In-process integration (Part 5) | Real handler + service + client, wired in Go test code | HTTP contract, validation, JSON serialisation, error mapping |
| **End-to-end (this part)** | **The real compiled binary, started as a process** | **`main.go` wiring, config loading, the full request path exactly as production sees it** |

The in-process tests in Part 5 are valuable — they are fast and cover a lot. But they cannot catch bugs in `main.go` itself (wrong env var name, wrong wiring order, missing middleware). End-to-end tests cover that gap.

---

## 8.2 The key insight: `PLAYLISTS_MB_BASE_URL`

The app already reads the MusicBrainz base URL from an environment variable:

```go
// cmd/server/main.go
mbBaseURL := getEnv("PLAYLISTS_MB_BASE_URL", "https://musicbrainz.org")
mbClient := musicbrainz.NewClient(mbBaseURL, mbUserAgent, logger)
```

This is the seam you need. By starting the binary with `PLAYLISTS_MB_BASE_URL` pointing at a fake server you control, you intercept all outbound MusicBrainz calls without changing any production code and without needing Docker.

---

## 8.3 The available approaches

### Option A — Real binary + Go mock server (no Docker)

Start the compiled binary as a child process from within your Go test. Run a `httptest.Server` in the same test process to fake MusicBrainz. Pass the mock's URL to the binary via env var.

```
┌─────────────────────────────────────────────────────┐
│  Go test process                                     │
│                                                      │
│  httptest.Server ←─── app binary (child process)    │
│  (fake MusicBrainz)         │                        │
│                             ↑                        │
│  http.Get ──────────────────┘                        │
│  (your test assertions)                              │
└─────────────────────────────────────────────────────┘
```

**Pros**
- No Docker required — works anywhere Go is installed
- Fastest startup (the binary starts in milliseconds)
- Tests the exact binary that will be deployed
- Everything is plain Go — no new tools to learn
- You control mock responses with the same `httptest` you already know

**Cons**
- You must build the binary before running the tests (one `go build` step)
- If the test process crashes hard, the child process may be left running (handled with `t.Cleanup`, shown below)
- Does not test the Docker image itself

---

### Option B — Testcontainers-go (Docker-based)

`github.com/testcontainers/testcontainers-go` is a Go library that starts and stops Docker containers programmatically from inside your Go tests. You build a Docker image of your app, and Testcontainers starts it as a container.

```
┌─────────────────────────────────────────────────────┐
│  Go test process                                     │
│                                                      │
│  http.Get ──→ [app container] ──→ [mock container]  │
│                                   (WireMock or       │
│                                    httptest on host) │
└─────────────────────────────────────────────────────┘
```

**Pros**
- Tests the Docker image — the thing that actually ships
- True environment parity with CI and production
- Industry standard for containerised services (very popular in Java, growing fast in Go)
- Handles container lifecycle (start, wait for readiness, stop) in Go code — no shell scripts

**Cons**
- Requires Docker to be running
- Slower: building the image and starting a container takes several seconds
- More setup than Option A
- For the mock server you have two sub-choices:
  - Run a mock container (e.g. WireMock — a Java HTTP mock server) — adds a second container and a non-Go dependency
  - Point the app container at a `httptest.Server` running in the test process on the host, reachable from the container via `host.docker.internal` (Mac/Windows) or `172.17.0.1` (Linux) — simpler, no extra container

---

### Option C — Docker Compose + Makefile (manual orchestration)

Write a `docker-compose.yml` that starts the app and a mock server (e.g. WireMock). Run `docker compose up` from a Makefile target before your tests, then `docker compose down` afterwards. The tests themselves are just Go HTTP calls against `localhost`.

**Pros**
- Familiar tooling if you already use Docker Compose
- Easy to inspect running containers while tests are in progress

**Cons**
- Orchestration lives outside Go — shell scripts and Makefiles, not test code
- Harder to share state between the mock setup and the test assertions
- Container startup is not automatically waited on; you need shell-level health-check loops
- Not idiomatic Go — the community has moved towards Testcontainers

---

## 8.4 Recommendation

**Start with Option A.** It is the right choice for this project at this stage because:

- You have no Docker image yet. Option B requires one.
- It tests the real binary with zero extra dependencies.
- It is pure Go and takes about 30 lines of setup.
- The mock server is the same `httptest` you already understand from Part 5.

When you have a Docker image and want to verify it runs correctly end-to-end, add Option B on top. The test assertions themselves will be nearly identical; only the binary-startup code changes.

Skip Option C. It gives you the downsides of a container (slower, Docker required) without the Go integration benefits of Testcontainers.

---

## 8.5 Implementing Option A

### Project layout

Put e2e tests in a top-level `e2e/` directory inside the same Go module. Use a build tag so they are never run by `make test` or `make testint`:

```
playlists/
├── cmd/
│   └── server/
│       └── main.go
├── internal/
│   └── ...
├── e2e/                        ← new
│   ├── search_test.go
│   └── helpers_test.go
├── go.mod
└── Makefile
```

> **Why the same module and not a separate one?** A separate module is useful when the test project is maintained by a different team or deployed separately. For a single-developer project this is unnecessary friction — you would have to `go get` your own app and keep versions in sync. One module is simpler.

### Build tag

Every file in `e2e/` starts with:

```go
//go:build e2e
```

This keeps them invisible to `go test ./...`. They only compile when you explicitly pass `-tags=e2e`.

### `TestMain` — the setup and teardown hook

Go's testing package calls `TestMain(m *testing.M)` before and after the test suite if you define it. This is where you start the binary and the mock server, wait for readiness, and clean up. It is the Go equivalent of a JUnit `@BeforeAll`/`@AfterAll`.

```
TestMain starts
   ↓
Start fake MusicBrainz (httptest.Server)
   ↓
Build / locate the binary
   ↓
Start the binary as a child process (with mock URL in env)
   ↓
Wait until the app is accepting connections
   ↓
m.Run() ← all Test* functions run here
   ↓
Kill the binary, close the mock server
   ↓
os.Exit with the test result code
```

### `e2e/helpers_test.go`

```go
//go:build e2e

package e2e_test

import (
    "fmt"
    "net"
    "net/http"
    "net/http/httptest"
    "os"
    "os/exec"
    "testing"
    "time"
)

// appURL is the base URL of the running app binary. Set by TestMain.
var appURL string

// TestMain runs once before all tests in this package.
// It starts the fake MusicBrainz server and the real app binary,
// waits for the app to be ready, then hands off to m.Run().
func TestMain(m *testing.M) {
    // 1. Start the fake MusicBrainz server.
    //    This runs in the same process as the tests.
    fakeMB := startFakeMusicBrainz()
    defer fakeMB.Close()

    // 2. Pick a free port for the app.
    port := freePort()
    appURL = fmt.Sprintf("http://localhost:%d", port)

    // 3. Start the real app binary as a child process.
    cmd := exec.Command("../bin/server") // path to the compiled binary
    cmd.Env = append(os.Environ(),
        fmt.Sprintf("PLAYLISTS_PORT=%d", port),
        "PLAYLISTS_MB_BASE_URL="+fakeMB.URL,
        "PLAYLISTS_MB_USER_AGENT=e2e-test/0.0.1 ( test@example.com )",
        "PLAYLISTS_LOG_LEVEL=error", // silence logs during tests
        "PLAYLISTS_LOG_FORMAT=json",
    )
    cmd.Stdout = os.Stdout // uncomment to see app logs while debugging
    cmd.Stderr = os.Stderr
    if err := cmd.Start(); err != nil {
        fmt.Fprintf(os.Stderr, "failed to start server: %v\n", err)
        os.Exit(1)
    }

    // 4. Wait until the app is accepting connections.
    waitForServer(appURL)

    // 5. Run all Test* functions.
    code := m.Run()

    // 6. Kill the app and exit with the test result.
    _ = cmd.Process.Kill()
    os.Exit(code)
}

// startFakeMusicBrainz returns an httptest.Server that acts as MusicBrainz.
// Individual tests can override this by setting fakeMBHandler before calling.
// For the default behaviour we return a 503 so tests that forget to configure
// the mock fail loudly rather than silently hitting a nil response.
func startFakeMusicBrainz() *httptest.Server {
    return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Tests override this via fakeMBHandler (see below).
        if fakeMBHandler != nil {
            fakeMBHandler(w, r)
            return
        }
        http.Error(w, "no mock configured", http.StatusServiceUnavailable)
    }))
}

// fakeMBHandler is set by individual tests to control what the fake
// MusicBrainz server returns. Reset to nil after each test.
var fakeMBHandler func(http.ResponseWriter, *http.Request)

// freePort asks the OS for an available TCP port.
func freePort() int {
    l, err := net.Listen("tcp", ":0")
    if err != nil {
        panic(err)
    }
    defer l.Close()
    return l.Addr().(*net.TCPAddr).Port
}

// waitForServer polls appURL until it gets any HTTP response or times out.
// We use the search endpoint because there is no dedicated health endpoint yet.
func waitForServer(baseURL string) {
    deadline := time.Now().Add(10 * time.Second)
    probe := baseURL + "/api/v1/songs/search?title=probe&artist=probe"
    for time.Now().Before(deadline) {
        resp, err := http.Get(probe)
        if err == nil {
            resp.Body.Close()
            return // server is up (any response code counts)
        }
        time.Sleep(50 * time.Millisecond)
    }
    fmt.Fprintln(os.Stderr, "server did not become ready in time")
    os.Exit(1)
}

// setFakeMB sets the MusicBrainz mock handler for the current test and
// registers a cleanup that resets it afterwards.
func setFakeMB(t *testing.T, handler func(http.ResponseWriter, *http.Request)) {
    t.Helper()
    fakeMBHandler = handler
    t.Cleanup(func() { fakeMBHandler = nil })
}
```

> **`freePort`** — asks the OS to bind to port 0, which picks a random available port, then immediately closes the listener and returns the port number. There is a tiny race window between releasing the port and the app claiming it, but in practice it is never a problem on a developer machine or CI.

> **`waitForServer`** — a simple retry loop. The app binary needs a moment to start up. Rather than `time.Sleep(1 * time.Second)` (which is either too short or wastes time), we poll every 50ms and give up after 10 seconds. Any HTTP response — even a 400 Bad Request from the validation check — means the server is up.

### `e2e/search_test.go`

```go
//go:build e2e

package e2e_test

import (
    "encoding/json"
    "fmt"
    "net/http"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

const mbHappyPathFixture = `{
  "recordings": [
    {
      "id": "b1a9c0e2-0000-0000-0000-000000000001",
      "title": "Bohemian Rhapsody",
      "length": 354000,
      "disambiguation": "studio recording",
      "artist-credit": [{"artist": {"id": "0383dadf-2a4e-4d10-a46a-e9e041da8eb3", "name": "Queen"}}],
      "releases": [{"id": "1dc4c347-a1db-32aa-b14f-bc9cc507b843", "title": "A Night at the Opera", "date": "1975-11-21"}]
    }
  ]
}`

func TestE2E_Search_HappyPath(t *testing.T) {
    setFakeMB(t, func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        fmt.Fprint(w, mbHappyPathFixture)
    })

    resp, err := http.Get(appURL + "/api/v1/songs/search?title=Bohemian+Rhapsody&artist=Queen")
    require.NoError(t, err)
    defer resp.Body.Close()

    assert.Equal(t, http.StatusOK, resp.StatusCode)

    var body struct {
        Results []struct {
            MBID        string `json:"mbid"`
            Title       string `json:"title"`
            Artist      string `json:"artist"`
            ReleaseDate string `json:"releaseDate"`
        } `json:"results"`
    }
    require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
    require.Len(t, body.Results, 1)
    assert.Equal(t, "Bohemian Rhapsody", body.Results[0].Title)
    assert.Equal(t, "Queen", body.Results[0].Artist)
}

func TestE2E_Search_ValidationError(t *testing.T) {
    // No mock needed — validation fires before the app calls MusicBrainz.
    resp, err := http.Get(appURL + "/api/v1/songs/search?title=X&artist=Queen")
    require.NoError(t, err)
    defer resp.Body.Close()

    assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestE2E_Search_MusicBrainzDown(t *testing.T) {
    setFakeMB(t, func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusServiceUnavailable)
    })

    resp, err := http.Get(appURL + "/api/v1/songs/search?title=Bohemian+Rhapsody&artist=Queen")
    require.NoError(t, err)
    defer resp.Body.Close()

    assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}
```

> **Why `var body struct{...}`?** Instead of importing `internal/search.Response`, we decode into an anonymous struct. This is intentional: e2e tests should treat your API as a black box — exactly as an external consumer would. If you import internal types you are testing implementation details rather than the public contract.

### Makefile targets

```makefile
.PHONY: build teste2e

build:
	go build -o bin/server ./cmd/server

teste2e: build
	go test -v -tags=e2e ./e2e/...
```

`teste2e` depends on `build` so the binary is always fresh before the tests run.

### Running the tests

```bash
make teste2e
```

Expected output:

```
=== RUN   TestE2E_Search_HappyPath
--- PASS: TestE2E_Search_HappyPath (0.01s)
=== RUN   TestE2E_Search_ValidationError
--- PASS: TestE2E_Search_ValidationError (0.00s)
=== RUN   TestE2E_Search_MusicBrainzDown
--- PASS: TestE2E_Search_MusicBrainzDown (0.01s)
PASS
ok  	github.com/davidsilvasanmartin/playlists-go/e2e
```

> **Note on the "MB unavailable" test:** the app retries 3 times with a 2-second delay, so this test will take up to 4 seconds. Refer to section 5.5 for making the retry delay configurable so e2e tests can inject a shorter one.

---

## 8.6 When to add Option B (Testcontainers)

Once you have a `Dockerfile`, add Testcontainers if you want to answer the question: *"Does our Docker image work?"* That is a different and complementary question to what Option A answers.

The setup looks like this (outline only, not full code):

```go
import "github.com/testcontainers/testcontainers-go"

func TestMain(m *testing.M) {
    ctx := context.Background()

    // Start the app container.
    req := testcontainers.ContainerRequest{
        Image:        "playlists:latest",       // your Docker image
        ExposedPorts: []string{"8080/tcp"},
        Env: map[string]string{
            "PLAYLISTS_MB_BASE_URL": "http://host.docker.internal:MOCK_PORT",
            // ...
        },
        WaitingFor: wait.ForHTTP("/api/v1/songs/search?title=probe&artist=probe"),
    }
    container, _ := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
        ContainerRequest: req,
        Started:          true,
    })
    defer container.Terminate(ctx)

    // Get the mapped port and set appURL.
    host, _ := container.Host(ctx)
    port, _ := container.MappedPort(ctx, "8080")
    appURL = fmt.Sprintf("http://%s:%s", host, port.Port())

    // The mock server still runs in the test process.
    // The container reaches it via host.docker.internal (Mac/Windows).

    os.Exit(m.Run())
}
```

The test functions themselves (`TestE2E_Search_HappyPath`, etc.) are **identical** to Option A — only `TestMain` changes. This is the real value of the approach: the test assertions are completely decoupled from the infrastructure that starts the app.

---

## 8.7 Summary

| | Option A (binary + Go mock) | Option B (Testcontainers) |
|---|---|---|
| Requires Docker | No | Yes |
| Tests the compiled binary | Yes | Yes |
| Tests the Docker image | No | Yes |
| Startup speed | ~100ms | ~5–10s |
| Complexity | Low | Medium |
| Good for | Daily development, CI without Docker | Pre-release, image verification |

**Start with Option A.** Add Option B when you have a `Dockerfile` and want to verify it.
