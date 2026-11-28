package store

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/Aixxww/AiT/logger"
)

// Exact C12 and C37 wait payloads from 2026-11-28 live dr.jsonl where the LLM
// emitted a JSON number for trigger.required_close.
const (
	c12LiveWaitJSON = `[{"symbol":"LTCUSDT","action":"wait","blocked_reason_code":"entry_not_in_zone","blocked_signal_symbol":"LTCUSDT","confidence":55,"trigger":{"trigger_price":44.61,"required_close":44.62,"expires_in_bars":6,"action_if_triggered":"review long on 5m close reclaim of zone with taker_buy_15m>=0.52"}}]`
	c37LiveWaitJSON = `[{"symbol":"SLXUSDT","action":"wait","blocked_reason_code":"entry_not_in_zone","blocked_signal_symbol":"SLXUSDT","trigger":{"trigger_price":0.0363363,"required_close":0.0363363,"expires_in_bars":6,"action_if_triggered":"re_check_live_zone_flow_then_open_conservative"}},{"symbol":"BTWUSDT","action":"wait","blocked_reason_code":"confirmation_missing","blocked_signal_symbol":"BTWUSDT"},{"symbol":"OPGUSDT","action":"wait","blocked_reason_code":"confirmation_missing","blocked_signal_symbol":"OPGUSDT"}]`
)

func captureStoreLogs(t *testing.T, fn func()) (string, int64) {
	t.Helper()
	var buf bytes.Buffer
	origOut := logger.Log.Out
	logger.Log.SetOutput(&buf)
	defer logger.Log.SetOutput(origOut)

	before := DecisionTriggerCoercionCount()
	fn()
	delta := DecisionTriggerCoercionCount() - before
	return buf.String(), delta
}

func TestStoreDecisionActionWaitTriggerCoercion_C12_C37(t *testing.T) {
	var c12 []DecisionAction
	logs12, delta12 := captureStoreLogs(t, func() {
		if err := json.Unmarshal([]byte(c12LiveWaitJSON), &c12); err != nil {
			t.Fatalf("C12 wait unmarshal failed: %v", err)
		}
	})
	if len(c12) != 1 || c12[0].Trigger == nil {
		t.Fatalf("C12 unexpected result: %+v", c12)
	}
	if c12[0].Trigger.TriggerPrice != 44.61 || c12[0].Trigger.RequiredClose != "44.62" || c12[0].Trigger.ExpiresInBars != 6 {
		t.Fatalf("C12 trigger mismatch: %+v", c12[0].Trigger)
	}
	if delta12 != 1 {
		t.Fatalf("C12 coercion count delta = %d, want 1", delta12)
	}
	if !strings.Contains(logs12, "[decision_parse_coercion]") ||
		!strings.Contains(logs12, "trigger.required_close") ||
		!strings.Contains(logs12, "from number to string") {
		t.Fatalf("C12 missing expected coercion warning log: %q", logs12)
	}

	var c37 []DecisionAction
	logs37, delta37 := captureStoreLogs(t, func() {
		if err := json.Unmarshal([]byte(c37LiveWaitJSON), &c37); err != nil {
			t.Fatalf("C37 wait unmarshal failed: %v", err)
		}
	})
	if len(c37) != 3 || c37[0].Trigger == nil || c37[0].Trigger.RequiredClose != "0.0363363" {
		t.Fatalf("C37 unexpected result: %+v", c37)
	}
	if delta37 != 1 {
		t.Fatalf("C37 coercion count delta = %d, want 1", delta37)
	}
	if !strings.Contains(logs37, "[decision_parse_coercion]") ||
		!strings.Contains(logs37, "raw=0.0363363") {
		t.Fatalf("C37 missing expected coercion warning log: %q", logs37)
	}

	// Verify DB toRecord() path uses the exact same unmarshal behavior.
	var rec *DecisionRecord
	_, deltaDB := captureStoreLogs(t, func() {
		dbRec := &DecisionRecordDB{Decisions: c12LiveWaitJSON}
		rec = dbRec.toRecord()
	})
	if len(rec.Decisions) != 1 || rec.Decisions[0].Trigger == nil || rec.Decisions[0].Trigger.RequiredClose != "44.62" {
		t.Fatalf("toRecord() failed to decode C12 wait trigger: %+v", rec.Decisions)
	}
	if deltaDB != 1 {
		t.Fatalf("toRecord() coercion count delta = %d, want 1", deltaDB)
	}
}

