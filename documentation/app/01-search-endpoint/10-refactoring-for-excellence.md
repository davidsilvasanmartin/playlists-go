# Part 10 — Refactoring for Excellence

This part reviews the current codebase against best practices — separation of concerns, the SOLID principles, and
testability — and proposes concrete improvements. No new features are added. The goal is a codebase that is easier to
extend, easier to test, and consistent in its contracts.

---

## What we are fixing and why

| # | Problem | Principle violated |
|---|---------|-------------------|
| 1 | `writeJSON`, `writeError`, `apiError` live in `search/handler.go` | Single Responsibility, Open/Closed |
| 2 | Unmatched URLs return Go's plain-text `404 page not found` | API consistency |
| 3 | Config values are loose strings scattered across `main.go` | Single Responsibility, testability |
| 4 | Service errors are logged twice (once in the service, once in the handler) | Single Responsibility |
| 5 | `router.go` depends on the concrete `*search.Handler` type | Dependency Inversion |

---

## 1. Extract `internal/httputil`

### The problem

`search/handler.go` already acknowledges this in a comment:

```go
// These can't live in internal/api because internal/api imports internal/search,
// and we don't want to create a circular dependency. We'll move these shared
// utilities somewhere else later such as in an internal/httputil package
```

The utilities (`writeJSON`, `writeError`, `apiError`) are general-purpose HTTP helpers. Keeping them in the `search`
package means every future handler (`song`, `playlist`) must either copy them or import `search` — both are wrong.

**Single Responsibility Principle:** the `search` package should only know about searching for songs. Formatting HTTP
responses is a separate concern.

**Open/Closed Principle:** adding a new handler should not require touching `search`. Moving the utilities to a neutral
package means new handlers can use them without any coupling to `search`.

### The fix

Create `internal/httputil/response.go`:

```go
package httputil

import (
    "encoding/json"
    "net/http"
    "time"
)

type APIError struct {
    Timestamp string `json:"timestamp"`
    Status    int    `json:"status"`
    Error     string `json:"error"`
    Message   string `json:"message"`
    Path      string `json:"path"`
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    _ = json.NewEncoder(w).Encode(v)
}

func WriteError(w http.ResponseWriter, r *http.Request, status int, message string) {
    WriteJSON(w, status, APIError{
        Timestamp: time.Now().UTC().Format(time.RFC3339),
        Status:    status,
        Error:     http.StatusText(status),
        Message:   message,
        Path:      r.URL.Path,
    })
}
```

Then update `search/handler.go` to import and use `httputil.WriteJSON` / `httputil.WriteError`, and delete the local
definitions. Update `api/router.go`'s version handler likewise.

### Why a package and not a file in `internal/api`?

`internal/api` already imports `internal/search`. If response helpers lived there, `search` would need to import `api`
to use them — a circular import. A dependency-free `internal/httputil` package sits below both in the import graph and
is importable by everyone.

---

## 2. Custom JSON 404 (and 405) responses

### The problem

When a client hits an unmapped URL, Go's `net/http` mux responds with:

```
404 page not found
```

Plain text. Content-Type `text/plain`. This breaks the API contract: every other error response is a JSON `APIError`
envelope. Any client that parses error responses will fail on this one.

### Is it worth fixing?

Yes. API consistency is not cosmetic — it is part of your public contract. If your API documentation says errors follow
the `APIError` shape, then _all_ errors must follow it, including routing errors. Breaking this rule forces every
client to special-case a single code path.

### The fix: catch-all route

In Go's stdlib mux, the pattern `"/"` matches anything that no more specific pattern matches. Register it last:

```go
mux.HandleFunc("GET /api/v1/version", ...)
mux.HandleFunc("GET /api/v1/songs/search", searchHandler.Search)

// Catch-all: any URL not matched above
mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
    httputil.WriteError(w, r, http.StatusNotFound, "the requested resource was not found")
})
```

