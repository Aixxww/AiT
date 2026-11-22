package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
	aittrader "github.com/Aixxww/AiT/trader"
)

// Fix 12: the prompt-final tier is persisted and every report names the
// tier layer it shows.

func tierLayerFixture() ([]kernel.CandidateCoin, []kernel.HunterV7PromptFinalTier) {
	cands := []kernel.CandidateCoin{
		{Symbol: "BTCUSDT", V7SetupType: "trend_breakout_long", V7ExecutionTier: "WATCH", V7TierReason: "confirmation_missing_x"},
		{Symbol: "ETHUSDT", V7SetupType: "funding_reversal", V7ExecutionTier: "REVIEWABLE", V7TierReason: "r"},
		{Symbol: "SOLUSDT", V7SetupType: "t2_ignition_early_long", V7ExecutionTier: "WATCH", V7TierReason: "t2_awaiting_trigger"},
		{Symbol: "HELDUSDT", V7SetupType: "x", V7ExecutionTier: "EXECUTABLE"},
	}
	final := []kernel.HunterV7PromptFinalTier{
		{Symbol: "BTCUSDT", SetupType: "trend_breakout_long", Tier: "REVIEWABLE", TierReason: "live_reviewable_x", AssembledTier: "WATCH", AssembledReason: "confirmation_missing_x"},
		{Symbol: "ETHUSDT", SetupType: "funding_reversal", Tier: "WATCH", TierReason: "prompt_readiness_missing", AssembledTier: "REVIEWABLE", AssembledReason: "r"},
		{Symbol: "SOLUSDT", SetupType: "t2_ignition_early_long", Tier: "WATCH", TierReason: "t2_awaiting_trigger", AssembledTier: "WATCH", AssembledReason: "t2_awaiting_trigger"},
	}
	return cands, final
}

func TestTierLayerReport(t *testing.T) {
	cands, final := tierLayerFixture()
	r := buildTierLayerReport(cands, final)
	if r.AssembledCounts["WATCH"] != 2 || r.AssembledCounts["REVIEWABLE"] != 1 || r.AssembledCounts["EXECUTABLE"] != 1 {
		t.Fatalf("assembled counts %v", r.AssembledCounts)
	}
	if r.PromptFinalCounts["REVIEWABLE"] != 1 || r.PromptFinalCounts["WATCH"] != 2 || r.NotInPrompt != 1 {
		t.Fatalf("prompt-final counts %v not-in-prompt %d", r.PromptFinalCounts, r.NotInPrompt)
	}
	if len(r.Promoted) != 1 || r.Promoted[0].Symbol != "BTCUSDT" || len(r.Demoted) != 1 || r.Demoted[0].Symbol != "ETHUSDT" || r.ReasonChanged != 0 {
		t.Fatalf("promoted %+v demoted %+v", r.Promoted, r.Demoted)
	}
	md := tierLayerMarkdown(r)
	for _, want := range []string{"assembled_tier", "prompt_final_tier", "module_status",
		"| promoted | BTCUSDT | `trend_breakout_long` | WATCH (`confirmation_missing_x`) | REVIEWABLE (`live_reviewable_x`) |",
		"| demoted | ETHUSDT |", "assembled candidates not shown in the prompt: 1"} {
		if !strings.Contains(md, want) {
			t.Fatalf("tier layer markdown lacks %q\n%s", want, md)
		}
	}
}

