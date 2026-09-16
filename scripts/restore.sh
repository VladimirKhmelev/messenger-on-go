#!/usr/bin/env bash
set -euo pipefail

COMPOSE_PROJECT="${COMPOSE_PROJECT:-messenger-on-go}"

if [ $# -ne 2 ]; then
  echo "usage: $0 <auth|chat|media> <path-to-dump.sql.gz>" >&2
  exit 1
fi

db="$1"
dump_file="$2"

case "$db" in
  auth|chat|media) ;;
  *)
    echo "restore: unknown database '$db' (expected auth, chat, or media)" >&2
    exit 1
    ;;
esac

if [ ! -f "$dump_file" ]; then
  echo "restore: dump file not found: $dump_file" >&2
  exit 1
fi

service="${db}-db"
container="${COMPOSE_PROJECT}-${service}-1"

if ! docker inspect "$container" >/dev/null 2>&1; then
  echo "restore: container $container not found" >&2
  exit 1
fi

echo "This will DROP and recreate all tables in the '$db' database (container: $container)."
echo "Dump: $dump_file"
read -r -p "Type the database name ('$db') to confirm: " confirmation
if [ "$confirmation" != "$db" ]; then
  echo "restore: confirmation did not match, aborting" >&2
  exit 1
fi

echo "restore: dropping and recreating schema public in $db"
docker exec "$container" psql -U "$db" -d "$db" -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"

echo "restore: loading $dump_file into $db"
gunzip -c "$dump_file" | docker exec -i "$container" psql -U "$db" -d "$db"

echo "restore: done"