> **Why does this work?** The stdlib mux uses longest-prefix matching. `"GET /api/v1/songs/search"` is more specific
> than `"/"`, so it wins for that path. `"/"` only fires when nothing else matches.

### What about 405 Method Not Allowed?

Go 1.22's mux is smarter than it used to be: if a path is registered but the method doesn't match (e.g. `POST
/api/v1/songs/search` when only `GET` is registered), the mux returns a `405 Method Not Allowed` — also in plain text.

Fixing 405 requires wrapping the mux itself, since the mux generates the response before your handler runs:

```go
// Wrap the mux so 405 responses go through our JSON formatter
func jsonMethodNotAllowed(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Use a recorder to intercept the response before it is sent
        rec := &interceptRecorder{ResponseWriter: w}
        next.ServeHTTP(rec, r)
        if rec.status == http.StatusMethodNotAllowed && !rec.written {
            httputil.WriteError(w, r, http.StatusMethodNotAllowed,
                "method not allowed for this resource")
            return
        }
        rec.flush(w)
    })
}
```

This is more complex to implement correctly (you need to buffer the response body). A pragmatic decision: fix 404 now
via the catch-all (covers the most common case), and revisit 405 only if a client actually hits it. Document the
decision.

---

## 3. A `Config` struct with Viper

### The problem

`main.go` reads all environment variables inline and passes them as loose strings to constructors:

```go
port        := getEnv("PLAYLISTS_PORT", "8080")
mbBaseURL   := getEnv("PLAYLISTS_MB_BASE_URL", "https://musicbrainz.org")
mbUserAgent := mustGetEnv("PLAYLISTS_MB_USER_AGENT")
logLevel    := getEnv("PLAYLISTS_LOG_LEVEL", "info")
logFormat   := getEnv("PLAYLISTS_LOG_FORMAT", "json")
```

Four problems:

1. **Single Responsibility:** `main.go` mixes config loading, logger construction, dependency wiring, and server
   startup. Each is a different concern.
2. **Testability:** there is no way to construct a test config without setting real environment variables. A struct
   eliminates that.
3. **Implicit defaults hide misconfiguration:** a server that silently starts on a stale default because of a typo in
   the deployment config is harder to debug than one that refuses to start with a clear error. All variables should be
   required and explicit.
4. **Two libraries for one job:** `godotenv` loads files, `os.Getenv` reads values. Viper handles both.

### Why Viper — and why now

The planning document ruled out Viper for `main.go` inline config reading, and that was the right call at the time:
adding ~10 transitive dependencies to replace five `os.Getenv` calls in `main.go` is not a good trade. The calculus
changes once config loading is its own package. Now Viper is not replacing five lines in `main.go` — it is the entire
implementation of `internal/config`. The cost (one dependency) is paid once; the benefit (a battle-tested,
widely-understood config library that reads files, env vars, and remote sources with a unified API) accrues every time
a variable is added or the app grows.

Viper is the de-facto standard for Go application configuration. Any Go developer joining the project will be
immediately familiar with it.

### The naive validation fix — and why it does not scale

Before choosing the solution, it is worth naming the trap. The obvious required-validation approach is one `if` block
per variable:

```go
if cfg.Port == "" {
    return nil, fmt.Errorf("PLAYLISTS_PORT is required")
}
if cfg.MBBaseURL == "" {
    return nil, fmt.Errorf("PLAYLISTS_MB_BASE_URL is required")
}
// ... one block per variable, forever
```

This is O(n) imperative code: every new variable requires a new block, and each variable's name appears in two places
(the read call and the validation block), so a rename is a two-site change. The solution below avoids this entirely.

### Key naming: why full names throughout

Before looking at the implementation, there is an important design decision to understand.

Viper stores dotenv file keys exactly as they appear in the file, lowercased. So `PLAYLISTS_PORT=8080` from a file is
stored under the key `playlists_port`. Viper's `SetEnvPrefix("PLAYLISTS")` shorthand — where `GetString("port")`
automatically reads `PLAYLISTS_PORT` from the environment — does **not** apply to values loaded from files. The prefix
logic only runs when Viper resolves a key against the real process environment.

