#!/usr/bin/env bash
# Tier A harness supervisor (TA-4, docs/tier-a-improvement-plan-20261120.md).
# Replaces the ad-hoc run144.sh used in the 2026-11-19 Tier A run.
#
# It runs cmd/hunter_v7_validate under the TA-2 exit codes and the TA-3 state
# file. It never trades and needs no Binance credentials: the harness reads
# public market data through --binance-url only.
#
# Usage:
#   supervise.sh --bin PATH --out-dir DIR --run-id ID --binance-url URL \
#                [--state-file PATH] [--dry-run] [-- HARNESS_ARGS...]
#
# Decisions on harness exit:
#   0        done                         -> exit 0
#   3        deterministic contract stop  -> exit 3, no restart (STOPPED_DETERMINISTIC)
#   1        fatal / config error         -> exit 1, no restart (STOPPED_FATAL)
#   126/127  binary not executable/found  -> exit 66, no restart (STOPPED_ENV)
#   2, kill  transient / crash            -> wait for relay health, back off, restart with --resume
#
# Discipline:
#   - the binary must exist and be executable before every launch, and must
#     not live under /tmp (the Tier A 144-round run died in 40 s because a VM
#     replacement wiped /tmp/hunter_v7_validate: 5 x exit 127);
#   - backoff 30 s x 2^n, capped at 600 s; n resets after 2 accepted rounds;
#   - at most 5 restarts in any rolling hour, then stop (STOPPED_RESTART_BUDGET);
#   - relay health (GET <binance-url>/fapi/v1/time) must pass before a
#     restart; waiting for it does not spend the restart budget;
#   - heartbeat file written on every accepted round (from the TA-3 state file);
#   - flock: one supervisor per run id.
#
# Exit codes of this script: 0 done, 1 harness fatal, 2 usage, 3 harness
# deterministic stop, 4 restart budget exhausted, 66 binary missing,
# 75 another supervisor holds the lock, 130 interrupted.
#
# Tunables (environment): BACKOFF_BASE=30 BACKOFF_CAP=600
# MAX_RESTARTS_PER_HOUR=5 HEALTH_POLL=30 HEARTBEAT_POLL=15
# ALLOW_TMP_BIN=0 HEALTH_CMD (test hook: replaces the curl health check).
set -uo pipefail

BACKOFF_BASE="${BACKOFF_BASE:-30}"
BACKOFF_CAP="${BACKOFF_CAP:-600}"
MAX_RESTARTS_PER_HOUR="${MAX_RESTARTS_PER_HOUR:-5}"
HEALTH_POLL="${HEALTH_POLL:-30}"
HEARTBEAT_POLL="${HEARTBEAT_POLL:-15}"
ALLOW_TMP_BIN="${ALLOW_TMP_BIN:-0}"

BIN="" OUT_DIR="" RUN_ID="" BINANCE_URL="" STATE_FILE="" DRY_RUN=0
HARNESS_ARGS=()

usage() {
	sed -n '2,40p' "$0" | sed 's/^# \{0,1\}//'
	exit 2
}

while [ $# -gt 0 ]; do
	case "$1" in
	--bin) BIN="${2:-}"; shift 2 ;;
	--out-dir) OUT_DIR="${2:-}"; shift 2 ;;
	--run-id) RUN_ID="${2:-}"; shift 2 ;;
	--binance-url) BINANCE_URL="${2:-}"; shift 2 ;;
	--state-file) STATE_FILE="${2:-}"; shift 2 ;;
	--dry-run) DRY_RUN=1; shift ;;
	-h | --help) usage ;;
	--) shift; HARNESS_ARGS=("$@"); break ;;
	*) echo "supervise.sh: unknown argument: $1" >&2; usage ;;
	esac
done

if [ -z "$BIN" ] || [ -z "$OUT_DIR" ] || [ -z "$RUN_ID" ] || [ -z "$BINANCE_URL" ]; then
	echo "supervise.sh: --bin, --out-dir, --run-id and --binance-url are required" >&2
	exit 2
