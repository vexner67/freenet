ifneq (,$(wildcard .env))
	include .env
	export
endif

.PHONY: run build test fmt lint tidy check

run:
	go run ./cmd/gateway

build:
	go build ./cmd/gateway

test:
	go test ./...

fmt:
	go fmt ./...

lint:
	golangci-lint run

tidy:
	go mod tidy

check:
	$(MAKE) fmt
	$(MAKE) lint
	$(MAKE) test