This means using `SetEnvPrefix` while also loading dotenv files creates a split: short keys (`port`) work for env
vars, but only full keys (`playlists_port`) work for file values. Rather than manage two naming conventions, this
implementation uses full key names everywhere:

- Viper keys: `playlists_port`, `playlists_mb_base_url`, …
- `mapstructure` tags on the struct: same
- The dotenv files: `PLAYLISTS_PORT=8080` — unchanged from the current format

`AutomaticEnv()` without a prefix uppercases the Viper key to find the env var:
`playlists_port` → `PLAYLISTS_PORT`. This matches the real environment variable name exactly, so both sources
resolve correctly through the same key.

### The fix

```bash
go get github.com/spf13/viper
```

Remove `godotenv` from `go.mod`:

```bash
go get github.com/joho/godotenv@none
go mod tidy
```

Create `internal/config/config.go`:

```go
package config

import (
    "errors"
    "fmt"
    "os"
    "strings"

    "github.com/spf13/viper"
)

// Config holds all configuration values required to run the server.
// mapstructure tags use the full environment variable name (lowercased) so
// that Viper resolves values consistently whether they come from a dotenv
// file or the real process environment.
type Config struct {
    Port        string `mapstructure:"playlists_port"`
    MBBaseURL   string `mapstructure:"playlists_mb_base_url"`
    MBUserAgent string `mapstructure:"playlists_mb_user_agent"`
    LogLevel    string `mapstructure:"playlists_log_level"`
    LogFormat   string `mapstructure:"playlists_log_format"`
}

// requiredKeys lists every Viper key the app needs.
// Each key is the lowercased form of the full environment variable name.
//
// Adding a new variable is a two-line change: add the struct field above and
// add the key here. No imperative validation blocks to update.
var requiredKeys = []string{
    "playlists_port",
    "playlists_mb_base_url",
    "playlists_mb_user_agent",
    "playlists_log_level",
    "playlists_log_format",
}

// Load populates Config from three sources, in increasing priority order:
//
//  1. .development.env — committed defaults, loaded first
//  2. .env             — personal overrides, gitignored, merged on top
//  3. process env      — injected by Docker / CI, always wins
//
// Both files are optional. If a file does not exist the error is silently
// ignored — this is expected in Docker and CI where neither file is present.
// Any other file error (permission denied, malformed content) is returned.
func Load() (*Config, error) {
    v := viper.New()
    v.SetConfigType("dotenv")

    // ── 1. committed defaults ─────────────────────────────────────────────
    v.SetConfigFile(".development.env")
    if err := v.ReadInConfig(); err != nil && !errors.Is(err, os.ErrNotExist) {
        return nil, fmt.Errorf("read .development.env: %w", err)
    }

    // ── 2. personal overrides ─────────────────────────────────────────────
    v.SetConfigFile(".env")
    if err := v.MergeInConfig(); err != nil && !errors.Is(err, os.ErrNotExist) {
        return nil, fmt.Errorf("merge .env: %w", err)
    }

    // ── 3. process environment (highest priority) ─────────────────────────
    // AutomaticEnv uppercases the Viper key to find the env var:
    // "playlists_port" → PLAYLISTS_PORT. No prefix is set because the full
    // variable name is already encoded in the key.
    v.AutomaticEnv()
    // BindEnv must be called for each key so that Unmarshal resolves env vars.
    // AutomaticEnv alone only affects Get* calls; Unmarshal iterates Viper's
    // internal key registry and silently returns empty strings for keys it has
    // never seen from a file or BindEnv/SetDefault — even if the env var exists.
    for _, key := range requiredKeys {
        _ = v.BindEnv(key)
    }

    // ── 4. validate all required keys are present ─────────────────────────
    var missing []string
    for _, key := range requiredKeys {
        if v.GetString(key) == "" {
            missing = append(missing, strings.ToUpper(key))
        }
    }
    if len(missing) > 0 {
        return nil, fmt.Errorf("missing required environment variables: %s",
            strings.Join(missing, ", "))
    }

    // ── 5. populate struct ────────────────────────────────────────────────
    var cfg Config
    if err := v.Unmarshal(&cfg); err != nil {
        return nil, fmt.Errorf("unmarshal config: %w", err)
    }
    return &cfg, nil
}
```

