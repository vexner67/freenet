#!/usr/bin/env bash
# Runs on the server, in /opt/freenet, as the deploy user: GitHub Actions
# copies compose.yaml, deploy/ and monitoring/ there and calls
#
#   deploy/deploy.sh <commit> <ghcr user> < <ghcr token>
#
# It pulls that commit's images, (re)starts what changed and waits until
# everything is healthy; the services apply their migrations as they
# start. The token, read from stdin, lets Docker pull from GHCR for this
# run only. Rolling back is deploying an older commit the same way.
set -euo pipefail

tag="${1:?usage: deploy.sh <commit> <ghcr user> < token}"
user="${2:?usage: deploy.sh <commit> <ghcr user> < token}"

cd "$(dirname "$0")/.."

# The commit runs from now on, also for a plain `docker compose` here.
sed -i "s/^FREENET_TAG=.*/FREENET_TAG=$tag/" .env

# Grafana signs its admin in with a password of this server, never the
# "admin" it is born with.
grep -q '^GRAFANA_ADMIN_PASSWORD=.' .env || echo "GRAFANA_ADMIN_PASSWORD=$(openssl rand -hex 16)" >> .env

# A database of a service added after the server was set up gets its
# credentials here, once; the port is not published (compose.prod.yaml).
for db in WORLD; do
	grep -q "^${db}_POSTGRES_PASSWORD=." .env && continue
	{
		echo "${db}_POSTGRES_USER=freenet"
		echo "${db}_POSTGRES_PASSWORD=$(openssl rand -hex 24)"
		echo "${db}_POSTGRES_DB=freenet"
		echo "${db}_POSTGRES_PORT=5439"
	} >> .env
done

docker login ghcr.io --username "$user" --password-stdin >/dev/null
trap 'docker logout ghcr.io >/dev/null' EXIT

docker compose pull --quiet

# A Caddyfile Caddy cannot read, with this server's .env, would take the
# site down: the deployment fails here instead, before anything restarts.
docker compose run --rm --no-deps --entrypoint caddy caddy \
	validate --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null 2>&1 ||
	{ echo "deploy/Caddyfile does not work with this .env"; exit 1; }

# Who Alertmanager notifies, from this server's .env; a configuration it
# cannot read fails the deployment here, as the Caddyfile above.
alerts=$(deploy/alertmanager.sh)
docker compose run --rm --no-deps --entrypoint amtool alertmanager \
	check-config /etc/alertmanager/alertmanager.yml >/dev/null 2>&1 ||
	{ echo "alertmanager.yml does not work with this .env"; exit 1; }

# Certificates of the services close to their end are issued anew, and
# node-exporter is told when each ends; the services read a renewed one
# themselves.
deploy/certs.sh

docker compose up --detach --wait --wait-timeout 300 --remove-orphans

# A file mounted into a container changes there only on a restart.
if [ "$alerts" = changed ]; then
	docker compose restart alertmanager >/dev/null
fi

# So do Prometheus and Jaeger read their files only as they start:
# restart the one that runs something else than what was deployed.
running() { docker compose exec -T "$1" cat "$2" | cmp -s - "$3"; }
if ! running prometheus /etc/prometheus/prometheus.yml monitoring/prometheus.yml ||
	! running prometheus /etc/prometheus/alerts.yml monitoring/alerts.yml; then
	docker compose restart prometheus >/dev/null
fi
if ! running jaeger /etc/jaeger/config.yml monitoring/jaeger.yml; then
	docker compose restart jaeger >/dev/null
fi

# Fluent Bit too, and its image has nothing to look inside with; it
# remembers where it stopped in every log.
docker compose restart fluent-bit >/dev/null

# Grafana keeps the password it first started with in its volume: one
# that still lets "admin" in with "admin" gets the password of .env.
if docker compose exec -T grafana wget -qO /dev/null \
	--header "Authorization: Basic $(printf admin:admin | base64)" http://localhost:3000/api/admin/stats 2>/dev/null; then
	docker compose exec -T grafana sh -c 'grafana cli admin reset-admin-password "$GF_SECURITY_ADMIN_PASSWORD"' >/dev/null
	echo "grafana: the admin password is now GRAFANA_ADMIN_PASSWORD of .env"
fi

# Images of older commits, kept a day for a quick rollback: eight images
# a deployment fill a small disk in a few days.
docker image prune --all --force --filter "until=24h" >/dev/null

echo "deployed $tag"
docker compose ps --format 'table {{.Service}}\t{{.Status}}'
