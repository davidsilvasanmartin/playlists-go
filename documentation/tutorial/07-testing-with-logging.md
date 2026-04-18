# Part 7 — Testing with Logging

Part 6 added a `*zap.Logger` parameter to every constructor. This part explains how to handle that in tests — and why the answer is simpler than you might expect.

---

## 7.1 The core principle: do not mock loggers

When you add logging to a struct, the logger becomes a dependency just like the MusicBrainz client. A natural instinct is to mock it in tests so you can assert that specific log messages were emitted. Resist this instinct.

**Mocking loggers is almost always the wrong approach** because:

1. **Log messages are not part of the API contract.** If you write `assert.Equal(t, "search called", capturedMsg)` in a test, any rephrasing of that log message breaks the test, even though the behaviour of the code did not change. Tests should describe observable behaviour (return values, side effects), not implementation details.

2. **It couples tests to phrasing.** The moment you assert on exact log output, every log message becomes a de-facto API that you must version and maintain.

3. **It adds noise.** A mock logger interface with methods for every level (`Debug`, `Info`, `Warn`, `Error`, `Fatal`, …) is boilerplate that obscures the real test setup.

The right mental model: logging is an *operational concern*, not a *behavioural one*. Tests verify behaviour. Monitoring and alerting verify operational concerns.

**What you should do instead:** supply a real logger that *discards its output*. Your tests run cleanly without log noise, the code under test exercises its logging paths (good for coverage and for detecting panics in log code), and you are free to rephrase log messages without touching a single test.

---

## 7.2 `zap.NewNop()` — the silent logger

Zap ships with `zap.NewNop()`, a constructor that returns a fully functional `*zap.Logger` that silently discards every log entry it receives. It satisfies the `*zap.Logger` type (it is not an interface), so it drops directly into any place a real logger would go.

```go
logger := zap.NewNop()
// logger.Info(...)  → discarded
// logger.Error(...) → discarded
// logger.Debug(...) → discarded
```

No setup, no teardown, no configuration. This is what you will pass to every constructor in your tests.

---

## 7.3 Updating unit tests — `internal/musicbrainz`

The `withRetry` function gained a `*zap.Logger` parameter. The tests for it live in `internal/musicbrainz/client_impl_test.go` and call `withRetry` directly. They each need one extra argument.

**`internal/musicbrainz/client_impl_test.go` — full updated file:**

```go
package musicbrainz

import (
    "context"
    "errors"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "go.uber.org/zap"
)

// nopLogger is a convenience variable so we do not have to call zap.NewNop()
// on every single test line. It is unexported and scoped to this test file.
var nopLogger = zap.NewNop()

// ── withRetry ────────────────────────────────────────────────────────────────

func TestWithRetry_SucceedsOnFirstAttempt(t *testing.T) {
    calls := 0
    err := withRetry(context.Background(), 3, time.Millisecond, nopLogger, func() error {
        calls++
        return nil
    })
    require.NoError(t, err)
    assert.Equal(t, 1, calls)
}

func TestWithRetry_RetriesAndSucceeds(t *testing.T) {
    calls := 0
    err := withRetry(context.Background(), 3, time.Millisecond, nopLogger, func() error {
        calls++
        if calls < 3 {
            return errors.New("transient error")
        }
        return nil
    })
    require.NoError(t, err)
    assert.Equal(t, 3, calls)
}

func TestWithRetry_ExhaustsAttempts(t *testing.T) {
    sentinel := errors.New("always fails")
    calls := 0
    err := withRetry(context.Background(), 3, time.Millisecond, nopLogger, func() error {
        calls++
        return sentinel
    })
    assert.ErrorIs(t, err, sentinel)
    assert.Equal(t, 3, calls)
}

func TestWithRetry_RespectsContextCancellation(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())

    calls := 0
    err := withRetry(ctx, 5, 50*time.Millisecond, nopLogger, func() error {
        calls++
        if calls == 1 {
            cancel()
        }
        return errors.New("error")
    })

    assert.ErrorIs(t, err, context.Canceled)
    assert.Equal(t, 1, calls)
}

// ── mapRecordings ────────────────────────────────────────────────────────────

func TestMapRecordings_FullData(t *testing.T) {
    raw := []mbRecording{
        {
            ID:             "mbid-001",
            Title:          "Bohemian Rhapsody",
            Length:         354000,
            Disambiguation: "studio recording",
            ArtistCredit: []mbCredit{
                {Artist: mbArtistInfo{ID: "artist-001", Name: "Queen"}},
            },
            Releases: []mbRelease{
                {ID: "release-001", Title: "A Night at the Opera", Date: "1975-11-21"},
            },
        },
    }

    got := mapRecordings(raw)
    require.Len(t, got, 1)
    assert.Equal(t, "mbid-001", got[0].MBID)
    assert.Equal(t, "Bohemian Rhapsody", got[0].Title)
    assert.Equal(t, 354000, got[0].DurationMs)
    assert.Equal(t, "studio recording", got[0].Disambiguation)
    assert.Equal(t, "Queen", got[0].Artist)
    assert.Equal(t, "artist-001", got[0].ArtistMBID)
    assert.Equal(t, "A Night at the Opera", got[0].Album)
    assert.Equal(t, "release-001", got[0].AlbumMBID)
    assert.Equal(t, "1975-11-21", got[0].ReleaseDate)
}

func TestMapRecordings_Empty(t *testing.T) {
    got := mapRecordings(nil)
    assert.Empty(t, got)
}

func TestMapRecordings_NoArtistOrRelease(t *testing.T) {
    raw := []mbRecording{{ID: "mbid-002", Title: "Unknown"}}
    got := mapRecordings(raw)
    require.Len(t, got, 1)
    assert.Equal(t, "mbid-002", got[0].MBID)
    assert.Empty(t, got[0].Artist)
    assert.Empty(t, got[0].Album)
}
```

