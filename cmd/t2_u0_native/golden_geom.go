package main

import (
	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/store"
)

// Golden configs and geometry for the U6 golden replay. For the pre-U6
// baseline this file is replaced by an equivalent that builds the geometry
// the way the pre-U6 engine did and returns the default config for
// "explicit_profiles" (the profile fields do not exist there).

type goldenConfig struct {
	name string
	cfg  *store.StrategyConfig
}

func goldenBase() *store.StrategyConfig {
	c := &store.StrategyConfig{}
	c.CoinSource.SourceType = "hunter_v7"
	c.CoinSource.Hunter = &store.HunterConfig{}
	return c
}

func goldenConfigs() []goldenConfig {
	custom := goldenBase()
	custom.RiskControl.MinRiskRewardRatio = 1.3
	custom.RiskControl.MinStopLossPriceMovePct = 1.5
	custom.RiskControl.MaxTakeProfitPriceMovePct = 5
	custom.RiskControl.MaxEntryPriceDeviationPct = 0.4
	custom.RiskControl.T2MinRiskRewardRatio = 1.25
	custom.RiskControl.T2MinStopLossPriceMovePct = 0.6

	// Every class profile written out at the current default values, and
	// the setup overrides written out at their defaults.
	explicit := goldenBase()
	cur := store.V7ClassRiskProfile{
		Native: store.V7StackRiskProfile{MinRiskRewardRatio: 1.5, MinStopLossPriceMovePct: 2.0, MaxTakeProfitPriceMovePct: 4.0},
		T2:     store.V7StackRiskProfile{MinRiskRewardRatio: 1.2, MinStopLossPriceMovePct: 0.5, MaxTakeProfitPriceMovePct: 4.0},
	}
	explicit.RiskControl.ClassProfiles = map[string]store.V7ClassRiskProfile{
		store.V7ClassImpulse: cur, store.V7ClassTrigger: cur, store.V7ClassReversal: cur, store.V7ClassContext: cur,
	}
	explicit.RiskControl.SetupGeometryOverrides = store.DefaultV7SetupGeometryOverrides()
	return []goldenConfig{{"default", goldenBase()}, {"strategy_custom", custom}, {"explicit_profiles", explicit}}
}

func goldenGeometry(cfg *store.StrategyConfig) kernel.HunterV7ExecutionGeometry {
	return kernel.HunterV7ExecutionGeometryForConfig(cfg)
}

func goldenExtra(c kernel.CandidateCoin) float64 { return c.V7RequiredMaxTP }
