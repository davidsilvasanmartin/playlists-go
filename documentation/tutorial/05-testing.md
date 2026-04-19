# Part 5 — Testing

The tests are organized into two layers:

| Layer | Command | What runs |
|---|---|---|
| Unit | `make test` | `internal/musicbrainz` and `internal/search` in isolation |
| Integration | `make testint` | Full HTTP stack: real handler + real service + fake MusicBrainz server |

Integration tests carry the `//go:build integration` tag so they are excluded from `go test ./...` and only run when explicitly requested with `-tags=integration`.

---

## A quick primer for Go newcomers

Before diving in, here are a few things that trip up people new to Go testing.

### `require` vs `assert`

Both come from `github.com/stretchr/testify`. The difference is what happens when a check fails:

- **`assert`** marks the test as failed but continues running. Good for checking multiple things independently.
- **`require`** marks the test as failed *and stops the test immediately*. Use this whenever continuing would be pointless or dangerous — for example, if `err` is not nil there is nothing useful to check on the result that follows.

A rule of thumb: use `require` for preconditions (no error, non-nil slice, correct length) and `assert` for the individual field checks that follow.

### White-box vs black-box test packages

Go lets you choose between two package declarations for test files:

- `package search` — same package as the code under test. You can access unexported identifiers (functions, types, variables). Called a *white-box* test.
- `package search_test` — a separate package that imports `search` like any other consumer. You can only access exported identifiers. Called a *black-box* test.

The unit tests in this project use white-box packages (`package musicbrainz`, `package search`) because they test unexported helpers or internal wiring. The integration test uses `package search_test` because it tests the handler as an external consumer would.

### What is mocking?

Unit tests are supposed to test *one thing in isolation*. The search `Service` depends on a `musicbrainz.Client` to fetch data. In a unit test we do not want to make real HTTP calls — they are slow, flaky, and require network access. Instead we replace the real client with a controlled stand-in.

There are two ways to do this:

- **Hand-written fake** — a small struct you write yourself that satisfies the interface. Simple, but you have to write the same boilerplate for every interface.
- **Mock (testify/mock)** — a struct generated or written using `testify/mock`. It lets you declare *per-test* what calls to expect, what to return, and whether all expected calls were actually made. More powerful, and consistent across the codebase.

This project uses `testify/mock`. After reading this section you will see how little code it actually requires.

---

## 5.1 Unit tests — `internal/musicbrainz`

### `internal/musicbrainz/client_impl_test.go`

These tests use `package musicbrainz` (white-box) so they can call the unexported helpers `withRetry` and `mapRecordings` directly. No mocking is needed here — both functions are pure logic with no external dependencies.

```go
package musicbrainz

import (
    "context"
    "errors"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// ── withRetry ────────────────────────────────────────────────────────────────

func TestWithRetry_SucceedsOnFirstAttempt(t *testing.T) {
    calls := 0
    err := withRetry(context.Background(), 3, time.Millisecond, func() error {
        calls++
        return nil
    })
    require.NoError(t, err)
    assert.Equal(t, 1, calls)
}

func TestWithRetry_RetriesAndSucceeds(t *testing.T) {
    calls := 0
    err := withRetry(context.Background(), 3, time.Millisecond, func() error {
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
    err := withRetry(context.Background(), 3, time.Millisecond, func() error {
        calls++
        return sentinel
    })
    assert.ErrorIs(t, err, sentinel)
    assert.Equal(t, 3, calls)
}

func TestWithRetry_RespectsContextCancellation(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())

    calls := 0
    err := withRetry(ctx, 5, 50*time.Millisecond, func() error {
        calls++
        if calls == 1 {
            cancel() // cancel after the first attempt
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

---

## 5.2 Unit tests — `internal/search`

The service depends on `musicbrainz.Client`. We mock it with `testify/mock` so each test controls exactly what the client returns.

### Getting the mock dependency

`testify/mock` is part of the `testify` module you already have, but it pulls in an indirect dependency (`stretchr/objx`) that may not be in your `go.sum` yet. Run this once after adding the mock import:

```bash
go mod tidy
```

### How `testify/mock` works

There are three moving parts:

1. **Define the mock struct.** Embed `mock.Mock` and implement the interface. Inside each method, call `m.Called(...)` — this records that the method was called and returns whatever you configured with `On`.

2. **Configure expectations.** In each test, call `mbClient.On("MethodName", arg1, arg2).Return(value1, value2)`. `mock.Anything` is a wildcard that matches any value for a given argument — useful for `context.Context` which you rarely care to match precisely.

3. **Assert expectations.** Call `mbClient.AssertExpectations(t)` at the end of each test. This fails the test if any `On(...)` call was never triggered, catching bugs where the code under test silently skips a dependency call it should have made.

### The mock type

```go
// mockMBClient is a testify/mock implementation of musicbrainz.Client.
type mockMBClient struct {
    mock.Mock
}

