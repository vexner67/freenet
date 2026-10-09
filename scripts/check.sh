#!/usr/bin/env bash
# Checks the whole repository as CI does: every Go module of go.work gets
# gofmt, go vet, golangci-lint and go test -race; then the website tests.
# All modules are checked even after a failure, so one run shows them all.
#
#   scripts/check.sh                 unit tests
#   scripts/check.sh --integration   also the PostgreSQL and Redis tests,
#                                    against throwaway containers: each
#                                    service gets its own migrated database,
#                                    shared packages that need PostgreSQL
#                                    an empty one; and the monitoring of the
#                                    stack: Prometheus and Alertmanager
#                                    configs and the alert rule tests, with
#                                    the tools of their compose.yaml images
#
# Needs Go, golangci-lint and Node.js; --integration also Docker.
set -euo pipefail

cd "$(dirname "$0")/.."

# The goose of the services' go.mod, for the integration databases.
goose=github.com/pressly/goose/v3/cmd/goose@v3.28.0

die() {
	echo "check: $*" >&2
	exit 1
}

usage() {
	awk 'NR > 1 && /^#/ { sub(/^# ?/, ""); print; next } NR > 1 { exit }' "$0"
	exit 0
}

integration=false
for arg in "$@"; do
	case "$arg" in
	--integration) integration=true ;;
	-h | --help) usage ;;
	*) die "unknown argument $arg (see --help)" ;;
	esac
done

command -v golangci-lint >/dev/null || die "golangci-lint is not installed: https://golangci-lint.run/docs/welcome/install/"
command -v node >/dev/null || die "Node.js is not installed (website tests)"

pg=
redis=

cleanup() {
	local containers=()
	[ -n "$pg" ] && containers+=("$pg")
	[ -n "$redis" ] && containers+=("$redis")

	if [ ${#containers[@]} -gt 0 ]; then
		docker rm -f "${containers[@]}" >/dev/null 2>&1 || true
	fi
}
trap cleanup EXIT

# host_port prints the host port Docker published for a container port.
host_port() {
	docker port "$1" "$2" | head -1 | sed 's/.*://'
}

start_databases() {
	echo "==> starting throwaway PostgreSQL and Redis"
	pg=$(docker run -d --rm -e POSTGRES_PASSWORD=check -p 127.0.0.1::5432 postgres:18)
	redis=$(docker run -d --rm -p 127.0.0.1::6379 redis:7-alpine)

	# Over TCP: while the image initialises the database, its temporary
	# server listens on the socket only.
	for _ in $(seq 1 60); do
		docker exec "$pg" pg_isready -q -h 127.0.0.1 -U postgres && break
		sleep 1
	done
	docker exec "$pg" pg_isready -q -h 127.0.0.1 -U postgres || die "PostgreSQL did not start"

	TEST_REDIS_ADDR="127.0.0.1:$(host_port "$redis" 6379/tcp)"
	export TEST_REDIS_ADDR
}

# database_for creates a fresh database for module $1, migrated if the
# module has migrations, and prints its URL.
database_for() {
	local module=$1 name=${1//\//_} url
	url="postgres://postgres:check@127.0.0.1:$(host_port "$pg" 5432/tcp)/$name?sslmode=disable"

	docker exec "$pg" psql -q -U postgres -c "CREATE DATABASE \"$name\"" >/dev/null

	[ -d "$module/migrations" ] || { echo "$url"; return 0; }

	local out
	if ! out=$(go run "$goose" -dir "$module/migrations" postgres "$url" up 2>&1); then
		echo "$out" >&2
		return 1
	fi

	echo "$url"
}

# check_module checks one module. It runs as "check_module || ...", where
# bash ignores set -e, so every step returns on failure by hand.
check_module() {
	local module=$1

	local unformatted
	unformatted=$(gofmt -l "$module") || return 1
	if [ -n "$unformatted" ]; then
		echo "not gofmt-ed:"
		echo "$unformatted"
		return 1
	fi

	(cd "$module" && go vet ./... && golangci-lint run ./...) || return 1

	local db=
	# Services test against their migrated database, shared packages that
	# need PostgreSQL against an empty one.
	if $integration && { [ -d "$module/migrations" ] || grep -rqs --include='*_test.go' TEST_DATABASE_URL "$module"; }; then
		db=$(database_for "$module") || return 1
	fi

	(cd "$module" && TEST_DATABASE_URL=$db go test -race -count=1 ./...)
}

$integration && start_databases

modules=()
while read -r module; do
	modules+=("$module")
done < <(sed -n 's|^[[:space:]]*\./||p' go.work)

failed=()
for module in "${modules[@]}"; do
	echo "==> $module"
	check_module "$module" || failed+=("$module")
done

echo "==> web"
node --test web/test/*.test.js >/dev/null || failed+=(web)

# check_server_alertmanager renders the server's Alertmanager configuration
# with a made-up Telegram chat and checks it as the server would.
check_server_alertmanager() {
	local dir
	dir=$(mktemp -d)
	printf 'TELEGRAM_BOT_TOKEN=123456:test-token_x\nTELEGRAM_CHAT_ID=-100123\n' > "$dir/env"

	deploy/alertmanager.sh "$dir/env" "$dir/alertmanager.yml" >/dev/null &&
		grep -q 'chat_id: -100123' "$dir/alertmanager.yml" &&
		docker run --rm -v "$dir:/m:ro" --entrypoint amtool "$1" check-config /m/alertmanager.yml
	local status=$?

	rm -rf "$dir"
	return $status
}

# check_monitoring checks the configs with the promtool and amtool of the
# very images the stack runs, and runs the tests of the alert rules.
check_monitoring() {
	local prometheus alertmanager
	prometheus=$(sed -n 's|^ *image: \(prom/prometheus:.*\)$|\1|p' compose.yaml)
	alertmanager=$(sed -n 's|^ *image: \(prom/alertmanager:.*\)$|\1|p' compose.yaml)

	docker run --rm -v "$PWD/monitoring:/m:ro" -w /m --entrypoint promtool "$prometheus" check config prometheus.yml &&
		docker run --rm -v "$PWD/monitoring:/m:ro" -w /m --entrypoint promtool "$prometheus" test rules alerts_test.yml &&
		docker run --rm -v "$PWD/monitoring:/m:ro" --entrypoint amtool "$alertmanager" check-config /m/alertmanager.yml &&
		check_server_alertmanager "$alertmanager" &&
		node -e 'JSON.parse(require("fs").readFileSync(process.argv[1]))' monitoring/grafana/dashboards/freenet.json
}

if $integration; then
	echo "==> monitoring"
	# Quiet unless it fails, as the tests of the modules are.
	if ! out=$(check_monitoring 2>&1); then
		echo "$out"
		failed+=(monitoring)
	fi
fi

if [ ${#failed[@]} -gt 0 ]; then
	die "failed: ${failed[*]}"
fi

echo "==> all checks passed"
