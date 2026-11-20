#!/usr/bin/env bash
# Self-test for scripts/tier_a/supervise.sh and tunnel.sh (TA-4).
# Uses fake harness / ssh binaries in a temp directory; contacts no network
# and starts no real harness. Run: scripts/tier_a/selftest.sh
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
SUP="$HERE/supervise.sh"
TUN="$HERE/tunnel.sh"
W="$(mktemp -d "${TMPDIR:-/tmp}/tier-a-selftest.XXXXXX")"
cleanup() {
	jobs -p | xargs -r kill 2>/dev/null
	pkill -f -- "$W/" 2>/dev/null # anything still running from the temp dir
	rm -rf "$W"
}
trap cleanup EXIT

pass=0 fail=0
ok() { pass=$((pass + 1)); echo "PASS  $*"; }
ko() { fail=$((fail + 1)); echo "FAIL  $*"; }
check() { # check DESCRIPTION COMMAND...
	local d="$1"; shift
	if "$@"; then ok "$d"; else ko "$d"; fi
}
wait_for() { # wait_for SECONDS COMMAND...
	local n=$(($1 * 10)); shift
	while [ "$n" -gt 0 ]; do "$@" && return 0; sleep 0.1; n=$((n - 1)); done
	return 1
}

# Fake harness: behaviour per invocation from $W/plan (one exit code per line),
# records its arguments, optionally writes a TA-3 style state file.
mk_fake_harness() {
	cat >"$W/fake-harness" <<'EOF'
#!/usr/bin/env bash
dir=$(dirname "$0")
n=$(( $(cat "$dir/calls" 2>/dev/null || echo 0) + 1 )); echo "$n" >"$dir/calls"
echo "$*" >>"$dir/args"
state=""; prev=""
for a in "$@"; do [ "$prev" = "--state-file" ] && state="$a"; prev="$a"; done
code=$(sed -n "${n}p" "$dir/plan"); [ -z "$code" ] && code=$(tail -n1 "$dir/plan")
acc=$(( n * 2 ))
if [ -n "$state" ] && [ "${FAKE_WRITE_STATE:-1}" = "1" ]; then
  for i in $(seq $((acc - 1)) $acc); do
    printf '{\n  "schema_version": 1,\n  "last_round": %s,\n  "accepted_rounds": %s\n}\n' "$i" "$i" >"$state.tmp" && mv "$state.tmp" "$state"
    sleep "${FAKE_ROUND_SLEEP:-0}"
  done
fi
sleep "${FAKE_SLEEP:-0}"
exit "$code"
EOF
	chmod +x "$W/fake-harness"
}

