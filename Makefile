.PHONY: run build test testint vet tidy help

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

## testint: run unit + integration tests (no Docker required)
testint:
	go test -tags=integration -v -count=1 ./...

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