func (m *mockMBClient) Search(ctx context.Context, title, artist string) ([]musicbrainz.Recording, error) {
    args := m.Called(ctx, title, artist)
    // args.Get(0) returns interface{}. We guard against nil before the
    // type-assertion to avoid a panic in the error-path tests.
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).([]musicbrainz.Recording), args.Error(1)
}
```

This is the most important piece to understand, so let's go through it line by line.

**`m.Called(ctx, title, artist)`**

`Called` is a method provided by the embedded `mock.Mock`. It does two things at once:

1. It records that `Search` was called with these specific arguments. This is how `AssertExpectations` later knows whether the call happened.
2. It looks up the matching `On("Search", ...)` rule you registered in the test and returns the values you specified in `.Return(...)`, packaged into an `Arguments` value (assigned here to `args`).

Think of it as the mock asking: "someone called Search — what did the test tell me to return for these arguments?"

**`args` — what is it?**

`args` is a value of type `mock.Arguments`, which is just a slice of `interface{}` (Go's way of saying "a value of any type"). Each slot in the slice corresponds to one return value from `.Return(...)`.

So if in a test you wrote:
```go
mbClient.On("Search", mock.Anything, "Bohemian Rhapsody", "Queen").Return(recordings, nil)
```
then inside `Called`, `args` will be `[recordings, nil]` — index 0 is `recordings`, index 1 is `nil`.

**`args.Get(0)` and `args.Error(1)`**

Because `args` is a slice of `interface{}`, you need helpers to pull values back out:

- `args.Get(n)` returns the value at index `n` as a plain `interface{}`. You then use a *type assertion* — the `.([]musicbrainz.Recording)` syntax — to convert it back to the concrete type you actually want. This is necessary because Go is statically typed; the compiler does not know what type is hiding inside an `interface{}` until you tell it.
- `args.Error(n)` is a convenience method for the special case where the value is an `error`. It returns `nil` if the stored value is nil (which would panic with a plain type assertion), and returns the `error` otherwise.

**The nil check and why it matters**

When a test wants to simulate a failure it passes `nil` as the first return value:
```go
mbClient.On("Search", ...).Return(nil, errors.New("network error"))
```

At that point `args.Get(0)` holds a plain `nil` — not a `nil` of type `[]musicbrainz.Recording`, just a bare untyped `nil`. Trying to type-assert a bare `nil` to any concrete type panics at runtime. The guard:
```go
if args.Get(0) == nil {
    return nil, args.Error(1)
}
```
catches this before the assertion runs and returns a typed nil slice directly, which is safe and correct.

### `internal/search/service_test.go`

```go
package search

import (
    "context"
    "errors"
    "testing"

    "github.com/davidsilvasanmartin/playlists-go/internal/musicbrainz"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "github.com/stretchr/testify/require"
)

// mockMBClient is a testify/mock implementation of musicbrainz.Client.
type mockMBClient struct {
    mock.Mock
}

func (m *mockMBClient) Search(ctx context.Context, title, artist string) ([]musicbrainz.Recording, error) {
    args := m.Called(ctx, title, artist)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).([]musicbrainz.Recording), args.Error(1)
}

func TestService_Search_MapsResults(t *testing.T) {
    recordings := []musicbrainz.Recording{
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
    }

    mbClient := new(mockMBClient)
    mbClient.On("Search", mock.Anything, "Bohemian Rhapsody", "Queen").Return(recordings, nil)

    svc := NewService(mbClient)

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

    mbClient.AssertExpectations(t)
}

func TestService_Search_EmptyResults(t *testing.T) {
    mbClient := new(mockMBClient)
    mbClient.On("Search", mock.Anything, "Nonexistent Song", "No Artist").Return([]musicbrainz.Recording{}, nil)

    svc := NewService(mbClient)

    results, err := svc.Search(context.Background(), "Nonexistent Song", "No Artist")
    require.NoError(t, err)
    assert.Empty(t, results)

    mbClient.AssertExpectations(t)
}

