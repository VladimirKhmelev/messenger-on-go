#!/usr/bin/env bash
# Impairs the network between the ws-gateways and chat-service with netem,
# to compare gRPC and QUIC on a bad network. The profiles are those of the
# testbed of quic-rpc-go (testbed/scenarios/profiles.yaml) without the
# bandwidth limits and the outages: each direction gets half the rtt, and
# the jitter, loss and reordering in full.
#
# Usage: scripts/netem.sh PROFILE|off|show
#   PROFILE  datacenter, nearby-zones, interregion, congested or degraded
#   off      removes netem and restores the offloads
#   show     prints the qdiscs and filters with their counters
#
# netem goes on eth0 of each service, through a container of the tools
# image in its network namespace, and only on the packets to the other
# side, with a netem for each peer: a gateway's to chat-service,
# chat-service's to either gateway. Postgres, Redis, NATS and the WebSocket
# clients are not impaired. As on the testbed, TSO and GSO are off on those
# eth0, so that netem drops and delays single packets rather than batches
# of up to 64 KB, and GRO is on at their host ends, the first devices after
# netem, which then take the packets in order; start the stack with
# QUIC_GO_DISABLE_GSO=true for the same reason. A recreated container comes
# up unimpaired.
set -euo pipefail

project=${COMPOSE_PROJECT:-messenger-on-go}
image=messenger-netem
dir=$(dirname "$0")
server=chat-service
gateways=(ws-gateway-1 ws-gateway-2)

# netem_args prints the netem of one direction for a profile. With jitter, a
# rate far above the traffic keeps netem from reordering the packets.
netem_args() {
	case $1 in
	datacenter) echo "delay 500us" ;;
	nearby-zones) echo "delay 2500us 1000us distribution normal loss random 0.1% rate 100gbit" ;;
	interregion) echo "delay 75000us 10000us distribution normal loss random 0.5% rate 100gbit" ;;
	congested) echo "delay 25000us 5000us distribution normal loss random 1% rate 100gbit" ;;
	# Losses of 2% in bursts of 4 packets on average.
	degraded) echo "delay 50000us 20000us distribution normal loss gemodel 0.510204% 25% reorder 1% rate 100gbit" ;;
	*) return 1 ;;
	esac
}

container() { echo "$project-$1-1"; }

# in_ns runs a command of the tools image in the network namespace of a
# service, or of the host for "host".
in_ns() {
	local net=host
	[[ $1 == host ]] || net=container:$(container "$1")
	docker run --rm --network "$net" --cap-add NET_ADMIN "$image" "${@:2}"
}

ip_of() {
	docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$(container "$1")"
}

# host_end prints the name of the host end of the eth0 of a service.
host_end() {
	local index
	index=$(in_ns "$1" cat /sys/class/net/eth0/iflink)
	in_ns host ip -o link | awk -F': ' -v i="$index" '$1 == i { sub(/@.*/, "", $2); print $2 }'
}

# impair puts a netem with args on eth0 of a service for the packets to
# each of the peers.
impair() {
	local service=$1 args=$2 band=2 peer
	shift 2
	local script="tc qdisc del dev eth0 root 2>/dev/null; ethtool -K eth0 tso off gso off &&
		tc qdisc add dev eth0 root handle 1: prio bands $(($# + 1)) priomap 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0"
	for peer; do
		script+=" && tc qdisc add dev eth0 parent 1:$band handle ${band}0: netem limit 100000 $args"
		script+=" && tc filter add dev eth0 parent 1: protocol ip prio 1 u32 match ip dst $(ip_of "$peer")/32 flowid 1:$band"
		band=$((band + 1))
	done
	in_ns "$service" sh -c "$script"
	in_ns host ethtool -K "$(host_end "$service")" gro on
}

# restore takes netem off a service and gives its veth the offloads docker
# sets.
restore() {
	in_ns "$1" sh -c 'tc qdisc del dev eth0 root 2>/dev/null; ethtool -K eth0 tso on gso on'
	in_ns host ethtool -K "$(host_end "$1")" gro off
}

usage() {
	echo "usage: $0 datacenter|nearby-zones|interregion|congested|degraded|off|show" >&2
	exit 2
}

[[ $# == 1 ]] || usage
if [[ $1 != off && $1 != show ]]; then
	args=$(netem_args "$1") || usage
fi
docker image inspect "$image" >/dev/null 2>&1 ||
	docker build --network host --build-arg HTTP_PROXY --build-arg HTTPS_PROXY -t "$image" -f "$dir/netem.Dockerfile" "$dir"

case $1 in
off)
	for s in "$server" "${gateways[@]}"; do restore "$s"; done
	echo "netem: off"
	;;
show)
	for s in "$server" "${gateways[@]}"; do
		echo "== $s"
		in_ns "$s" sh -c 'tc -s qdisc show dev eth0; tc filter show dev eth0'
	done
	;;
*)
	for s in "$server" "${gateways[@]}"; do
		docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "$(container "$s")" | grep -qx 'QUIC_GO_DISABLE_GSO=true' ||
			echo "netem: $s runs with GSO of quic-go; start the stack with QUIC_GO_DISABLE_GSO=true" >&2
	done
	for g in "${gateways[@]}"; do impair "$g" "$args" "$server"; done
	impair "$server" "$args" "${gateways[@]}"
	echo "netem: $1 between ${gateways[*]} and $server"
	;;
esac
