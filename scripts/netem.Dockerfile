# The tools of scripts/netem.sh, which runs them in the network namespaces
# of the services and of the host.
FROM alpine:3.20
RUN apk add --no-cache iproute2 ethtool
