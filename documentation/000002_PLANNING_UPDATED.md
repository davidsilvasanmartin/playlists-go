# Playlists App — Planning Document (Go)

---

## 1. User Experience Review

### The two-page model (Explore + Playlists)

Your proposed model maps to a well-understood pattern: a **catalog** page and a **library** page. This is how Spotify,
Apple Music, and YouTube Music all work at their core. The mental model is sound:

- **Explore** — discover and add songs to your local catalog by querying MusicBrainz.
- **Playlists** — organize songs from your catalog into playlists.

The key distinction worth enforcing: **Explore is not a search that plays songs — it is a cataloging tool.** The user is
building a local, durable copy of song metadata. Once a song is in the catalog, it is safe even if MusicBrainz changes
its data.

### What successful apps do differently

| App           | "Add to library" model                                          |
|---------------|-----------------------------------------------------------------|
| Spotify       | Search → click heart/+ → song saved to your library             |
| Apple Music   | Search → Add to Library — explicit two-step                     |
| YouTube Music | Add to playlist directly from search, no intermediate "library" |
| Plex          | Scan local files, metadata fetched automatically                |

The most common pattern is a **two-step confirmation**: search results appear, then the user explicitly clicks "Add to
catalog" on a specific result. This prevents accidental adds of wrong versions (e.g. live recording vs. studio version).

### Recommended UX improvements

1. **Distinguish "search results" from "catalog entries."** The Explore page should show ephemeral search results. The
   user picks the correct recording and clicks "Add to catalog." Only at that point is anything persisted. This avoids
   cluttering your database with every search result ever seen.

2. **Versioning awareness.** MusicBrainz distinguishes recordings (a specific performance) from tracks (a recording's
   appearance on a release). A song like "Bohemian Rhapsody" has dozens of recordings. Your UI should surface enough
   detail (release year, release name, duration) to let the user pick the right one.

3. **Catalog page (implicit third page).** You will naturally need a way to browse songs already in your catalog, even
   before playlists exist. Plan for a simple `GET /songs` catalog listing from the start.

4. **Smart playlists as a first-class concept.** Rather than a footnote, smart playlists (by genre, year, artist, etc.)
   should be designed as a peer of manual playlists — same UI surface, different creation flow.

---

## 2. API Design

### 2.1 Conventions

- Base path: `/api/v1`
- All responses: `application/json`
- Error envelope:

```json
{
  "timestamp": "2026-03-21T10:00:00Z",
  "status": 503,
  "error": "Service Unavailable",
  "message": "MusicBrainz API is currently unreachable",
  "path": "/api/v1/songs/search"
}
```

- Successful collections always return an array (never null), even when empty.

---

### 2.2 MusicBrainz Search

#### `GET /api/v1/songs/search`

> **GET vs POST:**  `GET` is the correct verb here. This endpoint has no side effects — it is a pure read from
> MusicBrainz. `GET` is idempotent, cacheable by HTTP intermediaries, and semantically accurate for a search. A request
> body on a GET is technically allowed by HTTP but ignored by many proxies and unsupported by some clients. Query
> parameters are the standard.

Search for recordings on MusicBrainz. Results are **not persisted** — this is a pure proxy with cleanup. The user
browses results and explicitly adds individual songs.

**Query parameters:**

| Parameter | Required | Constraint    | Description     |
|-----------|----------|---------------|-----------------|
| `title`   | Yes      | ≥2 characters | Recording title |
| `artist`  | Yes      | ≥2 characters | Artist name     |

**Example:** `GET /api/v1/songs/search?title=Bohemian+Rhapsody&artist=Queen`

> **Note on genres in search results:** Genres are **not** returned here. The MusicBrainz search endpoint returns
> abbreviated recording data; full genre tags require a separate lookup call (made automatically when the user adds a
> song to the catalog via `POST /api/v1/songs`). Displaying genres in search results is therefore not possible without a
> separate API call per result, which would violate the 1 req/sec limit.

