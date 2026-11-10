#!/usr/bin/env bash
# Runs the whole backend locally: notification, message, club,
# federation, user, post, media, auth and gateway, each from its own directory with its own
# .env. Output is merged and labelled by service; Ctrl+C stops everything
# gracefully. If any service dies, the others are stopped too.
#
#   scripts/dev.sh               storage from the .env files (inmem by default)
#   scripts/dev.sh --persistent  PostgreSQL and Redis from compose.yaml
#   scripts/dev.sh --rooms       add rooms, local LiveKit and development mTLS
#
# Written for the bash 3.2 that ships with macOS.
set -euo pipefail

cd "$(dirname "$0")/.."

services=(notification message club federation user post media auth gateway)
colors=(31 37 91 92 36 95 35 34 33 32)

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
rooms=false
for arg in "$@"; do
	case "$arg" in
	--persistent) persistent=true ;;
	--rooms) rooms=true ;;
	-h | --help) usage ;;
	*) die "unknown argument $arg (see --help)" ;;
	esac
done

if $rooms; then
	services=(notification message club federation room user post media auth gateway)
	[ -f room/.env ] || cp room/.env.example room/.env
fi

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

if $rooms; then
	if [ ! -f certs/ca.pem ] || [ ! -f certs/jwt-private.pem ]; then
		make certs
	else
		deploy/certs.sh certs .data/node/certs.prom
	fi
	export TLS_CA_PATH="$PWD/certs/ca.pem"
	export JWT_PUBLIC_KEY_PATH="$PWD/certs/jwt-public.pem"
	export JWT_PRIVATE_KEY_PATH="$PWD/certs/jwt-private.pem"
	export ROOM_SERVICE_ADDR=localhost:7059
	export LIVEKIT_URL=http://localhost:7880
	export LIVEKIT_PUBLIC_URL=ws://localhost:8080
	# Only the development signalling listener is published; admin JWTs stay local.
	docker rm -f freenet-dev-livekit >/dev/null 2>&1 || true
	docker run -d --name freenet-dev-livekit -p 127.0.0.1:7880:7880 -p 127.0.0.1:7881:7881 -p 127.0.0.1:7882:7882/udp livekit/livekit-server:v1.13.11 --dev --bind 0.0.0.0 --node-ip 127.0.0.1 --udp-port 7882 >/dev/null
	export LIVEKIT_API_KEY=devkey
	export LIVEKIT_API_SECRET=secret
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

	$rooms && docker stop freenet-dev-livekit >/dev/null 2>&1 || true
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
	(cd "$svc"
		if $rooms; then
			export TLS_CERT_PATH="$OLDPWD/certs/$svc.pem" TLS_KEY_PATH="$OLDPWD/certs/$svc-key.pem"
		fi
		METRICS_PORT=$metrics_port exec "$bin/$svc") > >(label "$tag ") 2>&1 &
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
