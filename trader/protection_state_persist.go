package trader

// #93 / audit cluster 4 (D4, D5): the protector's per-position state lives
// in the #92 lifecycle record (no second record, no second lock).
//
//   - D4: TP0/TP1/TP2/giveback/trail flags, the peak PnL and the first-seen
//     time are written to the position's lifecycle row when they change and
//     loaded when the protector first sees the position after a start. The
//     peak is no longer rebuilt from the last 20 prompts.
//   - D5: the take-profit to re-place after a partial comes from the record,
//     falling back to the exchange's open orders.

import (
	"strings"
	"sync"
	"time"

	"github.com/Aixxww/AiT/logger"
	"github.com/Aixxww/AiT/store"
)

// lifecycleRowIDs caches posKey -> lifecycle row id for the protector.
type lifecycleRowCache struct {
	mu  sync.Mutex
	ids map[string]uint
}

func (c *lifecycleRowCache) get(key string) (uint, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id, ok := c.ids[key]
	return id, ok
}

func (c *lifecycleRowCache) set(key string, id uint) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ids == nil {
		c.ids = map[string]uint{}
	}
	c.ids[key] = id
}

func (c *lifecycleRowCache) drop(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.ids, key)
}

// positionLifecycleRow returns the open (or intent) lifecycle row of a
// position. When create is set and no row exists (a position opened before
// the record existed, or outside the bot), an "open" row is adopted for it.
func (at *AutoTrader) positionLifecycleRow(symbol, side string, create bool) *store.PositionLifecycle {
	ds := at.lifecycleStore()
	if ds == nil || symbol == "" || side == "" {
		return nil
	}
	key := symbol + "_" + side
	if id, ok := at.lifecycleRows.get(key); ok {
		if rec, err := ds.GetLifecycle(id); err == nil && (rec.Status == store.LifecycleStatusOpen || rec.Status == store.LifecycleStatusIntent) {
			return rec
		}
		at.lifecycleRows.drop(key)
	}
	rec, err := ds.LatestLifecycle(at.id, symbol, side, store.LifecycleStatusOpen, store.LifecycleStatusIntent)
	if err != nil {
		logger.Warnf("  ⚠ [LIFECYCLE] %s %s protection state lookup failed: %v", symbol, side, err)
		return nil
	}
	if rec == nil && create {
		rec = &store.PositionLifecycle{TraderID: at.id, Symbol: symbol, Side: side, Status: store.LifecycleStatusOpen,
			Note: "adopted by the protector (no open record)"}
		if err := ds.CreateLifecycle(rec); err != nil {
			logger.Warnf("  ⚠ [LIFECYCLE] %s %s protection row not created: %v", symbol, side, err)
			return nil
		}
	}
	if rec != nil {
		at.lifecycleRows.set(key, rec.ID)
	}
	return rec
}

// applyPersistedProtection loads the persisted flags, peak and first-seen
// time into a freshly created protection state.
func applyPersistedProtection(state *positionProtectionState, rec *store.PositionLifecycle) {
	if state == nil || rec == nil {
		return
	}
	state.TP0Done = state.TP0Done || rec.TP0Done
	state.TP1Done = state.TP1Done || rec.TP1Done
	state.TP2Done = state.TP2Done || rec.TP2Done
	state.GivebackDone = state.GivebackDone || rec.GivebackDone
	state.TrailDone = state.TrailDone || rec.TrailDone
	if rec.PeakPnLPct > state.PeakPnLPct {
		state.PeakPnLPct = rec.PeakPnLPct
	}
	if rec.InitialQuantity > state.InitialQuantity {
		state.InitialQuantity = rec.InitialQuantity
	}
	if rec.FirstSeenAt > 0 {
		if t := msTime(rec.FirstSeenAt); state.OpenedAt.IsZero() || t.Before(state.OpenedAt) {
			state.OpenedAt = t
		}
	}
	if state.ExchangeTakeProfit <= 0 && rec.TakeProfit > 0 {
		state.ExchangeTakeProfit = rec.TakeProfit // D5
	}
}