// Every round-report tier surface names its layer.
func TestRoundReportNamesTierLayers(t *testing.T) {
	cands, final := tierLayerFixture()
	r := validationReport{
		Signals: []local.V7SignalOutput{
			{Symbol: "BTCUSDT", SetupType: "trend_breakout_long", Status: local.V7StatusWaitConfirm},
			{Symbol: "XUSDT", SetupType: "none", Status: local.V7StatusWaitConfirm},
		},
		TierLayers: buildTierLayerReport(cands, final),
	}
	md := formatMarkdown(r, "raw.json")
	for _, want := range []string{
		"| # | Symbol | Dir | Setup | Status (module) | Assembled tier | Prompt-final tier |",
		"| `trend_breakout_long` | `wait_confirm` | WATCH | REVIEWABLE |",
		"| `none` | `wait_confirm` | - | - |",
		"status 分布 [module status]",
		"[assembled tier, re-assembled per signal]",
		"[prompt-final tier, parsed from the prompt]",
		"## Tier layers (fix 12)",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("round report lacks %q", want)
		}
	}
	run := formatRunSummaryMarkdown(validationRunSummary{Round: []validationRoundSummary{{Round: 1}}}, "x.json")
	if !strings.Contains(run, "Open-review (prompt-final)") || !strings.Contains(run, "**prompt-final tier**") {
		t.Fatal("run summary must name the open-review tier layer")
	}
}

// The DB row keeps the assembled execution_tier and gains the
// prompt-final tier; the outcome report shows both.
func TestPersistAndOutcomeReportCarryPromptFinalTier(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "v.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	raw := []local.V7SignalOutput{
		{Symbol: "BTCUSDT", Direction: local.V7DirLong, SetupType: "trend_breakout_long", Status: local.V7StatusWaitConfirm,
			EntryZone: local.V7PriceZone{Lower: 99, Upper: 101}, Invalidation: local.V7InvalidationRule{Price: 97}, TP1Price: 104},
		{Symbol: "SOLUSDT", Direction: local.V7DirLong, SetupType: "t2_ignition_early_long", Status: local.V7StatusWaitConfirm},
	}
	cands := []kernel.CandidateCoin{
		{Symbol: "BTCUSDT", V7SetupType: "trend_breakout_long", V7ExecutionTier: "REVIEWABLE", V7TierReason: "assembled_r"},
		{Symbol: "SOLUSDT", V7SetupType: "t2_ignition_early_long", V7ExecutionTier: "WATCH", V7TierReason: "t2_awaiting_trigger"},
	}
	final := []kernel.HunterV7PromptFinalTier{{Symbol: "BTCUSDT", SetupType: "trend_breakout_long", Tier: "WATCH", TierReason: "prompt_readiness_missing"}}
	tr := aittrader.NewSignalOutcomeTracker(&aittrader.TrackerConfig{TimeoutDuration: 2 * time.Hour, MaxTracked: 10}, nil)
	opts := validationOptions{persistSignals: true, store: st, outcomeTracker: tr}
	if err := persistValidationSignals(opts, 3, raw, cands, final, time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
	var rows []store.HunterV7SignalRecord
	st.GormDB().Order("id").Find(&rows)
	if len(rows) != 2 {
		t.Fatalf("rows %d", len(rows))
	}
	if rows[0].ExecutionTier != "REVIEWABLE" || rows[0].TierReason != "assembled_r" || rows[0].PromptFinalTier != "WATCH" || rows[0].PromptFinalTierReason != "prompt_readiness_missing" {
		t.Fatalf("BTC row %s/%s prompt-final %s/%s", rows[0].ExecutionTier, rows[0].TierReason, rows[0].PromptFinalTier, rows[0].PromptFinalTierReason)
	}
	if rows[1].ExecutionTier != "WATCH" || rows[1].PromptFinalTier != "" {
		t.Fatalf("row not in the prompt: %+v", rows[1])
	}
	// Tracking still follows the assembled tier (REVIEWABLE is tracked).
	if len(tr.GetActive()) != 1 || tr.GetActive()[0].Tier != "REVIEWABLE" {
		t.Fatalf("tracked %+v", tr.GetActive())
	}
	sum := buildValidationOutcomeSummary(opts, time.Now())
	if len(sum.Details) != 1 || sum.Details[0].Tier != "REVIEWABLE" || sum.Details[0].PromptTier != "WATCH" {
		t.Fatalf("outcome details %+v", sum.Details)
	}
	md := formatOutcomeSummaryMarkdown(sum, "o.json")
	if !strings.Contains(md, "| Assembled tier | Prompt-final tier |") || !strings.Contains(md, "| REVIEWABLE | WATCH |") {
		t.Fatalf("outcome markdown:\n%s", md)
	}
}
