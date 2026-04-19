.PHONY: run build test testint vet tidy help
VERSION ?= $(shell git describe --tags --always --dirty)

BINARY := bin/server

## run: start the development server (requires MB_USER_AGENT env var)
run:
	go run ./cmd/server

## build: compile the server binary to bin/server
build:
	@mkdir -p bin
	go build -o $(BINARY) ./cmd/server

## test: run unit tests
test:
	go test ./...

## teste2e: run e2e tests (no Docker required)
teste2e:
	go test -tags=e2e -v -count=1 -timeout=120s ./e2e/...

## vet: run go vet
vet:
	go vet ./...

## tidy: tidy and verify go.mod
tidy:
	go mod tidy
	go mod verify

## help: list available targets
help:
	@grep -E '^##' $(MAKEFILE_LIST) | sed 's/^## //'
