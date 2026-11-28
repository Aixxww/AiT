package kernel

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/Aixxww/AiT/logger"
	"github.com/Aixxww/AiT/store"
)

const (
	c12LiveWaitResponse = `<reasoning>LTC wait</reasoning><decision>[{"symbol":"LTCUSDT","action":"wait","blocked_reason_code":"entry_not_in_zone","blocked_signal_symbol":"LTCUSDT","confidence":55,"trigger":{"trigger_price":44.61,"required_close":44.62,"expires_in_bars":6,"action_if_triggered":"review long on 5m close reclaim of zone with taker_buy_15m>=0.52"}}]</decision>`
	c37LiveWaitResponse = `<reasoning>SLX/BTW/OPG waits</reasoning><decision>[{"symbol":"SLXUSDT","action":"wait","blocked_reason_code":"entry_not_in_zone","blocked_signal_symbol":"SLXUSDT","trigger":{"trigger_price":0.0363363,"required_close":0.0363363,"expires_in_bars":6,"action_if_triggered":"re_check_live_zone_flow_then_open_conservative"}},{"symbol":"BTWUSDT","action":"wait","blocked_reason_code":"confirmation_missing","blocked_signal_symbol":"BTWUSDT"},{"symbol":"OPGUSDT","action":"wait","blocked_reason_code":"confirmation_missing","blocked_signal_symbol":"OPGUSDT"}]</decision>`
)

func captureKernelLogs(t *testing.T, fn func()) (string, int64) {
	t.Helper()
	var buf bytes.Buffer
	origOut := logger.Log.Out
	logger.Log.SetOutput(&buf)
	defer logger.Log.SetOutput(origOut)

	before := store.DecisionTriggerCoercionCount()
	fn()
	delta := store.DecisionTriggerCoercionCount() - before
	return buf.String(), delta
}

func TestKernelDecisionWaitTriggerCoercion_C12_C37(t *testing.T) {
	var fd12 *FullDecision
	var err error
	logs12, delta12 := captureKernelLogs(t, func() {
		fd12, err = parseFullDecisionResponse(c12LiveWaitResponse, 121.78, 20, 20, 10, 10, nil, true, nil, FixedDecisionRRFloor(1.2), 0)
	})
	if err != nil {
		t.Fatalf("C12 parseFullDecisionResponse failed: %v", err)
	}
	if len(fd12.Decisions) != 1 || fd12.Decisions[0].Action != "wait" || fd12.Decisions[0].ValidatorRejection != "" {
		t.Fatalf("C12 unexpected decision: %+v", fd12.Decisions)
	}
	if fd12.Decisions[0].Trigger == nil || fd12.Decisions[0].Trigger.RequiredClose != "44.62" || fd12.Decisions[0].Trigger.TriggerPrice != 44.61 {
		t.Fatalf("C12 unexpected trigger: %+v", fd12.Decisions[0].Trigger)
	}
	if delta12 != 1 {
		t.Fatalf("C12 coercion count delta = %d, want 1", delta12)
	}
	if !strings.Contains(logs12, "[decision_parse_coercion]") ||
		!strings.Contains(logs12, "trigger.required_close from number to string") ||
		!strings.Contains(logs12, "raw=44.62") {
		t.Fatalf("C12 missing expected coercion warning log: %q", logs12)
	}

	var fd37 *FullDecision
	logs37, delta37 := captureKernelLogs(t, func() {
		fd37, err = parseFullDecisionResponse(c37LiveWaitResponse, 121.78, 20, 20, 10, 10, nil, true, nil, FixedDecisionRRFloor(1.2), 0)
	})
	if err != nil {
		t.Fatalf("C37 parseFullDecisionResponse failed: %v", err)
	}
	if len(fd37.Decisions) != 3 {
		t.Fatalf("C37 decision count = %d, want 3", len(fd37.Decisions))
	}
	if fd37.Decisions[0].Trigger == nil || fd37.Decisions[0].Trigger.RequiredClose != "0.0363363" {
		t.Fatalf("C37 unexpected trigger: %+v", fd37.Decisions[0].Trigger)
	}
	if delta37 != 1 {
		t.Fatalf("C37 coercion count delta = %d, want 1", delta37)
	}
	if !strings.Contains(logs37, "[decision_parse_coercion]") ||
		!strings.Contains(logs37, "raw=0.0363363") {
		t.Fatalf("C37 missing expected coercion warning log: %q", logs37)
	}

	// Also verify fenced-JSON path in extractDecisions.
	fenced := "<decision>\n```json\n" + `[{"symbol":"LTCUSDT","action":"wait","blocked_reason_code":"entry_not_in_zone","trigger":{"trigger_price":44.61,"required_close":44.62}}]` + "\n```\n</decision>"
	var dsFenced []Decision
	_, deltaFenced := captureKernelLogs(t, func() {
		dsFenced, err = extractDecisions(fenced)
	})
	if err != nil || len(dsFenced) != 1 || dsFenced[0].Trigger == nil || dsFenced[0].Trigger.RequiredClose != "44.62" {
		t.Fatalf("fenced extractDecisions failed: err=%v ds=%+v", err, dsFenced)
	}
	if deltaFenced != 1 {
		t.Fatalf("fenced coercion count delta = %d, want 1", deltaFenced)
	}
}