The only meaningful change is passing `nopLogger` as the fourth argument to every `withRetry` call. The `mapRecordings` tests are unchanged — that function does not take a logger.

**Why a package-level `nopLogger` variable?**  
It is shorter than calling `zap.NewNop()` every time and makes the test intent clear: this logger is deliberately a no-op, not an accident. `zap.NewNop()` is cheap to construct (no goroutines, no allocations at steady state), so sharing one instance per test file is fine.

---

## 7.4 Updating unit tests — `internal/search`

The service and handler constructors both gained a `*zap.Logger` parameter.

**`internal/search/service_test.go` — full updated file:**

```go
package search

import (
    "context"
    "errors"
    "testing"

    "github.com/davidsilvasanmartin/playlists-go/internal/musicbrainz"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "go.uber.org/zap"
)

// fakeMBClient is a hand-written fake that satisfies musicbrainz.Client.
type fakeMBClient struct {
    recordings []musicbrainz.Recording
    err        error
}

func (f *fakeMBClient) Search(_ context.Context, _, _ string) ([]musicbrainz.Recording, error) {
    return f.recordings, f.err
}

func TestService_Search_MapsResults(t *testing.T) {
    fake := &fakeMBClient{
        recordings: []musicbrainz.Recording{
            {
                MBID:           "mbid-001",
                Title:          "Bohemian Rhapsody",
                Artist:         "Queen",
                ArtistMBID:     "artist-001",
                Album:          "A Night at the Opera",
                AlbumMBID:      "release-001",
                ReleaseDate:    "1975-11-21",
                DurationMs:     354000,
                Disambiguation: "studio recording",
            },
        },
    }
    svc := NewService(fake, zap.NewNop())

    results, err := svc.Search(context.Background(), "Bohemian Rhapsody", "Queen")
    require.NoError(t, err)
    require.Len(t, results, 1)

    r := results[0]
    assert.Equal(t, "mbid-001", r.MBID)
    assert.Equal(t, "Bohemian Rhapsody", r.Title)
    assert.Equal(t, "Queen", r.Artist)
    assert.Equal(t, "artist-001", r.ArtistMBID)
    assert.Equal(t, "A Night at the Opera", r.Album)
    assert.Equal(t, "release-001", r.AlbumMBID)
    assert.Equal(t, "1975-11-21", r.ReleaseDate)
    assert.Equal(t, 354000, r.DurationMs)
    assert.Equal(t, "studio recording", r.Disambiguation)
}

func TestService_Search_EmptyResults(t *testing.T) {
    fake := &fakeMBClient{recordings: []musicbrainz.Recording{}}
    svc := NewService(fake, zap.NewNop())

    results, err := svc.Search(context.Background(), "Nonexistent Song", "No Artist")
    require.NoError(t, err)
    assert.Empty(t, results)
}

func TestService_Search_PropagatesError(t *testing.T) {
    fake := &fakeMBClient{err: errors.New("network error")}
    svc := NewService(fake, zap.NewNop())

    _, err := svc.Search(context.Background(), "Any Title", "Any Artist")
    assert.Error(t, err)
}
```

The only change is `zap.NewNop()` as the second argument to `NewService`. The test behaviour and assertions are identical to before.

---

## 7.5 Updating integration tests — `internal/search`

The integration tests build the full handler stack via `newTestApp`. That helper now needs to pass a logger to every constructor.

**`internal/search/handler_integration_test.go` — updated `newTestApp` function:**