fi
case "$RUN_ID" in *[!A-Za-z0-9._-]*) echo "supervise.sh: --run-id must be [A-Za-z0-9._-]" >&2; exit 2 ;; esac

mkdir -p "$OUT_DIR" || exit 2
STATE_FILE="${STATE_FILE:-$OUT_DIR/state-$RUN_ID.json}"
LOG="$OUT_DIR/supervisor-$RUN_ID.log"
HEARTBEAT="$OUT_DIR/heartbeat-$RUN_ID.json"
LOCK="$OUT_DIR/.supervise-$RUN_ID.lock"

# nap: an interruptible sleep. bash runs a trapped signal only after a
# foreground command ends, so a plain "sleep 3600" would delay shutdown.
NAP_PID=""
nap() {
	sleep "$1" &
	NAP_PID=$!
	wait "$NAP_PID"
	NAP_PID=""
}

log() {
	local line
	line="$(date -u +%Y-%m-%dT%H:%M:%SZ) supervise[$RUN_ID]: $*"
	echo "$line"
	echo "$line" >>"$LOG"
}

stop_with() { # stop_with CODE MARKER MESSAGE
	log "$2: $3"
	printf '%s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$3" >"$OUT_DIR/$2-$RUN_ID"
	exit "$1"
}

# One supervisor per run id.
exec 9>"$LOCK" || exit 2
if ! flock -n 9; then
	echo "supervise.sh: another supervisor is already running run $RUN_ID (lock $LOCK held); refusing to start" >&2
	exit 75
fi

check_bin() {
	if [ ! -e "$BIN" ]; then
		echo "supervise.sh: harness binary not found: $BIN -- refusing to start (rebuild it to a persistent path)" >&2
		return 1
	fi
	if [ ! -f "$BIN" ] || [ ! -x "$BIN" ]; then
		echo "supervise.sh: harness binary is not an executable file: $BIN -- refusing to start" >&2
		return 1
	fi
	case "$(cd "$(dirname "$BIN")" && pwd -P)/" in
	/tmp/* | /var/tmp/*)
		if [ "$ALLOW_TMP_BIN" != "1" ]; then
			echo "supervise.sh: harness binary is under a temp directory ($BIN); a VM replacement can wipe it mid-run -- use a persistent path (or ALLOW_TMP_BIN=1)" >&2
			return 1
		fi
		;;
	esac
	return 0
}

if ! check_bin; then
	[ -d "$OUT_DIR" ] && log "STOPPED_ENV: harness binary missing or unusable: $BIN (no round started)"
	exit 66
fi

harness_cmd() { # prints the command for this launch, one argument per line
	printf '%s\n' "$BIN" --out-dir "$OUT_DIR" --run-id "$RUN_ID" --state-file "$STATE_FILE" --binance-url "$BINANCE_URL"
	if [ -f "$STATE_FILE" ]; then
		printf '%s\n' --resume
	fi
	if [ ${#HARNESS_ARGS[@]} -gt 0 ]; then
		printf '%s\n' "${HARNESS_ARGS[@]}"
	fi
}

if [ "$DRY_RUN" = "1" ]; then
	echo "supervise.sh: dry run OK; would run:"
	harness_cmd | tr '\n' ' '
	echo
	exit 0
fi

relay_healthy() {
	if [ -n "${HEALTH_CMD:-}" ]; then
		bash -c "$HEALTH_CMD" >/dev/null 2>&1
		return $?
	fi
	curl -fsS --max-time 5 -o /dev/null "$BINANCE_URL/fapi/v1/time"
}

state_field() { # state_field NAME -> integer from the TA-3 state file, or 0
	local v=""
	[ -f "$STATE_FILE" ] && v="$(sed -n "s/^ *\"$1\": *\([0-9][0-9]*\).*/\1/p" "$STATE_FILE" | head -n1)"
	echo "${v:-0}"
}

write_heartbeat() { # write_heartbeat PHASE PID
	local tmp="$HEARTBEAT.tmp"
	printf '{"ts":"%s","run_id":"%s","phase":"%s","harness_pid":%s,"last_round":%s,"accepted_rounds":%s}\n' \
		"$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$RUN_ID" "$1" "${2:-0}" "$(state_field last_round)" "$(state_field accepted_rounds)" >"$tmp" &&
		mv -f "$tmp" "$HEARTBEAT"
}

