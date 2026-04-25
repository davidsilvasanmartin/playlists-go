.PHONY: run test e2e vet tidy help
VERSION ?= $(shell git describe --tags --always --dirty)

## run: start the development server (requires MB_USER_AGENT env var)
run:
	go run ./cmd/server

## test: run unit tests
test:
	go test ./...

## e2e: run e2e tests
e2e:
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
