package kernel

import (
	"fmt"

	"github.com/Aixxww/AiT/logger"
	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/store"
)

// ============================================================================
// Decision Validation
// ============================================================================

// HunterScoreInfo carries Hunter bidirectional scores for cross-validation.

// legacyContradictionConfidenceCap is the direction-contradiction cap when
// the strategy leaves min_confidence unset (#77); a configured value replaces
// it. The unset confidence FLOOR is not decided here: it comes from
// store.ResolveMinConfidence (#105 / AUD-B4).
const legacyContradictionConfidenceCap = 70

// hunterLowScoreConfidenceCap is the confidence a high-confidence open is
// capped to when its Hunter score is below 15 (a suspicious combination).
// It is applied before the confidence floor check (#138 M7a), so with a
// configured floor above it the open is rejected at validation.
const hunterLowScoreConfidenceCap = 65

type HunterScoreInfo struct {
	LongScore  float64
	ShortScore float64
	Direction  string
}

func validateDecisions(decisions []Decision, accountEquity float64, btcEthLeverage, altcoinLeverage int, btcEthPosRatio, altcoinPosRatio float64, hunterScoreMap map[string]HunterScoreInfo, priceMap map[string]float64, rrFloor DecisionRRFloor, minOpenConfidence int) error {
	for i := range decisions {
		if err := validateDecision(&decisions[i], accountEquity, btcEthLeverage, altcoinLeverage, btcEthPosRatio, altcoinPosRatio, hunterScoreMap, priceMap, rrFloor, minOpenConfidence); err != nil {
			return fmt.Errorf("decision #%d validation failed: %w", i+1, err)
		}
	}
	return nil
}

