#!/bin/sh
set -eu

source_config=/run/colnk/server.toml
target_config=/etc/colnk/server.toml

if [ -f "$source_config" ]; then
  install -d -m 700 /etc/colnk
  install -m 600 "$source_config" "$target_config"
fi

exec /usr/local/bin/colnk-server "$@"