**Response `200 OK`:**

```json
{
  "results": [
    {
      "mbid": "b1a9c0e2-1234-4abc-8def-000000000001",
      "title": "Bohemian Rhapsody",
      "artist": "Queen",
      "artistMbid": "0383dadf-2a4e-4d10-a46a-e9e041da8eb3",
      "album": "A Night at the Opera",
      "albumMbid": "1dc4c347-a1db-32aa-b14f-bc9cc507b843",
      "releaseDate": "1975-11-21",
      "durationMs": 354000,
      "disambiguation": "studio recording"
    },
    {
      "mbid": "b1a9c0e2-1234-4abc-8def-000000000002",
      "title": "Bohemian Rhapsody",
      "artist": "Queen",
      "artistMbid": "0383dadf-2a4e-4d10-a46a-e9e041da8eb3",
      "album": "Live at Wembley '86",
      "albumMbid": "2ef5c347-0000-0000-0000-bc9cc507b843",
      "releaseDate": "1992-05-26",
      "durationMs": 360000,
      "disambiguation": "live, 1986-07-12: Wembley Stadium, London, UK"
    }
  ]
}
```

**Response `400 Bad Request`** (validation failure):

```json
{
  "timestamp": "2026-03-21T10:00:00Z",
  "status": 400,
  "error": "Bad Request",
  "message": "Field 'title' must be at least 2 characters",
  "path": "/api/v1/songs/search"
}
```

**Response `503 Service Unavailable`** (MusicBrainz unreachable after retries):

```json
{
  "timestamp": "2026-03-21T10:00:00Z",
  "status": 503,
  "error": "Service Unavailable",
  "message": "MusicBrainz API is currently unreachable. Please try again later.",
  "path": "/api/v1/songs/search"
}
```

---

### 2.3 Song Catalog

These endpoints manage the **local catalog** — songs the user has explicitly saved.

#### `POST /api/v1/songs`

Add a song to the local catalog by its MusicBrainz ID. The backend performs a **full lookup** to MusicBrainz (
`GET /ws/2/recording/{mbid}?inc=artists+releases+genres`) to fetch authoritative data including genres. This call counts
toward the 1 req/sec rate limit.

**Request:**

```json
{
  "mbid": "b1a9c0e2-1234-4abc-8def-000000000001"
}
```

**Response `201 Created`:**

```json
{
  "id": 42,
  "mbid": "b1a9c0e2-1234-4abc-8def-000000000001",
  "title": "Bohemian Rhapsody",
  "artist": "Queen",
  "artistMbid": "0383dadf-2a4e-4d10-a46a-e9e041da8eb3",
  "album": "A Night at the Opera",
  "albumMbid": "1dc4c347-a1db-32aa-b14f-bc9cc507b843",
  "releaseDate": "1975-11-21",
  "durationMs": 354000,
  "disambiguation": "studio recording",
  "genres": [
    "rock",
    "hard rock",
    "classic rock"
  ],
  "createdAt": "2026-03-21T10:05:00Z"
}
```

**Response `409 Conflict`** (song with this MBID already in catalog).

#### `GET /api/v1/songs`

List all songs in the catalog. Supports optional query parameters: `?artist=Queen`, `?album=Opera`, `?title=Bohemian`,
`?genre=rock`.

**Response `200 OK`:**

```json
{
  "songs": [
    /* same shape as POST 201 response */
  ],
  "total": 1
}
```

#### `GET /api/v1/songs/{id}`

Retrieve a single catalog song by its local database ID.

#### `DELETE /api/v1/songs/{id}`

Remove a song from the catalog. This will also remove it from all playlists it belongs to.

---

### 2.4 Playlists

#### `GET /api/v1/playlists`

List all playlists (both manual and smart).

**Response `200 OK`:**

