#!/usr/bin/env bash
# Smoke test of the Docker stack (`make stack`): signs a new user up
# through the gateway and goes over the services, each behind mTLS and on
# its database: auth (code from its log), user (profile), media (a photo,
# written to its volume and served back), post (a post with it and a
# repost), notification (unread count), message (dialogs) and the website
# itself; then observability: every container healthy, a request ID from
# the gateway in the auth log, every service's metrics in Prometheus.
#
#   scripts/smoke.sh [base URL]   default http://localhost:8080
#
# PROMETHEUS_URL is the stack's Prometheus, default http://localhost:9090.
#
# Needs curl and jq.
set -euo pipefail

cd "$(dirname "$0")/.."

base=${1:-http://localhost:8080}
prometheus=${PROMETHEUS_URL:-http://localhost:9090}
services=(gateway auth user post media notification message)
email="smoke-$(date +%s)-$RANDOM@example.com"
nickname="smoke$RANDOM$RANDOM"

die() {
	echo "smoke: $*" >&2
	exit 1
}

# call METHOD PATH [JSON] prints the response body and checks the status
# against $expect (200 unless set).
call() {
	local args=(-sS -X "$1" -o /tmp/smoke.$$ -w '%{http_code}' -H 'Content-Type: application/json')
	[ -n "${token:-}" ] && args+=(-H "Authorization: Bearer $token")
	[ $# -ge 3 ] && args+=(-d "$3")

	local status
	status=$(curl "${args[@]}" "$base$2")
	[ "$status" = "${expect:-200}" ] || die "$1 $2: status $status, want ${expect:-200}: $(cat /tmp/smoke.$$)"
	cat /tmp/smoke.$$
	rm -f /tmp/smoke.$$
}

step() {
	echo "ok   $*"
}

for _ in $(seq 1 60); do
	curl -sf "$base/healthz" >/dev/null 2>&1 && break
	sleep 1
done
curl -sf "$base/healthz" >/dev/null || die "the gateway at $base is not up"
step "gateway is up"

curl -sf "$base/" | grep -q '<title>' || die "the website is not served"
step "website is served"

expect=404 call GET "/api/v1/users/$nickname" >/dev/null
step "user: unknown nickname is 404"

request_id=$(curl -sSf -o /dev/null -D - -X POST -H 'Content-Type: application/json' -d "{\"email\":\"$email\"}" \
	"$base/api/v1/auth/code" | tr -d '\r' | sed -n 's/^[Xx]-[Rr]equest-[Ii][Dd]: //p')
[ -n "$request_id" ] || die "POST /api/v1/auth/code: no X-Request-ID in the response"
line=
for _ in $(seq 1 20); do
	line=$(docker compose logs auth 2>/dev/null | grep -F "\"email\":\"$email\"" | tail -1 || true)
	[ -n "$line" ] && break
	sleep 0.5
done
[ -n "$line" ] || die "no sign-in code for $email in the auth log"
code=$(echo "$line" | grep -o '"code":"[0-9]*"' | grep -o '[0-9]\+')
echo "$line" | grep -qF "\"request_id\":\"$request_id\"" || die "the auth log line has no request_id $request_id: $line"
step "request ID: from the gateway to the auth log"
tokens=$(call POST /api/v1/auth/verify "{\"email\":\"$email\",\"code\":\"$code\"}")
token=$(echo "$tokens" | jq -r .accessToken)
step "auth: signed in with the code from the log"

call PUT /api/v1/me/profile "{\"name\":\"Smoke\",\"nickname\":\"$nickname\"}" | jq -e '.profileCompleted' >/dev/null
step "user: profile filled in"

# A 64x64 PNG of one colour.
photo_png=iVBORw0KGgoAAAANSUhEUgAAAEAAAABACAIAAAAlC+aJAAAAT0lEQVR42u3PQQkAAAgEsCtnF9OY2Qi+hcEKLNXzWgQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQELgt2A7EeCg5yYwAAAABJRU5ErkJggg==
photo=$(echo "$photo_png" | base64 -d | curl -sSf -X POST -H "Authorization: Bearer $token" -H 'Content-Type: image/png' \
	--data-binary @- "$base/api/v1/me/photos") || die "photo upload failed"
curl -sSf -o /dev/null "$base$(echo "$photo" | jq -r .small)" || die "the photo is not served"
step "media: photo stored and served"

post=$(expect=201 call POST /api/v1/me/posts "{\"text\":\"smoke test\",\"photos\":[$(echo "$photo" | jq .id)]}" | jq -r .id)
expect=201 call POST "/api/v1/posts/$post/reposts" '{"text":""}' | jq -e ".repost.id == \"$post\"" >/dev/null
call GET "/api/v1/users/$nickname/posts" | jq -e '.total == 2' >/dev/null
step "post: a post, its repost and the wall"

call GET /api/v1/notifications/unread-count | jq -e '.count == 0' >/dev/null
step "notification: unread count"

call GET /api/v1/dialogs | jq -e '.dialogs | length == 0' >/dev/null
step "message: dialogs"

# refresh replaces the tokens, as the website does after deleting and
# restoring the page.
refresh() {
	tokens=$(token='' call POST /api/v1/auth/refresh "{\"refreshToken\":$(echo "$tokens" | jq .refreshToken)}")
	token=$(echo "$tokens" | jq -r .accessToken)
}

call POST /api/v1/me/deactivate '{"reason":"smoke test"}' | jq -e '.deactivated and .purgeAt != null' >/dev/null
refresh
expect=403 call PUT /api/v1/me/profile "{\"name\":\"Smoke\",\"nickname\":\"$nickname\"}" >/dev/null
token='' call GET "/api/v1/users/$nickname" | jq -e '.deactivated' >/dev/null
call POST /api/v1/me/reactivate | jq -e '.deactivated == false' >/dev/null
refresh
call PUT /api/v1/me/profile "{\"name\":\"Smoke\",\"nickname\":\"$nickname\"}" >/dev/null
step "user: page deleted (its token restricted) and restored"

for svc in "${services[@]}"; do
	health=$(docker inspect -f '{{.State.Health.Status}}' "$(docker compose ps -q "$svc")" 2>/dev/null || true)
	[ "$health" = healthy ] || die "$svc is ${health:-not running}, want healthy"
done
step "healthcheck: every service is healthy"

# Prometheus scrapes every 10s: the first round may be on its way.
up=
for _ in $(seq 1 30); do
	up=$(curl -sf "$prometheus/api/v1/query?query=up" | jq -r '[.data.result[] | select(.value[1] == "1") | .metric.service] | sort | join(" ")' || true)
	[ "$up" = "$(printf '%s\n' "${services[@]}" | sort | xargs)" ] && break
	sleep 1
done
[ "$up" = "$(printf '%s\n' "${services[@]}" | sort | xargs)" ] || die "Prometheus at $prometheus sees up: ${up:-nothing}"
step "metrics: Prometheus scrapes every service"

echo "smoke test passed"
