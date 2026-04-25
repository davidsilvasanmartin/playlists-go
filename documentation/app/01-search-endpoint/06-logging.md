# Part 6 — Structured Logging with Zap

Logging is the primary tool for understanding what your app is doing while it runs. This part adds structured, levelled logging across the entire application using the [Zap](https://github.com/uber-go/zap) library.

---

## 6.1 Why structured logging?

Traditional logging looks like this:

```
2024/01/15 12:34:56 searching MusicBrainz for "Bohemian Rhapsody" by "Queen"
```

That line is readable to humans, but it is nearly impossible to query programmatically. If you have thousands of log lines per minute in production you need to filter, aggregate, and alert on them — and plain text does not support that well.

**Structured logging** emits each log entry as a machine-readable document (usually JSON) where every piece of data has a named field:

```json
{"level":"info","ts":1705319696.123,"caller":"search/service.go:22","msg":"search","title":"Bohemian Rhapsody","artist":"Queen","results":2}
```

This format lets a log-aggregation tool (Fluentbit, Logstash, Datadog, …) index every field and let you write queries like "show me all requests where status = 503 in the last hour" without parsing free text.

### Why Zap specifically?

Zap is the most widely used structured logging library in the Go ecosystem. It has two distinct APIs:

| API | Example | Notes |
|---|---|---|
| **Logger** (structured) | `logger.Info("msg", zap.String("key", "val"))` | Fastest; all fields are typed |
| **SugaredLogger** | `logger.Sugar().Infow("msg", "key", "val")` | Looser, printf-style; slightly slower |

We will use the non-sugared `*zap.Logger` throughout. Every field is an explicit typed argument (`zap.String`, `zap.Int`, `zap.Error`, etc.). This may feel verbose at first, but it eliminates whole classes of bugs — you can never accidentally pass an integer where a string is expected, and Zap can serialise each field without reflection, making it extremely fast.

---

## 6.2 Installing Zap

Add Zap to your module:

```bash
go get go.uber.org/zap
```

This adds `go.uber.org/zap` and its internal dependency `go.uber.org/zap/zapcore` to `go.mod` and `go.sum`. You will import both in this part.

---

## 6.3 The two logging modes

Zap ships with two pre-built configurations that match the two environments in which the app runs:

### Development mode

```
2024-01-15T12:34:56.123+0000  INFO  search/service.go:22  search  {"title": "Bohemian Rhapsody", "artist": "Queen"}
```

- Human-readable, line-per-entry format
- Coloured level labels (INFO in green, WARN in yellow, ERROR in red)
- Caller file and line number printed on every line
- Stack traces on warnings and above by default

Created with `zap.NewDevelopmentConfig()`.

### Production mode (JSON)

```json
{"level":"info","ts":1705319696.123,"caller":"search/service.go:22","msg":"search","title":"Bohemian Rhapsody","artist":"Queen"}
```

- JSON, one object per line, written to stdout
- In a production system, Fluentbit (or a similar agent) reads this stdout stream and forwards the log entries to your log-aggregation platform (Elasticsearch, Splunk, Datadog, …)
- No colour; level is a machine-readable string
- Timestamps are Unix epoch seconds with millisecond precision

Created with `zap.NewProductionConfig()`.

### Choosing between them with an environment variable

We want `dev` mode during local development and `json` (production) mode everywhere else. Two new variables go into `.development.env` (the committed defaults file):

```
PLAYLISTS_LOG_LEVEL=debug
PLAYLISTS_LOG_FORMAT=dev
```

In production (or CI), neither file is present and these variables are injected by the container runtime. If neither is set, we default to `info` level and JSON format — safe choices for a production system.

---

## 6.4 Building the logger

Create a helper function that reads the two environment variables and constructs the right `*zap.Logger`. This function belongs in `main.go` for now because logger construction is a startup concern.

Add this to `cmd/server/main.go` (we will show the full updated file in 6.11):

```go
import (
    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
)

// buildLogger creates a *zap.Logger configured from environment variables.
//
//   PLAYLISTS_LOG_LEVEL  — one of: debug, info, warn, error  (default: info)
//   PLAYLISTS_LOG_FORMAT — one of: dev, json                 (default: json)
//
// "dev" format uses a coloured, human-readable encoder that prints to stdout.
// "json" format uses a compact JSON encoder suited for log-aggregation pipelines.
func buildLogger(level, format string) (*zap.Logger, error) {
    // zapcore.Level is an integer type that represents log severity.
    // UnmarshalText parses strings like "debug", "info", "warn", "error".
    var zapLevel zapcore.Level
    if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
        // Unknown level string — fall back to info rather than crashing.
        zapLevel = zapcore.InfoLevel
    }

    // zap.NewAtomicLevelAt wraps the level in a struct that can be changed at
    // runtime (useful for dynamic log-level endpoints — not needed yet, but
    // it is the idiomatic way to set a level in Zap configs).
    atomicLevel := zap.NewAtomicLevelAt(zapLevel)

    var cfg zap.Config
    if format == "dev" {
        // NewDevelopmentConfig returns a Config that uses the console encoder
        // (coloured, human-readable). Stack traces are enabled on Warn+.
        cfg = zap.NewDevelopmentConfig()
    } else {
        // NewProductionConfig returns a Config that uses the JSON encoder and
        // writes to stdout. This is the right choice for container environments.
        cfg = zap.NewProductionConfig()
    }

    cfg.Level = atomicLevel

    // Build() compiles the Config into a *zap.Logger. The only realistic error
    // here is an invalid output path, so we treat it as fatal in main().
    return cfg.Build()
}
```

**Key concept — `zap.Config` vs `zap.Logger`:**  
A `zap.Config` is a blueprint (output paths, encoder, level). `cfg.Build()` compiles it into a live `*zap.Logger`. Separating the two makes configuration testable without actually writing to files.

---

## 6.5 Dependency injection — how the logger travels through the app

The logger is a *shared dependency*: many structs need it. There are three common ways to share a logger in Go:

| Approach | How | Problem |
|---|---|---|
| Global variable | `var Log = zap.NewNop()` | Hidden coupling, hard to test |
| Context | `ctx.Value(loggerKey)` | Requires type assertion, implicit contract |
| **Constructor injection** | `NewService(mb, logger)` | Explicit, testable — ✓ our choice |

Constructor injection means: every struct that needs to log receives a `*zap.Logger` as a parameter to its constructor (`NewClient`, `NewService`, `NewHandler`). The struct stores it as a field. The caller (`main.go`) is the only place that creates the logger and passes it down.

This approach is consistent with how the existing code already injects the MusicBrainz client into the search service, so it should feel familiar.

---

## 6.6 HTTP logging middleware

An HTTP middleware is a function that wraps an `http.Handler` and intercepts every request before and/or after the real handler runs. We will use one to log a single line for each HTTP request that the server receives.

The user explicitly does not want to pass the logger through `context.Context`. Instead, the middleware *closes over* the logger — it captures a reference to it in the function literal. This is idiomatic Go and keeps the data flow explicit.

Create a new file `internal/api/middleware.go`:

```go
package api

import (
    "net/http"
    "time"

    "go.uber.org/zap"
)

// statusRecorder wraps http.ResponseWriter so we can capture the HTTP status
// code that the handler writes. The standard ResponseWriter does not expose
// the status after the fact, so we intercept WriteHeader.
type statusRecorder struct {
    http.ResponseWriter
    status int
}

// WriteHeader intercepts the status code before delegating to the real writer.
// If the handler never calls WriteHeader explicitly (which happens when it only
// calls Write), the status defaults to 200, matching net/http behaviour.
func (sr *statusRecorder) WriteHeader(status int) {
    sr.status = status
    sr.ResponseWriter.WriteHeader(status)
}

// LoggingMiddleware returns middleware that logs one structured line per request.
//
// It logs: HTTP method, URL path, response status, elapsed time, and the
// client's remote address.
//
// The logger is captured in the closure — callers do not need to thread it
// through context.Context.
//
// Usage:
//
//   mux := http.NewServeMux()
//   // ... register routes ...
//   return LoggingMiddleware(logger)(mux)
func LoggingMiddleware(logger *zap.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()

            // Wrap the ResponseWriter so we can read the status after the
            // handler returns.  Default to 200 so routes that never call
            // WriteHeader explicitly are logged correctly.
            rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

            // Call the real handler.
            next.ServeHTTP(rec, r)

            // Log after the handler returns so we have the final status and
            // the accurate elapsed time.
            logger.Info("request",
                zap.String("method", r.Method),
                zap.String("path", r.URL.Path),
                zap.String("query", r.URL.RawQuery),
                zap.Int("status", rec.status),
                zap.Duration("duration", time.Since(start)),
                zap.String("remoteAddr", r.RemoteAddr),
            )
        })
    }
}
```

**Why a closure and not a method?**  
Middleware is applied once at startup, not per-request. Returning a function (`func(http.Handler) http.Handler`) is the conventional middleware signature in Go's standard library ecosystem. The outer call (`LoggingMiddleware(logger)`) happens once; the inner `http.HandlerFunc` runs on every request.

**Why embed `http.ResponseWriter`?**  
Go allows struct embedding: `statusRecorder` embeds `http.ResponseWriter`, which means it automatically satisfies the `http.ResponseWriter` interface and forwards all calls (`Header()`, `Write()`, `Flush()`, …) to the wrapped writer. We only need to override `WriteHeader` to intercept the status code.

---

## 6.7 Updating the router

The router now needs to wrap the mux with the logging middleware and accept the logger. Update `internal/api/router.go`:

```go
package api

import (
    "net/http"

    "github.com/davidsilvasanmartin/playlists-go/internal/search"
    "go.uber.org/zap"
)

// NewRouter builds the application's HTTP handler with all routes registered
// and the logging middleware applied.
func NewRouter(logger *zap.Logger, searchHandler *search.Handler) http.Handler {
    mux := http.NewServeMux()

    mux.HandleFunc("GET /api/v1/songs/search", searchHandler.Search)

    // Wrap the entire mux so every request — including 404s for unregistered
    // routes — is logged.
    return LoggingMiddleware(logger)(mux)
}
```

Note that `NewRouter` now returns `http.Handler` instead of `*http.ServeMux`. The middleware wraps the mux and the result is an opaque handler — callers only need the `http.Handler` interface anyway.

---

## 6.8 Updating the MusicBrainz client

The client makes outbound HTTP calls, retries on failure, and enforces rate limiting. These are exactly the kinds of events that are invaluable during debugging. Add a `*zap.Logger` field to `clientImpl` and update `withRetry` to accept one.

**`internal/musicbrainz/client_impl.go` — full updated file:**

```go
package musicbrainz

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "net/url"
    "time"

    "go.uber.org/zap"
    "golang.org/x/time/rate"
)

type clientImpl struct {
    httpClient *http.Client
    limiter    *rate.Limiter
    baseURL    string
    userAgent  string
    logger     *zap.Logger
}

// NewClient initializes and returns a new Client for interacting with the
// MusicBrainz API.
func NewClient(baseURL string, userAgent string, logger *zap.Logger) Client {
    return &clientImpl{
        httpClient: &http.Client{Timeout: 10 * time.Second},
        limiter:    rate.NewLimiter(rate.Every(time.Second), 1),
        baseURL:    baseURL,
        userAgent:  userAgent,
        logger:     logger,
    }
}

func (c *clientImpl) Search(ctx context.Context, title string, artist string) ([]Recording, error) {
    c.logger.Debug("starting MusicBrainz search",
        zap.String("title", title),
        zap.String("artist", artist),
    )

    var result []Recording
    err := withRetry(ctx, 3, 2*time.Second, c.logger, func() error {
        if err := c.limiter.Wait(ctx); err != nil {
            return err
        }
        recordings, err := c.doSearch(ctx, title, artist)
        if err != nil {
            return err
        }
        result = recordings
        return nil
    })
    if err != nil {
        return nil, err
    }

    c.logger.Debug("MusicBrainz search complete",
        zap.String("title", title),
        zap.String("artist", artist),
        zap.Int("recordings", len(result)),
    )
    return result, nil
}

func (c *clientImpl) doSearch(ctx context.Context, title string, artist string) ([]Recording, error) {
    q := fmt.Sprintf(`recording:"%s" AND artistname:"%s"`, title, artist)
    params := url.Values{
        "query": []string{q},
        "fmt":   []string{"json"},
    }
    endpoint := c.baseURL + "/ws/2/recording?" + params.Encode()

    c.logger.Debug("sending HTTP request to MusicBrainz", zap.String("url", endpoint))

    req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
    if err != nil {
        return nil, err
    }
    req.Header.Set("User-Agent", c.userAgent)

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("MusicBrainz returned status %d", resp.StatusCode)
    }

    var mbResp mbSearchResponse
    if err := json.NewDecoder(resp.Body).Decode(&mbResp); err != nil {
        return nil, err
    }

    return mapRecordings(mbResp.Recordings), nil
}

func mapRecordings(raw []mbRecording) []Recording {
    out := make([]Recording, 0, len(raw))
    for _, r := range raw {
        rec := Recording{
            MBID:           r.ID,
            Title:          r.Title,
            DurationMs:     r.Length,
            Disambiguation: r.Disambiguation,
        }
        if len(r.ArtistCredit) > 0 {
            rec.ArtistMBID = r.ArtistCredit[0].Artist.ID
            rec.Artist = r.ArtistCredit[0].Artist.Name
        }
        if len(r.Releases) > 0 {
            rec.AlbumMBID = r.Releases[0].ID
            rec.Album = r.Releases[0].Title
            rec.ReleaseDate = r.Releases[0].Date
        }
        out = append(out, rec)
    }
    return out
}

// withRetry calls fn up to maxAttempts times, waiting delay between failures.
// It logs each failed attempt and gives up after the last one, returning the
// final error.  Context cancellation is respected between retries.
func withRetry(ctx context.Context, maxAttempts int, delay time.Duration, logger *zap.Logger, fn func() error) error {
    var err error
    for i := range maxAttempts {
        err = fn()
        if err == nil {
            return nil
        }

        if i < maxAttempts-1 {
            logger.Warn("MusicBrainz request failed, will retry",
                zap.Int("attempt", i+1),
                zap.Int("maxAttempts", maxAttempts),
                zap.Error(err),
                zap.Duration("retryIn", delay),
            )
            select {
            case <-time.After(delay):
            case <-ctx.Done():
                return ctx.Err()
            }
        }
    }

    logger.Error("MusicBrainz request failed after all retry attempts",
        zap.Int("attempts", maxAttempts),
        zap.Error(err),
    )
    return err
}
```

**What changed:**
- `clientImpl` gains a `logger *zap.Logger` field
- `NewClient` gains a `logger *zap.Logger` parameter and stores it
- `Search` logs at `Debug` level when it starts and when it completes
- `doSearch` logs the outbound URL at `Debug` level (useful for diagnosing query construction)
- `withRetry` now accepts a `*zap.Logger` parameter; it logs each retry at `Warn` and logs final failure at `Error`

**On log levels:** Use `Debug` for information you only need when actively investigating a problem. Use `Info` for normal operational events (server started, request received). Use `Warn` for recoverable issues (retrying). Use `Error` for failures that affect the user or require attention.

---

## 6.9 Updating the search service

The service is thin — it calls the MusicBrainz client and maps the results. Logging here is optional but helps establish a clear boundary: you can see exactly what the service received and what it returned, independently of the HTTP or client layers.

**`internal/search/service.go` — full updated file:**

```go
package search

import (
    "context"

    "github.com/davidsilvasanmartin/playlists-go/internal/musicbrainz"
    "go.uber.org/zap"
)

// Service is the contract for the search business logic.
type Service interface {
    Search(ctx context.Context, title string, artist string) ([]Result, error)
}

type service struct {
    mb     musicbrainz.Client
    logger *zap.Logger
}

func NewService(mb musicbrainz.Client, logger *zap.Logger) Service {
    return &service{mb: mb, logger: logger}
}

func (s *service) Search(ctx context.Context, title string, artist string) ([]Result, error) {
    s.logger.Debug("service.Search called",
        zap.String("title", title),
        zap.String("artist", artist),
    )

    recordings, err := s.mb.Search(ctx, title, artist)
    if err != nil {
        s.logger.Error("MusicBrainz client returned an error",
            zap.String("title", title),
            zap.String("artist", artist),
            zap.Error(err),
        )
        return nil, err
    }

    results := make([]Result, len(recordings))
    for i, r := range recordings {
        results[i] = Result{
            MBID:           r.MBID,
            Title:          r.Title,
            Artist:         r.Artist,
            ArtistMBID:     r.ArtistMBID,
            Album:          r.Album,
            AlbumMBID:      r.AlbumMBID,
            ReleaseDate:    r.ReleaseDate,
            DurationMs:     r.DurationMs,
            Disambiguation: r.Disambiguation,
        }
    }

    s.logger.Debug("service.Search returning results",
        zap.String("title", title),
        zap.String("artist", artist),
        zap.Int("count", len(results)),
    )
    return results, nil
}
```

The service logs at `Debug` level (these are internal traces, not interesting in production) and at `Error` level if the client returns a failure (that error will propagate up, so this adds context to the chain of log events).

---

## 6.10 Updating the search handler

The handler is responsible for parsing and validating HTTP requests and mapping service results to HTTP responses. It is a good place to log validation failures (so you can see what bad requests are coming in) and unexpected service errors.

**`internal/search/handler.go` — full updated file:**

```go
package search

import (
    "encoding/json"
    "net/http"
    "time"

    "go.uber.org/zap"
)

// Handler handles HTTP requests for the search feature.
type Handler struct {
    service Service
    logger  *zap.Logger
}

func NewHandler(svc Service, logger *zap.Logger) *Handler {
    return &Handler{service: svc, logger: logger}
}

func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
    title := r.URL.Query().Get("title")
    artist := r.URL.Query().Get("artist")

    if len(title) < 2 {
        h.logger.Debug("validation failed: title too short",
            zap.String("title", title),
        )
        writeError(w, r, http.StatusBadRequest, "Field 'title' must be at least 2 characters")
        return
    }
    if len(artist) < 2 {
        h.logger.Debug("validation failed: artist too short",
            zap.String("artist", artist),
        )
        writeError(w, r, http.StatusBadRequest, "Field 'artist' must be at least 2 characters")
        return
    }

    results, err := h.service.Search(r.Context(), title, artist)
    if err != nil {
        h.logger.Error("search service error",
            zap.String("title", title),
            zap.String("artist", artist),
            zap.Error(err),
        )
        writeError(w, r, http.StatusServiceUnavailable, "MusicBrainz API is currently unreachable. Please try again later")
        return
    }

    writeJSON(w, http.StatusOK, Response{Results: results})
}

// ── response helpers ──────────────────────────────────────────────────────────

type apiError struct {
    Timestamp string `json:"timestamp"`
    Status    int    `json:"status"`
    Error     string `json:"error"`
    Message   string `json:"message"`
    Path      string `json:"path"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    _ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, message string) {
    writeJSON(w, status, apiError{
        Timestamp: time.Now().UTC().Format(time.RFC3339),
        Status:    status,
        Error:     http.StatusText(status),
        Message:   message,
        Path:      r.URL.Path,
    })
}
```

**Why `Debug` for validation failures, not `Warn`?**  
Validation failures are caused by the client sending a bad request — they are expected and normal. Logging them at `Warn` would produce noise in production dashboards and make it harder to spot real problems. `Debug` is appropriate because you only care about them when debugging a specific issue. The HTTP logging middleware will already log the 400 status code at `Info`, so the event is not invisible.

---

## 6.11 Updating main.go

The entry point creates the logger and threads it through every constructor. Here is the full updated `cmd/server/main.go`:

```go
package main