```json
{
  "playlists": [
    {
      "id": 1,
      "name": "Road trip",
      "type": "MANUAL",
      "songCount": 12,
      "createdAt": "2026-01-15T08:00:00Z",
      "updatedAt": "2026-03-20T14:22:00Z"
    },
    {
      "id": 2,
      "name": "All 80s rock",
      "type": "SMART",
      "songCount": 47,
      "createdAt": "2026-02-01T09:00:00Z",
      "updatedAt": "2026-03-21T00:00:00Z"
    }
  ]
}
```

#### `POST /api/v1/playlists`

Create a new playlist.

**Request (manual):**

```json
{
  "name": "Road trip",
  "type": "MANUAL"
}
```

**Request (smart):**

```json
{
  "name": "All 80s rock",
  "type": "SMART",
  "criteria": [
    {
      "field": "GENRE",
      "operator": "CONTAINS",
      "value": "rock"
    },
    {
      "field": "RELEASE_YEAR",
      "operator": "BETWEEN",
      "value": "1980",
      "valueTo": "1989"
    }
  ]
}
```

**Response `201 Created`:** full playlist object (see GET below).

#### `GET /api/v1/playlists/{id}`

Get a playlist with its songs resolved.

**Response `200 OK` (manual):**

```json
{
  "id": 1,
  "name": "Road trip",
  "type": "MANUAL",
  "createdAt": "2026-01-15T08:00:00Z",
  "updatedAt": "2026-03-20T14:22:00Z",
  "songs": [
    {
      "position": 1,
      "song": {
        /* full song object */
      }
    }
  ]
}
```

**Response `200 OK` (smart):**

```json
{
  "id": 2,
  "name": "All 80s rock",
  "type": "SMART",
  "criteria": [
    {
      "field": "GENRE",
      "operator": "CONTAINS",
      "value": "rock"
    },
    {
      "field": "RELEASE_YEAR",
      "operator": "BETWEEN",
      "value": "1980",
      "valueTo": "1989"
    }
  ],
  "createdAt": "2026-02-01T09:00:00Z",
  "updatedAt": "2026-03-21T00:00:00Z",
  "songs": [
    /* dynamically resolved at request time */
  ]
}
```

#### `PUT /api/v1/playlists/{id}`

Update a playlist's name and songs or, for smart playlists, its criteria. If manual, full playlist must be provided.
This allows then updating the order as well. This endpoint should be used also to add or remove songs from an existing
manual playlist.

#### `DELETE /api/v1/playlists/{id}`

Delete a playlist. Songs in the catalog are not deleted.

---

## 3. Database Schema

### 3.1 Songs table

This table is the canonical local catalog. Data is fetched from MusicBrainz once at add-time and stored durably. The
`mbid` column is the stable external identifier.

```sql
-- songs: canonical local catalog
CREATE TABLE songs (
    id             BIGSERIAL PRIMARY KEY,
    mbid           VARCHAR(36)  NOT NULL UNIQUE,   -- MusicBrainz Recording ID
    title          VARCHAR(500) NOT NULL,
    artist         VARCHAR(500) NOT NULL,
    artist_mbid    VARCHAR(36),                    -- MusicBrainz Artist ID
    album          VARCHAR(500),
    album_mbid     VARCHAR(36),                    -- MusicBrainz Release ID
    release_date   DATE,
    duration_ms    INTEGER,
    disambiguation VARCHAR(500),                   -- e.g. "live", "acoustic version"
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_songs_artist       ON songs (artist);
CREATE INDEX idx_songs_release_date ON songs (release_date);
```

Genres are stored in a separate normalised table (see section 3.2) and linked via a join table.

---

### 3.2 Genres tables

MusicBrainz returns community-voted genre tags on recordings. These are normalised into two tables so that smart
playlist criteria like `GENRE CONTAINS "rock"` can be evaluated with a join rather than a full-text scan.

