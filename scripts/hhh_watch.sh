#!/usr/bin/env bash
set -euo pipefail

DB_PATH="${DB_PATH:-data/data.db}"
TRADER_MATCH="${TRADER_MATCH:-1780952647}"
INTERVAL_SECONDS="${INTERVAL_SECONDS:-30}"
LOG_PATH="${LOG_PATH:-reports/hhh-monitor-loop-20260609.log}"

mkdir -p "$(dirname "$LOG_PATH")"

query() {
  sqlite3 -noheader -separator ' | ' "$DB_PATH" "$1"
}

latest_decision() {
  query "
    select
      id,
      cycle_number,
      timestamp,
      coalesce(ai_request_duration_ms, 0),
      coalesce(total_tokens, 0),
      replace(replace(substr(decision_json, 1, 260), char(10), ' '), char(13), ' ')
    from decision_records
    where trader_id like '%${TRADER_MATCH}%'
    order by id desc
    limit 1;
  "
}

latest_signals() {
  query "
    select
      coalesce(symbol, ''),
      coalesce(direction, ''),
      coalesce(setup_type, ''),
      coalesce(execution_tier, ''),
      coalesce(tier_reason, ''),
      printf('%.2f', coalesce(ai_priority, 0)),
      printf('%.2f', coalesce(taker_buy_15m, 0)),
      coalesce(blocked_gate, '')
    from hunter_v7_signal_records
    where timestamp = (select max(timestamp) from hunter_v7_signal_records)
      and execution_tier in ('EXECUTABLE', 'REVIEWABLE', 'WATCH')
    order by id desc
    limit 5;
  "
}

latest_orders() {
  query "
    select
      coalesce(id, 0),
      coalesce(symbol, ''),
      coalesce(side, ''),
      coalesce(status, ''),
      coalesce(quantity, ''),
      coalesce(avg_fill_price, ''),
      coalesce(created_at, '')
    from trader_orders
    where trader_id like '%${TRADER_MATCH}%'
    order by id desc
    limit 3;
  "
}

position_count() {
  query "
    select coalesce(status, 'none'), count(*)
    from trader_positions
    where trader_id like '%${TRADER_MATCH}%'
    group by status;
  "
}

while true; do
  now="$(date '+%Y-%m-%dT%H:%M:%S%z')"
  {
    echo "[$now] latest_decision"
    latest_decision || true
    echo "[$now] active_signal_tiers"
    latest_signals || true
    echo "[$now] latest_orders"
    latest_orders || true
    echo "[$now] positions"
    position_count || true
    echo
  } >> "$LOG_PATH"
  sleep "$INTERVAL_SECONDS"
done
