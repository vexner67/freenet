#!/usr/bin/env bash
# Prepares a fresh Ubuntu 24.04 server for Freenet, once, as root:
#
#   scp deploy/bootstrap.sh scripts/gen-certs.sh root@<server>:/root/
#   ssh root@<server> 'bash /root/bootstrap.sh "<deploy public key>"'
#
# Safe to run again: it changes only what is not done yet and never
# replaces the secrets in /opt/freenet/.env or the certificates.
#
# - SSH by key only: no passwords, root by key only, few attempts; fail2ban
#   bans who keeps guessing.
# - ufw lets in only SSH, HTTP and HTTPS (HTTP/3 too). Docker publishes
#   ports past ufw, which is why deploy/compose.prod.yaml publishes none
#   but Caddy's.
# - Security updates install themselves (unattended-upgrades).
# - Docker from its own repository, logs rotated (auth logs sign-in codes
#   while there is no SMTP).
# - The user freenet owns /opt/freenet and deploys there (GitHub Actions
#   logs in as it with the deploy key); it is in the docker group, which is
#   as good as root, so its key is for the deployment only.
# - /opt/freenet/.env gets random database passwords and secrets, and
#   certs/ the mTLS certificates and JWT keys (scripts/gen-certs.sh).
set -euo pipefail

deploy_key="${1:?usage: bootstrap.sh \"<public key of the deploy user>\"}"
app_user=freenet
app_dir=/opt/freenet

export DEBIAN_FRONTEND=noninteractive

step() { echo "==> $*"; }

# Config files the provider changed (cloud.cfg) stay as they are, without
# a question no one is there to answer.
apt() { apt-get -yq -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold "$@"; }

step "packages and security updates"
dpkg --configure -a --force-confdef --force-confold
apt-get update -q
apt upgrade
apt install ca-certificates curl gnupg ufw fail2ban unattended-upgrades openssl rsync
cat > /etc/apt/apt.conf.d/20auto-upgrades <<'EOF'
APT::Periodic::Update-Package-Lists "1";
APT::Periodic::Unattended-Upgrade "1";
EOF

step "SSH: keys only"
cat > /etc/ssh/sshd_config.d/00-freenet.conf <<'EOF'
# First match wins in sshd: this file comes before the cloud image's.
PasswordAuthentication no
KbdInteractiveAuthentication no
PermitRootLogin prohibit-password
PermitEmptyPasswords no
MaxAuthTries 3
X11Forwarding no
EOF
# Refuse to cut ourselves off: root must already have a key.
test -s /root/.ssh/authorized_keys || { echo "root has no SSH key: add one before turning passwords off"; exit 1; }
# Ubuntu 24.04 starts sshd from a socket: until the first connection there
# is no /run/sshd, and sshd -t refuses to check without it.
install -d -m 755 /run/sshd
sshd -t
systemctl try-reload-or-restart ssh

step "fail2ban for SSH"
cat > /etc/fail2ban/jail.d/sshd.local <<'EOF'
[sshd]
enabled = true
backend = systemd
maxretry = 5
findtime = 10m
bantime = 1h
EOF
systemctl enable --now fail2ban
systemctl restart fail2ban

step "firewall: SSH, HTTP, HTTPS"
ufw default deny incoming
ufw default allow outgoing
ufw limit 22/tcp
ufw allow 80/tcp
ufw allow 443/tcp
ufw allow 443/udp
ufw --force enable

step "swap for a small server"
if ! swapon --show | grep -q . && [ "$(awk '/MemTotal/ {print $2}' /proc/meminfo)" -lt 4000000 ]; then
	fallocate -l 2G /swapfile
	chmod 600 /swapfile
	mkswap /swapfile
	swapon /swapfile
	grep -q '^/swapfile' /etc/fstab || echo '/swapfile none swap sw 0 0' >> /etc/fstab
fi

step "Docker"
if ! command -v docker >/dev/null; then
	install -m 0755 -d /etc/apt/keyrings
	curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
	chmod a+r /etc/apt/keyrings/docker.asc
	echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" \
		> /etc/apt/sources.list.d/docker.list
	apt-get update -q
	apt install docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
fi
cat > /etc/docker/daemon.json <<'EOF'
{
  "log-driver": "json-file",
  "log-opts": { "max-size": "10m", "max-file": "5" }
}
EOF
systemctl enable docker
systemctl restart docker

step "the deploy user"
id "$app_user" >/dev/null 2>&1 || useradd --create-home --shell /bin/bash "$app_user"
usermod -aG docker "$app_user"
install -d -m 700 -o "$app_user" -g "$app_user" "/home/$app_user/.ssh"
grep -qxF "$deploy_key" "/home/$app_user/.ssh/authorized_keys" 2>/dev/null ||
	echo "$deploy_key" >> "/home/$app_user/.ssh/authorized_keys"
chown "$app_user:$app_user" "/home/$app_user/.ssh/authorized_keys"
chmod 600 "/home/$app_user/.ssh/authorized_keys"

step "$app_dir: secrets and certificates"
install -d -m 750 -o "$app_user" -g "$app_user" "$app_dir" "$app_dir/deploy" "$app_dir/.data" "$app_dir/.data/media"

if [ ! -f "$app_dir/.env" ]; then
	secret() { openssl rand -hex "$1"; }
	{
		echo "# Written by deploy/bootstrap.sh; secrets, never in git."
		echo "COMPOSE_PROJECT_NAME=freenet"
		echo "COMPOSE_FILE=compose.yaml:deploy/compose.prod.yaml"
		echo "COMPOSE_PROFILES=app"
		echo "FREENET_UID=$(id -u "$app_user")"
		echo "FREENET_GID=$(id -g "$app_user")"
		echo "# The commit running now; deploy/deploy.sh sets it."
		echo "FREENET_TAG="
		echo "# The domain once there is one (Caddy gets its certificate), :80 until then."
		echo "SITE_ADDRESS=:80"
		echo "# www.<domain>, redirected to SITE_ADDRESS; an unused port without a domain."
		echo "REDIRECT_ADDRESS=:8081"
		port=5433
		for db in AUTH USER POST NOTIFICATION MESSAGE CLUB; do
			echo "${db}_POSTGRES_USER=freenet"
			echo "${db}_POSTGRES_PASSWORD=$(secret 24)"
			echo "${db}_POSTGRES_DB=freenet"
			# Not published in production (compose.prod.yaml); compose.yaml
			# needs a value.
			echo "${db}_POSTGRES_PORT=$port"
			port=$((port + 1))
		done
		echo "REDIS_PORT=6379"
		echo "HASH_SECRET=$(secret 32)"
		echo "GRAFANA_ADMIN_PASSWORD=$(secret 16)"
		echo "# Sign-in codes: console (in \`docker compose logs auth\`) or smtp with SMTP_*."
		echo "MAILER=console"
		echo "SMTP_HOST="
		echo "SMTP_PORT="
		echo "SMTP_USERNAME="
		echo "SMTP_PASSWORD="
		echo "SMTP_FROM="
	} > "$app_dir/.env"
	chown "$app_user:$app_user" "$app_dir/.env"
	chmod 600 "$app_dir/.env"
fi

if [ ! -f "$app_dir/certs/ca.pem" ]; then
	bash /root/gen-certs.sh "$app_dir/certs"
	chown -R "$app_user:$app_user" "$app_dir/certs"
	chmod 700 "$app_dir/certs"
fi

step "done: $(docker --version), $(docker compose version)"