```sql
CREATE TABLE genres (
    id   BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE   -- stored lowercase: "rock", "hard rock"
);

CREATE TABLE song_genres (
    song_id  BIGINT NOT NULL REFERENCES songs(id) ON DELETE CASCADE,
    genre_id BIGINT NOT NULL REFERENCES genres(id) ON DELETE CASCADE,
    PRIMARY KEY (song_id, genre_id)
);

CREATE INDEX idx_song_genres_genre_id ON song_genres (genre_id);
```

Genre names are stored in lowercase and normalised on write (e.g. `"Hard Rock"` → `"hard rock"`). If a genre already
exists in the `genres` table it is reused; otherwise it is inserted. This is an upsert at the application level.

---

### 3.3 Playlist schema

There are two playlist types: **manual** (ordered list of songs) and **smart** (rule-based).

```sql
CREATE TABLE playlists (
    id         BIGSERIAL PRIMARY KEY,
    name       VARCHAR(255) NOT NULL,
    type       VARCHAR(20)  NOT NULL,   -- 'MANUAL' | 'SMART'
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- Only used by MANUAL playlists
CREATE TABLE playlist_songs (
    playlist_id BIGINT  NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    song_id     BIGINT  NOT NULL REFERENCES songs(id) ON DELETE CASCADE,
    position    INTEGER NOT NULL,
    PRIMARY KEY (playlist_id, song_id)
);

-- Only used by SMART playlists
CREATE TABLE playlist_criteria (
    id          BIGSERIAL PRIMARY KEY,
    playlist_id BIGINT       NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    field       VARCHAR(50)  NOT NULL,
    operator    VARCHAR(20)  NOT NULL,
    value       VARCHAR(500) NOT NULL,
    value_to    VARCHAR(500)
);
```

A single `playlists` table with a `type` discriminator keeps listing queries simple. The two auxiliary tables are
populated only for the relevant type, enforced by application logic.

---

### 3.4 Smart playlist resolution

When `GET /api/v1/playlists/{id}` is called for a `SMART` playlist, the backend dynamically queries the `songs` table
(with joins to `song_genres`/`genres` as needed) using the stored criteria. The query is built at runtime in Go using
safe parameterised SQL fragments — no string interpolation.

Supported fields initially:

| Field          | DB join                          | Operators                                        |
|----------------|----------------------------------|--------------------------------------------------|
| `ARTIST`       | `songs.artist`                   | `EQUALS`, `CONTAINS`                             |
| `ALBUM`        | `songs.album`                    | `EQUALS`, `CONTAINS`                             |
| `TITLE`        | `songs.title`                    | `CONTAINS`                                       |
| `RELEASE_YEAR` | `songs.release_date` (year part) | `EQUALS`, `BETWEEN`, `GREATER_THAN`, `LESS_THAN` |
| `DURATION_MS`  | `songs.duration_ms`              | `BETWEEN`, `GREATER_THAN`, `LESS_THAN`           |
| `GENRE`        | `song_genres` → `genres.name`    | `EQUALS`, `CONTAINS`                             |

> **sqlc note:** Smart playlist queries cannot be statically defined because the WHERE clause varies at runtime. Write a
> small query builder in plain Go that assembles parameterised `$1`, `$2`, … fragments and appends them to a base
> query. This is the one place where sqlc's static queries are insufficient; the builder is small and easily tested.

---

## 4. Back-End Architecture

### 4.1 Project layout

Go does not have a framework-enforced module system, but you can achieve the same isolation through **package
boundaries**. The rule: packages inside `internal/` are only importable by code in the same module. Sub-packages can
further restrict access by keeping unexported identifiers.

