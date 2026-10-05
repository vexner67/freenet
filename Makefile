.PHONY: dev dev-persistent up ps down certs web-test

# The whole backend and the website in one terminal; Ctrl+C stops it.
dev:
	./scripts/dev.sh

# The same on PostgreSQL and Redis from compose.yaml (started first).
dev-persistent:
	./scripts/dev.sh --persistent

up:
	docker compose up -d

ps:
	docker compose ps

down:
	docker compose down

# Development CA and mTLS certificates for auth, user, post and gateway.
certs:
	./scripts/gen-certs.sh certs

# Unit tests of the website scripts (needs Node.js).
web-test:
	node --test web/test/*.test.js
