#!/usr/bin/env bash
# Smoke test of the Docker stack (`make stack`): signs a new user up
# through the gateway and goes over the services, each behind mTLS and on
# its database: auth (code from its log), user (profile), media (a photo,
# written to its volume and served back), post (a post with it, a repost,
# a poll and the faves), notification (unread count), message (dialogs and chats), club
# (a club and a post for it), the feed with an ignored source, a second
# person whose follow and whose appointment in the club travel through the
# outboxes of user and club and who gets a sealed message, and the
# website itself; then observability: every
# container healthy, a request ID from the gateway in the auth log, that
# line in VictoriaLogs and the request as a trace in Jaeger, the machine
# and the certificates watched, every
# service's metrics in Prometheus, its alert rules loaded and sent to
# Alertmanager, the Grafana dashboard.
#
#   scripts/smoke.sh [base URL]   default http://localhost:8080
#
# PROMETHEUS_URL, ALERTMANAGER_URL, GRAFANA_URL, VICTORIALOGS_URL and
# JAEGER_URL are the stack's, default http://localhost:9090, :9093, :3001,
# :9428 and :16686.
#
# Needs curl and jq.
set -euo pipefail

cd "$(dirname "$0")/.."

base=${1:-http://localhost:8080}
prometheus=${PROMETHEUS_URL:-http://localhost:9090}
alertmanager=${ALERTMANAGER_URL:-http://localhost:9093}
grafana=${GRAFANA_URL:-http://localhost:3001}
logs=${VICTORIALOGS_URL:-http://localhost:9428}
jaeger=${JAEGER_URL:-http://localhost:16686}
services=(gateway auth user post media notification message club world)
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

# Not grep -q: it would stop reading at the title, and curl, still
# writing, would fail on the closed pipe under pipefail.
curl -sf "$base/" | grep '<title>' >/dev/null || die "the website is not served"
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

# Fluent Bit sends what the containers log every few seconds.
collected=
for _ in $(seq 1 60); do
	collected=$(curl -sfG "$logs/select/logsql/query" --data-urlencode "query=request_id:\"$request_id\" service:auth" | head -1 || true)
	[ -n "$collected" ] && break
	sleep 1
done
[ -n "$collected" ] || die "VictoriaLogs at $logs has no line of auth with request_id $request_id"
step "logs: the line of auth is in VictoriaLogs, found by its request ID"

# The same request as a trace: the span of the gateway carries the request
# ID, the spans of the call to auth are its children. Spans are sent in
# batches, every few seconds.
traced=
since=$(date -u -d '1 hour ago' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -v-1H +%Y-%m-%dT%H:%M:%SZ)
for _ in $(seq 1 60); do
	traced=$(curl -sfG "$jaeger/api/v3/traces" --data-urlencode query.service_name=gateway \
		--data-urlencode "query.attributes={\"request_id\":\"$request_id\"}" \
		--data-urlencode "query.start_time_min=$since" --data-urlencode "query.start_time_max=$(date -u +%Y-%m-%dT%H:%M:%SZ)" |
		jq -r '[.result.resourceSpans[]?.resource.attributes[] | select(.key == "service.name") | .value.stringValue] | unique | join(" ")' || true)
	[ "$traced" = "auth gateway" ] && break
	sleep 1
done
[ "$traced" = "auth gateway" ] || die "Jaeger at $jaeger has no trace of request $request_id through the gateway and auth: ${traced:-nothing}"
step "tracing: the request is one trace through the gateway and auth, found by its request ID"
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
# The photo is that post's: another one does not get it.
expect=400 call POST /api/v1/me/posts "{\"photos\":[$(echo "$photo" | jq .id)]}" | jq -e '.error.message == "photo not found"' >/dev/null
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
# The feed hears of the new club from club's outbox, a moment later.
in_feed=''
for _ in $(seq 1 20); do
	if call GET /api/v1/feed | jq -e '[.posts[] | select(.fromWall)] | length == 1' >/dev/null; then
		in_feed=yes
		break
	fi
	sleep 0.5
done
[ -n "$in_feed" ] || die "the post of the club is not in the feed of its founder"
step "club: a club, a post for it on its wall and in the feed"

poll=$(expect=201 call POST /api/v1/me/posts '{"poll":{"title":"Smoke?","options":["yes","no"]}}' | jq -r 'select(.poll.options | length == 2) | .id')
call PUT "/api/v1/posts/$poll/poll/vote" '{"options":[1]}' | jq -e '.voted == [1] and .voters == 1' >/dev/null
call GET "/api/v1/posts/$poll" | jq -e '.poll.options[1].votes == 1 and .poll.voted == [1]' >/dev/null
token='' call GET "/api/v1/posts/$poll/poll/voters?option=1" | jq -e '.total == 1' >/dev/null
step "post: a poll, a vote in it and who voted"

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

# A second person, for what takes two: what one does reaches the other
# through the outbox of the service it was done in, a moment later.
email2="smoke2-$(date +%s)-$RANDOM@example.com"
nickname2="smoke$RANDOM$RANDOM"
token='' expect=204 call POST /api/v1/auth/code "{\"email\":\"$email2\"}" >/dev/null
code2=
for _ in $(seq 1 20); do
	code2=$(docker compose logs auth 2>/dev/null | grep -F "\"email\":\"$email2\"" | tail -1 | grep -o '"code":"[0-9]*"' | grep -o '[0-9]\+' || true)
	[ -n "$code2" ] && break
	sleep 0.5
done
[ -n "$code2" ] || die "no sign-in code for $email2 in the auth log"
token2=$(token='' call POST /api/v1/auth/verify "{\"email\":\"$email2\",\"code\":\"$code2\"}" | jq -r .accessToken)
token=$token2 call PUT /api/v1/me/profile "{\"name\":\"Smoke Two\",\"nickname\":\"$nickname2\"}" >/dev/null

# eventually CHECK... retries a check for up to 10 seconds.
eventually() {
	for _ in $(seq 1 20); do
		"$@" 2>/dev/null && return 0
		sleep 0.5
	done
	return 1
}

token=$token2 call GET /api/v1/feed | jq -e '.posts | length == 0' >/dev/null
token=$token2 call POST "/api/v1/users/$nickname/follow" | jq -e '.isFollowing' >/dev/null
# The post service hears whom the second person reads, and the first one
# hears of the follower: user's outbox, the streams of post and notification.
reads() { token=$token2 call GET /api/v1/feed | jq -e '.posts | length > 0' >/dev/null; }
eventually reads || die "the posts of a followed person are not in the feed of the follower"
followed() { call GET /api/v1/notifications | jq -e '[.notifications[] | select(.type == "follow")] | length == 1' >/dev/null; }
eventually followed || die "the followed person has no notification of the follower"
step "user: a follow reaches the feed of the follower and the notifications of the followed"

token=$token2 call PUT "/api/v1/clubs/$club/membership" | jq -e '.role == "member"' >/dev/null
expect=204 call PUT "/api/v1/clubs/$club/admins/$nickname2" >/dev/null
# club's outbox, the stream of notification.
appointed() { token=$token2 call GET /api/v1/notifications | jq -e '[.notifications[] | select(.type == "club_admin" and .actor.club)] | length == 1' >/dev/null; }
eventually appointed || die "the new admin of the club has no notification of it"
step "club: a member made an admin hears of it"

# Messages of dialogs are sealed by the browsers: the server takes sealed
# bytes for the keys of both as they are now, and no plain words. These
# bytes open for no one; the server never tries.
key1=$(head -c 32 /dev/urandom | base64 | tr -d '\n')
key2=$(head -c 32 /dev/urandom | base64 | tr -d '\n')
sealed=$({ printf '\001'; echo "$key1" | base64 --decode; echo "$key2" | base64 --decode; head -c 28 /dev/urandom; } | base64 | tr -d '\n')
expect=204 call PUT /api/v1/me/message-key "{\"publicKey\":\"$key1\"}" >/dev/null
expect=409 call POST "/api/v1/dialogs/$nickname2/messages" "{\"sealed\":\"$sealed\"}" | jq -e '.error.message == "the encryption keys changed"' >/dev/null
token=$token2 expect=204 call PUT /api/v1/me/message-key "{\"publicKey\":\"$key2\"}" >/dev/null
call GET "/api/v1/users/$nickname2/message-key" | jq -e --arg k "$key2" '.publicKey == $k' >/dev/null
expect=201 call POST "/api/v1/dialogs/$nickname2/messages" "{\"sealed\":\"$sealed\"}" >/dev/null
token=$token2 call GET /api/v1/dialogs | jq -e --arg s "$sealed" '.dialogs[0].lastMessage.sealed == $s' >/dev/null
expect=400 call POST "/api/v1/dialogs/$nickname2/messages" '{"text":"plain"}' | jq -e '.error.message == "messages are end-to-end encrypted"' >/dev/null
step "message: dialogs sealed for the keys of both"

# The world answers through the gateway: a new page is nobody's citizen,
# whether a world is open or not.
call GET "/api/v1/users/$nickname/world" | jq -e '.state == null' >/dev/null
step "world: a new page is no citizen"

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

# Prometheus scrapes every 30s: the first round may be on its way.
up=
for _ in $(seq 1 60); do
	up=$(curl -sf "$prometheus/api/v1/query?query=up%7Bjob%3D%22freenet%22%7D" | jq -r '[.data.result[] | select(.value[1] == "1") | .metric.service] | sort | join(" ")' || true)
	[ "$up" = "$(printf '%s\n' "${services[@]}" | sort | xargs)" ] && break
	sleep 1
done
[ "$up" = "$(printf '%s\n' "${services[@]}" | sort | xargs)" ] || die "Prometheus at $prometheus sees up: ${up:-nothing}"
step "metrics: Prometheus scrapes every service"

# Every database with a service of its own, and Redis.
watched="auth club message notification post redis user world"
stores=
for _ in $(seq 1 60); do
	stores=$(curl -sf "$prometheus/api/v1/query?query=pg_up%20or%20redis_up" |
		jq -r '[.data.result[] | select(.value[1] == "1") | .metric.db // "redis"] | sort | join(" ")' || true)
	[ "$stores" = "$watched" ] && break
	sleep 1
done
[ "$stores" = "$watched" ] || die "Prometheus at $prometheus sees the databases and Redis up: ${stores:-none}"
step "metrics: Prometheus watches every database and Redis"

# The machine itself: its disk, its memory and when the mTLS certificates
# end (deploy/certs.sh, which `make stack` runs). The alerts look at the
# disk mounted at /; Docker Desktop runs in a machine of its own, whose
# root is no disk.
disk='node_filesystem_avail_bytes{mountpoint="/"}'
[ "$(uname)" = Linux ] || disk='node_filesystem_avail_bytes'
host=
for _ in $(seq 1 60); do
	host=$(curl -sfG "$prometheus/api/v1/query" --data-urlencode "query=(count($disk) > bool 0) + count(node_memory_MemAvailable_bytes) + count(freenet_certificate_not_after_seconds > time())" |
		jq -r '.data.result[0].value[1] // empty' || true)
	[ "$host" = 11 ] && break
	sleep 1
done
[ "$host" = 11 ] || die "Prometheus at $prometheus does not see the disk, the memory and the nine certificates of the machine: ${host:-nothing}"
curl -sfG "$prometheus/api/v1/query" --data-urlencode 'query=redis_memory_max_bytes' | jq -e '.data.result[0].value[1] | tonumber > 0' >/dev/null \
	|| die "Redis has no memory limit"
step "metrics: Prometheus watches the machine, the certificates and the memory limit of Redis"

# Rules that fail to evaluate (a typo in a metric name does not) say so in
# their health.
rules=$(curl -sf "$prometheus/api/v1/rules?type=alert") || die "no rules from Prometheus at $prometheus"
echo "$rules" | jq -e '[.data.groups[].rules[]] | length >= 17 and all(.health == "ok" or .health == "unknown")' >/dev/null \
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