func TestStoreDecisionActionWaitTriggerBoolAndActionIfTriggered(t *testing.T) {
	payload := `{"symbol":"ETHUSDT","action":"wait","trigger":{"trigger_price":2500.5,"required_close":true,"expires_in_bars":3,"action_if_triggered":1}}`
	var act DecisionAction
	logs, delta := captureStoreLogs(t, func() {
		if err := json.Unmarshal([]byte(payload), &act); err != nil {
			t.Fatalf("wait trigger bool+number unmarshal failed: %v", err)
		}
	})
	if act.Trigger == nil || act.Trigger.RequiredClose != "true" || act.Trigger.ActionIfTriggered != "1" {
		t.Fatalf("unexpected trigger: %+v", act.Trigger)
	}
	if delta != 2 {
		t.Fatalf("coercion count delta = %d, want 2", delta)
	}
	if !strings.Contains(logs, "trigger.required_close from bool to string") ||
		!strings.Contains(logs, "trigger.action_if_triggered from number to string") {
		t.Fatalf("missing expected logs: %q", logs)
	}

	// Normal string fields emit zero warnings and zero coercions.
	normal := `{"symbol":"ETHUSDT","action":"wait","trigger":{"trigger_price":2500.5,"required_close":"5m_or_15m_close_below_trigger","expires_in_bars":3,"action_if_triggered":"open_short"}}`
	var actNormal DecisionAction
	logsNormal, deltaNormal := captureStoreLogs(t, func() {
		if err := json.Unmarshal([]byte(normal), &actNormal); err != nil {
			t.Fatalf("normal wait unmarshal failed: %v", err)
		}
	})
	if deltaNormal != 0 || strings.Contains(logsNormal, "decision_parse_coercion") {
		t.Fatalf("unexpected coercion on normal string trigger: delta=%d logs=%q", deltaNormal, logsNormal)
	}
}

func TestStoreDecisionActionOpenAndNonWaitFailClosed(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"open_long numeric required_close", `{"symbol":"LTCUSDT","action":"open_long","leverage":5,"price":44.61,"trigger":{"trigger_price":44.61,"required_close":44.62}}`},
		{"open_short numeric required_close", `{"symbol":"LTCUSDT","action":"open_short","leverage":5,"price":44.61,"trigger":{"trigger_price":44.61,"required_close":44.62}}`},
		{"open numeric required_close", `{"symbol":"LTCUSDT","action":"open","leverage":5,"price":44.61,"trigger":{"trigger_price":44.61,"required_close":44.62}}`},
		{"open_long bool required_close", `{"symbol":"LTCUSDT","action":"open_long","trigger":{"required_close":true}}`},
		{"open_long numeric action_if_triggered", `{"symbol":"LTCUSDT","action":"open_long","trigger":{"action_if_triggered":123}}`},
		{"hold numeric required_close", `{"symbol":"LTCUSDT","action":"hold","trigger":{"required_close":44.62}}`},
		{"close_long numeric required_close", `{"symbol":"LTCUSDT","action":"close_long","trigger":{"required_close":44.62}}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var act DecisionAction
			logs, delta := captureStoreLogs(t, func() {
				if err := json.Unmarshal([]byte(tc.raw), &act); err == nil {
					t.Fatalf("expected fail-closed error, got nil (act=%+v)", act)
				}
			})
			if delta != 0 || strings.Contains(logs, "decision_parse_coercion") {
				t.Fatalf("non-wait failure must not log/count coercion: delta=%d logs=%q", delta, logs)
			}
		})
	}

	// Standalone DecisionTrigger without action context stays fail-closed.
	var tr DecisionTrigger
	if err := json.Unmarshal([]byte(`{"trigger_price":44.61,"required_close":44.62}`), &tr); err == nil {
		t.Fatalf("standalone DecisionTrigger must stay fail-closed")
	}
}

func TestStoreDecisionActionWaitNonTriggerAndGarbageFailClosed(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		// (c) Non-trigger fields on wait must never be coerced.
		{"wait string in price", `{"symbol":"LTCUSDT","action":"wait","price":"44.61"}`},
		{"wait string in confidence", `{"symbol":"LTCUSDT","action":"wait","confidence":"55"}`},
		{"wait number in reasoning", `{"symbol":"LTCUSDT","action":"wait","reasoning":44.62}`},
		{"wait number in blocked_reason_code", `{"symbol":"LTCUSDT","action":"wait","blocked_reason_code":44.62}`},
		{"wait number in symbol", `{"symbol":12345,"action":"wait"}`},
		{"wait string in trigger_price", `{"symbol":"LTCUSDT","action":"wait","trigger":{"trigger_price":"44.61"}}`},
		{"wait string in expires_in_bars", `{"symbol":"LTCUSDT","action":"wait","trigger":{"expires_in_bars":"6"}}`},

		// (d) Garbage string / malformed trigger content must still fail.
		{"wait garbage string trigger", `{"symbol":"LTCUSDT","action":"wait","trigger":"garbage"}`},
		{"wait garbage string trigger_price", `{"symbol":"LTCUSDT","action":"wait","trigger":{"trigger_price":"garbage"}}`},
		{"wait garbage string expires_in_bars", `{"symbol":"LTCUSDT","action":"wait","trigger":{"expires_in_bars":"garbage"}}`},
		{"wait object in required_close", `{"symbol":"LTCUSDT","action":"wait","trigger":{"required_close":{"k":"v"}}}`},
		{"wait array in required_close", `{"symbol":"LTCUSDT","action":"wait","trigger":{"required_close":[44.62]}}`},
		{"wait partial numeric + invalid action_if_triggered", `{"symbol":"LTCUSDT","action":"wait","trigger":{"required_close":44.62,"action_if_triggered":["garbage"]}}`},
		{"wait unquoted garbage token", `{"symbol":"LTCUSDT","action":"wait","trigger":{"required_close":garbage}}`},
		{"wait overflow number in required_close", `{"symbol":"LTCUSDT","action":"wait","trigger":{"required_close":1e999}}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var act DecisionAction
			logs, delta := captureStoreLogs(t, func() {
				if err := json.Unmarshal([]byte(tc.raw), &act); err == nil {
					t.Fatalf("expected parse error for %s, got nil (act=%+v)", tc.name, act)
				}
			})
			if delta != 0 || strings.Contains(logs, "decision_parse_coercion") {
				t.Fatalf("failed parse must not emit coercion warning/count: delta=%d logs=%q", delta, logs)
			}
		})
	}
}