```go
//go:build integration

package search_test

import (
    "encoding/json"
    "fmt"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/davidsilvasanmartin/playlists-go/internal/musicbrainz"
    "github.com/davidsilvasanmartin/playlists-go/internal/search"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "go.uber.org/zap"
)

// newTestApp builds the real handler stack pointed at the given MusicBrainz
// base URL.  A no-op logger is used so tests produce no log output.
func newTestApp(mbBaseURL string) http.Handler {
    logger := zap.NewNop()
    mbClient := musicbrainz.NewClient(mbBaseURL, "playlists-test/0.0.1 ( test@example.com )", logger)
    svc := search.NewService(mbClient, logger)
    handler := search.NewHandler(svc, logger)

    mux := http.NewServeMux()
    mux.HandleFunc("GET /api/v1/songs/search", handler.Search)
    return mux
}
```

> **Note:** `newTestApp` does not apply `LoggingMiddleware` because that middleware belongs to `internal/api`, which the `search_test` package does not import. For integration tests that want to test the full router (including the middleware), you would use `api.NewRouter(logger, handler)` instead of constructing a bare mux. Neither choice is wrong — what matters is testing the contract of the piece you care about.

The rest of the integration test file (`mbSearchFixture`, all the `TestIntegration_*` functions) is identical to what is documented in Part 5. Only `newTestApp` changes.

---

## 7.6 The `zaptest` package — when you do want to assert on logs

There is one legitimate case for capturing log output in a test: when the log message *is* the observable behaviour — for example, testing a logging middleware whose entire job is to emit a structured log entry.

For those cases, Zap provides `go.uber.org/zap/zaptest/observer`, a package that captures log entries into an in-memory sink you can query.

```go
import (
    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
    "go.uber.org/zap/zaptest/observer"
)

func TestLoggingMiddleware_LogsRequest(t *testing.T) {
    // Create an observer core that captures log entries.
    core, logs := observer.New(zapcore.InfoLevel)
    logger := zap.New(core)

    // Build the middleware with the capturing logger.
    handler := LoggingMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    }))

    req := httptest.NewRequest(http.MethodGet, "/api/v1/songs/search", nil)
    rec := httptest.NewRecorder()
    handler.ServeHTTP(rec, req)

    // Assert on the captured entries.
    require.Equal(t, 1, logs.Len())
    entry := logs.All()[0]
    assert.Equal(t, "request", entry.Message)
    assert.Equal(t, "GET", entry.ContextMap()["method"])
    assert.Equal(t, "/api/v1/songs/search", entry.ContextMap()["path"])
    assert.Equal(t, int64(200), entry.ContextMap()["status"])
}
```

Install the package with:

```bash
go get go.uber.org/zap/zaptest
```

**When to use `observer`:**

| Situation | Use |
|---|---|
| Testing a handler, service, or client | `zap.NewNop()` |
| Testing a middleware whose purpose is to log | `observer.New(level)` |
| Testing a component that changes behaviour based on log level | `observer.New(level)` |

Keep the bar high. If you can assert on a return value or an HTTP response instead of a log entry, do that.

---

## 7.7 Temporarily enabling logs in a failing test

When a test is failing and you cannot tell why, it is sometimes useful to see the logs the code under test is emitting. You have two options:

### Option 1 — `zap.NewDevelopment()`

Replace `zap.NewNop()` with a development logger for the duration of your investigation:

```go
// Temporary: replace with zap.NewNop() before committing.
logger, _ := zap.NewDevelopment()
defer logger.Sync()
svc := NewService(fake, logger)
```

This prints coloured log output to stdout alongside your test output. Remember to revert before committing.

### Option 2 — `zaptest.NewLogger(t)`

The `zaptest` package also provides a `NewLogger` helper that routes log output through `t.Log`, meaning the logs appear only when the test fails (or when you run with `-v`):

```go
import "go.uber.org/zap/zaptest"

func TestService_Search_MapsResults(t *testing.T) {
    logger := zaptest.NewLogger(t)
    svc := NewService(fake, logger)
    // ...
}
```

This is the cleanest option because it integrates with Go's test output system — you get logs when you need them and silence when you do not. Once you have finished debugging, you can either leave `zaptest.NewLogger(t)` in place (it is harmless) or replace it with `zap.NewNop()` if you prefer explicit silence.

---

## 7.8 Summary

| Situation | Logger to use | Why |
|---|---|---|
| Unit test — any struct | `zap.NewNop()` | Discards output; zero noise; tests focus on behaviour |
| Integration test helper (`newTestApp`) | `zap.NewNop()` | Same reason; the HTTP log line is not what the test is checking |
| Test for the logging middleware itself | `observer.New(level)` | The log entry is the observable output being tested |
| Debugging a failing test | `zaptest.NewLogger(t)` | Integrates with `t.Log`; disappears on test pass |
| Quick local investigation | `zap.NewDevelopment()` | Visible immediately; revert before committing |

The guiding rule: **tests assert on behaviour, not on log messages**. A logger in a test is a plumbing detail — give it the minimal wiring needed and move on.
