#!/bin/sh
set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
project_dir=$(dirname "$script_dir")

cd "$project_dir"
./scripts/init-dev-config.sh >/dev/null
config_dir=${COLNK_CONFIG_DIR:-"$project_dir/.colnk-dev"}
config_path=${COLNK_CLIENT_CONFIG:-"$config_dir/client.toml"}
mkdir -p bin
GOTOOLCHAIN=auto go build -trimpath -o bin/colnk ./cmd/colnk
exec bin/colnk --config "$config_path" "$@"
