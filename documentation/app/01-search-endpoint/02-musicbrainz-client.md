# Part 2 — MusicBrainz Client

The `internal/musicbrainz` package is a pure infrastructure adapter. It has no knowledge of the rest of the application. It exposes a `Client` interface so the rest of the code never imports the concrete implementation directly — this makes it trivially mockable in tests.

---

## 2.1 `internal/musicbrainz/client.go` — the interface

```go
package musicbrainz

import "context"

// Client is the contract for communicating with the MusicBrainz API.
// Only Search is needed for the search endpoint; Lookup will be added
// when the POST /api/v1/songs endpoint is implemented.
type Client interface {
    Search(ctx context.Context, title, artist string) ([]Recording, error)
}
```

---

## 2.2 `internal/musicbrainz/types.go` — domain type + raw JSON structs

Two distinct sets of types live here:

- **`Recording`** — the clean domain model the rest of the app sees.
- **`mb*` structs** — unexported, matching the raw MusicBrainz JSON exactly.

```go
package musicbrainz

// Recording is the domain model returned by Client.Search.
// Fields may be empty if MusicBrainz did not return them.
type Recording struct {
    MBID           string
    Title          string
    Artist         string
    ArtistMBID     string
    Album          string
    AlbumMBID      string
    ReleaseDate    string
    DurationMs     int
    Disambiguation string
}

// ── raw MusicBrainz JSON structs (unexported) ──────────────────────────────

type mbSearchResponse struct {
    Recordings []mbRecording `json:"recordings"`
}

type mbRecording struct {
    ID             string      `json:"id"`
    Title          string      `json:"title"`
    Length         int         `json:"length"`
    Disambiguation string      `json:"disambiguation"`
    ArtistCredit   []mbCredit  `json:"artist-credit"`
    Releases       []mbRelease `json:"releases"`
}

type mbCredit struct {
    Artist mbArtistInfo `json:"artist"`
}

type mbArtistInfo struct {
    ID   string `json:"id"`
    Name string `json:"name"`
}

type mbRelease struct {
    ID    string `json:"id"`
    Title string `json:"title"`
    Date  string `json:"date"`
}
```

> **Note on releases:** MusicBrainz returns multiple releases for a recording (e.g. original album, remaster, deluxe edition). This client picks the **first** one in the list. A more sophisticated picker (prefer `Official` status, earliest date) can be added later.

---

## 2.3 `internal/musicbrainz/client_impl.go` — the implementation

This is the most important file in the package. It combines three concerns:

1. **Rate limiting** — `golang.org/x/time/rate` token bucket, 1 req/sec
2. **Retry with backoff** — hand-rolled helper, no external library
3. **HTTP mechanics** — build URL, set `User-Agent`, decode JSON

```go
package musicbrainz

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "net/url"
    "time"

    "golang.org/x/time/rate"
)

type clientImpl struct {
    httpClient *http.Client
    limiter    *rate.Limiter
    baseURL    string
    userAgent  string
}

// NewClient constructs the concrete MusicBrainz client.
// baseURL is e.g. "https://musicbrainz.org"; userAgent must satisfy MB's policy.
func NewClient(baseURL, userAgent string) Client {
    return &clientImpl{
        httpClient: &http.Client{Timeout: 10 * time.Second},
        limiter:    rate.NewLimiter(rate.Every(time.Second), 1),
        baseURL:    baseURL,
        userAgent:  userAgent,
    }
}

// Search queries MusicBrainz for recordings matching title and artist.
// The limiter is called inside the retry loop so every attempt (including
// retries) acquires a token before hitting the network.
func (c *clientImpl) Search(ctx context.Context, title, artist string) ([]Recording, error) {
    var result []Recording
    err := withRetry(ctx, 3, 2*time.Second, func() error {
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
    return result, err
}

func (c *clientImpl) doSearch(ctx context.Context, title, artist string) ([]Recording, error) {
    // MusicBrainz uses a Lucene-style query syntax.
    q := fmt.Sprintf(`recording:"%s" AND artistname:"%s"`, title, artist)
    params := url.Values{
        "query": {q},
        "fmt":   {"json"},
    }
    endpoint := c.baseURL + "/ws/2/recording?" + params.Encode()

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
        return nil, fmt.Errorf("musicbrainz returned status %d", resp.StatusCode)
    }

    var mbResp mbSearchResponse
    if err := json.NewDecoder(resp.Body).Decode(&mbResp); err != nil {
        return nil, err
    }

    return mapRecordings(mbResp.Recordings), nil
}

// mapRecordings converts raw MusicBrainz recordings to the domain type.
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
            rec.Artist = r.ArtistCredit[0].Artist.Name
            rec.ArtistMBID = r.ArtistCredit[0].Artist.ID
        }
        if len(r.Releases) > 0 {
            rec.Album = r.Releases[0].Title
            rec.AlbumMBID = r.Releases[0].ID
            rec.ReleaseDate = r.Releases[0].Date
        }
        out = append(out, rec)
    }
    return out
}

// withRetry calls fn up to maxAttempts times, waiting delay between attempts.
// It respects context cancellation. The rate limiter is called inside fn so
// every attempt acquires a token.
func withRetry(ctx context.Context, maxAttempts int, delay time.Duration, fn func() error) error {
    var err error
    for i := range maxAttempts {
        err = fn()
        if err == nil {
            return nil
        }
        if i < maxAttempts-1 {
            select {
            case <-time.After(delay):
            case <-ctx.Done():
                return ctx.Err()
            }
        }
    }
    return err
}
```

### Key design decisions

| Decision | Rationale |
|---|---|
| `limiter.Wait(ctx)` inside the retry callback | Guarantees the 1 req/sec limit even under retry |
| `rate.Every(time.Second), 1` (burst=1) | Strictly 1 token per second, no bursting |
| `withRetry` is unexported | It's an implementation detail; tested via the package's own `_test.go` |
| `clientImpl` is unexported | Callers use the `Client` interface, constructed via `NewClient` |

---

Continue to [Part 3 — Search Module](03-search-module.md).
