# Part 3 — Search Module

The `internal/search` package owns the `GET /api/v1/songs/search` endpoint end-to-end: DTOs, service logic, and HTTP handler. It imports `internal/musicbrainz` for the client interface, but nothing from `internal/api` — that avoids circular imports.

---

## 3.1 `internal/search/types.go` — DTOs

These are the shapes the handler reads from and writes to HTTP. They are deliberately separate from the `musicbrainz.Recording` domain type.

```go
package search

// Result is a single item in the search response.
type Result struct {
    MBID           string `json:"mbid"`
    Title          string `json:"title"`
    Artist         string `json:"artist"`
    ArtistMBID     string `json:"artistMbid"`
    Album          string `json:"album"`
    AlbumMBID      string `json:"albumMbid"`
    ReleaseDate    string `json:"releaseDate"`
    DurationMs     int    `json:"durationMs"`
    Disambiguation string `json:"disambiguation"`
}

// Response is the envelope returned by GET /api/v1/songs/search.
type Response struct {
    Results []Result `json:"results"`
}
```

---

## 3.2 `internal/search/service.go` — service layer

The service is a thin orchestrator: call the MB client, convert the results into DTOs.

```go
package search

import (
    "context"

    "github.com/davidsilvasanmartin/playlists-go/internal/musicbrainz"
)

// Service is the contract for the search business logic.
type Service interface {
    Search(ctx context.Context, title, artist string) ([]Result, error)
}

type service struct {
    mb musicbrainz.Client
}

// NewService creates a search service backed by the given MusicBrainz client.
func NewService(mb musicbrainz.Client) Service {
    return &service{mb: mb}
}

func (s *service) Search(ctx context.Context, title, artist string) ([]Result, error) {
    recordings, err := s.mb.Search(ctx, title, artist)
    if err != nil {
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
    return results, nil
}
```

---

## 3.3 `internal/search/handler.go` — HTTP handler

The handler is responsible for:
1. Parsing and **validating** query parameters (validation belongs in the handler, not the service)
2. Calling the service
3. Writing the JSON response or a structured error

```go
package search

import (
    "encoding/json"
    "net/http"
    "time"
)

// Handler handles HTTP requests for the search endpoint.
type Handler struct {
    service Service
}

// NewHandler creates a new search handler.
func NewHandler(svc Service) *Handler {
    return &Handler{service: svc}
}

// Search handles GET /api/v1/songs/search.
//
// Query params:
//   - title  (required, ≥2 chars)
//   - artist (required, ≥2 chars)
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
    title := r.URL.Query().Get("title")
    artist := r.URL.Query().Get("artist")

    if len(title) < 2 {
        writeError(w, r, http.StatusBadRequest, "Field 'title' must be at least 2 characters")
        return
    }
    if len(artist) < 2 {
        writeError(w, r, http.StatusBadRequest, "Field 'artist' must be at least 2 characters")
        return
    }

    results, err := h.service.Search(r.Context(), title, artist)
    if err != nil {
        writeError(w, r, http.StatusServiceUnavailable, "MusicBrainz API is currently unreachable. Please try again later.")
        return
    }

    writeJSON(w, http.StatusOK, Response{Results: results})
}

// ── response helpers ───────────────────────────────────────────────────────

// apiError is the standard error envelope defined in the API spec.
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

> **Why are `writeJSON`/`writeError` in the `search` package?**
> They cannot live in `internal/api` because `internal/api` will import `internal/search` (to register the handler) — putting shared utilities there would create a circular dependency. For now, duplicating these two tiny helpers per handler package is fine. When handler count grows, they move to a shared `internal/httputil` package.

---

Continue to [Part 4 — Wiring](04-wiring.md).