func TestService_Search_PropagatesError(t *testing.T) {
    mbClient := new(mockMBClient)
    mbClient.On("Search", mock.Anything, "Any Title", "Any Artist").Return(nil, errors.New("network error"))

    svc := NewService(mbClient)

    _, err := svc.Search(context.Background(), "Any Title", "Any Artist")
    assert.Error(t, err)

    mbClient.AssertExpectations(t)
}
```

---

## 5.3 Integration tests — `internal/search`

Integration tests exercise the full HTTP stack in one process:

```
Test HTTP client → Handler → Service → musicbrainz.Client → fake MB server (httptest)
```

No Docker, no database. The fake MusicBrainz server is an in-process `httptest.Server` that returns canned JSON. This tests JSON serialization, validation, URL routing, and error mapping end-to-end.

### Build tags

The line `//go:build integration` at the top of the file is a *build constraint*. It tells the Go toolchain: "only compile this file when the `integration` tag is explicitly passed." This keeps `go test ./...` (and therefore `make test`) fast by skipping these slower tests. They only run when you pass `-tags=integration` to the compiler, which `make testint` does for you.

The constraint must be the very first line of the file, followed by a blank line. If there is anything before it, Go ignores it.

### The `httptest` package

`net/http/httptest` is part of Go's standard library. It provides two key helpers:

- **`httptest.NewServer(handler)`** — starts a real HTTP server on a random local port and returns its URL. You can point any HTTP client at it. Call `defer server.Close()` to shut it down at the end of the test.
- **`httptest.NewRecorder()`** — a fake `http.ResponseWriter` you can inspect after calling a handler directly, without starting a real server. Used in pure handler tests.

The integration tests here use `httptest.NewServer` twice: once for the fake MusicBrainz API and once for the real application under test.

### `internal/search/handler_integration_test.go`

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
)

// mbSearchFixture is a minimal valid MusicBrainz search response.
const mbSearchFixture = `{
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
    },
    {
      "id": "b1a9c0e2-0000-0000-0000-000000000002",
      "title": "Bohemian Rhapsody",
      "length": 360000,
      "disambiguation": "live, 1986-07-12: Wembley Stadium, London, UK",
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
          "id": "2ef5c347-0000-0000-0000-bc9cc507b843",
          "title": "Live at Wembley '86",
          "date": "1992-05-26"
        }
      ]
    }
  ]
}`

// newTestApp builds the real handler stack pointed at the given MusicBrainz base URL.
func newTestApp(mbBaseURL string) http.Handler {
    mbClient := musicbrainz.NewClient(mbBaseURL, "playlists-test/0.0.1 ( test@example.com )")
    svc := search.NewService(mbClient)
    handler := search.NewHandler(svc)

    mux := http.NewServeMux()
    mux.HandleFunc("GET /api/v1/songs/search", handler.Search)
    return mux
}

func TestIntegration_Search_HappyPath(t *testing.T) {
    // Start a fake MusicBrainz server.
    fakeMB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        assert.Contains(t, r.URL.Path, "/ws/2/recording")
        assert.NotEmpty(t, r.Header.Get("User-Agent"))
        w.Header().Set("Content-Type", "application/json")
        fmt.Fprint(w, mbSearchFixture)
    }))
    defer fakeMB.Close()

    // Start the real app pointed at the fake MB server.
    app := httptest.NewServer(newTestApp(fakeMB.URL))
    defer app.Close()

    resp, err := http.Get(app.URL + "/api/v1/songs/search?title=Bohemian+Rhapsody&artist=Queen")
    require.NoError(t, err)
    defer resp.Body.Close()

    assert.Equal(t, http.StatusOK, resp.StatusCode)
    assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

    var body search.Response
    require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
    require.Len(t, body.Results, 2)

    first := body.Results[0]
    assert.Equal(t, "b1a9c0e2-0000-0000-0000-000000000001", first.MBID)
    assert.Equal(t, "Bohemian Rhapsody", first.Title)
    assert.Equal(t, "Queen", first.Artist)
    assert.Equal(t, "0383dadf-2a4e-4d10-a46a-e9e041da8eb3", first.ArtistMBID)
    assert.Equal(t, "A Night at the Opera", first.Album)
    assert.Equal(t, "1dc4c347-a1db-32aa-b14f-bc9cc507b843", first.AlbumMBID)
    assert.Equal(t, "1975-11-21", first.ReleaseDate)
    assert.Equal(t, 354000, first.DurationMs)
    assert.Equal(t, "studio recording", first.Disambiguation)
}

func TestIntegration_Search_EmptyResults(t *testing.T) {
    fakeMB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        fmt.Fprint(w, `{"recordings": []}`)
    }))
    defer fakeMB.Close()

    app := httptest.NewServer(newTestApp(fakeMB.URL))
    defer app.Close()

    resp, err := http.Get(app.URL + "/api/v1/songs/search?title=NonexistentSong&artist=NoArtist")
    require.NoError(t, err)
    defer resp.Body.Close()

    assert.Equal(t, http.StatusOK, resp.StatusCode)

    var body search.Response
    require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
    // The spec requires an array, never null, even when empty.
    require.NotNil(t, body.Results)
    assert.Empty(t, body.Results)
}

