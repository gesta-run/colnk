#!/bin/sh
set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
project_dir=$(dirname "$script_dir")
allowed_port=18080
denied_port=18081
agent_port=18082

cd "$project_dir"
if lsof -nP -iTCP:"$allowed_port" -sTCP:LISTEN >/dev/null 2>&1 || \
   lsof -nP -iTCP:"$denied_port" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Test ports $allowed_port or $denied_port are already in use" >&2
  exit 1
fi

write_test_configs() {
  cat >"$test_state/client.toml" <<EOF
endpoint = "localhost:7443"
root = "/"

[auth]
apiKey = "sk-test-001"

[network]
allowCIDRs = ["100.64.0.1/32"]
allowPorts = [$allowed_port]
dnsSuffixes = ["colnk"]

[logging]
auditResources = false
EOF
  cat >"$test_state/server.toml" <<'EOF'
listen = ":7443"
mountpoint = "/mnt/local"
allowOther = true
metadataCacheTTL = "10s"

[auth]
apiKey = "sk-test-001"

[network]
interface = "local0"
proxyPort = 15001
maxTCPConnections = 256

[dns]
listen = "127.0.0.1:53"
upstream = "1.1.1.1:53"
configureResolver = true
EOF
  chmod 600 "$test_state/client.toml" "$test_state/server.toml"
}

test_state=$(mktemp -d)
export COLNK_CONFIG_DIR="$test_state"
write_test_configs
fixture_file=$(mktemp "$project_dir/.colnk-smoke.XXXXXX")
renamed_file="$fixture_file.renamed"
symlink_file="$fixture_file.link"
client_pid=""
allowed_pid=""
denied_pid=""

cleanup() {
  for process_id in "$client_pid" "$allowed_pid" "$denied_pid"; do
    if [ -n "$process_id" ]; then
      kill "$process_id" 2>/dev/null || true
      wait "$process_id" 2>/dev/null || true
    fi
  done
  if [ -e "$fixture_file" ]; then rm "$fixture_file"; fi
  if [ -e "$renamed_file" ]; then rm "$renamed_file"; fi
  if [ -L "$symlink_file" ]; then rm "$symlink_file"; fi
  docker compose down >/dev/null 2>&1 || true
  rm -r "$test_state"
}
trap cleanup EXIT INT TERM

dump_diagnostics() {
  sed -n '1,200p' "$test_state/client.log" >&2
  docker compose logs --no-color --tail=200 >&2 || true
}

wait_for_mount() {
  maximum_attempts=$1
  attempt=0
  until docker compose exec -T server sh -lc 'mount | grep -q " on /mnt/local type fuse.colnk "' 2>/dev/null; do
    attempt=$((attempt + 1))
    if [ "$attempt" -gt "$maximum_attempts" ] || ! kill -0 "$client_pid" 2>/dev/null; then
      dump_diagnostics
      return 1
    fi
    sleep 0.2
  done
}

wait_for_unmount() {
  maximum_attempts=$1
  attempt=0
  while docker compose exec -T server sh -lc 'mount | grep -q " on /mnt/local type fuse.colnk "' 2>/dev/null; do
    attempt=$((attempt + 1))
    if [ "$attempt" -gt "$maximum_attempts" ]; then
      echo "FUSE mount did not fail after server restart" >&2
      dump_diagnostics
      return 1
    fi
    sleep 0.2
  done
}

verify_container_isolation() {
  server_container=$(docker compose ps -q server)
  if docker inspect -f '{{range .Mounts}}{{println .Destination}}{{end}}' "$server_container" | grep -q '^/mnt/local$'; then
    echo "Server container unexpectedly bind-mounted /mnt/local" >&2
    return 1
  fi
  test "$(docker inspect -f '{{.HostConfig.NetworkMode}}' "$server_container")" != "host"
  test "$(docker inspect -f '{{.HostConfig.Privileged}}' "$server_container")" = "false"
  ./scripts/check-agent.sh >/dev/null
  docker compose exec -T --user agent server test -d /mnt/local
}