reset_run() { rm -rf "$W/out" "$W/calls" "$W/args"; printf '%s\n' "$@" >"$W/plan"; }
sup() { ALLOW_TMP_BIN=1 BACKOFF_BASE=0 HEALTH_POLL=0 HEARTBEAT_POLL=1 HEALTH_CMD=true \
	"$SUP" --bin "$W/fake-harness" --out-dir "$W/out" --run-id t --binance-url http://127.0.0.1:1 "$@"; }

echo "== static checks"
check "supervise.sh is executable" test -x "$SUP"
check "tunnel.sh is executable" test -x "$TUN"
check "bash -n supervise.sh" bash -n "$SUP"
check "bash -n tunnel.sh" bash -n "$TUN"

echo "== supervise.sh: missing binary aborts before any round"
err="$("$SUP" --bin "$W/no-such-harness" --out-dir "$W/out-missing" --run-id t --binance-url http://127.0.0.1:1 --dry-run 2>&1)"; code=$?
check "dry run with a missing binary exits 66" test "$code" = 66
check "dry run error names the missing binary" grep -q "harness binary not found: $W/no-such-harness" <<<"$err"
err="$("$SUP" --bin "$W/no-such-harness" --out-dir "$W/out-missing" --run-id t --binance-url http://127.0.0.1:1 2>&1)"; code=$?
check "real start with a missing binary exits 66" test "$code" = 66
check "  ... and logs that no round started" grep -q "no round started" "$W/out-missing/supervisor-t.log"
check "  ... and writes no heartbeat" test ! -e "$W/out-missing/heartbeat-t.json"
mk_fake_harness
err="$("$SUP" --bin "$W/fake-harness" --out-dir "$W/out-tmp" --run-id t --binance-url http://127.0.0.1:1 --dry-run 2>&1)"; code=$?
check "a binary under /tmp is refused without ALLOW_TMP_BIN" test "$code" = 66
check "  ... with a clear reason" grep -q "persistent path" <<<"$err"
out="$(sup --dry-run 2>&1)"; code=$?
check "dry run with a present binary exits 0" test "$code" = 0
check "  ... and prints the harness command" grep -q -- "--run-id t --state-file $W/out/state-t.json" <<<"$out"

echo "== supervise.sh: flock refuses a second supervisor"
reset_run 0
(FAKE_SLEEP=4 ALLOW_TMP_BIN=1 BACKOFF_BASE=0 HEALTH_POLL=0 HEARTBEAT_POLL=1 HEALTH_CMD=true exec "$SUP" \
	--bin "$W/fake-harness" --out-dir "$W/out" --run-id t --binance-url http://127.0.0.1:1) >/dev/null 2>&1 &
first=$!
wait_for 5 test -e "$W/out/heartbeat-t.json"
err="$(sup 2>&1)"; code=$?
check "second supervisor for the same run exits 75" test "$code" = 75
check "  ... with a clear message" grep -q "another supervisor is already running run t" <<<"$err"
wait "$first"; code=$?
check "first supervisor finishes normally (exit 0)" test "$code" = 0
check "  ... and the harness ran exactly once" test "$(cat "$W/calls")" = 1

echo "== supervise.sh: exit-code decisions"
reset_run 3
sup >/dev/null 2>&1; code=$?
check "harness exit 3 -> supervisor exit 3, no restart" test "$code" = 3 -a "$(cat "$W/calls")" = 1
check "  ... STOPPED_DETERMINISTIC marker written" test -e "$W/out/STOPPED_DETERMINISTIC-t"
reset_run 1
sup >/dev/null 2>&1; code=$?
check "harness exit 1 -> supervisor exit 1, no restart" test "$code" = 1 -a "$(cat "$W/calls")" = 1
reset_run 2 0
rm -f "$W/hc"
HC='n=$(( $(cat '"$W"'/hc 2>/dev/null || echo 0) + 1 )); echo $n > '"$W"'/hc; [ $n -ge 2 ]'
ALLOW_TMP_BIN=1 BACKOFF_BASE=0 HEALTH_POLL=0 HEARTBEAT_POLL=1 HEALTH_CMD="$HC" \
	"$SUP" --bin "$W/fake-harness" --out-dir "$W/out" --run-id t --binance-url http://127.0.0.1:1 -- --rounds 144 >/dev/null 2>&1; code=$?
check "harness exit 2 then 0 -> supervisor exit 0 after one restart" test "$code" = 0 -a "$(cat "$W/calls")" = 2
check "  ... first launch without --resume" test "$(sed -n 1p "$W/args" | grep -c -- --resume)" = 0
check "  ... restart passes --resume and the harness args" grep -q -- "--resume --rounds 144" <(sed -n 2p "$W/args")
check "  ... relay health was checked before the restart (failed once, waited)" grep -q "relay health check failed" "$W/out/supervisor-t.log"
reset_run 2
MAX_RESTARTS_PER_HOUR=2 sup >/dev/null 2>&1; code=$?
check "restart budget: 2 restarts per hour -> stop with exit 4 after 3 launches" test "$code" = 4 -a "$(cat "$W/calls")" = 3
check "  ... STOPPED_RESTART_BUDGET marker written" test -e "$W/out/STOPPED_RESTART_BUDGET-t"
reset_run 0
FAKE_ROUND_SLEEP=1.5 sup >/dev/null 2>&1
check "heartbeat logged on accepted round 1" grep -q "heartbeat: accepted_rounds=1" "$W/out/supervisor-t.log"
check "heartbeat logged on accepted round 2" grep -q "heartbeat: accepted_rounds=2" "$W/out/supervisor-t.log"
check "heartbeat file records the final state" grep -q '"accepted_rounds":2' "$W/out/heartbeat-t.json"
check "backoff is 30 s x 2^n capped at 600 s by default" grep -q 'delay=$((BACKOFF_BASE \* (1 << streak)))' "$SUP"

echo "== tunnel.sh"
cat >"$W/fake-ssh" <<'EOF'
#!/usr/bin/env bash
echo "$*" >>"$(dirname "$0")/ssh-args"
sleep "${FAKE_SSH_SLEEP:-30}"
exit "${FAKE_SSH_EXIT:-0}"
EOF
chmod +x "$W/fake-ssh"
port=18997
tun() { SSH_BIN="$W/fake-ssh" HEALTH_CMD=true HEALTH_POLL=0.2 BACKOFF_BASE=0 "$TUN" --ssh-target relay@example.invalid --local-port "$port" --state-dir "$W/tun" "$@"; }
# Background variant: exec, so $! is the keeper itself and signals reach it.
tun_bg() { (exec env SSH_BIN="$W/fake-ssh" HEALTH_CMD=true HEALTH_POLL=0.2 BACKOFF_BASE=0 "$@" "$TUN" --ssh-target relay@example.invalid --local-port "$port" --state-dir "$W/tun") >/dev/null 2>&1 & }
out="$(tun --dry-run 2>&1)"; code=$?
check "tunnel dry run exits 0 and shows the forward" test "$code" = 0 -a -n "$(grep -- "-L 127.0.0.1:$port:127.0.0.1:8899" <<<"$out")"
err="$(SSH_BIN="$W/no-ssh" "$TUN" --ssh-target x --local-port "$port" --state-dir "$W/tun" 2>&1)"; code=$?
check "missing ssh binary exits 66" test "$code" = 66
tun_bg
first=$!
wait_for 5 grep -q healthy "$W/tun/tunnel-heartbeat-$port.json" 2>/dev/null
check "tunnel heartbeat written while healthy" grep -q '"status":"healthy"' "$W/tun/tunnel-heartbeat-$port.json"
err="$(tun 2>&1)"; code=$?
check "second tunnel keeper for the same port exits 75" test "$code" = 75
check "  ... with a clear message" grep -q "another tunnel keeper already holds local port $port" <<<"$err"
kill -TERM "$first"; wait "$first" 2>/dev/null
check "a stopped keeper logs why it exits (never silent)" grep -q "tunnel keeper exiting: signal" "$W/tun/tunnel-$port.log"
rm -rf "$W/tun"
tun_bg FAKE_SSH_SLEEP=0 FAKE_SSH_EXIT=255 MAX_RECONNECTS_PER_HOUR=2
first=$!
exited=1
wait_for 10 bash -c "! kill -0 $first 2>/dev/null" || exited=0
check "reconnect budget: keeper exits promptly on its own (no signal sent)" test "$exited" = 1
if [ "$exited" = 0 ]; then kill -TERM "$first" 2>/dev/null; fi
wait "$first" 2>/dev/null; code=$?
check "  ... with exit code 4" test "$code" = 4
check "  ... logging STOPPED_RECONNECT_BUDGET" grep -q "STOPPED_RECONNECT_BUDGET: 2 reconnects in the last hour (limit 2); not reconnecting, exit 4" "$W/tun/tunnel-$port.log"
check "  ... and the exit reason" grep -q "tunnel keeper exiting: reconnect budget exhausted (exit 4)" "$W/tun/tunnel-$port.log"
check "  ... no lingering keeper or ssh process" test -z "$(pgrep -f -- "--state-dir $W/tun" ; pgrep -f -- "$W/fake-ssh")"
check "every ssh exit is logged with its code" test "$(grep -c "ssh exited with code 255" "$W/tun/tunnel-$port.log")" -ge 3

echo "== $pass passed, $fail failed"
[ "$fail" -eq 0 ]
