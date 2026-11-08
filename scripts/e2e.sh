#!/usr/bin/env bash
# prepare builds native test binaries and certificates without invoking Docker.
# run starts a disposable stack; invoke it only when Docker use is intended.
set -euo pipefail
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
work=${E2E_WORK_DIR:-$root/out/e2e}
mkdir -p "$work"
work=$(cd -- "$work" && pwd)

case ${1:-} in
prepare)
    [[ $# == 1 ]] || { echo 'prepare takes no arguments' >&2; exit 2; }
    mkdir -p "$work/bin"
    export GOWORK=off CGO_ENABLED=0
    for service in auth-service chat-service ws-gateway media-service notification-worker; do
        entry=server
        [[ $service != notification-worker ]] || entry=worker
        (cd "$root/services/$service" && go build -o "$work/bin/$service" "./cmd/$entry")
    done
    (cd "$root/tools/e2e" && go build -o "$work/bin/e2e" .)
    CERTS_DIR="$work/certs" QUIC_SERVERS='auth-service chat-service' \
        QUIC_CLIENTS='ws-gateway media-service notification-worker' sh "$root/scripts/quic-certs.sh"
    "$work/bin/e2e" -seal -root "$root" -work "$work"
    echo "Prepared without Docker: $work"
    ;;
run)
    shift
    [[ -x $work/bin/e2e ]] || { echo 'Run scripts/e2e.sh prepare first' >&2; exit 2; }
    export E2E_REVISION
    E2E_REVISION=$(git -C "$root" rev-parse HEAD)
    exec "$work/bin/e2e" -root "$root" -work "$work" "$@"
    ;;
*)
    echo 'usage: scripts/e2e.sh prepare | run [-transport grpc|quic|all] [-observe 1s]' >&2
    exit 2
    ;;
esac
