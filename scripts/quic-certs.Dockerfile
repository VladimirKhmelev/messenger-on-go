# The tools of scripts/quic-certs.sh, which docker-compose.yml mounts into
# the one-shot service quic-certs and runs: the certificates are issued at
# start, with no network.
FROM alpine:3.20
RUN apk add --no-cache openssl
