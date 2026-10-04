package kernel

// Shadow-mode cross-round confirmation pool for whale_flow_reversal LONG.
//
// P1-4B (author's 0802/0803 design, implemented as SHADOW observation):
// when a whale_flow LONG candidate fails the taker gate marginally
// (taker < 0.56) but OI is accumulating, the doc hypothesizes the next
// round's taker recovery + price holding the entry band is a valid
// confirmation. This pool OBSERVES that hypothesis across rounds.
//
// SHADOW ONLY: it writes structured logs ([whale-shadow]) for offline
// analysis. It never changes tiers, prompts, or decisions. Promotion logic
// is intentionally absent — after ~2 weeks of observations the hit rate can
// be measured and a promotion proposal made with evidence.

import (
	"sync"
	"time"

	"github.com/Aixxww/AiT/logger"
)

const (
	// How long a pool entry survives without confirmation.
	whaleShadowPoolTTL = 30 * time.Minute

	// Entry gate: taker marginally below the 0.56 hard gate, but not noise.
	whaleShadowTakerEntryMin = 0.48
	whaleShadowTakerEntryMax = 0.56
	// OI must be accumulating (mirrors the doc's "OI 1h >= 5" condition).
	whaleShadowOIChange1hMin = 5.0
	// Zone gate mirrors hunterV7WhaleLongMaxZonePos.
	whaleShadowMaxZonePos = 45.0
	// Confirmation gate: taker recovered to this level.
	whaleShadowConfirmTaker = 0.54
)

type whaleShadowPoolEntry struct {
	Symbol         string
	AddedAt        time.Time
	TakerAtEntry   float64
	OIChangeAtEntry float64
	PriceAtEntry   float64
	ZoneLowAtEntry float64
	ZonePosAtEntry float64
}

type whaleShadowPool struct {
	mu      sync.Mutex
	entries map[string]*whaleShadowPoolEntry
	watched int64
	confirmed int64
	expired   int64
	invalidated int64
}

var globalWhaleShadowPool = &whaleShadowPool{entries: make(map[string]*whaleShadowPoolEntry)}

// ResetWhaleShadowPool clears the pool (tests / backtest isolation).
func ResetWhaleShadowPool() {
	globalWhaleShadowPool.mu.Lock()
	defer globalWhaleShadowPool.mu.Unlock()
	globalWhaleShadowPool.entries = make(map[string]*whaleShadowPoolEntry)
	globalWhaleShadowPool.watched = 0
	globalWhaleShadowPool.confirmed = 0
	globalWhaleShadowPool.expired = 0
	globalWhaleShadowPool.invalidated = 0
}

// WhaleShadowPoolStats returns observation counters for offline analysis.
func WhaleShadowPoolStats() (watched, confirmed, expired, invalidated int64) {
	globalWhaleShadowPool.mu.Lock()
	defer globalWhaleShadowPool.mu.Unlock()
	return globalWhaleShadowPool.watched, globalWhaleShadowPool.confirmed,
		globalWhaleShadowPool.expired, globalWhaleShadowPool.invalidated
}

func whaleShadowTakerOf(coin CandidateCoin) float64 {
	if coin.V7DerivativesCtx == nil {
		return 0
	}
	return coin.V7DerivativesCtx.TakerBuy15m
}

func whaleShadowZonePosOf(coin CandidateCoin) float64 {
	if coin.V7ConfirmSummary == nil {
		return 0
	}
	return coin.V7ConfirmSummary.EntryZonePosition
}

func whaleShadowFundingCrowded(coin CandidateCoin) bool {
	for _, tag := range coin.V7RiskTags {
		switch tag {
		case "funding_crowded", "rs_crowded_short_funding", "crowding_extreme":
			return true
		}
	}
	return false
}

func whaleShadowOIAccumulating(coin CandidateCoin) bool {
	if coin.V7DerivativesCtx == nil {
		return false
	}
	if coin.V7DerivativesCtx.OIChange1h >= whaleShadowOIChange1hMin {
		return true
	}
	for _, code := range coin.V7ReasonCodes {
		if code == "oi_1h_confirming_accumulation" {
			return true
		}
	}
	return false
}

// whaleShadowPoolObserve is called once per candidate per round (after tier
// classification). It only observes and logs; it never mutates the coin.
func whaleShadowPoolObserve(coin CandidateCoin, price float64) {
	if coin.V7SetupType != "whale_flow_reversal" || coin.Direction != "LONG" {
		return
	}
	now := time.Now()
	taker := whaleShadowTakerOf(coin)

	globalWhaleShadowPool.mu.Lock()
	defer globalWhaleShadowPool.mu.Unlock()

	if entry, ok := globalWhaleShadowPool.entries[coin.Symbol]; ok {
		// Already watching: check terminal states.
		if now.Sub(entry.AddedAt) > whaleShadowPoolTTL {
			delete(globalWhaleShadowPool.entries, coin.Symbol)
			globalWhaleShadowPool.expired++
			logger.Infof("[whale-shadow] expired symbol=%s watched_rounds_ttl_exceeded", coin.Symbol)
			return
		}
		if price > 0 && price < entry.ZoneLowAtEntry {
			delete(globalWhaleShadowPool.entries, coin.Symbol)
			globalWhaleShadowPool.invalidated++
			logger.Infof("[whale-shadow] invalidated symbol=%s price=%.4f broke entry band low=%.4f",
				coin.Symbol, price, entry.ZoneLowAtEntry)
			return
		}
		if taker >= whaleShadowConfirmTaker && price >= entry.ZoneLowAtEntry {
			delete(globalWhaleShadowPool.entries, coin.Symbol)
			globalWhaleShadowPool.confirmed++
			logger.Infof("[whale-shadow] CONFIRMED symbol=%s taker=%.3f->%.3f price=%.4f held_band (shadow only, no promotion)",
				coin.Symbol, entry.TakerAtEntry, taker, price)
			return
		}
		// Still watching.
		return
	}

	// Not in pool: evaluate entry conditions.
	if taker < whaleShadowTakerEntryMin || taker >= whaleShadowTakerEntryMax {
		return
	}
	if !whaleShadowOIAccumulating(coin) {
		return
	}
	zonePos := whaleShadowZonePosOf(coin)
	if zonePos <= 0 || zonePos > whaleShadowMaxZonePos {
		return
	}
	if whaleShadowFundingCrowded(coin) {
		return
	}
	if price <= 0 {
		return
	}
	zoneLow := coin.V7EntryZone.Lower
	if zoneLow <= 0 {
		// Fall back to a tight band under the observed price.
		zoneLow = price * 0.995
	}
	globalWhaleShadowPool.entries[coin.Symbol] = &whaleShadowPoolEntry{
		Symbol:          coin.Symbol,
		AddedAt:         now,
		TakerAtEntry:    taker,
		OIChangeAtEntry: coin.V7DerivativesCtx.OIChange1h,
		PriceAtEntry:    price,
		ZoneLowAtEntry:  zoneLow,
		ZonePosAtEntry:  zonePos,
	}
	globalWhaleShadowPool.watched++
	logger.Infof("[whale-shadow] watch symbol=%s taker=%.3f oi_1h=%.2f price=%.4f zone_pos=%.1f (shadow only)",
		coin.Symbol, taker, coin.V7DerivativesCtx.OIChange1h, price, zonePos)
}
