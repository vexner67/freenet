#!/usr/bin/env bash
# Smoke test of the Docker stack (`make stack`): signs a new user up
# through the gateway and goes over the services, each behind mTLS and on
# its database: auth (code from its log), user (profile), media (a photo,
# written to its volume and served back), post (a post with it, a repost,
# a poll, a note and the faves), notification (unread count), message (dialogs and chats), club
# (a club and a post for it), the feed with an ignored source and the website
# itself; then observability: every
# container healthy, a request ID from the gateway in the auth log, every
# service's metrics in Prometheus, its alert rules loaded and sent to
# Alertmanager, the Grafana dashboard.
#
#   scripts/smoke.sh [base URL]   default http://localhost:8080
#
# PROMETHEUS_URL, ALERTMANAGER_URL and GRAFANA_URL are the stack's, default
# http://localhost:9090, :9093 and :3001.
#
# Needs curl and jq.
set -euo pipefail

cd "$(dirname "$0")/.."

base=${1:-http://localhost:8080}
prometheus=${PROMETHEUS_URL:-http://localhost:9090}
alertmanager=${ALERTMANAGER_URL:-http://localhost:9093}
grafana=${GRAFANA_URL:-http://localhost:3001}
services=(gateway auth user post media notification message club)
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

call PATCH "/api/v1/posts/$post" '{"text":"smoke test, edited"}' | jq -e '.editedAt != null and .photos == []' >/dev/null
expect=204 call PUT "/api/v1/posts/$post/pin" >/dev/null
call GET "/api/v1/users/$nickname/posts" | jq -e ".posts[0].id == \"$post\" and .posts[0].pinned" >/dev/null
step "post: edited and pinned first on the wall"

call GET /api/v1/notifications/unread-count | jq -e '.count == 0' >/dev/null
step "notification: unread count"

call GET /api/v1/dialogs | jq -e '.dialogs | length == 0' >/dev/null
step "message: dialogs"

# A chat takes two other people, and the smoke test has one user: it goes
# as far as the services that would make the chat, the user service naming
# the people and the message service keeping the chats.
call GET /api/v1/me | jq -e '.privacy.chatsInvite == "following"' >/dev/null
expect=404 call POST /api/v1/chats '{"title":"Smoke chat","members":["nobody1","nobody2"]}' | jq -e '.error.message == "user not found"' >/dev/null
expect=404 call GET /api/v1/chats/01900000-0000-7000-8000-000000000000 | jq -e '.error.message == "chat not found"' >/dev/null
step "message: chats of several people"

club=$(expect=201 call POST /api/v1/clubs '{"name":"Smoke club","description":"smoke test"}' | jq -r .id)
expect=201 call POST "/api/v1/clubs/$club/posts" '{"text":"club news","fromClub":true}' | jq -e '.fromWall and .author.club' >/dev/null
call GET "/api/v1/clubs/$club/posts" | jq -e '.total == 1 and .canPost' >/dev/null
call GET /api/v1/feed | jq -e '[.posts[] | select(.fromWall)] | length == 1' >/dev/null
step "club: a club, a post for it on its wall and in the feed"

poll=$(expect=201 call POST /api/v1/me/posts '{"poll":{"title":"Smoke?","options":["yes","no"]}}' | jq -r 'select(.poll.options | length == 2) | .id')
call PUT "/api/v1/posts/$poll/poll/vote" '{"options":[1]}' | jq -e '.voted == [1] and .voters == 1' >/dev/null
call GET "/api/v1/posts/$poll" | jq -e '.poll.options[1].votes == 1 and .poll.voted == [1]' >/dev/null
token='' call GET "/api/v1/posts/$poll/poll/voters?option=1" | jq -e '.total == 1' >/dev/null
step "post: a poll, a vote in it and who voted"

note=$(expect=201 call POST /api/v1/me/notes '{"title":"Smoke","text":"a note"}' | jq -r .id)
expect=201 call POST "/api/v1/notes/$note/comments" '{"text":"a comment"}' >/dev/null
token='' call GET "/api/v1/users/$nickname/notes" | jq -e '.total == 1 and .notes[0].comments == 1' >/dev/null
token='' call GET "/api/v1/notes/$note/comments" | jq -e '.total == 1 and .comments[0].text == "a comment"' >/dev/null
step "post: a note, a comment under it and the list of notes"

call PUT "/api/v1/posts/$poll/like" >/dev/null
call GET /api/v1/me/faves/posts | jq -e --arg id "$poll" '.total == 1 and .posts[0].id == $id and .posts[0].liked' >/dev/null
call GET /api/v1/me/posts | jq -e --arg id "$poll" '.total >= 1 and ([.posts[] | select(.id == $id and .mine)] | length == 1)' >/dev/null
step "post: a liked post among the faves and the posts one wrote"

expect=204 call PUT "/api/v1/me/feed/ignored/clubs/$club" >/dev/null
call GET /api/v1/me/feed/ignored | jq -e --arg id "$club" '.sources | length == 1 and .[0].id == $id and .[0].club' >/dev/null
call GET /api/v1/feed | jq -e '[.posts[] | select(.fromWall)] | length == 0' >/dev/null
call GET "/api/v1/feed?ignored=true" | jq -e '[.posts[] | select(.fromWall)] | length == 1' >/dev/null
expect=204 call DELETE "/api/v1/me/feed/ignored?id=$club" >/dev/null
call GET /api/v1/feed | jq -e '[.posts[] | select(.fromWall)] | length == 1' >/dev/null
step "user: an ignored source left out of the feed and brought back"

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
	up=$(curl -sf "$prometheus/api/v1/query?query=up%7Bjob%3D%22freenet%22%7D" | jq -r '[.data.result[] | select(.value[1] == "1") | .metric.service] | sort | join(" ")' || true)
	[ "$up" = "$(printf '%s\n' "${services[@]}" | sort | xargs)" ] && break
	sleep 1
done
[ "$up" = "$(printf '%s\n' "${services[@]}" | sort | xargs)" ] || die "Prometheus at $prometheus sees up: ${up:-nothing}"
step "metrics: Prometheus scrapes every service"

# The exporters are scraped every 30s. Every database with a service of
# its own, and Redis.
watched="auth club message notification post redis user"
stores=
for _ in $(seq 1 60); do
	stores=$(curl -sf "$prometheus/api/v1/query?query=pg_up%20or%20redis_up" |
		jq -r '[.data.result[] | select(.value[1] == "1") | .metric.db // "redis"] | sort | join(" ")' || true)
	[ "$stores" = "$watched" ] && break
	sleep 1
done
[ "$stores" = "$watched" ] || die "Prometheus at $prometheus sees the databases and Redis up: ${stores:-none}"
step "metrics: Prometheus watches every database and Redis"

# Rules that fail to evaluate (a typo in a metric name does not) say so in
# their health.
rules=$(curl -sf "$prometheus/api/v1/rules?type=alert") || die "no rules from Prometheus at $prometheus"
echo "$rules" | jq -e '[.data.groups[].rules[]] | length >= 11 and all(.health == "ok" or .health == "unknown")' >/dev/null \
	|| die "Prometheus alert rules are missing or broken: $(echo "$rules" | jq -c '[.data.groups[].rules[] | {name, health, lastError}]')"
curl -sf "$prometheus/api/v1/alertmanagers" | jq -e '.data.activeAlertmanagers | length == 1' >/dev/null \
	|| die "Prometheus has no Alertmanager"
curl -sf -o /dev/null "$alertmanager/-/ready" || die "Alertmanager at $alertmanager is not ready"
step "alerts: rules loaded, Alertmanager ready"

# Anonymous viewers see the dashboard, and its data source answers.
curl -sf "$grafana/api/dashboards/uid/freenet" | jq -e '.dashboard.panels | length > 0' >/dev/null \
	|| die "no Freenet dashboard in Grafana at $grafana"
# Grafana may ask before its connection to Prometheus is up: a few tries.
reached=
for _ in $(seq 1 15); do
	curl -sf "$grafana/api/datasources/uid/prometheus/health" | jq -e '.status == "OK"' >/dev/null && reached=1 && break
	sleep 1
done
[ -n "$reached" ] || die "Grafana does not reach Prometheus"
step "grafana: the dashboard and its data source"

echo "smoke test passed"
