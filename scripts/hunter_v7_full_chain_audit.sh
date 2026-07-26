#!/usr/bin/env bash
# Hunter v7 lean-core refactor — full-chain audit gate.
# Runs every verification layer from the redesign plan (§7) in order and
# stops at the first failure. Usage: bash scripts/hunter_v7_full_chain_audit.sh [--live]
#   --live  additionally runs one live hunter_v7_validate round (needs network).
set -euo pipefail
cd "$(dirname "$0")/.."

step() { printf '\n\033[1m== %s\033[0m\n' "$*"; }

step "1/7 gofmt"
UNFORMATTED=$(gofmt -l kernel/ provider/ trader/ api/ store/ cmd/ engine/ datafetch/ 2>/dev/null || true)
if [ -n "$UNFORMATTED" ]; then echo "unformatted files:"; echo "$UNFORMATTED"; exit 1; fi
echo ok

step "2/7 go build ./..."
go build ./...

step "3/7 go vet ./..."
go vet ./...

step "4/7 golden replay (provider signal layer)"
go test ./provider/local/ -run 'TestHunterV7Golden' -count=1

step "5/7 golden prompt (kernel layer)"
go test ./kernel/ -run 'TestHunterV7GoldenPrompt' -count=1

step "6/7 full test suite"
go test ./...

step "7/7 coverage snapshot (kernel / provider / trader)"
go test -cover ./kernel/ ./provider/local/ ./trader/ | tee /tmp/hunter_v7_audit_cover.txt

if [ "${1:-}" = "--live" ]; then
  step "LIVE hunter_v7_validate round"
  OUT="reports/hunter-v7-audit-$(date +%Y%m%d-%H%M%S)"
  mkdir -p "$OUT"
  go run ./cmd/hunter_v7_validate -rounds 1 -max-output 160 -watch-output 60 -min-priority 20 -out-dir "$OUT"
  echo "live report: $OUT"
fi

printf '\n\033[1mAUDIT PASSED\033[0m\n'
