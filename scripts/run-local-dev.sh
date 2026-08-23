#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
project_dir=$(dirname "$script_dir")

cd "$project_dir"
./scripts/init-dev-secrets.sh >/dev/null
api_key=${COLNK_API_KEY:-$(sed -n '1p' .colnk-dev/api-key)}
mkdir -p bin
GOTOOLCHAIN=local go build -trimpath -o bin/colnk ./cmd/colnk
exec env COLNK_API_KEY="$api_key" COLNK_ACCEPT_RISK=1 bin/colnk start \
  --endpoint localhost:7443 "$@"