import (
    "log"
    "net/http"
    "os"

    "github.com/davidsilvasanmartin/playlists-go/internal/api"
    "github.com/davidsilvasanmartin/playlists-go/internal/musicbrainz"
    "github.com/davidsilvasanmartin/playlists-go/internal/search"
    "github.com/joho/godotenv"
    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
)

func main() {
    // ── config ────────────────────────────────────────────────────────────────
    _ = godotenv.Load(".development.env")
    _ = godotenv.Overload(".env")

    port        := getEnv("PLAYLISTS_PORT", "8080")
    mbBaseURL   := getEnv("PLAYLISTS_MB_BASE_URL", "https://musicbrainz.org")
    mbUserAgent := mustGetEnv("PLAYLISTS_MB_USER_AGENT")
    logLevel    := getEnv("PLAYLISTS_LOG_LEVEL", "info")
    logFormat   := getEnv("PLAYLISTS_LOG_FORMAT", "json")

    // ── logger ────────────────────────────────────────────────────────────────
    logger, err := buildLogger(logLevel, logFormat)
    if err != nil {
        // We cannot log this error with Zap because the logger failed to build.
        // Fall back to the standard library.
        log.Fatalf("failed to build logger: %v", err)
    }
    // Flush any buffered log entries before the process exits.
    // This is important in production to avoid losing the last few log lines.
    defer logger.Sync() //nolint:errcheck

    logger.Info("starting server",
        zap.String("port", port),
        zap.String("logLevel", logLevel),
        zap.String("logFormat", logFormat),
    )

    // ── dependencies ──────────────────────────────────────────────────────────
    mbClient      := musicbrainz.NewClient(mbBaseURL, mbUserAgent, logger)
    searchService := search.NewService(mbClient, logger)
    searchHandler := search.NewHandler(searchService, logger)

    // ── routing ───────────────────────────────────────────────────────────────
    router := api.NewRouter(logger, searchHandler)

    // ── server ────────────────────────────────────────────────────────────────
    addr := ":" + port
    logger.Info("server ready", zap.String("addr", addr))
    if err := http.ListenAndServe(addr, router); err != nil {
        logger.Fatal("server error", zap.Error(err))
    }
}

