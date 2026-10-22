#!/bin/sh
# Issues the certificates of the QUIC transport between the services
# (pkg/quictransport): a CA, and a directory per service with the CA
# certificate and, for the services that serve over QUIC, a certificate and
# a key for the service's name. It runs as the one-shot service quic-certs
# of docker-compose.yml before the services start and issues only what is
# missing, so the certificates outlive restarts; a new CA reissues them all.
# Remove the volumes to issue everything anew.
#
# QUIC_SERVERS and QUIC_CLIENTS list the services, separated by spaces;
# CERTS_DIR is where the directories go, /certs by default.
set -eu

dir=${CERTS_DIR:-/certs}
servers=${QUIC_SERVERS:-}
clients=${QUIC_CLIENTS:-}
days=3650

missing=false
[ -f "$dir/ca/ca.pem" ] || missing=true
for s in $servers; do [ -f "$dir/$s/cert.pem" ] || missing=true; done
for s in $servers $clients; do [ -f "$dir/$s/ca.pem" ] || missing=true; done
if [ "$missing" = false ]; then
	echo "quic-certs: nothing to issue"
	exit 0
fi
command -v openssl >/dev/null 2>&1 || { echo "quic-certs: needs openssl" >&2; exit 1; }

new_ca=false
if [ ! -f "$dir/ca/ca.pem" ]; then
	mkdir -p "$dir/ca"
	openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days "$days" \
		-subj "/CN=messenger QUIC CA" \
		-addext "basicConstraints=critical,CA:TRUE" -addext "keyUsage=critical,keyCertSign,cRLSign" \
		-keyout "$dir/ca/ca.key" -out "$dir/ca/ca.pem" 2>/dev/null
	chmod 600 "$dir/ca/ca.key"
	new_ca=true
	echo "quic-certs: issued the CA"
fi

for s in $servers; do
	if [ "$new_ca" = false ] && [ -f "$dir/$s/cert.pem" ]; then
		continue
	fi
	mkdir -p "$dir/$s"
	openssl req -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -subj "/CN=$s" \
		-keyout "$dir/$s/key.pem" -out "$dir/$s/cert.csr" 2>/dev/null
	printf 'subjectAltName=DNS:%s\nextendedKeyUsage=serverAuth\nkeyUsage=critical,digitalSignature\n' "$s" >"$dir/$s/cert.ext"
	openssl x509 -req -in "$dir/$s/cert.csr" -CA "$dir/ca/ca.pem" -CAkey "$dir/ca/ca.key" \
		-CAcreateserial -CAserial "$dir/ca/ca.srl" -days "$days" -extfile "$dir/$s/cert.ext" \
		-out "$dir/$s/cert.pem" 2>/dev/null
	rm -f "$dir/$s/cert.csr" "$dir/$s/cert.ext"
	# The services run as an unprivileged user; only the service's own
	# volume holds its key.
	chmod 644 "$dir/$s/key.pem"
	echo "quic-certs: issued the certificate of $s"
done

for s in $servers $clients; do
	mkdir -p "$dir/$s"
	cp "$dir/ca/ca.pem" "$dir/$s/ca.pem"
done
