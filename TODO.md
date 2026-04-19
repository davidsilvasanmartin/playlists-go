- pgx as DB driver; sqlc to write Go code based on queries; Goose for migrations.
- Perhaps look into tools for validation such as https://github.com/go-playground/validator
- I probably want to separate APP project from E2E project because for E2E images,
Docker is losing its cache every time I modify ANY FILE (because of the `COPY . .`)