// buildLogger constructs a *zap.Logger from the given level and format strings.
//
//   level  — one of: debug, info, warn, error  (default: info on parse failure)
//   format — one of: dev, json                 (default: json)
func buildLogger(level, format string) (*zap.Logger, error) {
    var zapLevel zapcore.Level
    if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
        zapLevel = zapcore.InfoLevel
    }

    atomicLevel := zap.NewAtomicLevelAt(zapLevel)

    var cfg zap.Config
    if format == "dev" {
        cfg = zap.NewDevelopmentConfig()
    } else {
        cfg = zap.NewProductionConfig()
    }
    cfg.Level = atomicLevel

    return cfg.Build()
}

func getEnv(key, fallback string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return fallback
}

// mustGetEnv returns the value of an environment variable or crashes the
// process if the variable is not set or is empty.
func mustGetEnv(key string) string {
    v := os.Getenv(key)
    if v == "" {
        log.Fatalf("required environment variable %q is not set", key)
    }
    return v
}
```

**Why `logger.Sync()` with `defer`?**  
Zap may buffer log entries internally for performance. `Sync()` flushes that buffer. Without `defer logger.Sync()`, the last few log lines before a crash or clean shutdown might be lost. The call is deferred so it runs even if the function returns early via a panic.

**Why `logger.Fatal` instead of `log.Fatal` for the server error?**  
`logger.Fatal` writes a structured log entry with all the Zap fields, then calls `os.Exit(1)`. It is the Zap-idiomatic way to terminate on an unrecoverable error. Using `log.Fatal` (standard library) here would produce a plain-text line inconsistent with the rest of the structured output.

---

## 6.12 Updating .development.env

Add the two new logging variables to the committed defaults file:

```bash
# .development.env

