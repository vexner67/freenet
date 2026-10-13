#!/usr/bin/env bash
# Keeps the mTLS certificates of the services fresh and says when they end:
#
#   deploy/certs.sh [certs directory] [metrics file] [days]
#
# A service certificate with less than <days> (60) left is issued anew by
# the CA of the directory (ca.pem, ca-key.pem), with a new key, the way
# scripts/gen-certs.sh issued it. The services read their certificate again
# as its files change (pkg/mtls), so nothing restarts. The CA itself and
# the JWT keys are not touched: replacing the CA means running
# scripts/gen-certs.sh and restarting everything.
#
# The metrics file (.data/node/certs.prom, read by node-exporter) gets
# freenet_certificate_not_after_seconds for the CA and every certificate;
# the alert CertificateExpiring looks at it. deploy/deploy.sh runs this on
# every deployment, `make stack` locally.
set -euo pipefail

cd "$(dirname "$0")/.."

dir=${1:-certs}
metrics=${2:-.data/node/certs.prom}
days=${3:-60}
valid=825

# issue NAME writes a new key and certificate of the service NAME.
issue() {
	local name=$1 tmp
	tmp=$(mktemp -d "$dir/.renew.XXXXXX")

	openssl ecparam -name prime256v1 -genkey -noout -out "$tmp/key.pem"
	openssl req -new -key "$tmp/key.pem" -subj "/CN=$name" -out "$tmp/csr"
	cat > "$tmp/ext" <<EXT
basicConstraints=CA:FALSE
keyUsage=critical,digitalSignature
extendedKeyUsage=serverAuth,clientAuth
subjectAltName=DNS:localhost,DNS:$name,IP:127.0.0.1
EXT
	openssl x509 -req -in "$tmp/csr" -CA "$dir/ca.pem" -CAkey "$dir/ca-key.pem" -CAcreateserial \
		-CAserial "$tmp/srl" -days "$valid" -sha256 -extfile "$tmp/ext" -out "$tmp/cert.pem" 2>/dev/null

	chmod 600 "$tmp/key.pem"
	chmod 644 "$tmp/cert.pem"
	# A service that reads between the two keeps the pair it has: the new
	# certificate does not match the old key.
	mv "$tmp/key.pem" "$dir/$name-key.pem"
	mv "$tmp/cert.pem" "$dir/$name.pem"
	rm -r "$tmp"
}

# not_after FILE prints when the certificate ends, in seconds since 1970.
not_after() {
	local end
	end=$(openssl x509 -in "$1" -noout -enddate | cut -d= -f2)
	# GNU date, then the one of macOS.
	date -u -d "$end" +%s 2>/dev/null || date -u -j -f '%b %e %T %Y %Z' "$end" +%s
}

names=()
for key in "$dir"/*-key.pem; do
	name=$(basename "$key" -key.pem)
	[ "$name" != ca ] && [ -f "$dir/$name.pem" ] && names+=("$name")
done
[ ${#names[@]} -gt 0 ] || { echo "certs: no certificates in $dir" >&2; exit 1; }

for name in "${names[@]}"; do
	if ! openssl x509 -in "$dir/$name.pem" -noout -checkend $((days * 86400)) >/dev/null; then
		[ -f "$dir/ca-key.pem" ] || { echo "certs: $name.pem ends within $days days and there is no ca-key.pem to renew it" >&2; exit 1; }
		issue "$name"
		echo "renewed $name"
	fi
done

mkdir -p "$(dirname "$metrics")"
tmp=$(mktemp "$metrics.XXXXXX")
trap 'rm -f "$tmp"' EXIT
{
	echo "# HELP freenet_certificate_not_after_seconds When an mTLS certificate ends, seconds since 1970."
	echo "# TYPE freenet_certificate_not_after_seconds gauge"
	for name in ca "${names[@]}"; do
		echo "freenet_certificate_not_after_seconds{name=\"$name\"} $(not_after "$dir/$name.pem")"
	done
} > "$tmp"
# node-exporter runs as nobody and reads it through the bind mount.
chmod 644 "$tmp"
mv "$tmp" "$metrics"
trap - EXIT
