package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/kernel"
	local "github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// Fix 15: the run summary states the settings that were in effect.

func effectiveFixtureStrategy() *store.StrategyConfig {
	c := store.GetDefaultStrategyConfig("en")
	c.CoinSource.SourceType = "hunter_v7"
	c.RiskControl.MinRiskRewardRatio = 3.0 // stored old value
	c.RiskControl.T2MinRiskRewardRatio = 1.3
	c.CoinSource.Hunter = &store.HunterConfig{V7T2Enabled: true, V7MinAIPriority: 55,
		V7T2Setups: []string{"ignition_early_long", "ignition_early_short"}}
	return &c
}

// roundCfg mirrors runValidation's router / T2 config for a strategy.
func roundCfg(s *store.StrategyConfig) local.V7Config {
	cfg := local.DefaultV7Config()
	if s != nil && s.CoinSource.Hunter != nil {
		h := s.CoinSource.Hunter
		cfg.MinAIPriority = h.V7MinAIPriority
		if h.V7T2Enabled {
			cfg.T2 = &local.V7T2Config{Enabled: true, EnabledSetups: h.V7T2Setups, Lifecycle: t2LifecycleParams(h.V7T2Lifecycle)}
			cfg.FinalConflict = kernel.HunterV7FinalConflictConfigFor(h)
		}
	}
	return cfg
}

func TestEffectiveFlagsWithStoredStrategy(t *testing.T) {
	s := effectiveFixtureStrategy()
	opts := validationOptions{strategyID: "tierA", topDetail: 220, maxWorkers: 12, twoTier: true, slowEvery: 4, promptReprice: true}
	ef := buildEffectiveFlags(opts, s, roundCfg(s), executionGeometryFromStrategy(s), promptStrategyConfig(3, s))
	if !ef.Strategy.Loaded || ef.Strategy.ConfiguredMinRR != 3.0 || ef.Strategy.NativeMinRRSource != "strategy min_risk_reward_ratio" {
		t.Fatalf("strategy %+v", ef.Strategy)
	}
	// The stored 3.0 was in effect, not the code default 1.2.
	if ef.AssemblyGeometry.NativeMinRR != 3.0 || ef.PromptGeometry.NativeMinRR != 3.0 {
		t.Fatalf("native floor %v / %v", ef.AssemblyGeometry.NativeMinRR, ef.PromptGeometry.NativeMinRR)
	}
	// The harness assembly geometry does not carry the strategy's T2 floor;
	// the prompt geometry does. Both are reported, and the gap is noted.
	if ef.AssemblyGeometry.T2MinRR != store.DefaultT2MinRiskRewardRatio || ef.PromptGeometry.T2MinRR != 1.3 {
		t.Fatalf("T2 floor %v / %v", ef.AssemblyGeometry.T2MinRR, ef.PromptGeometry.T2MinRR)
	}
	notes := strings.Join(ef.Notes, "\n")
	for _, want := range []string{"assembly and prompt geometries differ", "t2_min_risk_reward_ratio 1.30, but the assembly used 1.20",
		"min_risk_reward_ratio (3.00) differs from the code default (1.20)"} {
		if !strings.Contains(notes, want) {
			t.Fatalf("notes lack %q:\n%s", want, notes)
		}
	}
	if !ef.T2.Enabled || len(ef.T2.EnabledSetups) != 2 || ef.T2.EnabledSetupsFrom != "configured list" || ef.Router.MinAIPriority != 55 || !ef.FinalConflict.Enabled {
		t.Fatalf("t2 / router %+v %+v", ef.T2, ef.Router)
	}
	found := false
	for _, x := range ef.AssemblyGeometry.ShadowOnlySetups {
		found = found || x == "ignition_early_short"
	}
	if !found || ef.Harness.StrategyID != "tierA" || !ef.Harness.TwoTier || ef.PromptPayloadVersion == "" {
		t.Fatalf("shadow-only %v harness %+v", ef.AssemblyGeometry.ShadowOnlySetups, ef.Harness)
	}
	md := effectiveFlagsMarkdown(ef, "## Effective settings\n\n")
	for _, want := range []string{"| native RR floor (assembly) | 3.00 (strategy min_risk_reward_ratio; stored value 3.00) |",
		"| T2 RR floor (assembly / prompt) | 1.20 / 1.30", "| T2 module | on |", "ignition_early_short", "> NOTE: "} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown lacks %q\n%s", want, md)
		}
	}
}

