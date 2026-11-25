package kernel

import (
	"github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// hunterV7RouterConfig is the single V7Config build (#95 §5.4.1). It was
// duplicated in scoreFromSnapshot and the cold/test-run path; both now call
// this. Every router value comes from store.ResolveHunterV7, so the router,
// the decision record and the Strategy Studio "effective config" view read
// the same resolution. It advances the cycle counter.
func (e *StrategyEngine) hunterV7RouterConfig() local.V7Config {
	resolved := store.ResolveHunterV7(e.config)
	v7cfg := local.DefaultV7Config()
	v7cfg.MinRiskRewardRatio = resolved.MinRR // #77 (unset resolves to 1.2)
	v7cfg.MaxOutput = resolved.MaxOutput      // min(v7_max_output, hunter_limit), <= MaxCandidateCoins (#87 / AUD-A6)
	v7cfg.WatchOutput = resolved.WatchOutput
	v7cfg.MinAIPriority = resolved.MinAIPriority
	v7cfg.Aggressive = resolved.Aggressive
	v7cfg.MinOutput = resolved.MinOutput
	v7cfg.FallbackMinAIPriority = resolved.FallbackMinAIPriority
	v7cfg.CorrelationMaxPerTheme = resolved.CorrelationMaxPerTheme

	coinSource := e.config.CoinSource
	if coinSource.Hunter != nil && coinSource.Hunter.V7T2Enabled {
		v7cfg.T2 = hunterV7T2ModuleConfig(coinSource.Hunter)
		if e.v7T2Store == nil {
			e.v7T2Store = local.NewV7T2EventStore()
		}
		v7cfg.T2Store = e.v7T2Store
		v7cfg.FinalConflict = hunterV7FinalConflictConfig(coinSource.Hunter)
	}
	// U10: closed-bar features for both stacks (opt-in; default forming).
	if coinSource.Hunter != nil {
		v7cfg.ClosedBarFeatures = hunterV7ClosedBarFeatures(coinSource.Hunter)
	}
	e.v7CycleCounter++
	v7cfg.CycleNumber = e.v7CycleCounter
	if e.v7WatchState == nil {
		e.v7WatchState = local.NewV7SignalStateManager()
	}
	v7cfg.WatchStateManager = e.v7WatchState
	return v7cfg
}
