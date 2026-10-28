.PHONY: build test fmt lint tidy check map

# The prototype of the map: the physical map and the provinces of a seed.
SEED ?= 42
OUT ?= /tmp

build:
	go build ./...

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

map:
	go run ./cmd/mapgen -seed $(SEED) -out $(OUT)/map-$(SEED).png -provinces $(OUT)/provinces-$(SEED).png
