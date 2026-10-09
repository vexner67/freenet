#!/usr/bin/env bash
# Renders the configuration of Alertmanager on the server:
#
#   deploy/alertmanager.sh [env file] [output]
#
# monitoring/alertmanager.yml, from git, with a Telegram chat for its
# receiver when the env file (.env) sets TELEGRAM_BOT_TOKEN and
# TELEGRAM_CHAT_ID; without them nobody is notified. The output
# (alertmanager.yml, outside monitoring/, which each deployment replaces)
# holds the token: the directory is the deploy user's only. Prints
# "changed" when the output changed, for deploy.sh to restart Alertmanager.
set -euo pipefail

cd "$(dirname "$0")/.."

env_file=${1:-.env}
out=${2:-alertmanager.yml}

# value is the last NAME= of the env file, "" without one.
value() { sed -n "s/^$1=//p" "$env_file" | tail -n 1; }

token=$(value TELEGRAM_BOT_TOKEN)
chat=$(value TELEGRAM_CHAT_ID)

tmp=$(mktemp "$out.XXXXXX")
trap 'rm -f "$tmp"' EXIT
cp monitoring/alertmanager.yml "$tmp"

if [ -n "$token" ] || [ -n "$chat" ]; then
	# Checked before they go into YAML: nothing else can sneak in with them.
	[[ $token =~ ^[0-9]+:[A-Za-z0-9_-]+$ ]] || { echo "TELEGRAM_BOT_TOKEN is not a bot token" >&2; exit 1; }
	[[ $chat =~ ^-?[0-9]+$ ]] || { echo "TELEGRAM_CHAT_ID is not a chat ID" >&2; exit 1; }

	cat >> "$tmp" <<YAML
    telegram_configs:
      - bot_token: '$token'
        chat_id: $chat
        send_resolved: true
YAML
fi

# Alertmanager runs as nobody and reads it through the bind mount.
chmod 644 "$tmp"

if ! cmp -s "$tmp" "$out" 2>/dev/null; then
	mv "$tmp" "$out"
	trap - EXIT
	echo changed
fi
