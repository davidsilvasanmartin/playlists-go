# Dockerfile to run the app in a Production-like environment
FROM golang:1.26-trixie AS builder
LABEL authors="github.com/davidsilvasanmartin"
# The app's version. Use as `docker build --build-arg VERSION=1.4.2 -t playlists:1.4.2`
ARG VERSION=dev

WORKDIR /app

COPY go.mod go.sum /
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux \
    go build \
    -ldflags="-X main.version=${VERSION}"\
    -o /app/bin/server ./cmd/server

FROM debian:trixie-slim

COPY --from=builder /app/bin/server /server

ENTRYPOINT ["/server"]