func TestKernelDecisionWaitTriggerBoolAndNormalString(t *testing.T) {
	resp := `<decision>[{"symbol":"BTCUSDT","action":"wait","blocked_reason_code":"confirmation_missing","trigger":{"trigger_price":90000,"required_close":false,"expires_in_bars":2,"action_if_triggered":1}}]</decision>`
	var ds []Decision
	var err error
	logs, delta := captureKernelLogs(t, func() {
		ds, err = extractDecisions(resp)
	})
	if err != nil {
		t.Fatalf("extractDecisions failed: %v", err)
	}
	if len(ds) != 1 || ds[0].Trigger == nil || ds[0].Trigger.RequiredClose != "false" || ds[0].Trigger.ActionIfTriggered != "1" {
		t.Fatalf("unexpected trigger: %+v", ds)
	}
	if delta != 2 {
		t.Fatalf("coercion count delta = %d, want 2", delta)
	}
	if !strings.Contains(logs, "trigger.required_close from bool to string") ||
		!strings.Contains(logs, "trigger.action_if_triggered from number to string") {
		t.Fatalf("missing expected logs: %q", logs)
	}

	// Normal string fields must produce zero warnings.
	normal := `<decision>[{"symbol":"BTCUSDT","action":"wait","blocked_reason_code":"confirmation_missing","trigger":{"trigger_price":90000,"required_close":"5m_or_15m_close_below_trigger","expires_in_bars":2,"action_if_triggered":"open_short"}}]</decision>`
	logsNormal, deltaNormal := captureKernelLogs(t, func() {
		if _, err := extractDecisions(normal); err != nil {
			t.Fatalf("normal wait failed: %v", err)
		}
	})
	if deltaNormal != 0 || strings.Contains(logsNormal, "decision_parse_coercion") {
		t.Fatalf("unexpected coercion on normal string trigger: delta=%d logs=%q", deltaNormal, logsNormal)
	}
}

func TestKernelDecisionOpenAndNonWaitFailClosed(t *testing.T) {
	cases := []struct {
		name string
		json string
	}{
		{"open_long numeric required_close", `[{"symbol":"LTCUSDT","action":"open_long","leverage":5,"position_size_usd":50,"stop_loss":43.5,"take_profit":47.0,"confidence":80,"trigger":{"trigger_price":44.61,"required_close":44.62,"expires_in_bars":6}}]`},
		{"open_short numeric required_close", `[{"symbol":"LTCUSDT","action":"open_short","leverage":5,"position_size_usd":50,"stop_loss":46.0,"take_profit":42.0,"confidence":80,"trigger":{"trigger_price":44.61,"required_close":44.62,"expires_in_bars":6}}]`},
		{"open numeric required_close", `[{"symbol":"LTCUSDT","action":"open","leverage":5,"position_size_usd":50,"trigger":{"trigger_price":44.61,"required_close":44.62}}]`},
		{"open_long bool required_close", `[{"symbol":"LTCUSDT","action":"open_long","leverage":5,"position_size_usd":50,"trigger":{"required_close":true}}]`},
		{"open_long numeric action_if_triggered", `[{"symbol":"LTCUSDT","action":"open_long","leverage":5,"position_size_usd":50,"trigger":{"action_if_triggered":99}}]`},
		{"hold numeric required_close", `[{"symbol":"LTCUSDT","action":"hold","trigger":{"required_close":44.62}}]`},
		{"close_short numeric required_close", `[{"symbol":"LTCUSDT","action":"close_short","trigger":{"required_close":44.62}}]`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := "<decision>" + tc.json + "</decision>"
			logs, delta := captureKernelLogs(t, func() {
				if ds, err := extractDecisions(resp); err == nil {
					t.Fatalf("expected fail-closed error, got nil (ds=%+v)", ds)
				}
			})
			if delta != 0 || strings.Contains(logs, "decision_parse_coercion") {
				t.Fatalf("open/non-wait failure must not log/count coercion: delta=%d logs=%q", delta, logs)
			}
		})
	}

	// Standalone kernel.DecisionTrigger stays fail-closed.
	var tr DecisionTrigger
	if err := json.Unmarshal([]byte(`{"trigger_price":44.61,"required_close":44.62}`), &tr); err == nil {
		t.Fatalf("standalone kernel.DecisionTrigger must stay fail-closed")
	}
}

