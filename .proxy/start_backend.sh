#!/bin/bash
cd /workspace/AiT || exit 1
# Do NOT `source .env` here: RSA_PRIVATE_KEY is a multiline/quoted PEM and bash
# source corrupts it. The Go binary loads .env via godotenv.
set -a
# Load only simple KEY=VALUE lines safely (skip PEM bodies / continuations)
while IFS= read -r line || [[ -n "$line" ]]; do
  [[ -z "$line" || "$line" =~ ^[[:space:]]*# ]] && continue
  if [[ "$line" =~ ^[A-Za-z_][A-Za-z0-9_]*= ]]; then
    key="${line%%=*}"
    # Skip RSA PEM — leave for godotenv
    [[ "$key" == "RSA_PRIVATE_KEY" ]] && continue
    export "$line" 2>/dev/null || true
  fi
done < /workspace/AiT/.env
set +a
export API_SERVER_PORT=8080
export TZ=Asia/Shanghai
export AIT_PROXY_URL=http://127.0.0.1:10809
export AIT_BINANCE_PROXY_URL=http://127.0.0.1:10809
export HTTP_PROXY=http://127.0.0.1:10809
export HTTPS_PROXY=http://127.0.0.1:10809
export ALL_PROXY=socks5://127.0.0.1:10808
export NO_PROXY=127.0.0.1,localhost,::1
export http_proxy="$HTTP_PROXY" https_proxy="$HTTPS_PROXY" all_proxy="$ALL_PROXY" no_proxy="$NO_PROXY"
# Ensure relative DB_PATH resolves under AiT
export DB_PATH="${DB_PATH:-data/data.db}"
exec /workspace/AiT/bin/ait
