#!/usr/bin/env bash
# Tier A SSH tunnel keeper (TA-4, docs/tier-a-improvement-plan-20261120.md).
# Replaces the ad-hoc tunnel144.sh. Keeps a local port forwarded to the
# read-only relay so the harness can use --binance-url http://127.0.0.1:PORT.
#
# Usage:
#   tunnel.sh --ssh-target USER@HOST --local-port 8899 \
#             [--remote-host 127.0.0.1] [--remote-port 8899] [--ssh-key PATH] \
#             [--state-dir DIR] [--dry-run]
#
# Discipline (same as supervise.sh):
#   - flock: one tunnel keeper per local port;
#   - every ssh exit is logged with its code (tunnel144.sh logged "ssh exited
#     code=0" and later vanished with no exit line);
#   - backoff 30 s x 2^n, capped at 600 s; n resets after 10 min of health;
#   - at most 5 reconnects in any rolling hour; when the budget is spent the
#     keeper EXITS with code 4 (STOPPED_RECONNECT_BUDGET, the counterpart of
#     supervise.sh exit 4 STOPPED_RESTART_BUDGET); the supervisor then waits
#     on relay health, so nothing restarts blindly;
#   - health probe every HEALTH_POLL s through the tunnel
#     (GET http://127.0.0.1:PORT/fapi/v1/time); 3 failures in a row force a
#     reconnect; each probe writes the heartbeat file;
#   - an EXIT trap logs why the keeper stops. A SIGKILL or VM replacement
#     cannot be logged; a stale heartbeat file shows it.
#
# Exit codes: 0 stopped by signal, 2 usage, 4 reconnect budget exhausted
# (STOPPED_RECONNECT_BUDGET), 66 ssh missing,
# 75 another keeper holds the port lock, 76 local port already in use.
#
# Tunables (environment): BACKOFF_BASE=30 BACKOFF_CAP=600
# MAX_RECONNECTS_PER_HOUR=5 HEALTH_POLL=30 HEALTH_FAILS=3 HEALTHY_RESET=600
# SSH_BIN=ssh HEALTH_CMD (test hook: replaces the curl probe).
set -uo pipefail

BACKOFF_BASE="${BACKOFF_BASE:-30}"
BACKOFF_CAP="${BACKOFF_CAP:-600}"
MAX_RECONNECTS_PER_HOUR="${MAX_RECONNECTS_PER_HOUR:-5}"
HEALTH_POLL="${HEALTH_POLL:-30}"
HEALTH_FAILS="${HEALTH_FAILS:-3}"
HEALTHY_RESET="${HEALTHY_RESET:-600}"
SSH_BIN="${SSH_BIN:-ssh}"

TARGET="" LPORT="" RHOST="127.0.0.1" RPORT="8899" KEY="" STATE_DIR="" DRY_RUN=0

usage() {
	sed -n '2,32p' "$0" | sed 's/^# \{0,1\}//'
	exit 2
}

while [ $# -gt 0 ]; do
	case "$1" in
	--ssh-target) TARGET="${2:-}"; shift 2 ;;
	--local-port) LPORT="${2:-}"; shift 2 ;;
	--remote-host) RHOST="${2:-}"; shift 2 ;;
	--remote-port) RPORT="${2:-}"; shift 2 ;;
	--ssh-key) KEY="${2:-}"; shift 2 ;;
	--state-dir) STATE_DIR="${2:-}"; shift 2 ;;
	--dry-run) DRY_RUN=1; shift ;;
	-h | --help) usage ;;
	*) echo "tunnel.sh: unknown argument: $1" >&2; usage ;;
	esac
done

if [ -z "$TARGET" ] || [ -z "$LPORT" ]; then
	echo "tunnel.sh: --ssh-target and --local-port are required" >&2
	exit 2
fi
case "$LPORT$RPORT" in *[!0-9]*) echo "tunnel.sh: ports must be numeric" >&2; exit 2 ;; esac

STATE_DIR="${STATE_DIR:-${XDG_STATE_HOME:-$HOME/.local/state}/ait-tier-a}"
mkdir -p "$STATE_DIR" || exit 2
LOG="$STATE_DIR/tunnel-$LPORT.log"
HEARTBEAT="$STATE_DIR/tunnel-heartbeat-$LPORT.json"
LOCK="$STATE_DIR/.tunnel-$LPORT.lock"

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
	line="$(date -u +%Y-%m-%dT%H:%M:%SZ) tunnel[$LPORT]: $*"
	echo "$line"
	echo "$line" >>"$LOG"
}

