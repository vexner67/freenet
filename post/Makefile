MIGRATIONS_DIR := migrations

ifneq (,$(wildcard .env))
	include .env
	export
endif

.PHONY: run build test fmt lint tidy proto check migrate-create migrate-up migrate-down migrate-status

run:
	go run ./cmd/post

build:
	go build ./cmd/post

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
		--go_opt=module=github.com/vexner67/freenet/post \
		--go-grpc_out=. \
		--go-grpc_opt=module=github.com/vexner67/freenet/post \
		api/post/v1/post.proto

check:
	$(MAKE) fmt
	$(MAKE) lint
	$(MAKE) test

migrate-create:
	goose -dir $(MIGRATIONS_DIR) create $(name) sql

migrate-up:
	goose -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)" up

migrate-down:
	goose -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)" down

migrate-status:
	goose -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)" status