// persistProtectionState writes the state's flags, peak and initial
// quantity to the position's lifecycle row.
func (at *AutoTrader) persistProtectionState(symbol, side string, state *positionProtectionState) {
	if state == nil {
		return
	}
	rec := at.positionLifecycleRow(symbol, side, true)
	if rec == nil {
		return
	}
	at.updateLifecycle(rec, map[string]interface{}{
		"tp0_done":         state.TP0Done,
		"tp1_done":         state.TP1Done,
		"tp2_done":         state.TP2Done,
		"giveback_done":    state.GivebackDone,
		"trail_done":       state.TrailDone,
		"peak_pnl_pct":     state.PeakPnLPct,
		"initial_quantity": state.InitialQuantity,
	})
}

// persistPeakPnL writes a new peak (only increases are written).
func (at *AutoTrader) persistPeakPnL(symbol, side string, peak float64) {
	rec := at.positionLifecycleRow(symbol, side, true)
	if rec == nil || peak <= rec.PeakPnLPct {
		return
	}
	at.updateLifecycle(rec, map[string]interface{}{"peak_pnl_pct": peak})
}

// persistedPeakPnL is the stored peak of a position (0 = none).
func (at *AutoTrader) persistedPeakPnL(symbol, side string) float64 {
	if rec := at.positionLifecycleRow(symbol, side, false); rec != nil {
		return rec.PeakPnLPct
	}
	return 0
}

// persistedFirstSeen returns the stored first-seen time (ms), or records
// firstSeenMs as the first-seen time when none is stored.
func (at *AutoTrader) persistedFirstSeen(symbol, side string, firstSeenMs int64) int64 {
	rec := at.positionLifecycleRow(symbol, side, true)
	if rec == nil {
		return 0
	}
	if rec.FirstSeenAt > 0 {
		return rec.FirstSeenAt
	}
	if firstSeenMs > 0 {
		at.updateLifecycle(rec, map[string]interface{}{"first_seen_at": firstSeenMs})
	}
	return firstSeenMs
}

// reprotectTakeProfit is the take-profit to keep for the remainder after a
// partial (D5): the protection state's, then the lifecycle record's, then
// the exchange's open take-profit order. It also reports whether a
// take-profit order is already open on the side, in which case nothing is
// placed (the existing order keeps protecting the remainder).
func (at *AutoTrader) reprotectTakeProfit(symbol, side string) (tp float64, source string, openOnExchange bool) {
	if state := at.protectionStateFor(symbol, side); state != nil && state.ExchangeTakeProfit > 0 {
		tp, source = state.ExchangeTakeProfit, "state"
	}
	if tp <= 0 {
		if rec := at.positionLifecycleRow(symbol, side, false); rec != nil && rec.TakeProfit > 0 {
			tp, source = rec.TakeProfit, "lifecycle record"
		}
	}
	if at.trader == nil {
		return tp, source, false
	}
	orders, err := at.trader.GetOpenOrders(symbol)
	if err != nil {
		logger.Warnf("  ⚠ %s %s open orders unavailable for the take-profit check: %v", symbol, side, err)
		return tp, source, false
	}
	positionSide := protectionPositionSide(side)
	for _, o := range orders {
		if !strings.Contains(strings.ToUpper(o.Type), "TAKE_PROFIT") {
			continue
		}
		if o.PositionSide != "" && !strings.EqualFold(o.PositionSide, positionSide) && !strings.EqualFold(o.PositionSide, "BOTH") {
			continue
		}
		price := o.StopPrice
		if price <= 0 {
			price = o.Price
		}
		if tp <= 0 && price > 0 {
			tp, source = price, "exchange open order"
		}
		openOnExchange = true
	}
	return tp, source, openOnExchange
}

func msTime(ms int64) time.Time { return time.UnixMilli(ms) }