verify_filesystem() {
  agent_fixture="/mnt/local$fixture_file"
  docker compose exec -T server sh -c 'printf "agent-write-through-fuse\n" > "$1"; sync "$1"; mv "$1" "$1.renamed"' sh "$agent_fixture"
  test "$(sed -n '1p' "$renamed_file")" = "agent-write-through-fuse"
  printf 'local-cache-invalidation\n' >"$renamed_file"
  test "$(docker compose exec -T server sed -n '1p' "$agent_fixture.renamed")" = "local-cache-invalidation"
  docker compose exec -T server ln -s "$agent_fixture.renamed" "$agent_fixture.link"
  test "$(readlink "$symlink_file")" = "$renamed_file"
  test "$(docker compose exec -T server readlink "$agent_fixture.link")" = "$agent_fixture.renamed"
  docker compose exec -T server rm "$agent_fixture.link"
  docker compose exec -T server rm "$agent_fixture.renamed"
  test ! -e "$renamed_file"
}

verify_network() {
  docker compose exec -T server sh -lc 'test "$(dig +short host.colnk)" = 100.64.0.1'
  docker compose exec -T server sh -lc 'test -n "$(dig +short example.com)"'
  docker compose exec -T server sh -lc "ip route get 1.1.1.1 | grep -q 'dev eth0'"
  if docker compose exec -T server ping -c 1 -W 1 100.64.0.1 >/dev/null 2>&1; then
    echo "ICMP was unexpectedly forwarded through local0" >&2
    return 1
  fi
  if docker compose exec -T server dig +time=1 +tries=1 @100.64.0.1 host.colnk >/dev/null 2>&1; then
    echo "UDP was unexpectedly forwarded through local0" >&2
    return 1
  fi
  docker compose exec -T server sh -lc "curl --fail --silent --max-time 5 http://host.colnk:$allowed_port/README.md | grep -q 'CoLnk connects a remote agent environment'"
  if docker compose exec -T server curl --fail --silent --max-time 2 "http://host.colnk:$denied_port/README.md" >/dev/null 2>&1; then
    echo "Denied TCP port unexpectedly reached the provider host" >&2
    return 1
  fi
  if grep -F '"GET ' "$test_state/denied-http.log" >/dev/null; then
    echo "Denied TCP port reached the local test server" >&2
    return 1
  fi

  docker compose exec -d server sh -lc "printf 'HTTP/1.1 200 OK\r\nContent-Length: 16\r\nConnection: close\r\n\r\nagent-localhost\n' | nc -l -p $agent_port"
  sleep 0.2
  test "$(docker compose exec -T server curl --fail --silent "http://127.0.0.1:$agent_port/")" = "agent-localhost"
  docker compose exec -T server sh -lc "ip link set local0 down; bridge_result=0; if curl --silent --max-time 2 http://host.colnk:$allowed_port/README.md >/dev/null; then bridge_result=1; fi; ip link set local0 up; exit \"\$bridge_result\""

  docker compose exec -T server tc qdisc add dev eth0 root netem delay 50ms loss 1% rate 20mbit
  docker compose exec -T server sh -lc "curl --fail --silent --max-time 10 http://host.colnk:$allowed_port/README.md | grep -q 'CoLnk connects a remote agent environment'"
  docker compose exec -T server tc qdisc del dev eth0 root
}

verify_reconnect() {
  docker compose restart server >/dev/null
  wait_for_unmount 150
  wait_for_mount 150
  docker compose exec -T server sh -lc "curl --fail --silent --max-time 5 http://host.colnk:$allowed_port/README.md | grep -q 'CoLnk connects a remote agent environment'"
}

verify_safe_logging() {
  docker compose logs --no-color >"$test_state/compose.log"
  if grep -F 'sk-test-001' "$test_state/client.log" "$test_state/compose.log" >/dev/null; then
    echo "Credential leaked into logs" >&2
    return 1
  fi
  if grep -F 'agent-write-through-fuse' "$test_state/compose.log" >/dev/null; then
    echo "Server logged plaintext file content" >&2
    return 1
  fi
}

make build-macos >/dev/null
docker compose up -d --build --force-recreate

python3 -m http.server "$allowed_port" --bind 127.0.0.1 >"$test_state/allowed-http.log" 2>&1 &
allowed_pid=$!
python3 -m http.server "$denied_port" --bind 127.0.0.1 >"$test_state/denied-http.log" 2>&1 &
denied_pid=$!

./bin/colnk --config "$test_state/client.toml" \
  >"$test_state/client.log" 2>&1 &
client_pid=$!

wait_for_mount 100

verify_container_isolation
verify_filesystem
verify_network
verify_reconnect
verify_safe_logging

echo "Docker FUSE, transparent TCP, split DNS, isolation, and reconnect tests passed"
