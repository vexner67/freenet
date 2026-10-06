#!/usr/bin/env bash
# Creates a development CA and mTLS certificates for the services in certs/.
# Each certificate's common name is the service identity checked by
# pkg/mtls (auth, user, post, media, notification, message,
# gateway); SANs cover localhost and the service name.
# Re-running replaces everything, including the CA.
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
chmod 600 ./*-key.pem

echo "certificates for ${services[*]} written to $(pwd)"
