#!/bin/sh
set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
project_dir=$(dirname "$script_dir")
install_dir=${COLNK_INSTALL_DIR:-"$HOME/.local/bin"}
config_dir="$HOME/Library/Application Support/CoLnk"

mkdir -p "$install_dir"
cd "$project_dir"
GOTOOLCHAIN=auto go build -trimpath -o "$install_dir/colnk" ./cmd/colnk
mkdir -p "$config_dir"
chmod 700 "$config_dir"
if [ ! -e "$config_dir/client.toml" ]; then
  cp configs/client.toml.example "$config_dir/client.toml"
  chmod 600 "$config_dir/client.toml"
fi

echo "Installed colnk to $install_dir/colnk"
echo "Edit $config_dir/client.toml, then run: colnk"
