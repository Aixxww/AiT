#!/usr/bin/env bash
# Control hunter_v7 persist/track background loop: start|stop|status
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PID_FILE="${PID_FILE:-$ROOT/.pids/hunter_v7_persist_track.pid}"
LOG_DIR="${LOG_DIR:-$ROOT/.logs/v7_persist}"
LOOP="${LOOP:-$ROOT/scripts/hunter_v7_persist_track_loop.sh}"
LOCK_FILE="${LOCK_FILE:-/tmp/hunter_v7_persist_track.loop.lock}"

cmd="${1:-status}"
case "$cmd" in
  start)
    if [[ -f "$PID_FILE" ]] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
      echo "already running pid=$(cat "$PID_FILE")"
      exit 0
    fi
    mkdir -p "$LOG_DIR" "$(dirname "$PID_FILE")"
    # Explicit production defaults; do not inherit proof-run OUT_DIR/3m overrides.
    setsid nohup env -u OUT_DIR \
      POST_TRACK_DURATION="${POST_TRACK_DURATION:-8m}" \
      POST_TRACK_INTERVAL="${POST_TRACK_INTERVAL:-60s}" \
      TRACK_ACTIVE_ONLY="${TRACK_ACTIVE_ONLY:-true}" \
      INTERVAL_SECONDS="${INTERVAL_SECONDS:-1200}" \
      MAX_WORKERS="${MAX_WORKERS:-6}" \
      TOP_DETAIL="${TOP_DETAIL:-180}" \
      MAX_OUTPUT="${MAX_OUTPUT:-30}" \
      WATCH_OUTPUT="${WATCH_OUTPUT:-8}" \
      MIN_PRIORITY="${MIN_PRIORITY:-45}" \
      SKIP_FIRST_ONCE="${SKIP_FIRST_ONCE:-0}" \
      LOG_DIR="$LOG_DIR" \
      PID_FILE="$PID_FILE" \
      bash "$LOOP" >>"$LOG_DIR/loop.nohup.log" 2>&1 &
    sleep 1
    if [[ -f "$PID_FILE" ]] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
      echo "started pid=$(cat "$PID_FILE")"
    else
      echo "started (check $LOG_DIR/loop.log)"; pgrep -af hunter_v7_persist_track_loop || true
    fi
    ;;
  stop)
    if [[ -f "$PID_FILE" ]]; then
      pid=$(cat "$PID_FILE")
      kill "$pid" 2>/dev/null || true
      pkill -P "$pid" 2>/dev/null || true
      # belt-and-suspenders for validate child
      pkill -f '/workspace/AiT/bin/hunter_v7_validate' 2>/dev/null || true
      rm -f "$PID_FILE"
      echo "stopped $pid"
    else
      pkill -f 'hunter_v7_persist_track_loop.sh' 2>/dev/null || true
      pkill -f '/workspace/AiT/bin/hunter_v7_validate' 2>/dev/null || true
      echo "no pid file; attempted pkill"
    fi
    ;;
  status)
    if [[ -f "$PID_FILE" ]] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
      echo "running pid=$(cat "$PID_FILE")"
    else
      echo "not running"
    fi
    pgrep -af 'hunter_v7_persist_track|hunter_v7_validate' || true
    python3 - <<'PY' || true
import sqlite3
con=sqlite3.connect('/workspace/AiT/data/data.db')
n=con.execute('select count(*) from hunter_v7_signal_records').fetchone()[0]
print('hunter_v7_signal_records=', n)
rows=con.execute("select coalesce(track_status,'(null)'), count(*) from hunter_v7_signal_records group by 1").fetchall()
print('by_track_status=', rows)
openable=con.execute("select count(*) from hunter_v7_signal_records where execution_tier in ('EXECUTABLE','REVIEWABLE')").fetchone()[0]
metrics=con.execute("select count(*) from hunter_v7_signal_records where track_current_price!=0 or track_mfe!=0 or track_mae!=0 or track_pnl_pct!=0").fetchone()[0]
print('openable_tiers=', openable, 'with_track_metrics=', metrics)
PY
    ;;
  *)
    echo "usage: $0 {start|stop|status}"; exit 2
    ;;
esac
