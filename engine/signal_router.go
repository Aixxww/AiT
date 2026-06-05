package engine

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// ============================================================================
// Signal Router
// Transforms scored IndicatorSets into actionable TradeSignals.
// ============================================================================

// SignalRouter manages signal generation with cooldown tracking.
type SignalRouter struct {
	hub      *IndicatorHub
	cfg      HubConfig
	cooldown map[string]time.Time // symbol → last trade time
	mu       sync.Mutex
}

// NewSignalRouter creates a new SignalRouter.
func NewSignalRouter(hub *IndicatorHub, cfg HubConfig) *SignalRouter {
	return &SignalRouter{
		hub:      hub,
		cfg:      cfg,
		cooldown: make(map[string]time.Time),
	}
}

// Route takes scored indicator sets and produces trade signals.
func (r *SignalRouter) Route(sets []*IndicatorSet) []*TradeSignal {
	r.mu.Lock()
	defer r.mu.Unlock()

	snap := r.hub.store.Current()
	if snap == nil {
		return nil
	}

	// Step 1: Filter — direction != NEUTRAL, score >= MinScore
	filtered := make([]*IndicatorSet, 0, len(sets))
	for _, set := range sets {
		if set.Direction == 0 {
			continue // skip neutral
		}
		if set.FinalScore < r.cfg.MinScore {
			continue
		}
		filtered = append(filtered, set)
	}

	// Step 2: Sort by FinalScore descending
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].FinalScore > filtered[j].FinalScore
	})

	// Step 3: Take top MaxSignalsPerCycle
	if r.cfg.MaxSignalsPerCycle > 0 && len(filtered) > r.cfg.MaxSignalsPerCycle {
		filtered = filtered[:r.cfg.MaxSignalsPerCycle]
	}

	// Step 4: Check cooldown and build signals
	now := time.Now()
	cooldownDur := time.Duration(r.cfg.CooldownMinutes) * time.Minute
	signals := make([]*TradeSignal, 0, len(filtered))

	for _, set := range filtered {
		// Check cooldown
		if lastTrade, ok := r.cooldown[set.Symbol]; ok {
			if now.Sub(lastTrade) < cooldownDur {
				continue // still in cooldown
			}
		}

		// Look up the original snapshot for this symbol
		symSnap, ok := snap.Symbols[set.Symbol]
		if !ok || symSnap == nil {
			continue
		}

		// Step 5: Compute SL/TP
		sl, tp1, tp2, tp3 := calcSLTP(symSnap, set.Direction, set.ATR14, r.cfg)

		// Step 6: Build signal reason strings
		bullSignals, bearSignals, reasons := buildSignalReasons(set)

		// Step 7: Assign grade
		grade := determineGrade(set.FinalScore, r.cfg)

		signal := &TradeSignal{
			Symbol:      set.Symbol,
			Direction:   set.Direction,
			FinalScore:  set.FinalScore,
			Grade:       grade,
			TechScore:   set.TechBullScore + set.TechBearScore,
			QuantScore:  set.QuantBullScore + set.QuantBearScore,
			SocialScore: set.SocialBullScore + set.SocialBearScore,

			EntryPrice: symSnap.Price,
			StopLoss:   sl,
			TP1:        tp1,
			TP2:        tp2,
			TP3:        tp3,

			BullSignals: bullSignals,
			BearSignals: bearSignals,
			Reasons:     reasons,

			Indicators: set,
			Snapshot:   symSnap,
			Timestamp:  now,
		}

		signals = append(signals, signal)
	}

	return signals
}

// RecordTrade records a trade execution for cooldown tracking.
func (r *SignalRouter) RecordTrade(symbol string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cooldown[symbol] = time.Now()
}

// ClearCooldown removes cooldown for a specific symbol.
func (r *SignalRouter) ClearCooldown(symbol string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cooldown, symbol)
}

// ClearAllCooldowns removes all cooldowns.
func (r *SignalRouter) ClearAllCooldowns() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cooldown = make(map[string]time.Time)
}

// ============================================================================
// Signal Reason Building
// ============================================================================

