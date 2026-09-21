#!/usr/bin/env bash
# One-shot Hunter v7 scan with DB persist + 1m outcome tracking (no live orders).
# Requires Hy2 proxy (127.0.0.1:10809) for Binance.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

LOG_DIR="${LOG_DIR:-$ROOT/.logs/v7_persist}"
OUT_DIR="${OUT_DIR:-$LOG_DIR/runs}"
DB_PATH="${DB_PATH:-$ROOT/data/data.db}"
STRATEGY_ID="${STRATEGY_ID:-6d80cdf6-8af3-4cc9-adb2-10d5027dd506}"
BIN="${HUNTER_V7_VALIDATE_BIN:-$ROOT/bin/hunter_v7_validate}"

# Conservative defaults: avoid Binance spam; still enough for win-rate sample growth.
ROUNDS="${ROUNDS:-1}"
MAX_WORKERS="${MAX_WORKERS:-6}"
TOP_DETAIL="${TOP_DETAIL:-180}"
MAX_OUTPUT="${MAX_OUTPUT:-30}"
WATCH_OUTPUT="${WATCH_OUTPUT:-8}"
MIN_PRIORITY="${MIN_PRIORITY:-45}"
POST_TRACK_DURATION="${POST_TRACK_DURATION:-8m}"
POST_TRACK_INTERVAL="${POST_TRACK_INTERVAL:-60s}"
TRACK_ACTIVE_ONLY="${TRACK_ACTIVE_ONLY:-true}"

mkdir -p "$LOG_DIR" "$OUT_DIR" "$(dirname "$DB_PATH")"

# Proxy for Binance (do not source full .env — PEM breaks bash).
export AIT_PROXY_URL="${AIT_PROXY_URL:-http://127.0.0.1:10809}"
export AIT_BINANCE_PROXY_URL="${AIT_BINANCE_PROXY_URL:-http://127.0.0.1:10809}"
export HTTP_PROXY="${HTTP_PROXY:-http://127.0.0.1:10809}"
export HTTPS_PROXY="${HTTPS_PROXY:-http://127.0.0.1:10809}"
export ALL_PROXY="${ALL_PROXY:-socks5://127.0.0.1:10808}"
export NO_PROXY="${NO_PROXY:-127.0.0.1,localhost,::1}"
export http_proxy="$HTTP_PROXY" https_proxy="$HTTPS_PROXY" all_proxy="$ALL_PROXY" no_proxy="$NO_PROXY"
export TZ="${TZ:-Asia/Shanghai}"

if [[ ! -x "$BIN" ]]; then
  echo "[persist-once] building $BIN"
  mkdir -p "$ROOT/bin"
  go build -o "$BIN" ./cmd/hunter_v7_validate
fi

STAMP="$(date +%Y%m%d-%H%M%S)"
RUN_OUT="$OUT_DIR/$STAMP"
mkdir -p "$RUN_OUT"

echo "[persist-once] start stamp=$STAMP strategy=$STRATEGY_ID db=$DB_PATH post_track=$POST_TRACK_DURATION"
echo "[persist-once] out=$RUN_OUT"

# Defaults in binary are already persist/track=true; set explicitly for clarity.
set +e
"$BIN" \
  -db "$DB_PATH" \
  -strategy-id "$STRATEGY_ID" \
  -rounds "$ROUNDS" \
  -max-workers "$MAX_WORKERS" \
  -top-detail "$TOP_DETAIL" \
  -max-output "$MAX_OUTPUT" \
  -watch-output "$WATCH_OUTPUT" \
  -min-priority "$MIN_PRIORITY" \
  -persist-signals=true \
  -track-outcomes=true \
  -post-track-duration "$POST_TRACK_DURATION" \
  -post-track-interval "$POST_TRACK_INTERVAL" \
  -track-active-only="$TRACK_ACTIVE_ONLY" \
  -out-dir "$RUN_OUT" \
  >"$RUN_OUT/stdout.log" 2>"$RUN_OUT/stderr.log"
RC=$?
set -e

echo "[persist-once] exit=$RC"
tail -n 40 "$RUN_OUT/stdout.log" || true
if [[ $RC -ne 0 ]]; then
  echo "[persist-once] stderr (tail):"
  tail -n 60 "$RUN_OUT/stderr.log" || true
fi
exit $RC
