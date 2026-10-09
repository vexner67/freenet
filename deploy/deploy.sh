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

docker compose up --detach --wait --wait-timeout 300 --remove-orphans

# A file mounted into a container changes there only on a restart.
if [ "$alerts" = changed ]; then
	docker compose restart alertmanager >/dev/null
fi

# Images of older commits, kept a few days for a quick rollback.
docker image prune --all --force --filter "until=72h" >/dev/null

echo "deployed $tag"
docker compose ps --format 'table {{.Service}}\t{{.Status}}'