func TestStoreDecisionActionU8bDuplicateKeysActionEdgeCasesAndSanitize(t *testing.T) {
	failClosedCases := []struct {
		name string
		raw  string
	}{
		// (a) open_long / open_short with duplicate trigger keys must fail closed (pre-U8 behavior).
		{"open_long dup trigger numeric then null", `{"symbol":"LTCUSDT","action":"open_long","leverage":5,"price":44.61,"stop_loss":43.5,"take_profit":47.0,"confidence":80,"trigger":{"required_close":44.62},"trigger":null}`},
		{"open_long dup trigger garbage then valid", `{"symbol":"LTCUSDT","action":"open_long","leverage":5,"price":44.61,"stop_loss":43.5,"take_profit":47.0,"confidence":80,"trigger":"garbage","trigger":{"required_close":"5m"}}`},
		{"open_short dup trigger numeric then null", `{"symbol":"LTCUSDT","action":"open_short","leverage":5,"price":44.61,"trigger":{"required_close":44.62},"trigger":null}`},

		// (b) wait with nested duplicate string fields where an occurrence is invalid.
		{"wait nested dup required_close invalid first", `{"symbol":"LTCUSDT","action":"wait","trigger":{"required_close":["bad"],"required_close":"5m"}}`},
		{"wait nested dup required_close overflow first", `{"symbol":"LTCUSDT","action":"wait","trigger":{"required_close":1e999,"required_close":"5m"}}`},
		{"wait nested dup required_close coerced first invalid second", `{"symbol":"LTCUSDT","action":"wait","trigger":{"required_close":44.62,"required_close":["bad"]}}`},
		{"wait nested dup action_if_triggered invalid first", `{"symbol":"LTCUSDT","action":"wait","trigger":{"action_if_triggered":["bad"],"action_if_triggered":"open_long"}}`},

		// (c) wait with duplicate "trigger" keys where any occurrence is malformed.
		{"wait dup trigger first garbage", `{"symbol":"LTCUSDT","action":"wait","trigger":"garbage","trigger":{"required_close":"5m"}}`},
		{"wait dup trigger first invalid field", `{"symbol":"LTCUSDT","action":"wait","trigger":{"required_close":["bad"]},"trigger":{"required_close":"5m"}}`},
		{"wait dup trigger first coerced second garbage", `{"symbol":"LTCUSDT","action":"wait","trigger":{"required_close":44.62},"trigger":"garbage"}`},

		// (d) action edge cases must fail closed with zero coercion.
		{"action uppercase WAIT", `{"symbol":"LTCUSDT","action":"WAIT","trigger":{"required_close":44.62}}`},
		{"action padded wait", `{"symbol":"LTCUSDT","action":" wait ","trigger":{"required_close":44.62}}`},
		{"action open_long after trigger", `{"symbol":"LTCUSDT","trigger":{"required_close":44.62},"action":"open_long","leverage":5,"price":44.61}`},
		{"duplicate action ending open_long", `{"symbol":"LTCUSDT","action":"wait","action":"open_long","trigger":{"required_close":44.62}}`},
		{"duplicate mixed-case Action ending open_long", `{"symbol":"LTCUSDT","action":"wait","Action":"open_long","trigger":{"required_close":44.62}}`},

		// (e) non-integer float expires_in_bars on wait must fail closed.
		{"wait float expires_in_bars", `{"symbol":"LTCUSDT","action":"wait","trigger":{"trigger_price":44.61,"required_close":"5m","expires_in_bars":3.5}}`},
	}

	for _, tc := range failClosedCases {
		t.Run(tc.name, func(t *testing.T) {
			var act DecisionAction
			logs, delta := captureStoreLogs(t, func() {
				if err := json.Unmarshal([]byte(tc.raw), &act); err == nil {
					t.Fatalf("expected parse error for %s, got nil (act=%+v)", tc.name, act)
				}
			})
			if delta != 0 || strings.Contains(logs, "decision_parse_coercion") {
				t.Fatalf("failed parse must not emit coercion warning/count: delta=%d logs=%q", delta, logs)
			}
		})
	}

	// (b) wait with nested duplicate required_close: first numeric + second plain string -> parses, coercion counted once, last value wins.
	t.Run("wait nested dup required_close numeric then string", func(t *testing.T) {
		raw := `{"symbol":"LTCUSDT","action":"wait","trigger":{"trigger_price":44.61,"required_close":44.62,"required_close":"5m"}}`
		var act DecisionAction
		logs, delta := captureStoreLogs(t, func() {
			if err := json.Unmarshal([]byte(raw), &act); err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
		})
		if act.Trigger == nil || act.Trigger.RequiredClose != "5m" || act.Trigger.TriggerPrice != 44.61 {
			t.Fatalf("unexpected trigger: %+v", act.Trigger)
		}
		if delta != 1 {
			t.Fatalf("coercion count delta = %d, want 1", delta)
		}
		if !strings.Contains(logs, "trigger.required_close from number to string") || !strings.Contains(logs, "raw=44.62") {
			t.Fatalf("missing coercion warning log: %q", logs)
		}
	})

	// (c) wait with duplicate well-formed trigger keys: first coerced + second plain string -> last trigger wins, coercion counted once.
	t.Run("wait dup trigger numeric then plain string", func(t *testing.T) {
		raw := `{"symbol":"LTCUSDT","action":"wait","trigger":{"required_close":44.62},"trigger":{"trigger_price":44.61,"required_close":"5m"}}`
		var act DecisionAction
		logs, delta := captureStoreLogs(t, func() {
			if err := json.Unmarshal([]byte(raw), &act); err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
		})
		if act.Trigger == nil || act.Trigger.RequiredClose != "5m" || act.Trigger.TriggerPrice != 44.61 {
			t.Fatalf("unexpected trigger: %+v", act.Trigger)
		}
		if delta != 1 {
			t.Fatalf("coercion count delta = %d, want 1", delta)
		}
		if !strings.Contains(logs, "raw=44.62") {
			t.Fatalf("missing coercion warning log: %q", logs)
		}
	})

	// (f) symbol containing newline and ANSI escape on coerced wait must be quoted with no raw newline/escape.
	t.Run("wait coerced symbol log injection sanitized", func(t *testing.T) {
		dirtySym := "LTCUSDT\nFAKE [INFO] injected\x1b[31m"
		raw := `{"symbol":"LTCUSDT\nFAKE [INFO] injected\u001b[31m","action":"wait","trigger":{"required_close":44.62}}`
		var act DecisionAction
		logs, delta := captureStoreLogs(t, func() {
			if err := json.Unmarshal([]byte(raw), &act); err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
		})
		if delta != 1 {
			t.Fatalf("coercion count delta = %d, want 1", delta)
		}
		logLine := strings.TrimSuffix(logs, "\n")
		wantQuoted := "symbol=" + strconv.QuoteToASCII(dirtySym)
		if !strings.Contains(logLine, wantQuoted) {
			t.Fatalf("log line missing quoted symbol %s: %q", wantQuoted, logLine)
		}
		if strings.Contains(logLine, "\n") || strings.Contains(logLine, "\r") || strings.Contains(logLine, "\x1b") {
			t.Fatalf("log line leaked raw control characters: %q", logLine)
		}
	})

	// (f) clean symbol and empty symbol (<none>) pass through unquoted.
	t.Run("wait coerced empty symbol uses clean placeholder", func(t *testing.T) {
		raw := `{"action":"wait","trigger":{"required_close":44.62}}`
		var act DecisionAction
		logs, delta := captureStoreLogs(t, func() {
			if err := json.Unmarshal([]byte(raw), &act); err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
		})
		if delta != 1 || !strings.Contains(logs, "symbol=<none> raw=44.62") {
			t.Fatalf("expected unquoted <none> placeholder: delta=%d logs=%q", delta, logs)
		}
	})
}
