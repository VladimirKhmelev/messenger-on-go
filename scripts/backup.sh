#!/usr/bin/env bash
set -euo pipefail

BACKUP_DIR="${BACKUP_DIR:-/root/messenger-on-go-backups}"
RETENTION_DAYS="${RETENTION_DAYS:-14}"
COMPOSE_PROJECT="${COMPOSE_PROJECT:-messenger-on-go}"
TIMESTAMP="$(date -u +%Y%m%dT%H%M%SZ)"

DATABASES=(
  "auth-db:auth"
  "chat-db:chat"
  "media-db:media"
)

mkdir -p "$BACKUP_DIR"

for entry in "${DATABASES[@]}"; do
  service="${entry%%:*}"
  db="${entry##*:}"
  container="${COMPOSE_PROJECT}-${service}-1"
  out_file="${BACKUP_DIR}/${db}-${TIMESTAMP}.sql.gz"

  if ! docker inspect "$container" >/dev/null 2>&1; then
    echo "backup: container $container not found, skipping $db" >&2
    continue
  fi

  echo "backup: dumping $db from $container -> $out_file"
  docker exec "$container" pg_dump -U "$db" "$db" | gzip > "$out_file"
done

echo "backup: pruning dumps older than ${RETENTION_DAYS} days in $BACKUP_DIR"
find "$BACKUP_DIR" -name '*.sql.gz' -mtime "+${RETENTION_DAYS}" -delete

echo "backup: done"