func TestIntegration_Search_ValidationError_TitleTooShort(t *testing.T) {
    // No fake MB server needed — validation fails before the client is called.
    app := httptest.NewServer(newTestApp("http://unused"))
    defer app.Close()

    resp, err := http.Get(app.URL + "/api/v1/songs/search?title=Q&artist=Queen")
    require.NoError(t, err)
    defer resp.Body.Close()

    assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

    var body map[string]any
    require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
    assert.Equal(t, float64(400), body["status"])
    assert.Equal(t, "Bad Request", body["error"])
    assert.Contains(t, body["message"], "title")
}

func TestIntegration_Search_ValidationError_ArtistMissing(t *testing.T) {
    app := httptest.NewServer(newTestApp("http://unused"))
    defer app.Close()

    resp, err := http.Get(app.URL + "/api/v1/songs/search?title=Bohemian+Rhapsody")
    require.NoError(t, err)
    defer resp.Body.Close()

    assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

    var body map[string]any
    require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
    assert.Contains(t, body["message"], "artist")
}

func TestIntegration_Search_MusicBrainzUnavailable(t *testing.T) {
    // Fake MB server always returns 503.
    fakeMB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusServiceUnavailable)
    }))
    defer fakeMB.Close()

    app := httptest.NewServer(newTestApp(fakeMB.URL))
    defer app.Close()

    resp, err := http.Get(app.URL + "/api/v1/songs/search?title=Bohemian+Rhapsody&artist=Queen")
    require.NoError(t, err)
    defer resp.Body.Close()

    assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

    var body map[string]any
    require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
    assert.Equal(t, float64(503), body["status"])
    assert.Contains(t, body["message"], "MusicBrainz")
}

func TestIntegration_Search_ForwardsQueryToMusicBrainz(t *testing.T) {
    var capturedQuery string
    fakeMB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        capturedQuery = r.URL.Query().Get("query")
        w.Header().Set("Content-Type", "application/json")
        fmt.Fprint(w, `{"recordings": []}`)
    }))
    defer fakeMB.Close()

    app := httptest.NewServer(newTestApp(fakeMB.URL))
    defer app.Close()

    _, err := http.Get(app.URL + "/api/v1/songs/search?title=Bohemian+Rhapsody&artist=Queen")
    require.NoError(t, err)

    // Verify the query forwarded to MusicBrainz contains both search terms.
    assert.Contains(t, capturedQuery, "Bohemian+Rhapsody")
    assert.Contains(t, capturedQuery, "Queen")
}
```

> **Note on the `search_test` package:** the integration test uses `package search_test` (black-box suffix) and imports `search.Response` explicitly. This means `Response` must be **exported** from `internal/search/types.go`, which it already is.

> **Note on retry timing:** in integration tests, the client is configured with 3 retries and a 2-second delay between them. The "MB unavailable" test will take up to 4 seconds because the client retries. If this becomes a problem you can add a constructor option to inject the retry delay for tests.

---

## 5.4 Running the tests

```bash
# Unit tests only (fast, no network)
make test

# Unit + integration tests
make testint
```

Expected output for `make testint`:

```
=== RUN   TestWithRetry_SucceedsOnFirstAttempt
--- PASS: TestWithRetry_SucceedsOnFirstAttempt (0.00s)
=== RUN   TestWithRetry_RetriesAndSucceeds
--- PASS: TestWithRetry_RetriesAndSucceeds (0.00s)
...
=== RUN   TestIntegration_Search_HappyPath
--- PASS: TestIntegration_Search_HappyPath (0.01s)
=== RUN   TestIntegration_Search_EmptyResults
--- PASS: TestIntegration_Search_EmptyResults (0.01s)
...
PASS
ok      github.com/davidsilvasanmartin/playlists-go/internal/musicbrainz
ok      github.com/davidsilvasanmartin/playlists-go/internal/search
```

---

## 5.5 Making retry delays test-friendly (optional)

The 2-second retry delay will slow down the "MB unavailable" integration test (3 attempts × 2s = 4s overhead). To avoid this, you can make the delay configurable via a functional option or a constructor parameter. Here is the minimal version using a package-level variable — acceptable for tests:

```go
// In client_impl.go, replace the hard-coded delay:
var retryDelay = 2 * time.Second

// In NewClient, use retryDelay instead of 2*time.Second.
// In tests (same package), override it:
//   retryDelay = time.Millisecond
```

This keeps the implementation simple while making tests fast. Only do this if the delay is actually a problem — for 5 integration tests, 4 seconds total is acceptable.
