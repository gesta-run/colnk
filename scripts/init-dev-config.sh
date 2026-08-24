#!/bin/sh
set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
project_dir=$(dirname "$script_dir")
config_dir=${COLNK_CONFIG_DIR:-"$project_dir/.colnk-dev"}
client_config="$config_dir/client.toml"
server_config="$config_dir/server.toml"

umask 077
mkdir -p "$config_dir"
chmod 700 "$config_dir"

read_api_key() {
  sed -n 's/^[[:space:]]*apiKey[[:space:]]*=[[:space:]]*"\([^"]*\)".*/\1/p' "$1" | sed -n '1p'
}

client_key=""
server_key=""
if [ -s "$client_config" ]; then client_key=$(read_api_key "$client_config"); fi
if [ -s "$server_config" ]; then server_key=$(read_api_key "$server_config"); fi
if { [ -s "$client_config" ] && [ -z "$client_key" ]; } || { [ -s "$server_config" ] && [ -z "$server_key" ]; }; then
  echo "Cannot read auth.apiKey from the existing development configuration" >&2
  exit 1
fi
if [ -n "$client_key" ] && [ -n "$server_key" ] && [ "$client_key" != "$server_key" ]; then
  echo "Client and server development configurations use different auth.apiKey values" >&2
  exit 1
fi
api_key=${client_key:-$server_key}
if [ -z "$api_key" ]; then api_key="sk-dev-$(openssl rand -hex 24)"; fi
if [ ! -s "$client_config" ]; then
  cat >"$client_config" <<EOF
endpoint = "localhost:7443"
root = "/"

[auth]
apiKey = "$api_key"

[network]
allowCIDRs = ["100.64.0.1/32"]
allowPorts = []
dnsSuffixes = ["colnk"]

[logging]
auditResources = false
EOF
fi
if [ ! -s "$server_config" ]; then
  cat >"$server_config" <<EOF
listen = ":7443"
mountpoint = "/mnt/local"
allowOther = true
metadataCacheTTL = "10s"

[auth]
apiKey = "$api_key"

[network]
interface = "local0"
proxyPort = 15001
maxTCPConnections = 256

[dns]
listen = "127.0.0.1:53"
upstream = "1.1.1.1:53"
configureResolver = true
EOF
fi
chmod 600 "$client_config" "$server_config"
echo "Development configuration is ready in $config_dir"
