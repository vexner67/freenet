ifneq (,$(wildcard .env))
	include .env
	export
endif

.PHONY: run build test fmt lint tidy proto check

run:
	go run ./cmd/media

build:
	go build ./cmd/media

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
		--go_opt=module=github.com/vexner67/freenet/media \
		--go-grpc_out=. \
		--go-grpc_opt=module=github.com/vexner67/freenet/media \
		api/media/v1/media.proto

check:
	$(MAKE) fmt
	$(MAKE) lint
	$(MAKE) test

