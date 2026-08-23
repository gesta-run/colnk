#!/bin/sh
set -eu

container_id=$(docker compose ps -q server)
if [ -z "$container_id" ]; then
  echo "Server container is not running" >&2
  exit 1
fi

docker exec "$container_id" test -d /mnt/local
docker exec "$container_id" ip link show local0
docker exec "$container_id" sh -lc 'mount | grep -q " on /mnt/local type fuse.colnk "'
docker exec "$container_id" sh -lc 'test "$(dig +short host.colnk)" = 100.64.0.1'

echo "Agent FUSE, local0, and split DNS checks passed"
