package trader

import (
	"sync/atomic"

	"github.com/Aixxww/AiT/store"
)

// #95 §5.4.1: the trader reads its trading-behaviour values from
// store.ResolveHunterV7. runCycle resolves once per cycle (refreshResolved);
// between cycles every reader (including the protector goroutine) sees the
// same resolution. Without a cycle (tests, startup) it resolves on demand.
type resolvedConfigCache struct {
	p atomic.Pointer[store.ResolvedHunterV7]
}

// refreshResolved resolves the strategy config for this cycle.
func (at *AutoTrader) refreshResolved() store.ResolvedHunterV7 {
	r := store.ResolveHunterV7(at.config.StrategyConfig)
	at.resolvedCfg.p.Store(&r)
	return r
}

// resolved returns this cycle's resolution (or a fresh one).
func (at *AutoTrader) resolved() store.ResolvedHunterV7 {
	if at == nil {
		return store.ResolveHunterV7(nil)
	}
	if r := at.resolvedCfg.p.Load(); r != nil {
		return *r
	}
	return store.ResolveHunterV7(at.config.StrategyConfig)
}