// Without a strategy the harness defaults apply, and the 1.5 assembly
// fallback is named as such.
func TestEffectiveFlagsWithoutStrategy(t *testing.T) {
	ef := buildEffectiveFlags(validationOptions{}, nil, roundCfg(nil), executionGeometryFromStrategy(nil), promptStrategyConfig(1, nil))
	if ef.Strategy.Loaded || ef.AssemblyGeometry.NativeMinRR != local.V7MinExecutableRR || !strings.HasPrefix(ef.Strategy.NativeMinRRSource, "unset: kernel fallback 1.50") {
		t.Fatalf("%+v %+v", ef.Strategy, ef.AssemblyGeometry)
	}
	if ef.T2.Enabled || !strings.Contains(strings.Join(ef.Notes, "\n"), "T2 module was off") {
		t.Fatalf("t2 %+v notes %v", ef.T2, ef.Notes)
	}
}

// The run summary carries round 1's resolved settings, the process flags,
// and the rounds that ran with different settings.
func TestRunSummaryEffectiveFlags(t *testing.T) {
	s := effectiveFixtureStrategy()
	opts := validationOptions{strategyID: "tierA", rounds: 3, roundInterval: time.Minute}
	ef1 := buildEffectiveFlags(opts, s, roundCfg(s), executionGeometryFromStrategy(s), promptStrategyConfig(3, s))
	s2 := effectiveFixtureStrategy()
	s2.RiskControl.MinRiskRewardRatio = 1.2 // the row was edited between rounds
	ef3 := buildEffectiveFlags(opts, s2, roundCfg(s2), executionGeometryFromStrategy(s2), promptStrategyConfig(3, s2))
	reports := []validationReport{{Round: 1, EffectiveFlags: ef1}, {Round: 2, EffectiveFlags: ef1}, {Round: 3, EffectiveFlags: ef3}}
	run := buildRunEffectiveFlags(opts, reports)
	if run.FromRound != 1 || run.AssemblyGeometry.NativeMinRR != 3.0 || len(run.RoundsWithDifferentSettings) != 1 || run.RoundsWithDifferentSettings[0] != 3 || run.Harness.Rounds != 3 {
		t.Fatalf("run flags %+v", run)
	}
	summary := buildValidationRunSummary(reports, time.Now())
	summary.EffectiveFlags = run
	md := formatRunSummaryMarkdown(summary, "x.json")
	if !strings.Contains(md, "## Effective settings (fix 15; resolved for round 1)") || !strings.Contains(md, "WARNING: rounds [3] ran with different resolved settings") {
		t.Fatalf("run summary markdown:\n%s", md)
	}
	if none := buildRunEffectiveFlags(opts, nil); none.Harness.StrategyID != "tierA" || len(none.Notes) != 1 {
		t.Fatalf("no reports: %+v", none)
	}
	// The round report shows the section too.
	if !strings.Contains(formatMarkdown(validationReport{EffectiveFlags: ef1}, "r.json"), "## Effective settings (fix 15)") {
		t.Fatal("round report lacks the effective settings section")
	}
}

// The written run summary (JSON and markdown) carries the record.
func TestWrittenRunSummaryHasEffectiveFlags(t *testing.T) {
	dir := t.TempDir()
	s := effectiveFixtureStrategy()
	opts := validationOptions{outDir: dir, strategyID: "tierA"}
	ef := buildEffectiveFlags(opts, s, roundCfg(s), executionGeometryFromStrategy(s), promptStrategyConfig(3, s))
	if err := writeValidationRunSummary(opts, runOutcome{reports: []validationReport{{Round: 1, EffectiveFlags: ef}}}); err != nil {
		t.Fatal(err)
	}
	js, _ := filepath.Glob(filepath.Join(dir, "hunter-v7-validation-run-summary-*.json"))
	mds, _ := filepath.Glob(filepath.Join(dir, "hunter-v7-validation-run-summary-*.md"))
	if len(js) != 1 || len(mds) != 1 {
		t.Fatalf("summary files %v %v", js, mds)
	}
	var got validationRunSummary
	b, _ := os.ReadFile(js[0])
	if err := json.Unmarshal(b, &got); err != nil || got.EffectiveFlags == nil || got.EffectiveFlags.AssemblyGeometry.NativeMinRR != 3.0 || got.EffectiveFlags.Strategy.ConfiguredT2MinRR != 1.3 {
		t.Fatalf("run summary JSON effective_flags %+v (%v)", got.EffectiveFlags, err)
	}
	md, _ := os.ReadFile(mds[0])
	if !strings.Contains(string(md), "| native RR floor (assembly) | 3.00") {
		t.Fatal("run summary markdown lacks the effective settings")
	}
}
