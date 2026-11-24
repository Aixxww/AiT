package trader

import (
	"fmt"
	"time"

	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/store"
)

// AUD-C3 (P0): max_positions across one decision response.
//
// Every open re-reads GetPositions(), and exchange clients may serve that
// from a short cache (Binance: 15 s). Two opens executed inside the cache
// window both saw the pre-order position count, so max_positions = 1 could be
// exceeded. Three layers now prevent that:
//  1. the position cache is invalidated after every order (exchange client
//     and invalidatePositionCacheAfterOrder below);
//  2. the opens accepted in the current response are counted, and the max
//     positions check uses max(positions seen, positions at response start +
//     opens submitted), so a position that the exchange has not reported
//     yet still occupies its slot;
//  3. before execution, the response's opens are capped at the free slots
//     (max_positions - positions - pending conditional entries); the excess
//     opens are not executed and are recorded as blocked.

// cycleOpenGate tracks the opens of one decision response. It exists only
// while the main loop executes a response (under conditional.entryMu).
type cycleOpenGate struct {
	baseline int // positions at response start
	accepted int // opens that reached the exchange in this response
}

func isOpenAction(action string) bool {
	return action == "open_long" || action == "open_short"
}

// maxPositionsLimit is the limit enforceMaxPositions applies: 0 when there
// is no strategy config (no limit), the configured value otherwise, and an
// error when the configured value is invalid (#77: no silent default).
func (at *AutoTrader) maxPositionsLimit() (int, error) {
	if at.config.StrategyConfig == nil {
		return 0, nil
	}
	return at.config.StrategyConfig.RiskControl.ValidateMaxPositions()
}

func (at *AutoTrader) pendingConditionalEntryCount() int {
	at.conditional.mu.Lock()
	defer at.conditional.mu.Unlock()
	return len(at.conditional.pending)
}

// beginCycleOpenGate starts counting opens for one response.
func (at *AutoTrader) beginCycleOpenGate(positionCount int) {
	at.openGate = &cycleOpenGate{baseline: positionCount}
}

func (at *AutoTrader) endCycleOpenGate() { at.openGate = nil }

// effectivePositionCount is the count enforceMaxPositions must use for an
// open: the exchange's view, but never less than what this response has
// already opened.
func (at *AutoTrader) effectivePositionCount(seen int) int {
	if g := at.openGate; g != nil && g.baseline+g.accepted > seen {
		return g.baseline + g.accepted
	}
	return seen
}

// noteOpenOrderSubmitted counts an open that reached the exchange (filled,
// adopted on read-back, or assumed filled), whatever happens afterwards.
func (at *AutoTrader) noteOpenOrderSubmitted() {
	if g := at.openGate; g != nil {
		g.accepted++
	}
}

// capOpensToFreeSlots keeps every non-open decision and at most freeSlots
// opens, in execution order. The dropped opens are returned for recording.
func (at *AutoTrader) capOpensToFreeSlots(decisions []kernel.Decision, positionCount int) (kept, dropped []kernel.Decision, freeSlots int) {
	limit, err := at.maxPositionsLimit()
	if err != nil {
		// Invalid config: keep no open (each would fail the same check).
		limit = 0
	} else if at.config.StrategyConfig == nil {
		return decisions, nil, -1
	}
	freeSlots = limit - positionCount - at.pendingConditionalEntryCount()
	if freeSlots < 0 {
		freeSlots = 0
	}
	opens := 0
	for _, d := range decisions {
		// A conditional registration reserves a slot as well (its
		// registration check counts pending entries).
		if isOpenAction(d.Action) {
			if opens >= freeSlots {
				dropped = append(dropped, d)
				continue
			}
			opens++
		}
		kept = append(kept, d)
	}
	return kept, dropped, freeSlots
}

func maxPositionsResponseCapError(d kernel.Decision, freeSlots, limit int) string {
	return fmt.Sprintf("❌ [RISK CONTROL] %s %s not executed: this response already uses the %d free position slot(s) of max_positions=%d (max_positions_response_cap)",
		d.Symbol, d.Action, freeSlots, limit)
}

// invalidatePositionCacheAfterOrder drops the exchange client's cached
// positions after an order (AUD-C3), for clients that cache.
func (at *AutoTrader) invalidatePositionCacheAfterOrder() {
	at.invalidateTraderPositionCacheForDecision()
}

// capResponseOpens applies capOpensToFreeSlots to a response before
// execution and records every dropped open as a blocked action.
func (at *AutoTrader) capResponseOpens(ctx *kernel.Context, decisions []kernel.Decision, record *store.DecisionRecord) []kernel.Decision {
	positionCount := 0
	if ctx != nil {
		positionCount = len(ctx.Positions)
	}
	kept, dropped, freeSlots := at.capOpensToFreeSlots(decisions, positionCount)
	for _, d := range dropped {
		limit, limitErr := at.maxPositionsLimit()
		msg := maxPositionsResponseCapError(d, freeSlots, limit)
		if limitErr != nil {
			msg = fmt.Sprintf("❌ [RISK CONTROL] %s %s not executed: %v", d.Symbol, d.Action, limitErr)
		}
		at.logErrorf("%s", msg)
		if record != nil {
			record.ExecutionLog = append(record.ExecutionLog, msg)
			record.Decisions = append(record.Decisions, store.DecisionAction{
				Action:            d.Action,
				Symbol:            d.Symbol,
				Leverage:          d.Leverage,
				StopLoss:          d.StopLoss,
				TakeProfit:        d.TakeProfit,
				Confidence:        d.Confidence,
				Reasoning:         d.Reasoning,
				BlockedReasonCode: d.BlockedReasonCode,
				Trigger:           decisionTriggerForStore(d.Trigger),
				Timestamp:         time.Now().UTC(),
				Success:           false,
				Error:             msg,
			})
		}
	}
	return kept
}