heartbeat_watch() { # heartbeat_watch PID: heartbeat on every accepted round
	local pid="$1" seen
	seen="$(state_field accepted_rounds)"
	while kill -0 "$pid" 2>/dev/null; do
		nap "$HEARTBEAT_POLL"
		local now
		now="$(state_field accepted_rounds)"
		if [ "$now" != "$seen" ]; then
			seen="$now"
			write_heartbeat accepted_round "$pid"
			log "heartbeat: accepted_rounds=$now last_round=$(state_field last_round)"
		fi
	done
}

HARNESS_PID=""
on_signal() {
	log "interrupted; stopping harness ${HARNESS_PID:-none}"
	[ -n "$HARNESS_PID" ] && kill -TERM "$HARNESS_PID" 2>/dev/null
	[ -n "$NAP_PID" ] && kill "$NAP_PID" 2>/dev/null
	exit 130
}
trap on_signal INT TERM

restarts=()   # epoch seconds of restarts in the last hour
streak=0      # restarts since the last 2-accepted-round progress
first_launch=1

while :; do
	if ! check_bin; then
		stop_with 66 STOPPED_ENV "harness binary missing or unusable before launch: $BIN"
	fi
	mapfile -t cmd < <(harness_cmd)
	before="$(state_field accepted_rounds)"
	if [ "$first_launch" = "1" ]; then
		log "start: ${cmd[*]}"
	else
		log "restart $streak: ${cmd[*]}"
	fi
	first_launch=0
	"${cmd[@]}" &
	HARNESS_PID=$!
	write_heartbeat started "$HARNESS_PID"
	heartbeat_watch "$HARNESS_PID" &
	watcher=$!
	wait "$HARNESS_PID"
	code=$?
	kill "$watcher" 2>/dev/null
	wait "$watcher" 2>/dev/null
	write_heartbeat "exited_$code" "$HARNESS_PID"
	HARNESS_PID=""
	after="$(state_field accepted_rounds)"
	log "harness exited with code $code (accepted rounds $before -> $after, last_round $(state_field last_round))"

	case "$code" in
	0) stop_with 0 DONE "run complete" ;;
	3) stop_with 3 STOPPED_DETERMINISTIC "harness reported a deterministic contract failure (exit 3); not restarting -- see the run summary and $OUT_DIR/rejected/" ;;
	1) stop_with 1 STOPPED_FATAL "harness fatal/config error (exit 1); not restarting" ;;
	126 | 127) stop_with 66 STOPPED_ENV "harness could not be executed (exit $code); not restarting" ;;
	esac

	# Resumable: exit 2 (transient) or a crash / kill.
	if [ $((after - before)) -ge 2 ]; then
		streak=0
	fi
	now="$(date +%s)"
	kept=()
	for t in "${restarts[@]+"${restarts[@]}"}"; do
		[ $((now - t)) -lt 3600 ] && kept+=("$t")
	done
	restarts=("${kept[@]+"${kept[@]}"}")
	if [ "${#restarts[@]}" -ge "$MAX_RESTARTS_PER_HOUR" ]; then
		stop_with 4 STOPPED_RESTART_BUDGET "${#restarts[@]} restarts in the last hour (limit $MAX_RESTARTS_PER_HOUR); not restarting"
	fi
	delay=$((BACKOFF_BASE * (1 << streak)))
	[ "$delay" -gt "$BACKOFF_CAP" ] && delay="$BACKOFF_CAP"
	log "resumable exit $code; backing off ${delay}s before restart (restarts in last hour: ${#restarts[@]})"
	nap "$delay"
	until relay_healthy; do
		log "relay health check failed ($BINANCE_URL/fapi/v1/time); waiting ${HEALTH_POLL}s (restart budget not spent)"
		nap "$HEALTH_POLL"
	done
	restarts+=("$(date +%s)")
	streak=$((streak + 1))
done