```
playlists/
├── cmd/
│   └── server/
│       └── main.go            -- wires everything together, starts HTTP server
├── internal/
│   ├── musicbrainz/           -- MusicBrainz HTTP client (no domain logic)
│   │   ├── client.go          -- Client interface (public contract)
│   │   ├── client_impl.go     -- implementation: rate limiter + retry + http
│   │   └── types.go           -- request/response structs
│   ├── song/                  -- local song catalog
│   │   ├── handler.go         -- HTTP handlers
│   │   ├── service.go         -- domain logic
│   │   ├── repository.go      -- wraps sqlc queries
│   │   └── types.go           -- DTOs
│   ├── playlist/              -- playlist management
│   │   ├── handler.go
│   │   ├── service.go
│   │   ├── repository.go
│   │   └── types.go
│   ├── search/                -- orchestrates search (calls musicbrainz, no persistence)
│   │   ├── handler.go
│   │   ├── service.go
│   │   └── types.go
│   └── api/
│       ├── router.go          -- registers all routes on a single mux
│       └── middleware.go      -- logging, error handling, CORS
├── db/
│   ├── migrations/            -- Goose SQL migration files
│   │   ├── 00001_create_songs.sql
│   │   ├── 00002_create_genres.sql
│   │   └── 00003_create_playlists.sql
│   └── queries/               -- sqlc input SQL files
│       ├── songs.sql
│       ├── genres.sql
│       └── playlists.sql
├── sqlc/                      -- sqlc generated Go code (do not edit)
│   ├── db.go
│   ├── models.go
│   └── *.sql.go
├── sqlc.yaml                  -- sqlc configuration
└── go.mod
```

**Inter-module communication rules:**

- `search` imports `musicbrainz.Client` (interface) — it does not import the implementation.
- `song` and `playlist` import their own `repository.go` wrappers, which in turn use sqlc-generated code.
- `playlist` imports `song` types only for embedding song data in playlist responses; it does not import `song`'s
  repository directly.
- `musicbrainz` has zero imports from other internal packages — pure infrastructure adapter.
- `cmd/server/main.go` is the only place that imports all packages and wires them together (dependency injection by
  hand).

---

### 4.2 HTTP routing

**Go 1.22+ standard library** (`net/http`) supports path parameters natively:

```go
mux := http.NewServeMux()
mux.HandleFunc("GET /api/v1/songs/search",  searchHandler.Search)
mux.HandleFunc("GET /api/v1/songs",         songHandler.List)
mux.HandleFunc("POST /api/v1/songs",        songHandler.Create)
mux.HandleFunc("GET /api/v1/songs/{id}",    songHandler.GetByID)
mux.HandleFunc("DELETE /api/v1/songs/{id}", songHandler.Delete)
// playlists similarly
```

Path parameters are read with `r.PathValue("id")`.