func validateDecision(d *Decision, accountEquity float64, btcEthLeverage, altcoinLeverage int, btcEthPosRatio, altcoinPosRatio float64, hunterScoreMap map[string]HunterScoreInfo, priceMap map[string]float64, rrFloor DecisionRRFloor, minOpenConfidence int) error {
	validActions := map[string]bool{
		"open_long":   true,
		"open_short":  true,
		"close_long":  true,
		"close_short": true,
		"hold":        true,
		"wait":        true,
	}

	if !validActions[d.Action] {
		return fmt.Errorf("invalid action: %s", d.Action)
	}

	if d.Action == "open_long" || d.Action == "open_short" {
		maxLeverage := altcoinLeverage
		posRatio := altcoinPosRatio
		maxPositionValue := accountEquity * posRatio
		if d.Symbol == "BTCUSDT" || d.Symbol == "ETHUSDT" {
			maxLeverage = btcEthLeverage
			posRatio = btcEthPosRatio
			maxPositionValue = accountEquity * posRatio
		}

		if d.Leverage <= 0 {
			return fmt.Errorf("leverage must be greater than 0: %d", d.Leverage)
		}
		if d.Leverage > maxLeverage {
			logger.Infof("⚠️  [Leverage Fallback] %s leverage exceeded (%dx > %dx), auto-adjusting to limit %dx",
				d.Symbol, d.Leverage, maxLeverage, maxLeverage)
			d.Leverage = maxLeverage
		}
		if d.PositionSizeUSD <= 0 {
			return fmt.Errorf("position size must be greater than 0: %.2f", d.PositionSizeUSD)
		}

		const minPositionSizeGeneral = 12.0
		const minPositionSizeBTCETH = 60.0

		if d.Symbol == "BTCUSDT" || d.Symbol == "ETHUSDT" {
			if d.PositionSizeUSD < minPositionSizeBTCETH {
				return fmt.Errorf("%s opening amount too small (%.2f USDT), must be ≥%.2f USDT", d.Symbol, d.PositionSizeUSD, minPositionSizeBTCETH)
			}
		} else {
			if d.PositionSizeUSD < minPositionSizeGeneral {
				return fmt.Errorf("opening amount too small (%.2f USDT), must be ≥%.2f USDT", d.PositionSizeUSD, minPositionSizeGeneral)
			}
		}

		tolerance := maxPositionValue * 0.01
		if d.PositionSizeUSD > maxPositionValue+tolerance {
			if d.Symbol == "BTCUSDT" || d.Symbol == "ETHUSDT" {
				return fmt.Errorf("BTC/ETH single coin position value cannot exceed %.0f USDT (%.1fx account equity), actual: %.0f", maxPositionValue, posRatio, d.PositionSizeUSD)
			} else {
				return fmt.Errorf("altcoin single coin position value cannot exceed %.0f USDT (%.1fx account equity), actual: %.0f", maxPositionValue, posRatio, d.PositionSizeUSD)
			}
		}
		if d.StopLoss <= 0 || d.TakeProfit <= 0 {
			return fmt.Errorf("stop loss and take profit must be greater than 0")
		}

		if d.Action == "open_long" {
			if d.StopLoss >= d.TakeProfit {
				return fmt.Errorf("for long positions, stop loss price must be less than take profit price")
			}
		} else {
			if d.StopLoss <= d.TakeProfit {
				return fmt.Errorf("for short positions, stop loss price must be greater than take profit price")
			}
		}

		// P0-2 / #106 (AUD-B5): the RR check below uses a REAL entry price,
		// never the model's own trigger.trigger_price and never a fabricated
		// fallback. See decisionRREntryPrice.
		entryPrice, ok := decisionRREntryPrice(d, priceMap)
		if !ok {
			return fmt.Errorf("risk/reward check has no real entry price for %s (no backend conditional trigger, no market price)", d.Symbol)
		}

		var riskPercent, rewardPercent, riskRewardRatio float64
		if d.Action == "open_long" {
			riskPercent = (entryPrice - d.StopLoss) / entryPrice * 100
			rewardPercent = (d.TakeProfit - entryPrice) / entryPrice * 100
			if riskPercent > 0 {
				riskRewardRatio = rewardPercent / riskPercent
			}
		} else {
			riskPercent = (d.StopLoss - entryPrice) / entryPrice * 100
			rewardPercent = (entryPrice - d.TakeProfit) / entryPrice * 100
			if riskPercent > 0 {
				riskRewardRatio = rewardPercent / riskPercent
			}
		}

		// F07/D1: the RR floor comes from strategy config
		// (RiskControl.MinRiskRewardRatio). When the strategy leaves it unset
		// the store default applies (#138 M9: the same default the router's
		// EffectiveMinRR uses; the pre-D1 literal 3.0 is gone).
		// U2/P2a: the floor is resolved per decision, so a Task-2
		// candidate's own floor applies to the decision that selects it.
		requiredRR := 0.0
		if rrFloor != nil {
			requiredRR = rrFloor(d)
		}
		if requiredRR <= 0 {
			requiredRR = store.DefaultMinRiskRewardRatio
		}
		if riskRewardRatio < requiredRR {
			return fmt.Errorf("risk/reward ratio too low (%.2f:1), must be ≥%.2f:1 [risk: %.2f%% reward: %.2f%%] [stop loss: %.2f take profit: %.2f]",
				riskRewardRatio, requiredRR, riskPercent, rewardPercent, d.StopLoss, d.TakeProfit)
		}

		// Confidence cross-validation with Hunter scores
		// #105 / AUD-B4: minOpenConfidence is the strategy's configured
		// min_confidence (0 = unset); the effective floor comes from the
		// resolver (configured value, else its documented 60).
		minConfidenceHard, confidenceConfigured := store.ResolveMinConfidence(minOpenConfidence)
		// The direction-contradiction cap lowers confidence to the open
		// threshold (the lowest size band), as it did when the threshold was
		// the old hard-coded 70; it no longer hard-codes 70 (#77).
		contradictionCap := legacyContradictionConfidenceCap
		if confidenceConfigured {
			contradictionCap = minConfidenceHard
		}

		// #138 M7a: the Hunter-score caps run BEFORE the floor check, so a
		// capped confidence is judged here. Before, the low-score cap (65)
		// ran after the check: the decision passed kernel validation with a
		// confidence below a configured floor (e.g. 75) and failed late in
		// the trader, with failed-open cooldown side effects.
		if hunterScoreMap != nil {
			if info, ok := hunterScoreMap[d.Symbol]; ok {
				selectedHunterScore := info.LongScore
				if d.Action == "open_short" {
					selectedHunterScore = info.ShortScore
				}

				// Direction contradiction: AI opens LONG but Hunter says SHORT is stronger
				if d.Action == "open_long" && info.ShortScore > info.LongScore+10 {
					if d.Confidence > 80 {
						logger.Infof("⚠  %s: AI confidence %d HIGH but Hunter SHORT (%.1f) > LONG (%.1f) — capping confidence to %d",
							d.Symbol, d.Confidence, info.ShortScore, info.LongScore, contradictionCap)
						d.Confidence = contradictionCap
					}
				}
				if d.Action == "open_short" && info.LongScore > info.ShortScore+10 {
					if d.Confidence > 80 {
						logger.Infof("⚠  %s: AI confidence %d HIGH but Hunter LONG (%.1f) > SHORT (%.1f) — capping confidence to %d",
							d.Symbol, d.Confidence, info.LongScore, info.ShortScore, contradictionCap)
						d.Confidence = contradictionCap
					}
				}

				// Low Hunter score + high AI confidence = suspicious
				if selectedHunterScore < 15 && d.Confidence > 85 {
					logger.Infof("⚠  %s: Hunter score %.1f LOW but AI confidence %d HIGH — capping confidence to %d",
						d.Symbol, selectedHunterScore, d.Confidence, hunterLowScoreConfidenceCap)
					d.Confidence = hunterLowScoreConfidenceCap
				}
			}
		}

		if d.Confidence < minConfidenceHard {
			return fmt.Errorf("confidence %d below absolute minimum %d", d.Confidence, minConfidenceHard)
		}
	}

	return nil
}

// decisionRREntryPrice is the entry price the kernel RR check measures from
// (#106 / AUD-B5):
//   - a Hunter v7 conditional entry: the backend trigger
//     (Decision.HunterV7TriggerPrice, hydrated from the selected signal
//     before validation); the trader registers it at that trigger;
//   - every other open: the live market price from priceMap; the trader
//     opens these at market.
//
// The model's trigger.trigger_price is never used: a trigger placed next to
// the stop inflated RR. With neither price available the check has no real
// entry and ok is false; the former SL+0.2*(TP-SL) fallback always gave RR
// 4.0 and so passed every floor.
func decisionRREntryPrice(d *Decision, priceMap map[string]float64) (float64, bool) {
	if d == nil {
		return 0, false
	}
	if d.HunterV7ConditionalEntry && d.HunterV7TriggerPrice > 0 {
		return d.HunterV7TriggerPrice, true
	}
	if p := priceMap[d.Symbol]; p > 0 {
		return p, true
	}
	if p := priceMap[market.Normalize(d.Symbol)]; p > 0 {
		return p, true
	}
	return 0, false
}