PLAYLISTS_PORT=8080
PLAYLISTS_MB_BASE_URL=https://musicbrainz.org
PLAYLISTS_MB_USER_AGENT=playlists-dev/0.0.1 ( dev@example.com )

# Logging
PLAYLISTS_LOG_LEVEL=debug
PLAYLISTS_LOG_FORMAT=dev
```

These defaults give you maximum verbosity with a coloured, human-readable format while developing. In any other environment (Docker, CI, production) where `.development.env` is absent, the app defaults to `info` level and JSON output.

---

## 6.13 Observing logs

Start the server:

```bash
make run
```

In `dev` format you will see output like:

```
2024-01-15T12:34:56.123+0000    INFO    cmd/server/main.go:38   starting server    {"port": "8080", "logLevel": "debug", "logFormat": "dev"}
2024-01-15T12:34:56.124+0000    INFO    cmd/server/main.go:48   server ready       {"addr": ":8080"}
```

After a search request:

```
2024-01-15T12:34:57.001+0000    DEBUG   search/handler.go:25    service.Search called   {"title": "Bohemian Rhapsody", "artist": "Queen"}
2024-01-15T12:34:57.001+0000    DEBUG   musicbrainz/client_impl.go:41  starting MusicBrainz search  {"title": "Bohemian Rhapsody", "artist": "Queen"}
2024-01-15T12:34:57.002+0000    DEBUG   musicbrainz/client_impl.go:75  sending HTTP request to MusicBrainz  {"url": "https://musicbrainz.org/ws/2/recording?..."}
2024-01-15T12:34:57.312+0000    DEBUG   musicbrainz/client_impl.go:47  MusicBrainz search complete  {"title": "Bohemian Rhapsody", "artist": "Queen", "recordings": 5}
2024-01-15T12:34:57.312+0000    DEBUG   search/service.go:40    service.Search returning results  {"title": "Bohemian Rhapsody", "artist": "Queen", "count": 5}
2024-01-15T12:34:57.312+0000    INFO    api/middleware.go:54    request  {"method": "GET", "path": "/api/v1/songs/search", "query": "title=Bohemian+Rhapsody&artist=Queen", "status": 200, "duration": "311ms", "remoteAddr": "127.0.0.1:54321"}
```

If MusicBrainz is unavailable:

```
2024-01-15T12:35:00.001+0000    WARN    musicbrainz/client_impl.go:113  MusicBrainz request failed, will retry  {"attempt": 1, "maxAttempts": 3, "error": "MusicBrainz returned status 503", "retryIn": "2s"}
2024-01-15T12:35:02.002+0000    WARN    musicbrainz/client_impl.go:113  MusicBrainz request failed, will retry  {"attempt": 2, "maxAttempts": 3, "error": "MusicBrainz returned status 503", "retryIn": "2s"}
2024-01-15T12:35:04.003+0000    ERROR   musicbrainz/client_impl.go:121  MusicBrainz request failed after all retry attempts  {"attempts": 3, "error": "MusicBrainz returned status 503"}
2024-01-15T12:35:04.003+0000    ERROR   search/service.go:30   MusicBrainz client returned an error  {"title": "Bohemian Rhapsody", "artist": "Queen", "error": "MusicBrainz returned status 503"}
2024-01-15T12:35:04.003+0000    ERROR   search/handler.go:40   search service error  {"title": "Bohemian Rhapsody", "artist": "Queen", "error": "MusicBrainz returned status 503"}
2024-01-15T12:35:04.003+0000    INFO    api/middleware.go:54   request  {"method": "GET", ..., "status": 503, "duration": "4002ms", ...}
```

This shows the full chain of events from the retry attempts all the way to the HTTP 503 response, with all the context you need to understand what went wrong.

---

## 6.14 Summary of all changes

| File | Change |
|---|---|
| `go.mod` / `go.sum` | `go get go.uber.org/zap` |
| `.development.env` | Add `PLAYLISTS_LOG_LEVEL=debug` and `PLAYLISTS_LOG_FORMAT=dev` |
| `cmd/server/main.go` | Build logger from env vars; pass to all constructors |
| `internal/api/middleware.go` | **New file** — `LoggingMiddleware` |
| `internal/api/router.go` | Accept `*zap.Logger`; return `http.Handler`; apply middleware |
| `internal/musicbrainz/client_impl.go` | Add `logger` field; log retries and search events |
| `internal/search/service.go` | Add `logger` field; log calls and errors |
| `internal/search/handler.go` | Add `logger` field; log validation failures and errors; fix `artist` query param bug |

### New environment variables

| Variable | Values | Default | Description |
|---|---|---|---|
| `PLAYLISTS_LOG_LEVEL` | `debug`, `info`, `warn`, `error` | `info` | Minimum severity to emit |
| `PLAYLISTS_LOG_FORMAT` | `dev`, `json` | `json` | Console (coloured) or JSON output |
