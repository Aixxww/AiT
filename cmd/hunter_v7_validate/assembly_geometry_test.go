package main

import (
	"reflect"
	"testing"

	"github.com/Aixxww/AiT/kernel"
	local "github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// Fix 17: the harness assembly uses the full production geometry of the
// prompt's strategy config, so assembled and prompt-final tiers are decided
// with the same floors.

func TestAssemblyGeometryEqualsPromptGeometry(t *testing.T) {
	s := effectiveFixtureStrategy() // stored min RR 3.0, T2 floor 1.3
	s.RiskControl.ClassProfiles = map[string]store.V7ClassRiskProfile{store.V7ClassTrigger: {T2: store.V7StackRiskProfile{MinRiskRewardRatio: 1.6}}}
	s.CoinSource.Hunter.V7ConditionalEntry = &store.V7ConditionalEntryConfig{Enabled: true, Classes: []string{store.V7ClassTrigger}}
	for name, cfg := range map[string]*store.StrategyConfig{"stored": s, "none": nil} {
		prompt := promptStrategyConfig(5, cfg)
		got, want := executionGeometryFromStrategy(cfg), kernel.HunterV7ExecutionGeometryForConfig(&prompt)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: assembly geometry\n%+v\nprompt geometry\n%+v", name, got, want)
		}
	}
	g := executionGeometryFromStrategy(s)
	if g.T2MinRR != 1.3 || g.MinRR != 3.0 || g.ClassProfiles[store.V7ClassTrigger].T2.MinRiskRewardRatio != 1.6 || !g.ConditionalClasses[store.V7ClassTrigger] {
		t.Fatalf("carried values %+v", g)
	}
	if n := executionGeometryFromStrategy(nil); n.MinRR != store.GetDefaultStrategyConfig("zh").RiskControl.MinRiskRewardRatio {
		t.Fatalf("no strategy: native floor %v, want the default config's 1.2 ", n.MinRR)
	}
}

// t2RRSignal is a TRIGGERED T2 LONG whose best target gives RR 1.25 from
// the entry-zone top (the T2 RR reference): entry 100, stop 98, TP1 102.45,
// TP2 102.5 (both inside the TP cap, so the backend check sees RR 1.25).
func t2RRSignal() local.V7SignalOutput {
	return (&local.V7T2Signal{Symbol: "AUSDT", Direction: "LONG", SetupType: "ignition_early_long", Status: "WATCH", Score: 70,
		Trigger: 100, Entry: 100, Stop: 98, Target1: 102.45, Target2: 102.5}).ToV7SignalOutputWithLifecycle("", "TRIGGERED")
}

func assembledReason(t *testing.T, cfg *store.StrategyConfig) string {
	t.Helper()
	c := kernel.AssembleHunterV7CandidateCoins([]local.V7SignalOutput{t2RRSignal()}, "BOTH", executionGeometryFromStrategy(cfg))
	if len(c) != 1 {
		t.Fatalf("candidates %d", len(c))
	}
	return c[0].V7TierReason
}

// The assembled tier decision (backend RR check) follows the carried
// floors: RR 1.25 passes the default T2 floor 1.2 and fails a stored 1.3.
func TestAssembledTierFollowsCarriedT2Floor(t *testing.T) {
	base := store.GetDefaultStrategyConfig("en")
	base.CoinSource.SourceType = "hunter_v7"
	base.CoinSource.Hunter = &store.HunterConfig{V7T2Enabled: true}
	if r := assembledReason(t, &base); r == "backend_rr_infeasible" {
		t.Fatalf("T2 floor 1.2: RR 1.25 must pass, got %s", r)
	}
	strict := base
	strict.RiskControl.T2MinRiskRewardRatio = 1.3
	if r := assembledReason(t, &strict); r != "backend_rr_infeasible" {
		t.Fatalf("stored T2 floor 1.3: RR 1.25 must fail the assembly RR check, got %s", r)
	}
	// The prompt-final tier agrees: same geometry, same verdict.
	pc := promptStrategyConfig(1, &strict)
	pg := kernel.HunterV7ExecutionGeometryForConfig(&pc)
	if c := kernel.AssembleHunterV7CandidateCoins([]local.V7SignalOutput{t2RRSignal()}, "BOTH", pg); c[0].V7TierReason != "backend_rr_infeasible" {
		t.Fatalf("prompt geometry verdict %s", c[0].V7TierReason)
	}
}
