#!/usr/bin/env bash
# Creates a development CA and mTLS certificates for the services in certs/.
# Each certificate's common name is the service identity checked by
# pkg/mtls (auth, user, post, media, notification, message,
# gateway); SANs cover localhost and the service name. Also the RSA key
# pair auth signs access tokens with (jwt-private.pem, PKCS#1) and the
# others verify them with (jwt-public.pem).
# Re-running replaces everything, including the CA and the JWT keys.
set -euo pipefail

out="${1:-certs}"
days=825
services=(auth user post media notification message gateway)

mkdir -p "$out"
cd "$out"

openssl ecparam -name prime256v1 -genkey -noout -out ca-key.pem
openssl req -x509 -new -key ca-key.pem -sha256 -days "$days" \
	-subj "/CN=freenet development CA" \
	-addext "basicConstraints=critical,CA:TRUE" \
	-addext "keyUsage=critical,keyCertSign,cRLSign" \
	-out ca.pem

for name in "${services[@]}"; do
	openssl ecparam -name prime256v1 -genkey -noout -out "$name-key.pem"
	openssl req -new -key "$name-key.pem" -subj "/CN=$name" -out "$name.csr"

	cat > "$name.ext" <<EXT
basicConstraints=CA:FALSE
keyUsage=critical,digitalSignature
extendedKeyUsage=serverAuth,clientAuth
subjectAltName=DNS:localhost,DNS:$name,IP:127.0.0.1
EXT

	openssl x509 -req -in "$name.csr" -CA ca.pem -CAkey ca-key.pem -CAcreateserial \
		-days "$days" -sha256 -extfile "$name.ext" -out "$name.pem" 2>/dev/null

	rm "$name.csr" "$name.ext"
done

rm -f ca.srl

# auth reads PKCS#1; OpenSSL 3 writes it only with -traditional, LibreSSL
# (macOS) always does and has no such flag.
traditional=
openssl rsa -help 2>&1 | grep -q -- -traditional && traditional=-traditional
openssl genrsa 2048 2>/dev/null | openssl rsa $traditional -out jwt-private.pem 2>/dev/null
openssl rsa -in jwt-private.pem -pubout -out jwt-public.pem 2>/dev/null

chmod 600 ./*-key.pem jwt-private.pem

echo "certificates for ${services[*]} and JWT keys written to $(pwd)"
