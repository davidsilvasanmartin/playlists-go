# Part 5 — Testing

The tests are organized into two layers:

| Layer | Command | What runs |
|---|---|---|
| Unit | `make test` | `internal/musicbrainz` and `internal/search` in isolation |
| Integration | `make testint` | Full HTTP stack: real handler + real service + fake MusicBrainz server |

Integration tests carry the `//go:build integration` tag so they are excluded from `go test ./...` and only run when explicitly requested with `-tags=integration`.

---

## 5.1 Unit tests — `internal/musicbrainz`

### `internal/musicbrainz/client_impl_test.go`

Tests the two unexported helpers: `withRetry` and `mapRecordings`.

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

The service needs a fake `musicbrainz.Client`. We write it by hand — it's six lines and avoids pulling in a mock framework.

### `internal/search/service_test.go`

```go
package search

import (
    "context"
    "errors"
    "testing"

    "github.com/davidsilvasanmartin/playlists-go/internal/musicbrainz"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
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
    svc := NewService(fake)

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
    svc := NewService(fake)

    results, err := svc.Search(context.Background(), "Nonexistent Song", "No Artist")
    require.NoError(t, err)
    assert.Empty(t, results)
}

func TestService_Search_PropagatesError(t *testing.T) {
    fake := &fakeMBClient{err: errors.New("network error")}
    svc := NewService(fake)

    _, err := svc.Search(context.Background(), "Any Title", "Any Artist")
    assert.Error(t, err)
}
```

---

## 5.3 Integration tests — `internal/search`

Integration tests exercise the full HTTP stack in one process:

```
Test HTTP client → Handler → Service → musicbrainz.Client → fake MB server (httptest)
```

No Docker, no database. The fake MusicBrainz server is an in-process `httptest.Server` that returns canned JSON. This tests JSON serialization, validation, URL routing, and error mapping end-to-end.

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