> **Alternative:** [`chi`](https://github.com/go-chi/chi) is a thin, idiomatic router with middleware support. Use it
> if you find the stdlib mux insufficient (e.g. you need route groups or sub-routers). It has no dependencies of its
> own. For this project, stdlib is sufficient to start with.

---

### 4.3 MusicBrainz client

- **HTTP client:** `net/http` standard library with a custom `http.Client` (configured timeout).
- **Rate limiting:** `golang.org/x/time/rate` — see section 4.4.
- **User-Agent:** `playlists/0.0.1 ( contact@example.com )` — MusicBrainz requires this.

The client is defined as an interface so that tests can inject a mock:

```go
// internal/musicbrainz/client.go
type Client interface {
    Search(ctx context.Context, title, artist string) ([]Recording, error)
    Lookup(ctx context.Context, mbid string) (*Recording, error)
}
```

The concrete implementation `clientImpl` (unexported) lives in `client_impl.go` and is constructed by a `NewClient`
factory function.

#### Operation 1 — Search

```
GET /ws/2/recording?query=recording:"{title}" AND artistname:"{artist}"&fmt=json
```

#### Operation 2 — Lookup by MBID

```
GET /ws/2/recording/{mbid}?inc=artists+releases+genres&fmt=json
```

Field mapping (same as before — see API design section 2.2 of the original document for the full table).

---

### 4.4 Rate limiting and retry

**Concern 1 — Rate limiting (proactive):** ensure outgoing requests do not exceed 1/sec.
**Concern 2 — Retry with backoff (reactive):** handle `429 Too Many Requests` and transient errors.

#### Rate limiting → `golang.org/x/time/rate`

`golang.org/x/time/rate` is the canonical Go token-bucket rate limiter, maintained by the Go team as part of the
extended standard library. It is the industry-standard choice in Go.

```go
import "golang.org/x/time/rate"

limiter := rate.NewLimiter(rate.Every(time.Second), 1)
// In the client, before each request:
if err := limiter.Wait(ctx); err != nil {
    return nil, err
}
```

`limiter.Wait(ctx)` blocks until a token is available, honouring context cancellation. The limiter is a field on
`clientImpl`, initialised once in `NewClient`.

#### Retry → hand-rolled helper

A minimal retry loop is idiomatic Go and requires no external dependency:

```go
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

`limiter.Wait` is called **inside** the retry callback, so every attempt (including retries) acquires a token before
hitting the network. This guarantees the 1 req/sec limit even under retry conditions.

> **Not recommended:** any heavyweight resilience library (e.g. `sony/gobreaker`, `failsafe-go`). The retry logic here
> is 15 lines and trivially testable. Bring in a library only if you later need circuit-breaking.

---

### 4.5 Layer pattern per module

Each module follows the same internal layering:

```
handler.go   (HTTP layer — parse request, call service, write response)
    └── service.go  (domain/orchestration — business logic)
        └── repository.go  (data access — wraps sqlc generated queries)
```

- **DTOs** are plain Go structs in `types.go`, separate from sqlc-generated models.
- **Handlers** parse JSON with `encoding/json`, validate manually or with
  [`go-playground/validator`](https://github.com/go-playground/validator), then call the service.
- **Error handling** is explicit: services return typed errors (e.g. `ErrNotFound`, `ErrAlreadyExists`); handlers
  map these to HTTP status codes in a central `writeError` helper.
- **No global state** — all dependencies (DB, rate limiter, etc.) are passed via constructors.

**Validation approach:** Keep it simple. For the small number of inputs in this API, manual validation (check string
length, required fields) in the handler is clearest. Only reach for `go-playground/validator` if the number of
validated fields grows significantly.

---

### 4.6 Database access

**sqlc** generates type-safe Go functions from SQL queries you write. The workflow:

1. Write migration SQL files under `db/migrations/` (Goose format).
2. Write query SQL files under `db/queries/` with sqlc annotations.
3. Run `sqlc generate` to produce Go code in `sqlc/`.
4. Wrap the generated `*Queries` in a `repository.go` per module.

**Database driver:** use [`pgx/v5`](https://github.com/jackc/pgx) directly. It is the most complete PostgreSQL driver
for Go, with better performance and richer type support than `lib/pq`. Configure sqlc to use `pgx/v5`:

```yaml
# sqlc.yaml
version: "2"
sql:
  - engine: "postgresql"
    queries: "db/queries"
    schema: "db/migrations"
    gen:
      go:
        package: "sqlc"
        out: "sqlc"
        sql_driver: "pgx/v5"
```

**Connection pooling:** use `pgxpool.Pool` (from `github.com/jackc/pgx/v5/pgxpool`). Pass the pool to `NewQueries`
as generated by sqlc.

**Goose migrations:** run on startup (before the HTTP server starts) using Goose's embedded Go API:

```go
goose.SetDialect("postgres")
goose.Up(db, "db/migrations")
```

Migration file naming: `00001_create_songs.sql`, `00002_create_genres.sql`, etc.

---

### 4.7 OpenAPI documentation

Go does not have Spring's annotation-driven OpenAPI generation. Two practical options:

**Option A — Write the OpenAPI spec by hand** (recommended to start with): maintain an `openapi.yaml` at the project
root. Serve it from a `GET /api/v1/openapi.yaml` endpoint. Use the
[Swagger UI Docker image](https://hub.docker.com/r/swaggerapi/swagger-ui) in `docker-compose.yml` to render it
locally. This is the simplest approach and keeps the spec as a first-class artefact.

**Option B — Generate from code comments** using [`swaggo/swag`](https://github.com/swaggo/swag): annotate handlers
with special comments and run `swag init` to generate the spec. More automation, but the comment syntax is verbose and
the generated spec is sometimes imprecise.

**Recommendation:** start with Option A. The API surface is small, and a hand-written spec is easier to keep accurate
and review in PRs.

---

### 4.8 Testing strategy

| Layer              | Tool                                               | Notes                                                                              |
|--------------------|----------------------------------------------------|------------------------------------------------------------------------------------|
| Unit tests         | Go standard `testing` package + `testify/assert`   | Pure Go, no Docker. For services (mock the repository interface), client logic.    |
| Integration tests  | `testcontainers-go` + `net/http` client            | Real HTTP, real Postgres in Docker, real app binary. MusicBrainz mocked.           |
| MusicBrainz mock   | `net/http/httptest.NewServer`                      | Simple in-process HTTP server serving canned JSON responses. No external process.  |

#### Unit tests

- Define interfaces for all external dependencies (DB repository, MusicBrainz client).
- Use `testify/mock` or hand-written fakes to inject them in service tests.
- Test the rate limiter and retry logic by injecting a fake HTTP server that returns 429 or errors.
- File naming convention: `*_test.go` in the same package (white-box) or a `_test` package suffix (black-box).

#### Integration tests

- A `TestMain` function in a `_integration_test.go` file (or a separate `_test` build tag) starts:
  1. A PostgreSQL container via `testcontainers-go`.
  2. A fake MusicBrainz HTTP server via `httptest.NewServer`.
  3. The real application binary pointed at the test database and fake MB server.
- Tests send real HTTP requests using `net/http` and assert on the JSON response.
- Seed data: plain SQL `INSERT` statements run via `pgx` in `TestMain` or per-test setup functions.
- Build tag `//go:build integration` (or `!unit`) keeps integration tests out of `go test ./...` by default; run
  explicitly with `go test -tags=integration ./...`.

**`testcontainers-go`** spins up a real PostgreSQL Docker container, runs Goose migrations, and provides the
connection string to the app. The container is torn down after the test suite completes.

**`httptest.NewServer`** as the MusicBrainz mock is idiomatic Go: it's in the standard library, starts in
milliseconds, and you can vary responses per test by swapping the handler.

---

### 4.9 Recommended technologies

| Concern            | Technology                                    | Rationale                                                                 |
|--------------------|-----------------------------------------------|---------------------------------------------------------------------------|
| HTTP server        | `net/http` (Go stdlib, 1.22+)                 | Path params built in; no dependency; sufficient for this API              |
| HTTP client        | `net/http` (Go stdlib)                        | Standard; configure timeout via `http.Client{Timeout: 10*time.Second}`    |
| Database driver    | `pgx/v5` (`github.com/jackc/pgx/v5`)          | Best-in-class PostgreSQL driver for Go; used by sqlc                      |
| Connection pool    | `pgxpool` (bundled with pgx/v5)               | Built-in pool; pass to sqlc-generated `*Queries`                          |
| SQL queries        | `sqlc`                                        | Type-safe, compile-time checked; no ORM magic                             |
| Migrations         | Goose                                         | SQL-first migrations with embedded Go API for startup runs                |
| Rate limiting      | `golang.org/x/time/rate`                      | Go team's token-bucket implementation; `limiter.Wait(ctx)` blocks cleanly |
| Retry              | Hand-rolled helper (15 lines)                 | No dependency; trivially testable; sufficient for 2 retries               |
| JSON               | `encoding/json` (Go stdlib)                   | Standard; no extra dependency                                             |
| Validation         | Manual checks in handlers                     | Sufficient for small input surface; add `go-playground/validator` later   |
| Config             | `os.Getenv` / `godotenv`                      | Read `DATABASE_URL`, `PORT`, `MB_BASE_URL` from environment               |
| Testing assertions | `testify/assert` + `testify/mock`             | Standard Go testing helper; lightweight                                   |
| Integration tests  | `testcontainers-go`                           | Spins up real Postgres in Docker per test run                             |
| MB mock in tests   | `net/http/httptest` (Go stdlib)               | In-process HTTP server; no external process required                      |
| OpenAPI docs       | Hand-written `openapi.yaml` + Swagger UI      | Simple, accurate, reviewable                                              |

**Not recommended:**

- Any Go web framework (Gin, Echo, Fiber) — the stdlib mux in Go 1.22 is sufficient; frameworks add indirection.
- GORM or other ORMs — sqlc gives you type safety without the magic.
- Kafka — not justified for 1 req/sec rate limiting.
- External retry libraries (`failsafe-go`, `avast/retry-go`) — the inline helper is clearer and smaller.

---

### 4.10 Development lifecycle

Local development database (`docker-compose.yml`):

- Service name: `postgres`
- Database: `playlists`
- User: `playlists`
- Password: `playlists`
- Port: `5432`

Configuration is passed via environment variables (no YAML config files):

| Variable        | Default             | Description                        |
|-----------------|---------------------|------------------------------------|
| `DATABASE_URL`  | (required)          | `postgres://playlists:playlists@localhost:5432/playlists` |
| `PORT`          | `8080`              | HTTP listen port                   |
| `MB_BASE_URL`   | `https://musicbrainz.org` | Override for tests              |
| `MB_USER_AGENT` | (required)          | e.g. `playlists/0.0.1 ( me@example.com )` |

Startup sequence in `main.go`:

1. Read config from environment.
2. Open `pgxpool` connection.
3. Run Goose migrations (`goose.Up`).
4. Construct all dependencies (sqlc queries → repositories → services → handlers).
5. Register routes on `http.ServeMux`.
6. Start `http.ListenAndServe`.

---

## 5. Decisions Log

| Decision                  | Choice                                                     | Rationale                                                                                                                          |
|---------------------------|------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------|
| Search endpoint verb      | `GET` with query params                                    | Idempotent, cacheable, semantically correct for a read                                                                             |
| Song persistence trigger  | Re-fetch full details via lookup on `POST /api/v1/songs`   | Search results are abbreviated; lookup gives authoritative data including genres                                                   |
| `disambiguation` field    | Included                                                   | Critical for distinguishing studio vs. live vs. alternate versions                                                                 |
| Genres                    | From day one via lookup `inc=genres`                       | Required for smart playlist `GENRE` criterion; fetched during catalog add                                                          |
| Playlist schema           | Single `playlists` table with type discriminator           | One table for listings; avoids UNION; type enforced by application logic                                                           |
| Rate limiting library     | `golang.org/x/time/rate`                                   | Go team's canonical token-bucket; `limiter.Wait(ctx)` is idiomatic and context-aware                                              |
| Retry                     | Hand-rolled helper                                         | 15-line function; no dependency; trivially testable; no need for a library at this scale                                           |
| HTTP router               | Go stdlib `net/http` (1.22+)                               | Path params built in since Go 1.22; no framework needed for this API size                                                         |
| ORM / query layer         | sqlc + pgx/v5                                              | Type-safe queries without reflection; errors caught at `sqlc generate` time, not runtime                                           |
| Smart playlist queries    | Hand-built parameterised SQL in a query builder            | sqlc static queries cannot express runtime-variable WHERE clauses; a small builder is the correct Go approach                     |
| OpenAPI docs              | Hand-written `openapi.yaml` + Swagger UI Docker image      | Simplest accurate approach; spec is a first-class reviewable artefact                                                              |
| Integration test DB mock  | `testcontainers-go` (real Postgres)                        | Tests against real SQL dialect and constraints; no mock/prod divergence risk                                                       |
| MusicBrainz mock in tests | `net/http/httptest.NewServer`                              | Standard library; in-process; no external process; responses vary per test by swapping handler                                    |
