#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
project_dir=$(dirname "$script_dir")
secret_dir=${COLNK_SECRETS_DIR:-"$project_dir/.colnk-dev"}

umask 077
mkdir -p "$secret_dir"
if [ ! -s "$secret_dir/api-key" ]; then
  printf 'sk-dev-%s\n' "$(openssl rand -hex 24)" >"$secret_dir/api-key"
fi
echo "Development API key is ready in $secret_dir"
