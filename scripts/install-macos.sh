#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
project_dir=$(dirname "$script_dir")
install_dir=${COLNK_INSTALL_DIR:-"$HOME/.local/bin"}

mkdir -p "$install_dir"
cd "$project_dir"
GOTOOLCHAIN=local go build -trimpath -o "$install_dir/colnk" ./cmd/colnk

echo "Installed colnk to $install_dir/colnk"
echo "Start with: COLNK_API_KEY=sk-xxx colnk start --endpoint agent.example.com:7443"
