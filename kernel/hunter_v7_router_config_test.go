package kernel

import (
	"testing"

	"github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// #95: the store's router defaults are the provider's DefaultV7Config, so
// resolving through store.ResolveHunterV7 changes no effective value.
func TestResolverRouterDefaultsMatchProvider(t *testing.T) {
	d := local.DefaultV7Config()
	if d.MaxOutput != store.DefaultV7MaxOutput || d.WatchOutput != store.DefaultV7WatchOutput ||
		d.MinAIPriority != store.DefaultV7MinAIPriority || d.MinOutput != store.DefaultV7MinOutput ||
		d.FallbackMinAIPriority != store.DefaultV7FallbackMinAIPriority || d.CorrelationMaxPerTheme != store.DefaultV7CorrelationMaxPerTheme {
		t.Fatalf("store router defaults drifted from DefaultV7Config: %+v", d)
	}
}

// The single router build reproduces the old per-site wiring.
func TestHunterV7RouterConfigFromResolver(t *testing.T) {
	cfg := store.GetDefaultStrategyConfig("en")
	cfg.CoinSource.SourceType = "hunter_v7"
	cfg.CoinSource.HunterLimit = 7
	cfg.CoinSource.Hunter = &store.HunterConfig{V7MaxOutput: 30, V7WatchOutput: 4, V7MinAIPriority: 60, V7Aggressive: true}
	e := NewStrategyEngine(&cfg)
	v := e.hunterV7RouterConfig()
	if v.MaxOutput != 7 || v.WatchOutput != 4 || v.MinAIPriority != 60 || !v.Aggressive || v.MinOutput != 3 ||
		v.FallbackMinAIPriority != 45 || v.CorrelationMaxPerTheme != 3 || v.CycleNumber != 1 || v.WatchStateManager == nil {
		t.Fatalf("router config: %+v", v)
	}
	if v.EffectiveMinRR() != cfg.RiskControl.MinRiskRewardRatio {
		t.Fatalf("min RR %v vs %v", v.EffectiveMinRR(), cfg.RiskControl.MinRiskRewardRatio)
	}
	cfg.CoinSource.HunterLimit = 0
	cfg.CoinSource.Hunter.V7MaxOutput = 0
	if v := e.hunterV7RouterConfig(); v.MaxOutput != store.MaxCandidateCoins || v.CycleNumber != 2 {
		t.Fatalf("unset count must cap at %d: %+v", store.MaxCandidateCoins, v.MaxOutput)
	}
}
