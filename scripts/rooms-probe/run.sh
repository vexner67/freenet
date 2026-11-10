#!/usr/bin/env bash
# Run from the repository root. No production credentials or microphone access.
set -euo pipefail
cd "$(dirname "$0")/../.."
web=$(mktemp -d)
container="freenet-rooms-probe-$$"
cleanup() {
  docker stop "$container" >/dev/null 2>&1 || true
  rm -rf "$web"
}
trap cleanup EXIT
mkdir -p "$web/static/vendor"
cp scripts/rooms-probe/index.html "$web/index.html"
cp scripts/rooms-probe/probe.js "$web/static/probe.js"
cp web/static/vendor/livekit-client.mjs "$web/static/vendor/livekit-client.mjs"
docker run -d --rm --name "$container" \
  -p 127.0.0.1:7880:7880 -p 127.0.0.1:7881:7881 -p 127.0.0.1:7882:7882/udp \
  livekit/livekit-server:v1.13.11 --dev --bind 0.0.0.0 --node-ip 127.0.0.1 --udp-port 7882 >/dev/null
go run ./gateway/cmd/rooms-probe -web "$web"
