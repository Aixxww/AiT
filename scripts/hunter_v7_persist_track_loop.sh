#!/usr/bin/env bash
# Background loop: periodic Hunter v7 persist + outcome track (signal-only, no orders).
# Safe cadence defaults: one scan ~every 20m with ~8m post-track inside each run.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

LOG_DIR="${LOG_DIR:-$ROOT/.logs/v7_persist}"
PID_FILE="${PID_FILE:-$ROOT/.pids/hunter_v7_persist_track.pid}"
LOCK_FILE="${LOCK_FILE:-/tmp/hunter_v7_persist_track.loop.lock}"
INTERVAL_SECONDS="${INTERVAL_SECONDS:-1200}"  # 20 minutes between cycle starts
ONCE_SCRIPT="${ONCE_SCRIPT:-$ROOT/scripts/hunter_v7_persist_track_once.sh}"
# Inside each cycle: shorter post-track so total wall < INTERVAL
export POST_TRACK_DURATION="${POST_TRACK_DURATION:-8m}"
export POST_TRACK_INTERVAL="${POST_TRACK_INTERVAL:-60s}"
export TRACK_ACTIVE_ONLY="${TRACK_ACTIVE_ONLY:-true}"
# Avoid inheriting one-off OUT_DIR from proof runs
unset OUT_DIR || true
export LOG_DIR

mkdir -p "$LOG_DIR" "$(dirname "$PID_FILE")"
LOOP_LOG="${LOOP_LOG:-$LOG_DIR/loop.log}"

log() {
  local ts
  ts="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  printf '%s [v7-persist-loop] %s\n' "$ts" "$*" | tee -a "$LOOP_LOG"
}

exec 9>"$LOCK_FILE"
if ! flock -n 9; then
  log "another loop holds $LOCK_FILE; exiting"
  exit 0
fi

echo $$ >"$PID_FILE"
log "started pid=$$ interval=${INTERVAL_SECONDS}s post_track=$POST_TRACK_DURATION skip_first=${SKIP_FIRST_ONCE:-0}"
trap 'log "stopping pid=$$"; rm -f "$PID_FILE"; exit 0' INT TERM

FIRST=1
while true; do
  start_epoch=$(date +%s)
  if [[ "${SKIP_FIRST_ONCE:-0}" == "1" && "$FIRST" == "1" ]]; then
    log "skipping first once (SKIP_FIRST_ONCE=1); sleeping full interval=${INTERVAL_SECONDS}s"
    FIRST=0
    sleep "$INTERVAL_SECONDS"
    continue
  fi
  FIRST=0
  log "cycle begin"
  set +e
  bash "$ONCE_SCRIPT" >>"$LOOP_LOG" 2>&1
  rc=$?
  set -e
  log "cycle end rc=$rc"

  python3 - <<'PY' >>"$LOOP_LOG" 2>&1 || true
import sqlite3, os
db=os.environ.get("DB_PATH","/workspace/AiT/data/data.db")
con=sqlite3.connect(db)
cur=con.cursor()
n=cur.execute("select count(*) from hunter_v7_signal_records").fetchone()[0]
by=cur.execute("select coalesce(track_status,''), count(*) from hunter_v7_signal_records group by 1 order by 2 desc").fetchall()
openable=cur.execute("select count(*) from hunter_v7_signal_records where execution_tier in ('EXECUTABLE','REVIEWABLE')").fetchone()[0]
tracked=cur.execute("select count(*) from hunter_v7_signal_records where track_current_price!=0 or track_mfe!=0 or track_mae!=0 or track_pnl_pct!=0").fetchone()[0]
print(f"[counts] total={n} openable_tiers={openable} with_track_metrics={tracked} by_track_status={by}")
PY

  elapsed=$(( $(date +%s) - start_epoch ))
  sleep_for=$(( INTERVAL_SECONDS - elapsed ))
  if (( sleep_for < 60 )); then
    sleep_for=60
  fi
  log "sleep ${sleep_for}s before next cycle"
  sleep "$sleep_for"
done
