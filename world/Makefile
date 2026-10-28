MIGRATIONS_DIR := migrations

ifneq (,$(wildcard .env))
	include .env
	export
endif

.PHONY: run build test fmt lint tidy proto check map migrate-create migrate-up migrate-down migrate-status

# The prototype of the map: the physical map and the provinces of a seed.
SEED ?= 42
OUT ?= /tmp

run:
	go run ./cmd/world

build:
	go build ./cmd/world

test:
	go test ./...

fmt:
	go fmt ./...

lint:
	golangci-lint run

tidy:
	go mod tidy

proto:
	protoc \
		--proto_path=. \
		--go_out=. \
		--go_opt=module=github.com/vexner67/freenet/world \
		--go-grpc_out=. \
		--go-grpc_opt=module=github.com/vexner67/freenet/world \
		api/world/v1/world.proto

check:
	$(MAKE) fmt
	$(MAKE) lint
	$(MAKE) test

map:
	go run ./cmd/mapgen -seed $(SEED) -out $(OUT)/map-$(SEED).png -provinces $(OUT)/provinces-$(SEED).png

migrate-create:
	goose -dir $(MIGRATIONS_DIR) create $(name) sql

migrate-up:
	goose -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)" up

migrate-down:
	goose -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)" down

migrate-status:
	goose -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)" status
