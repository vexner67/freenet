#!/usr/bin/env bash
# Smoke test of the Docker stack (`make stack`): signs a new user up
# through the gateway and goes over the services, each behind mTLS and on
# its database: auth (code from its log), user (profile), media (a photo,
# written to its volume and served back), post (a post with it and a
# repost), notification (unread count), message (dialogs) and the website
# itself.
#
#   scripts/smoke.sh [base URL]   default http://localhost:8080
#
# Needs curl and jq.
set -euo pipefail

cd "$(dirname "$0")/.."

base=${1:-http://localhost:8080}
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

expect=204 call POST /api/v1/auth/code "{\"email\":\"$email\"}" >/dev/null
code=
for _ in $(seq 1 20); do
	code=$(docker compose logs auth 2>/dev/null | grep -F "\"email\":\"$email\"" | grep -o '"code":"[0-9]*"' | tail -1 | grep -o '[0-9]\+' || true)
	[ -n "$code" ] && break
	sleep 0.5
done
[ -n "$code" ] || die "no sign-in code for $email in the auth log"
token=$(call POST /api/v1/auth/verify "{\"email\":\"$email\",\"code\":\"$code\"}" | jq -r .accessToken)
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

echo "smoke test passed"