exec 9>"$LOCK" || exit 2
if ! flock -n 9; then
	echo "tunnel.sh: another tunnel keeper already holds local port $LPORT (lock $LOCK); refusing to start" >&2
	exit 75
fi

if ! command -v "$SSH_BIN" >/dev/null 2>&1; then
	echo "tunnel.sh: ssh binary not found: $SSH_BIN" >&2
	exit 66
fi

port_in_use() { (exec 3<>"/dev/tcp/127.0.0.1/$LPORT") 2>/dev/null; }

ssh_cmd() {
	printf '%s\n' "$SSH_BIN" -N -o ExitOnForwardFailure=yes -o ServerAliveInterval=15 -o ServerAliveCountMax=3 \
		-o BatchMode=yes -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new \
		-L "127.0.0.1:$LPORT:$RHOST:$RPORT"
	[ -n "$KEY" ] && printf '%s\n' -i "$KEY"
	printf '%s\n' "$TARGET"
}

if [ "$DRY_RUN" = "1" ]; then
	echo "tunnel.sh: dry run OK; would run:"
	ssh_cmd | tr '\n' ' '
	echo
	exit 0
fi

if port_in_use; then
	echo "tunnel.sh: local port $LPORT is already in use by another process (orphaned tunnel?); refusing to start" >&2
	exit 76
fi

probe() {
	if [ -n "${HEALTH_CMD:-}" ]; then
		bash -c "$HEALTH_CMD" >/dev/null 2>&1
		return $?
	fi
	curl -fsS --max-time 5 -o /dev/null "http://127.0.0.1:$LPORT/fapi/v1/time"
}

write_heartbeat() { # write_heartbeat STATUS PID
	local tmp="$HEARTBEAT.tmp"
	printf '{"ts":"%s","local_port":%s,"status":"%s","ssh_pid":%s}\n' \
		"$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$LPORT" "$1" "${2:-0}" >"$tmp" && mv -f "$tmp" "$HEARTBEAT"
}

SSH_PID=""
STOP_REASON="unexpected exit"
on_exit() {
	[ -n "$SSH_PID" ] && kill "$SSH_PID" 2>/dev/null
	[ -n "$NAP_PID" ] && kill "$NAP_PID" 2>/dev/null
	write_heartbeat stopped 0
	log "tunnel keeper exiting: $STOP_REASON"
}
trap on_exit EXIT
trap 'STOP_REASON="signal"; exit 0' INT TERM HUP

reconnects=()
streak=0
while :; do
	mapfile -t cmd < <(ssh_cmd)
	log "connecting: ${cmd[*]}"
	"${cmd[@]}" &
	SSH_PID=$!
	started="$(date +%s)"
	fails=0
	while kill -0 "$SSH_PID" 2>/dev/null; do
		nap "$HEALTH_POLL"
		kill -0 "$SSH_PID" 2>/dev/null || break
		if probe; then
			fails=0
			write_heartbeat healthy "$SSH_PID"
			if [ $(($(date +%s) - started)) -ge "$HEALTHY_RESET" ]; then
				streak=0
			fi
		else
			fails=$((fails + 1))
			write_heartbeat "probe_failed_$fails" "$SSH_PID"
			log "health probe failed ($fails/$HEALTH_FAILS)"
			if [ "$fails" -ge "$HEALTH_FAILS" ]; then
				log "tunnel unhealthy; killing ssh $SSH_PID to force a reconnect"
				kill "$SSH_PID" 2>/dev/null
				break
			fi
		fi
	done
	wait "$SSH_PID" 2>/dev/null
	code=$?
	SSH_PID=""
	write_heartbeat "down_exit_$code" 0
	log "ssh exited with code $code"

	now="$(date +%s)"
	kept=()
	for t in "${reconnects[@]+"${reconnects[@]}"}"; do
		[ $((now - t)) -lt 3600 ] && kept+=("$t")
	done
	reconnects=("${kept[@]+"${kept[@]}"}")
	if [ "${#reconnects[@]}" -ge "$MAX_RECONNECTS_PER_HOUR" ]; then
		log "STOPPED_RECONNECT_BUDGET: ${#reconnects[@]} reconnects in the last hour (limit $MAX_RECONNECTS_PER_HOUR); not reconnecting, exit 4"
		STOP_REASON="reconnect budget exhausted (exit 4)"
		exit 4
	fi
	delay=$((BACKOFF_BASE * (1 << streak)))
	[ "$delay" -gt "$BACKOFF_CAP" ] && delay="$BACKOFF_CAP"
	log "reconnecting in ${delay}s"
	nap "$delay"
	reconnects+=("$(date +%s)")
	[ "$streak" -lt 10 ] && streak=$((streak + 1))
done