`main.go` becomes:

```go
cfg, err := config.Load()
if err != nil {
    log.Fatal(err)
}
```

### Priority in practice

| Source | Priority | Present in Docker? | Present locally? |
|--------|----------|--------------------|------------------|
| Process env vars | Highest | Yes (injected by runtime) | Only if exported in shell |
| `.env` | Middle | No | Only if developer created it |
| `.development.env` | Lowest | No | Yes (committed) |

In Docker: both files are absent (the image only contains the compiled binary). Viper silently skips them and reads
solely from the injected process environment.

Locally without a `.env`: Viper reads `.development.env` and that is all. The committed defaults are the effective
config.

Locally with a `.env`: a developer's personal values in `.env` override `.development.env` for any keys that appear
in both. Keys absent from `.env` fall back to `.development.env`.

### Impact on e2e tests

`e2e/setup_test.go` starts the app container with an explicit `Env` map:

```go
Env: map[string]string{
    "PLAYLISTS_MB_BASE_URL":   wiremockURL,
    "PLAYLISTS_MB_USER_AGENT": "playlists-e2e/0.0.1 ( test@example.com )",
    "PLAYLISTS_LOG_LEVEL":     "debug",
    "PLAYLISTS_LOG_FORMAT":    "dev",
},
```

The Docker image does not contain `.development.env`, so `PLAYLISTS_PORT` is not set anywhere in that
environment. With the previous code it defaulted silently to `8080`; with all variables required the container
fails to start. **Add the missing variable:**

```go
Env: map[string]string{
    "PLAYLISTS_PORT":          "8080",
    "PLAYLISTS_MB_BASE_URL":   wiremockURL,
    "PLAYLISTS_MB_USER_AGENT": "playlists-e2e/0.0.1 ( test@example.com )",
    "PLAYLISTS_LOG_LEVEL":     "debug",
    "PLAYLISTS_LOG_FORMAT":    "dev",
},
```

This is a feature, not a nuisance: the test now explicitly documents every value the app container needs to run,
which is exactly the kind of clarity that prevents "works locally, breaks in CI" surprises.

### Key design decisions

**`viper.New()` instead of the global `viper` instance.** The global instance is a process-level singleton. Using a
local instance means `Load` is stateless: two calls do not interfere, and tests can call it in parallel without races.

**`MergeInConfig` for `.env`, not a second `ReadInConfig`.** `ReadInConfig` replaces the current config;
`MergeInConfig` overlays it. Using merge for the second file means only the keys present in `.env` override
`.development.env` — absent keys fall through to the defaults.

**`errors.Is(err, os.ErrNotExist)` to detect absent files.** When `SetConfigFile` is used (not `AddConfigPath`),
Viper returns an `*os.PathError` wrapping `syscall.ENOENT` for missing files — not `viper.ConfigFileNotFoundError`,
which is only returned when Viper searches paths. `errors.Is` unwraps the chain correctly.

**`v.GetString(key) == ""` instead of `v.IsSet(key)`.** Viper's `IsSet` has a known gotcha: with `AutomaticEnv`, it
can return `false` for a key that `GetString` would successfully resolve from the environment, depending on whether
the key was also explicitly bound. `GetString` always triggers the full resolution chain and is the reliable check.

### Testability benefit

Tests that need a config construct one directly, bypassing `Load` entirely:

```go
cfg := &config.Config{
    Port:        "8080",
    MBBaseURL:   "http://wiremock:8080",
    MBUserAgent: "test/0.0.1 ( test@example.com )",
    LogLevel:    "error",
    LogFormat:   "json",
}
```