func TestKernelDecisionWaitNonTriggerAndGarbageFailClosed(t *testing.T) {
	cases := []struct {
		name string
		json string
	}{
		// (c) Non-trigger fields on wait must never be coerced.
		{"wait string in price", `[{"symbol":"LTCUSDT","action":"wait","blocked_reason_code":"entry_not_in_zone","price":"44.61"}]`},
		{"wait string in confidence", `[{"symbol":"LTCUSDT","action":"wait","blocked_reason_code":"entry_not_in_zone","confidence":"55"}]`},
		{"wait number in reasoning", `[{"symbol":"LTCUSDT","action":"wait","blocked_reason_code":"entry_not_in_zone","reasoning":44.62}]`},
		{"wait number in blocked_reason_code", `[{"symbol":"LTCUSDT","action":"wait","blocked_reason_code":44.62}]`},
		{"wait number in symbol", `[{"symbol":123,"action":"wait","blocked_reason_code":"entry_not_in_zone"}]`},
		{"wait string in trigger_price", `[{"symbol":"LTCUSDT","action":"wait","blocked_reason_code":"entry_not_in_zone","trigger":{"trigger_price":"44.61"}}]`},
		{"wait string in expires_in_bars", `[{"symbol":"LTCUSDT","action":"wait","blocked_reason_code":"entry_not_in_zone","trigger":{"expires_in_bars":"6"}}]`},

		// (d) Garbage string / malformed trigger content must still fail.
		{"wait garbage string trigger", `[{"symbol":"LTCUSDT","action":"wait","blocked_reason_code":"entry_not_in_zone","trigger":"garbage"}]`},
		{"wait garbage string trigger_price", `[{"symbol":"LTCUSDT","action":"wait","blocked_reason_code":"entry_not_in_zone","trigger":{"trigger_price":"garbage"}}]`},
		{"wait garbage string expires_in_bars", `[{"symbol":"LTCUSDT","action":"wait","blocked_reason_code":"entry_not_in_zone","trigger":{"expires_in_bars":"garbage"}}]`},
		{"wait object in required_close", `[{"symbol":"LTCUSDT","action":"wait","blocked_reason_code":"entry_not_in_zone","trigger":{"required_close":{"k":"v"}}}]`},
		{"wait array in required_close", `[{"symbol":"LTCUSDT","action":"wait","blocked_reason_code":"entry_not_in_zone","trigger":{"required_close":["garbage"]}}]`},
		{"wait partial numeric + invalid action_if_triggered", `[{"symbol":"LTCUSDT","action":"wait","blocked_reason_code":"entry_not_in_zone","trigger":{"required_close":44.62,"action_if_triggered":{"bad":1}}}]`},
		{"wait unquoted garbage token", `[{"symbol":"LTCUSDT","action":"wait","blocked_reason_code":"entry_not_in_zone","trigger":{"required_close":garbage}}]`},
		{"wait overflow number in required_close", `[{"symbol":"LTCUSDT","action":"wait","blocked_reason_code":"entry_not_in_zone","trigger":{"required_close":1e999}}]`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := "<decision>" + tc.json + "</decision>"
			logs, delta := captureKernelLogs(t, func() {
				if ds, err := extractDecisions(resp); err == nil {
					t.Fatalf("expected parse error for %s, got nil (ds=%+v)", tc.name, ds)
				}
			})
			if delta != 0 || strings.Contains(logs, "decision_parse_coercion") {
				t.Fatalf("failed parse must not emit coercion warning/count: delta=%d logs=%q", delta, logs)
			}
		})
	}

	// (d) Wait with garbage blocked_reason_code still fails hunter_v7 validation.
	garbageCodeResp := `<decision>[{"symbol":"LTCUSDT","action":"wait","blocked_reason_code":"garbage_not_in_enum","trigger":{"trigger_price":44.61,"required_close":44.62}}]</decision>`
	fd, err := parseFullDecisionResponse(garbageCodeResp, 121.78, 20, 20, 10, 10, nil, true, nil, FixedDecisionRRFloor(1.2), 0)
	if err != nil {
		t.Fatalf("parseFullDecisionResponse unexpected error: %v", err)
	}
	if len(fd.Decisions) != 1 || !strings.Contains(fd.Decisions[0].ValidatorRejection, "invalid blocked_reason_code") {
		t.Fatalf("expected invalid blocked_reason_code rejection, got: %+v", fd.Decisions)
	}
}

