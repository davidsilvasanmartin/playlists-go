# Dockerfile to run the app in a Production-like environment
# To update a base image: docker pull <image>:<tag> && docker inspect --format='{{index .RepoDigests 0}}' <image>:<tag>
FROM golang:1.26.2-trixie@sha256:c0074c718b473f3827043f86532c4c0ff537e3fe7a81b8219b0d1ccfcc2c9a09 AS builder
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

FROM debian:trixie-20260406-slim@sha256:4ffb3a1511099754cddc70eb1b12e50ffdb67619aa0ab6c13fcd800a78ef7c7a

COPY --from=builder /app/bin/server /server

ENTRYPOINT ["/server"]