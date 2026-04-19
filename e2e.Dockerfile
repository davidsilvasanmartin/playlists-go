# Dockerfile for starting the app and running the E2E tests against it.
# I decided to delete my Production Dockerfile and create this one instead.
# The Production Dockerfile was a 2-stage Dockerfile. The app was built using the
# Go image and the binary was copied into a different slim image, from which
# the app was run. However, I was having an issue locally: the base, builder image
# was being created anew every time the E2E tests were run. A new image of almost
# 2GB in size was left in my local system. I would have run out of space if I
# would have kept doing that.
#
# NOTE: IF THIS FILE IS MODIFIED, then run `docker rmi playlists-e2e:latest`
FROM golang:1.26.2-trixie AS builder
LABEL authors="github.com/davidsilvasanmartin"
# The app's version. Use as `docker build --build-arg VERSION=1.4.2 -t playlists:1.4.2`
ARG VERSION=dev

WORKDIR /app

COPY . .

RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux \
    go build \
    -ldflags="-X main.version=${VERSION}"\
    -o /app/bin/server ./cmd/server

ENTRYPOINT ["/app/bin/server"]