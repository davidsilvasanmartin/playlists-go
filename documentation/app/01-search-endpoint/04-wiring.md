# Part 4 — Wiring

With all packages in place, this part connects them: the router registers routes, and `main.go` constructs the dependency graph and starts the server.

---

## 4.1 `internal/api/router.go` — route registration

The router's only job is to register handlers on a `*http.ServeMux`. It takes the concrete handler types as arguments, making `main.go` the single place that knows about all packages.

```go
package api

import (
    "net/http"

    "github.com/davidsilvasanmartin/playlists-go/internal/search"
)

// NewRouter builds and returns the application's HTTP mux with all routes registered.
func NewRouter(searchHandler *search.Handler) *http.ServeMux {
    mux := http.NewServeMux()

    mux.HandleFunc("GET /api/v1/songs/search", searchHandler.Search)

    return mux
}
```

> **Go 1.22 routing:** The `"GET /api/v1/songs/search"` pattern (method + space + path) is supported natively since Go 1.22. No third-party router is needed.

---

## 4.2 `cmd/server/main.go` — entry point

`main.go` is the composition root. It:
1. Reads config from environment variables
2. Constructs all dependencies in order
3. Starts the HTTP server

```go
package main

import (
    "log"
    "net/http"
    "os"

    "github.com/joho/godotenv"

    "github.com/davidsilvasanmartin/playlists-go/internal/api"
    "github.com/davidsilvasanmartin/playlists-go/internal/musicbrainz"
    "github.com/davidsilvasanmartin/playlists-go/internal/search"
)

func main() {
    // ── config ────────────────────────────────────────────────────────────
    // Load committed defaults first, then let .env override personal values.
    // Both files are optional — errors are silently discarded.
    // In Docker neither file exists; env vars are injected by the container runtime.
    _ = godotenv.Load(".development.env")
    _ = godotenv.Overload(".env")

    port := getEnv("PLAYLISTS_PORT", "8080")
    mbBaseURL := getEnv("PLAYLISTS_MB_BASE_URL", "https://musicbrainz.org")
    mbUserAgent := mustGetEnv("PLAYLISTS_MB_USER_AGENT")

    // ── dependencies ─────────────────────────────────────────────────────
    mbClient := musicbrainz.NewClient(mbBaseURL, mbUserAgent)
    searchService := search.NewService(mbClient)
    searchHandler := search.NewHandler(searchService)

    // ── routing ──────────────────────────────────────────────────────────
    mux := api.NewRouter(searchHandler)

    // ── server ───────────────────────────────────────────────────────────
    addr := ":" + port
    log.Printf("server listening on %s", addr)
    if err := http.ListenAndServe(addr, mux); err != nil {
        log.Fatalf("server error: %v", err)
    }
}

func getEnv(key, fallback string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return fallback
}

func mustGetEnv(key string) string {
    v := os.Getenv(key)
    if v == "" {
        log.Fatalf("required environment variable %q is not set", key)
    }
    return v
}
```

---

## 4.3 Running the server

`.development.env` is already committed to the repo with safe defaults (see section 1.4), so after cloning you can run the server immediately — no setup required:

```bash
# godotenv loads .development.env automatically — no source needed
go run ./cmd/server

# Or build first, then run
make build
./bin/server
```

To override a variable for a single run without editing any file:

```bash
PLAYLISTS_MB_USER_AGENT="playlists/0.0.1 ( alice@example.com )" go run ./cmd/server
```

Test it manually:

```bash
curl -s "http://localhost:8080/api/v1/songs/search?title=Bohemian+Rhapsody&artist=Queen" | jq .
```

Expected shape:
```json
{
  "results": [
    {
      "mbid": "...",
      "title": "Bohemian Rhapsody",
      "artist": "Queen",
      "artistMbid": "...",
      "album": "A Night at the Opera",
      "albumMbid": "...",
      "releaseDate": "1975-11-21",
      "durationMs": 354000,
      "disambiguation": "studio recording"
    }
  ]
}
```

Validation error:
```bash
curl -s "http://localhost:8080/api/v1/songs/search?title=Q&artist=Queen" | jq .
```
```json
{
  "timestamp": "2026-03-21T10:00:00Z",
  "status": 400,
  "error": "Bad Request",
  "message": "Field 'title' must be at least 2 characters",
  "path": "/api/v1/songs/search"
}
```

---

Continue to [Part 5 — Testing](05-testing.md).
