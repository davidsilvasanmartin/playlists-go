# Part 1 — Project Setup

This tutorial implements the `GET /api/v1/songs/search` endpoint from the planning document. This endpoint is a pure proxy: it forwards search queries to MusicBrainz, maps the results, and returns them. **No database is involved.**

The full implementation lives in three packages:
- `internal/musicbrainz` — HTTP client with rate limiting and retry
- `internal/search` — HTTP handler + service layer
- `internal/api` — route registration
- `cmd/server` — entry point

---

## 1.1 Initialize the Go module

From the project root:

```bash
go mod init github.com/davidsilvasanmartin/playlists-go
```

Change the module path to match your own GitHub handle if needed. The rest of the tutorial uses `github.com/davidsilvasanmartin/playlists-go`.

---

## 1.2 Add dependencies

```bash
go get golang.org/x/time/rate       # rate limiter
go get github.com/joho/godotenv     # .env file loader for local development
go get github.com/stretchr/testify  # test assertions
```

After running these, your `go.mod` should look similar to:

```
module github.com/davidsilvasanmartin/playlists-go

go 1.22

require (
    golang.org/x/time v0.11.0
    github.com/joho/godotenv v1.5.1
    github.com/stretchr/testify v1.10.0
)
```

---

## 1.3 Create the directory structure

```bash
mkdir -p cmd/server
mkdir -p internal/musicbrainz
mkdir -p internal/search
mkdir -p internal/api
```

Final layout for this tutorial:

```
playlists/
├── cmd/
│   └── server/
│       └── main.go
├── internal/
│   ├── musicbrainz/
│   │   ├── client.go          ← Client interface
│   │   ├── client_impl.go     ← Implementation: rate limiter, retry, HTTP
│   │   └── types.go           ← Domain type + raw MB JSON structs
│   ├── search/
│   │   ├── handler.go         ← HTTP handler for /api/v1/songs/search
│   │   ├── service.go         ← Orchestration: calls MB client, maps results
│   │   └── types.go           ← DTOs (SearchResult, SearchResponse)
│   └── api/
│       └── router.go          ← Registers all routes on http.ServeMux
├── .development.env           ← committed safe defaults for local development
├── .gitignore                 ← ignores .env (personal overrides)
├── Makefile
├── docker-compose.yml
└── go.mod
```

---

## 1.4 Configuration

### The variables

All environment variables are prefixed with `PLAYLISTS_` to avoid collisions with system or platform variables. `PORT`, for example, is already claimed by hosting platforms like Heroku and Railway.

| Variable                    | Required | Default                        | Description                                                     |
|-----------------------------|----------|--------------------------------|-----------------------------------------------------------------|
| `PLAYLISTS_PORT`            | No       | `8080`                         | HTTP listen port                                                |
| `PLAYLISTS_MB_BASE_URL`     | No       | `https://musicbrainz.org`      | MusicBrainz base URL (overridable for tests)                    |
| `PLAYLISTS_MB_USER_AGENT`   | Yes      | —                              | User-Agent sent to MusicBrainz (their API policy requires this) |

MusicBrainz requires a descriptive `User-Agent` so they can identify and contact you if your client misbehaves. Use the format `appname/version ( contact@example.com )`.

---

### How configuration works

The server reads all config from **environment variables** using `os.Getenv`. This is the natural format for Docker: when you run a container, Docker injects env vars into the process directly — the Go code sees them the same way it would see any other env var.

There is no YAML config file, and we are not using Viper. Here is why:

**Why not a YAML config file?** YAML is useful when you have dozens of nested configuration options (like a Kubernetes controller). For 3–4 flat string values, YAML adds unnecessary complexity: you would need to ship the file alongside the binary, mount it into Docker, and parse it. Env vars are simpler and are the format Docker and Docker Compose are already designed around.