No `os.Setenv` / `os.Unsetenv` dance, no risk of one test's environment leaking into another. `Load` itself can be
tested with a dedicated `TestLoad` that creates temporary dotenv files in a temp directory.

### Alternative approaches

**`caarlos0/env`** (zero transitive dependencies) uses struct tags to declare env var names and required validation
directly on each field. Clean and minimal. Cannot read dotenv files natively — you would still need godotenv or a
similar loader. Viper absorbs both concerns in one library.

**`kelseyhightower/envconfig`** is the older, well-established version of the same struct-tag idea. Stops at the
first missing variable rather than collecting all of them, which makes operator experience worse.

**Pure stdlib with a validation slice** avoids all dependencies but requires each variable to appear in two places
and provides no dotenv file loading.

---

## 4. Eliminate double-logging of errors

### The problem

When a MusicBrainz search fails, both the service and the handler log the error:

```go
// service.go
s.logger.Error("MusicBrainz client returned an error", zap.Error(err))

// handler.go
h.logger.Error("search service error", zap.Error(err))
```

The same error appears twice in every log stream. Correlating them wastes time and inflates log volume.

### The convention to adopt

**The layer that makes the decision logs the event.** The service knows what failed; the handler knows the HTTP
consequence. Neither needs to duplicate the other.

A clean split:

- **Service:** log at `Debug` level when it returns an error (optional — enough context for local debugging).
- **Handler:** log at `Error` level with the HTTP context (method, path, status). This is the authoritative record of
  a failed request.

Concretely, remove the `logger.Error` call from `service.Search` and keep only the one in the handler. The service
may keep a `Debug`-level log if it adds information (e.g. which downstream call failed), but it should not log at
`Error` — that judgment belongs to the handler which has the full request context.

> This is not a hard rule across all codebases — some teams log at both layers intentionally, using a correlation ID
> to link them. But without a correlation ID, double-logging is strictly noise.

---

## 5. Router: depend on interfaces, not concrete types

### The problem

```go
func NewRouter(logger *zap.Logger, searchHandler *search.Handler, version string) http.Handler {
```

`NewRouter` takes `*search.Handler` — a concrete type from another package. This means:

- **Dependency Inversion Principle:** `api` depends on the implementation of `search`, not on an abstraction.
- **Testability:** to test `NewRouter`, you must construct a real `search.Handler`, which transitively requires a real
  `search.Service` and `musicbrainz.Client`.

### The fix

Define a minimal interface in the `api` package for what the router needs:

```go
// in internal/api/router.go

type searchHandler interface {
    Search(http.ResponseWriter, *http.Request)
}

func NewRouter(logger *zap.Logger, sh searchHandler, version string) http.Handler {
    ...
}
```

`*search.Handler` already satisfies this interface implicitly — no changes needed in the `search` package. In tests,
you can now pass a lightweight fake:

```go
type fakeSearchHandler struct{}

func (f *fakeSearchHandler) Search(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    w.Write([]byte(`{"results":[]}`))
}
```

> **Interface granularity:** define the interface at the point of use (in `api`), not at the point of
> implementation (in `search`). This is the Go idiom: interfaces belong to the consumer, not the producer.

---

## Summary of changes

| File | Change |
|------|--------|
| `internal/httputil/response.go` | New file: `WriteJSON`, `WriteError`, `APIError` |
| `internal/search/handler.go` | Remove local HTTP helpers; import `httputil` |
| `internal/api/router.go` | Use `httputil.WriteJSON`; add catch-all 404; accept interface |
| `internal/config/config.go` | New file: `Config` struct with `env` struct tags and `Load()` |
| `cmd/server/main.go` | Use `config.Load()`; remove inline `getEnv`/`mustGetEnv` |
| `internal/search/service.go` | Remove duplicate `Error`-level log |

These are all mechanical changes — no behaviour visible to the client changes, except that unmapped URLs now return
JSON instead of plain text.
