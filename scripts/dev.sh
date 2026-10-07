#!/usr/bin/env bash
# Runs the whole backend locally: notification, message, user, post,
# media, auth and gateway, each from its own directory with its own .env. Output is
# merged and labelled by service; Ctrl+C stops everything gracefully. If
# any service dies, the others are stopped too.
#
#   scripts/dev.sh               storage from the .env files (inmem by default)
#   scripts/dev.sh --persistent  PostgreSQL and Redis from compose.yaml
#
# Written for the bash 3.2 that ships with macOS.
set -euo pipefail

cd "$(dirname "$0")/.."

services=(notification message user post media auth gateway)
colors=(31 37 36 35 34 33 32)

die() {
	echo "dev: $*" >&2
	exit 1
}

usage() {
	# The comment block at the top of this file.
	awk 'NR > 1 && /^#/ { sub(/^# ?/, ""); print; next } NR > 1 { exit }' "$0"
	exit 0
}

persistent=false
for arg in "$@"; do
	case "$arg" in
	--persistent) persistent=true ;;
	-h | --help) usage ;;
	*) die "unknown argument $arg (see --help)" ;;
	esac
done

# port_of prints GRPC_PORT or HTTP_PORT from a service's .env.
port_of() {
	sed -n -E 's/^(GRPC|HTTP)_PORT=([0-9]+).*/\2/p' "$1/.env" | head -1
}

# metrics_port_of prints METRICS_PORT from a service's .env or, without
# one, the service's port with a 9 in front of its last three digits:
# 7051 -> 9051, 8080 -> 9080.
metrics_port_of() {
	local port
	port=$(sed -n -E 's/^METRICS_PORT=([0-9]+).*/\1/p' "$1/.env" | head -1)
	[ -n "$port" ] || { port=$(port_of "$1"); port=9${port: -3}; }
	echo "$port"
}

for svc in "${services[@]}"; do
	[ -f "$svc/.env" ] || die "$svc/.env is missing: copy $svc/.env.example and fill it in"

	port=$(port_of "$svc")
	[ -n "$port" ] || die "$svc/.env has no GRPC_PORT/HTTP_PORT"

	for p in "$port" "$(metrics_port_of "$svc")"; do
		if nc -z localhost "$p" 2>/dev/null; then
			die "port $p of $svc is busy: is the backend already running?"
		fi
	done
done

if $persistent; then
	echo "==> starting PostgreSQL and Redis (docker compose)"
	docker compose up -d --wait
	# Environment wins over .env, so this switches all services at once:
	# data to the databases, rate limits to the Redis from compose.yaml.
	export STORAGE=persistent
	export REDIS_ADDR="${REDIS_ADDR:-localhost:$(sed -n 's/^REDIS_PORT=//p' .env)}"
fi

bin=$(mktemp -d)
pids=()

stop() {
	trap - INT TERM EXIT

	if [ ${#pids[@]} -gt 0 ]; then
		echo "==> stopping"
		kill -TERM "${pids[@]}" 2>/dev/null || true
		wait "${pids[@]}" 2>/dev/null || true
	fi

	rm -rf "$bin"
}

trap 'stop; exit 130' INT TERM
trap stop EXIT

echo "==> building ${services[*]}"
for svc in "${services[@]}"; do
	go build -o "$bin/$svc" "./$svc/cmd/$svc"
done

# label prefixes every line of a service. It ignores Ctrl+C and ends when
# the service closes its output, so shutdown logs are not lost and the
# service never writes into a closed pipe.
label() {
	trap '' INT
	awk -v p="$1" '{ print p $0; fflush() }'
}

for i in "${!services[@]}"; do
	svc=${services[$i]}
	tag=$(printf '%-12s|' "$svc")
	[ -t 1 ] && tag=$(printf '\033[%sm%s\033[0m' "${colors[$i]}" "$tag")

	metrics_port=$(metrics_port_of "$svc")
	(cd "$svc" && METRICS_PORT=$metrics_port exec "$bin/$svc") > >(label "$tag ") 2>&1 &
	pids+=($!)
done

http_port=$(port_of gateway)
for _ in $(seq 1 50); do
	nc -z localhost "$http_port" 2>/dev/null && break
	sleep 0.2
done

mode="storage from .env"
$persistent && mode="persistent storage"
echo "==> Freenet is up ($mode): http://localhost:$http_port  (Ctrl+C to stop)"
metrics=
for svc in "${services[@]}"; do
	metrics="$metrics $svc :$(metrics_port_of "$svc")"
done
echo "    metrics at /metrics:$metrics"

# Supervise: bash 3.2 has no `wait -n`, so poll.
while true; do
	for i in "${!pids[@]}"; do
		if ! kill -0 "${pids[$i]}" 2>/dev/null; then
			echo "dev: ${services[$i]} exited, stopping the rest" >&2
			exit 1
		fi
	done
	sleep 1
done