**Why not Viper?** [Viper](https://github.com/spf13/viper) is a popular Go config library that reads YAML, TOML, JSON, env vars, and remote sources. It is designed for large applications with multiple config sources and override hierarchies. For this project it would add around 10 transitive dependencies for functionality that `os.Getenv` already handles in 5 lines.

---

### Two-file approach for local development

For local development you need the variables set before the server starts. We use `godotenv` — a small Go library (no transitive dependencies) that reads `.env` files at the start of `main()` and loads their contents into the process environment. In Docker, where env vars are injected by the container runtime, neither file will exist and `godotenv` silently does nothing.

There are two files, each with a distinct role:

**`.development.env`** — committed to the repository. Contains safe defaults that work out of the box for any developer who has just cloned the repo. No secrets here. The `PLAYLISTS_MB_USER_AGENT` placeholder is functional (MusicBrainz does not validate the email address).

```bash
# .development.env — committed, safe defaults for local development
PLAYLISTS_PORT=8080
PLAYLISTS_MB_BASE_URL=https://musicbrainz.org
PLAYLISTS_MB_USER_AGENT=playlists/0.0.1 ( dev@localhost )
```

**`.env`** — gitignored. Personal overrides. A developer creates this file only when they want to change something — for example, to put their real email address in the user agent, or to run the server on a different port.

```bash
# .env — gitignored, personal overrides only
PLAYLISTS_MB_USER_AGENT=playlists/0.0.1 ( alice@example.com )
```

This file only needs to contain the variables you actually want to override. Any variable absent from `.env` falls back to the value from `.development.env`.

Add `.env` to your `.gitignore`:

```
.env
```

> **Important format note:** there is no `export` keyword in either file. The `export` prefix is a bash shell concept used when sourcing a file into the current shell session. The `godotenv` library — and Docker Compose — both expect the plain `KEY=VALUE` format.

In `main.go`, load both files at the very top of `main()`:

```go
// Load committed defaults first, then let .env override personal values.
// Both files are optional — errors are silently discarded.
// In Docker neither file exists; env vars are injected by the container runtime.
_ = godotenv.Load(".development.env")
_ = godotenv.Overload(".env")
```

Two functions are used here intentionally:

- `godotenv.Load` sets a variable **only if it is not already set** in the environment. This is why `.development.env` is loaded first — it fills in defaults without stomping on anything.
- `godotenv.Overload` sets a variable **even if it is already set**. This is why `.env` is loaded second — it lets personal overrides win over the committed defaults.

Both return an error if the file does not exist, and both errors are discarded with `_ =`. This is deliberate: a missing file is expected (in Docker, in CI, when a developer has not created their personal `.env`), not a problem.

---

### Alternative: inline env vars with `go run`

You can also pass variables directly on the command line, which bypasses both files entirely:

```bash
PLAYLISTS_MB_USER_AGENT="playlists/0.0.1 ( your@email.com )" go run ./cmd/server
```

Prefix the `go run` command with `KEY=VALUE` pairs. This sets the variables only for that one process — your shell session is unaffected. `godotenv.Overload(".env")` will not override variables already present in the environment, so an inline value always wins over both env files.

---

### In Docker / Docker Compose

When users self-host this app with Docker Compose, they set variables in `docker-compose.yml` directly:

```yaml
services:
  api:
    image: playlists-api
    environment:
      - PLAYLISTS_PORT=8080
      - PLAYLISTS_MB_BASE_URL=https://musicbrainz.org
      - PLAYLISTS_MB_USER_AGENT=playlists/0.0.1 ( me@example.com )
```

Or they keep a `.env` file alongside their `docker-compose.yml` and reference it:

```yaml
services:
  api:
    image: playlists-api
    env_file:
      - .env
```

Docker Compose reads that `.env` file natively (same `KEY=VALUE` format) and injects the variables into the container. The Go binary never sees the file — it just sees the variables already present in its environment via `os.Getenv`. Neither `.development.env` nor `.env` is present inside the container image.

The configuration story for self-hosted users is: edit `.env` (or the `environment:` block in `docker-compose.yml`), then `docker compose up`. No Go tooling required.

---

Continue to [Part 2 — MusicBrainz Client](02-musicbrainz-client.md).