func TestKernelDecisionU8bDuplicateKeysActionEdgeCasesAndSanitize(t *testing.T) {
	failClosedCases := []struct {
		name string
		json string
	}{
		// (a) open_long / open_short with duplicate trigger keys must fail closed (pre-U8 behavior).
		{"open_long dup trigger numeric then null", `[{"symbol":"LTCUSDT","action":"open_long","leverage":5,"position_size_usd":50,"stop_loss":43.5,"take_profit":47.0,"confidence":80,"trigger":{"required_close":44.62},"trigger":null}]`},
		{"open_long dup trigger garbage then valid", `[{"symbol":"LTCUSDT","action":"open_long","leverage":5,"position_size_usd":50,"stop_loss":43.5,"take_profit":47.0,"confidence":80,"trigger":"garbage","trigger":{"required_close":"5m"}}]`},
		{"open_short dup trigger numeric then null", `[{"symbol":"LTCUSDT","action":"open_short","leverage":5,"position_size_usd":50,"trigger":{"required_close":44.62},"trigger":null}]`},

		// (b) wait with nested duplicate string fields where an occurrence is invalid.
		{"wait nested dup required_close invalid first", `[{"symbol":"LTCUSDT","action":"wait","blocked_reason_code":"entry_not_in_zone","trigger":{"required_close":["bad"],"required_close":"5m"}}]`},
		{"wait nested dup required_close overflow first", `[{"symbol":"LTCUSDT","action":"wait","blocked_reason_code":"entry_not_in_zone","trigger":{"required_close":1e999,"required_close":"5m"}}]`},
		{"wait nested dup required_close coerced first invalid second", `[{"symbol":"LTCUSDT","action":"wait","blocked_reason_code":"entry_not_in_zone","trigger":{"required_close":44.62,"required_close":["bad"]}}]`},
		{"wait nested dup action_if_triggered invalid first", `[{"symbol":"LTCUSDT","action":"wait","blocked_reason_code":"entry_not_in_zone","trigger":{"action_if_triggered":["bad"],"action_if_triggered":"open_long"}}]`},

		// (c) wait with duplicate "trigger" keys where any occurrence is malformed.
		{"wait dup trigger first garbage", `[{"symbol":"LTCUSDT","action":"wait","blocked_reason_code":"entry_not_in_zone","trigger":"garbage","trigger":{"required_close":"5m"}}]`},
		{"wait dup trigger first invalid field", `[{"symbol":"LTCUSDT","action":"wait","blocked_reason_code":"entry_not_in_zone","trigger":{"required_close":["bad"]},"trigger":{"required_close":"5m"}}]`},
		{"wait dup trigger first coerced second garbage", `[{"symbol":"LTCUSDT","action":"wait","blocked_reason_code":"entry_not_in_zone","trigger":{"required_close":44.62},"trigger":"garbage"}]`},

		// (d) action edge cases must fail closed with zero coercion.
		{"action uppercase WAIT", `[{"symbol":"LTCUSDT","action":"WAIT","trigger":{"required_close":44.62}}]`},
		{"action padded wait", `[{"symbol":"LTCUSDT","action":" wait ","trigger":{"required_close":44.62}}]`},
		{"action open_long after trigger", `[{"symbol":"LTCUSDT","trigger":{"required_close":44.62},"action":"open_long","leverage":5,"position_size_usd":50}]`},
		{"duplicate action ending open_long", `[{"symbol":"LTCUSDT","action":"wait","action":"open_long","trigger":{"required_close":44.62}}]`},
		{"duplicate mixed-case Action ending open_long", `[{"symbol":"LTCUSDT","action":"wait","Action":"open_long","trigger":{"required_close":44.62}}]`},

		// (e) non-integer float expires_in_bars on wait must fail closed.
		{"wait float expires_in_bars", `[{"symbol":"LTCUSDT","action":"wait","blocked_reason_code":"entry_not_in_zone","trigger":{"trigger_price":44.61,"required_close":"5m","expires_in_bars":3.5}}]`},
	}

	for _, tc := range failClosedCases {
		t.Run(tc.name, func(t *testing.T) {
			resp := "<decision>" + tc.json + "</decision>"
			logs, delta := captureKernelLogs(t, func() {
				if ds, err := extractDecisions(resp); err == nil {
					t.Fatalf("expected parse error for %s, got nil (ds=%+v)", tc.name, ds)
				}
			})
			if delta != 0 || strings.Contains(logs, "decision_parse_coercion") {
				t.Fatalf("failed parse must not emit coercion warning/count: delta=%d logs=%q", delta, logs)
			}
		})
	}

	// (b) wait with nested duplicate required_close: first numeric + second plain string -> parses, coercion counted once, last value wins.
	t.Run("wait nested dup required_close numeric then string", func(t *testing.T) {
		resp := `<decision>[{"symbol":"LTCUSDT","action":"wait","blocked_reason_code":"entry_not_in_zone","trigger":{"trigger_price":44.61,"required_close":44.62,"required_close":"5m"}}]</decision>`
		var ds []Decision
		var err error
		logs, delta := captureKernelLogs(t, func() {
			ds, err = extractDecisions(resp)
		})
		if err != nil {
			t.Fatalf("unexpected parse error: %v", err)
		}
		if len(ds) != 1 || ds[0].Trigger == nil || ds[0].Trigger.RequiredClose != "5m" || ds[0].Trigger.TriggerPrice != 44.61 {
			t.Fatalf("unexpected trigger: %+v", ds)
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
		resp := `<decision>[{"symbol":"LTCUSDT","action":"wait","blocked_reason_code":"entry_not_in_zone","trigger":{"required_close":44.62},"trigger":{"trigger_price":44.61,"required_close":"5m"}}]</decision>`
		var ds []Decision
		var err error
		logs, delta := captureKernelLogs(t, func() {
			ds, err = extractDecisions(resp)
		})
		if err != nil {
			t.Fatalf("unexpected parse error: %v", err)
		}
		if len(ds) != 1 || ds[0].Trigger == nil || ds[0].Trigger.RequiredClose != "5m" || ds[0].Trigger.TriggerPrice != 44.61 {
			t.Fatalf("unexpected trigger: %+v", ds)
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
		resp := `<decision>[{"symbol":"LTCUSDT\nFAKE [INFO] injected\u001b[31m","action":"wait","blocked_reason_code":"entry_not_in_zone","trigger":{"required_close":44.62}}]</decision>`
		var ds []Decision
		var err error
		logs, delta := captureKernelLogs(t, func() {
			ds, err = extractDecisions(resp)
		})
		if err != nil || len(ds) != 1 {
			t.Fatalf("unexpected parse error: err=%v ds=%+v", err, ds)
		}
		if delta != 1 {
			t.Fatalf("coercion count delta = %d, want 1", delta)
		}
		wantQuoted := "symbol=" + strconv.QuoteToASCII(dirtySym)
		var coercionLine string
		for _, line := range strings.Split(strings.TrimSuffix(logs, "\n"), "\n") {
			if strings.HasPrefix(line, "FAKE ") {
				t.Fatalf("log injection forged a new log line: %q", logs)
			}
			if strings.Contains(line, "[decision_parse_coercion]") {
				coercionLine = line
			}
		}
		if coercionLine == "" || !strings.Contains(coercionLine, wantQuoted) {
			t.Fatalf("coercion log line missing quoted symbol %s: %q", wantQuoted, logs)
		}
		if strings.Contains(logs, "\r") || strings.Contains(logs, "\x1b") || strings.Contains(logs, "\nFAKE") {
			t.Fatalf("log output leaked raw control characters or injected newline: %q", logs)
		}
	})

	// (f) clean symbol and empty symbol (<none>) pass through unquoted.
	t.Run("wait coerced empty symbol uses clean placeholder", func(t *testing.T) {
		resp := `<decision>[{"action":"wait","blocked_reason_code":"entry_not_in_zone","trigger":{"required_close":44.62}}]</decision>`
		var err error
		logs, delta := captureKernelLogs(t, func() {
			_, err = extractDecisions(resp)
		})
		if err != nil {
			t.Fatalf("unexpected parse error: %v", err)
		}
		if delta != 1 || !strings.Contains(logs, "symbol=<none> raw=44.62") {
			t.Fatalf("expected unquoted <none> placeholder: delta=%d logs=%q", delta, logs)
		}
	})
}