// buildSignalReasons generates human-readable signal descriptions.
func buildSignalReasons(set *IndicatorSet) (bullSignals, bearSignals, reasons []string) {
	bullSignals = make([]string, 0)
	bearSignals = make([]string, 0)
	reasons = make([]string, 0)

	// --- Technical Bull Signals ---
	if set.RSI14 < 30 {
		bullSignals = append(bullSignals, fmt.Sprintf("RSI oversold (%.1f)", set.RSI14))
	} else if set.RSI14 < 40 {
		bullSignals = append(bullSignals, fmt.Sprintf("RSI near oversold (%.1f)", set.RSI14))
	}

	if set.MACDHist > 0 {
		bullSignals = append(bullSignals, fmt.Sprintf("MACD histogram positive (%.4f)", set.MACDHist))
	}

	if set.BBLower > 0 && set.BBMiddle > 0 {
		price := set.BBMiddle
		bbRange := set.BBUpper - set.BBLower
		if bbRange > 0 {
			pos := (price - set.BBLower) / bbRange
			if pos < 0.15 {
				bullSignals = append(bullSignals, "Price near BB lower band")
			}
		}
	}

	if set.EMA20 > set.EMA50 && set.EMA50 > set.EMA200 && set.EMA200 > 0 {
		bullSignals = append(bullSignals, "EMA bullish alignment (20>50>200)")
	}

	// --- Technical Bear Signals ---
	if set.RSI14 > 70 {
		bearSignals = append(bearSignals, fmt.Sprintf("RSI overbought (%.1f)", set.RSI14))
	} else if set.RSI14 > 60 {
		bearSignals = append(bearSignals, fmt.Sprintf("RSI near overbought (%.1f)", set.RSI14))
	}

	if set.MACDHist < 0 {
		bearSignals = append(bearSignals, fmt.Sprintf("MACD histogram negative (%.4f)", set.MACDHist))
	}

	if set.BBUpper > 0 && set.BBMiddle > 0 {
		price := set.BBMiddle
		bbRange := set.BBUpper - set.BBLower
		if bbRange > 0 {
			pos := (price - set.BBLower) / bbRange
			if pos > 0.85 {
				bearSignals = append(bearSignals, "Price near BB upper band")
			}
		}
	}

	if set.EMA20 < set.EMA50 && set.EMA50 < set.EMA200 && set.EMA200 > 0 {
		bearSignals = append(bearSignals, "EMA bearish alignment (20<50<200)")
	}

	// --- Quant Signals ---
	if set.OIScore > 10 {
		bullSignals = append(bullSignals, fmt.Sprintf("OI+Price bullish (%.0f)", set.OIScore))
	} else if set.OIScore < -10 {
		bearSignals = append(bearSignals, fmt.Sprintf("OI+Price bearish (%.0f)", set.OIScore))
	}

	if set.OISpikeScore > 50 {
		reasons = append(reasons, fmt.Sprintf("OI spike detected (%.0f)", set.OISpikeScore))
	}

	if set.FundingScore < -20 {
		bullSignals = append(bullSignals, fmt.Sprintf("Negative funding rate (%.1f)", set.FundingScore))
	} else if set.FundingScore > 20 {
		bearSignals = append(bearSignals, fmt.Sprintf("Positive funding rate (%.1f)", set.FundingScore))
	}

	if set.TakerScore > 20 {
		bullSignals = append(bullSignals, fmt.Sprintf("Aggressive buying (%.0f)", set.TakerScore))
	} else if set.TakerScore < -20 {
		bearSignals = append(bearSignals, fmt.Sprintf("Aggressive selling (%.0f)", set.TakerScore))
	}

	if set.VolumeScore > 50 {
		reasons = append(reasons, fmt.Sprintf("Volume anomaly (%.0f)", set.VolumeScore))
	}

	// --- Social Signals ---
	if set.SocialHeatScore > 70 {
		reasons = append(reasons, fmt.Sprintf("High social heat (%.0f)", set.SocialHeatScore))
	}

	if set.SocialSentiment > 60 {
		bullSignals = append(bullSignals, fmt.Sprintf("Positive social sentiment (%.0f)", set.SocialSentiment))
	} else if set.SocialSentiment < 30 {
		bearSignals = append(bearSignals, fmt.Sprintf("Negative social sentiment (%.0f)", set.SocialSentiment))
	}

	if set.SocialVolumePct > 50 {
		reasons = append(reasons, fmt.Sprintf("Social volume spike (%.0f%%)", set.SocialVolumePct))
	}

	// --- General Reasons ---
	if set.FinalScore >= 80 {
		reasons = append(reasons, "Grade S — strong signal")
	} else if set.FinalScore >= 65 {
		reasons = append(reasons, "Grade A — good signal")
	}

	return
}
