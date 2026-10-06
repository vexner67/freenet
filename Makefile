.PHONY: dev dev-persistent up ps down certs web-test check check-integration stack smoke

# The whole backend and the website in one terminal; Ctrl+C stops it.
dev:
	./scripts/dev.sh

# The same on PostgreSQL and Redis from compose.yaml (started first).
dev-persistent:
	./scripts/dev.sh --persistent

up:
	docker compose up -d

ps:
	docker compose --profile app ps

# Stops everything compose started, the Docker stack included; data stays.
down:
	docker compose --profile app down

# The whole site in Docker: the services built from Dockerfile, with mTLS
# between them, on the databases of dev-persistent; http://localhost:8080
# (GATEWAY_PORT). They run as you, to read certs/ and write .data/media.
stack:
	@test -f certs/ca.pem -a -f certs/jwt-private.pem || { echo 'stack: no certs/ with JWT keys, run "make certs"'; exit 1; }
	@grep -q '^HASH_SECRET=.' .env || { echo 'stack: set HASH_SECRET in .env, e.g. to the output of "openssl rand -hex 32"'; exit 1; }
	mkdir -p .data/media
	FREENET_UID=$$(id -u) FREENET_GID=$$(id -g) docker compose --profile app up -d --build --wait

# Signs a user up through the running stack and touches every service.
smoke:
	./scripts/smoke.sh

# Every module as CI checks it: gofmt, vet, golangci-lint, go test -race,
# and the website tests; check-integration also runs the PostgreSQL and
# Redis tests against throwaway containers.
check:
	./scripts/check.sh

check-integration:
	./scripts/check.sh --integration

# Development CA, mTLS certificates for every service and the JWT keys.
certs:
	./scripts/gen-certs.sh certs

# Unit tests of the website scripts (needs Node.js).
web-test:
	node --test web/test/*.test.js
