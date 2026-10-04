package trader

// Protector A/B simulation harness (P0-2 / P0-3 validation).
//
// Causal test of EXIT mechanics only: identical entries + identical real
// market data (data.binance.vision via market.GetKlinesRange), only the
// protector behavior differs between runs.
//
// Run 1 (emulated baseline):  PROTECTOR_AB_VARIANT=baseline
//   - hard loss at fixed -12% (PlannedStopLossROEPct left unset)
//   - giveback/trail closes 100% of the position
// Run 2 (new):                PROTECTOR_AB_VARIANT=staged
//   - hard loss defers to a wider planned stop (P0-2)
//   - giveback/trail takes a 50% staged close, stop rebuilt at breakeven
//     + 0.15% buffer (P0-3)
//
// Honest-validation notes (P0-1):
//   - pure klines only: no OI ranking / net-flow / price-ranking inputs,
//     so no look-ahead bias from real-time APIs;
//   - fee_bps=5, slippage_bps=5 on every fill;
//   - entries are a fixed EMA-cross rule (entry-agnostic protector test),
//     NOT the AI strategy — the delta isolates exit mechanics.
//
// Usage:
//   PROTECTOR_AB_RUN=1 PROTECTOR_AB_VARIANT=baseline go test ./trader/ -run TestProtectorAB -v
//   PROTECTOR_AB_RUN=1 PROTECTOR_AB_VARIANT=staged   go test ./trader/ -run TestProtectorAB -v

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Aixxww/AiT/market"
)

const (
	abLeverage     = 10
	abFeeBps       = 5.0
	abSlippageBps  = 5.0
	abNotionalUSD  = 1000.0
	abPlannedSLPct = 2.0 // planned stop: 2% adverse price move (report's "最小 2% 位移")
)

var abSymbols = []string{"BTCUSDT", "ETHUSDT", "SOLUSDT", "DOGEUSDT"}

type abMetrics struct {
	trades      int
	wins        int
	grossWin    float64
	grossLoss   float64
	totalPnL    float64
	maxDD       float64
	totalBars   int
	hardLosses  int
	givebacks   int
}

func (m *abMetrics) record(pnl float64, bars int, action protectionAction) {
	m.trades++
	m.totalBars += bars
	m.totalPnL += pnl
	if pnl > 0 {
		m.wins++
		m.grossWin += pnl
	} else {
		m.grossLoss += -pnl
	}
	switch action {
	case protectionHardLossClose:
		m.hardLosses++
	case protectionGivebackClose, protectionTrailClose:
		m.givebacks++
	}
}

func (m *abMetrics) report() string {
	winRate := 0.0
	if m.trades > 0 {
		winRate = float64(m.wins) / float64(m.trades) * 100
	}
	avgWin, avgLoss := 0.0, 0.0
	if m.wins > 0 {
		avgWin = m.grossWin / float64(m.wins)
	}
	if m.trades-m.wins > 0 {
		avgLoss = m.grossLoss / float64(m.trades-m.wins)
	}
	pf := 0.0
	if m.grossLoss > 0 {
		pf = m.grossWin / m.grossLoss
	}
	avgBars := 0.0
	if m.trades > 0 {
		avgBars = float64(m.totalBars) / float64(m.trades)
	}
	return fmt.Sprintf("trades=%d win_rate=%.1f%% avg_win=$%.2f avg_loss=$%.2f profit_factor=%.2f total=$%.2f maxDD=$%.2f avg_bars=%.1f hardloss=%d giveback=%d",
		m.trades, winRate, avgWin, avgLoss, pf, m.totalPnL, m.maxDD, avgBars, m.hardLosses, m.givebacks)
}

func ema(values []float64, period int) []float64 {
	out := make([]float64, len(values))
	k := 2.0 / (float64(period) + 1)
	for i, v := range values {
		if i == 0 {
			out[i] = v
		} else {
			out[i] = v*k + out[i-1]*(1-k)
		}
	}
	return out
}

func TestProtectorAB(t *testing.T) {
	if os.Getenv("PROTECTOR_AB_RUN") == "" {
		t.Skip("set PROTECTOR_AB_RUN=1 to run the protector A/B simulation")
	}
	variant := os.Getenv("PROTECTOR_AB_VARIANT")
	if variant != "baseline" && variant != "staged" {
		t.Fatalf("PROTECTOR_AB_VARIANT must be baseline|staged, got %q", variant)
	}
	staged := variant == "staged"
	micro := os.Getenv("PROTECTOR_AB_MICROTP0") == "1"

	// Period is configurable for out-of-sample checks:
	// PROTECTOR_AB_START/END as YYYY-MM-DD, default 2026-09-01..2026-09-28.
	startStr := os.Getenv("PROTECTOR_AB_START")
	if startStr == "" {
		startStr = "2026-09-01"
	}
	endStr := os.Getenv("PROTECTOR_AB_END")
	if endStr == "" {
		endStr = "2026-09-28"
	}
	start, err := time.Parse("2006-01-02", startStr)
	if err != nil {
		t.Fatalf("bad PROTECTOR_AB_START: %v", err)
	}
	end, err := time.Parse("2006-01-02", endStr)
	if err != nil {
		t.Fatalf("bad PROTECTOR_AB_END: %v", err)
	}

	metrics := &abMetrics{}
	equity := 10000.0
	peakEquity := equity

	for _, symbol := range abSymbols {
		klines, err := market.GetKlinesRange(symbol, "15m", start, end)
		if err != nil {
			t.Fatalf("klines %s: %v", symbol, err)
		}
		closes := make([]float64, len(klines))
		for i, k := range klines {
			closes[i] = k.Close
		}
		e20 := ema(closes, 20)
		e50 := ema(closes, 50)

		// Walk bars; at most one open simulated position per symbol.
		i := 60
		for i < len(klines)-2 {
			// Entry signal: EMA20/EMA50 cross.
			var side string
			if e20[i-1] <= e50[i-1] && e20[i] > e50[i] {
				side = "long"
			} else if e20[i-1] >= e50[i-1] && e20[i] < e50[i] {
				side = "short"
			} else {
				i++
				continue
			}

			entryPrice := klines[i].Close
			qty := abNotionalUSD / entryPrice // base-asset quantity
			feeOpen := abNotionalUSD * abFeeBps / 10000

			// Planned stop: 2% adverse move (mirrors real plannedRisk.stopLoss).
			var stopPrice float64
			if side == "long" {
				stopPrice = entryPrice * (1 - abPlannedSLPct/100)
			} else {
				stopPrice = entryPrice * (1 + abPlannedSLPct/100)
			}
			plannedStop := stopPrice // fixed exchange stop for the whole trade

			// Planned TP0: simulate a "far" LLM plan (1.5% move). With micro
			// enabled the P1-4 cap pulls it closer. Checked on bar high/low
			// (resting limit order semantics).
			var plannedTP0 float64
			if side == "long" {
				plannedTP0 = entryPrice * 1.015
			} else {
				plannedTP0 = entryPrice * 0.985
			}
			if micro {
				plannedTP0 = microTP0Price(side, entryPrice, plannedStop, plannedTP0)
			}

			state := &positionProtectionState{
				InitialQuantity: qty,
				ActiveStopLoss:  stopPrice,
				OpenedAt:        time.UnixMilli(klines[i].OpenTime),
			}
			if staged {
				// P0-2: record the plan distance so the hard-loss backstop defers to it.
				state.PlannedStopLossROEPct = calculateLeveragedPnLPct(side, entryPrice, stopPrice, abLeverage)
			}

			realized := -feeOpen
			remaining := qty
			barsHeld := 0
			lastAction := protectionNone
			posOpen := true

			for j := i + 1; j < len(klines) && posOpen; j++ {
				barTime := time.UnixMilli(klines[j].OpenTime)
				protectionNow = func() time.Time { return barTime }
				markPrice := klines[j].Close

				// Planned TP0 as a resting limit order: fills on high/low touch.
				// Mirrors shouldTriggerPlannedTP0Price (only before TP0Done).
				if !state.TP0Done {
					touchedTP0 := (side == "long" && klines[j].High >= plannedTP0) ||
						(side == "short" && klines[j].Low <= plannedTP0)
					if touchedTP0 {
						fill := plannedTP0
						if side == "long" {
							fill = plannedTP0 * (1 - abSlippageBps/10000)
						} else {
							fill = plannedTP0 * (1 + abSlippageBps/10000)
						}
						closeQty := remaining * protectorTP0CloseRatio
						if closeQty*fill < protectorDefaultMinCloseNotional || (remaining-closeQty)*fill < protectorDefaultMinCloseNotional {
							closeQty = remaining
						}
						pnlPct := calculateLeveragedPnLPct(side, entryPrice, fill, abLeverage)
						realized += closeQty * entryPrice * pnlPct / 100 / float64(abLeverage)
						realized -= closeQty * fill * abFeeBps / 10000
						remaining -= closeQty
						state.TP0Done = true
						lastAction = protectionTP0
						if remaining <= 1e-12 {
							posOpen = false
							break
						}
						// Rebuild breakeven stop for the remainder (tight buffer,
						// as in rebuildProtectionStops).
						if side == "long" {
							stopPrice = entryPrice * (1 + protectorBreakevenBufferPct)
						} else {
							stopPrice = entryPrice * (1 - protectorBreakevenBufferPct)
						}
					}
				}

				// Planned stop (exchange stop order): if touched, the whole
			// remainder exits there. Checked before the software protector,
			// mirroring real precedence on non-gap bars. (On gap bars through
			// both levels the fill order is approximate — noted limitation.)
			// Planned stop (exchange stop order): active until the first partial
			// close replaces it with the breakeven stop (mirrors
			// rebuildProtectionStops cancelling prior stop orders).
			plannedTouched := remaining >= qty && ((side == "long" && markPrice <= plannedStop) ||
				(side == "short" && markPrice >= plannedStop))
			if plannedTouched {
				fill := plannedStop
				if side == "long" {
					fill = plannedStop * (1 - abSlippageBps/10000)
				} else {
					fill = plannedStop * (1 + abSlippageBps/10000)
				}
				pnlPct := calculateLeveragedPnLPct(side, entryPrice, fill, abLeverage)
				realized += remaining * entryPrice * pnlPct / 100 / float64(abLeverage)
				realized -= remaining * fill * abFeeBps / 10000
				remaining = 0
				lastAction = protectionHardLossClose // exited via stop
				posOpen = false
				break
			}

			// Resting breakeven stop: after any partial close the real system
				// rebuilds the stop at breakeven (+buffer). If touched, the
				// remainder exits there (as on exchange). Applies to both
				// variants; only the giveback buffer differs (P0-3).
				if remaining < qty {
					touched := (side == "long" && markPrice <= stopPrice) ||
						(side == "short" && markPrice >= stopPrice)
					if touched {
						fill := stopPrice
						if side == "long" {
							fill = stopPrice * (1 - abSlippageBps/10000)
						} else {
							fill = stopPrice * (1 + abSlippageBps/10000)
						}
						pnlPct := calculateLeveragedPnLPct(side, entryPrice, fill, abLeverage)
						realized += remaining * entryPrice * pnlPct / 100 / float64(abLeverage)
						realized -= remaining * fill * abFeeBps / 10000
						remaining = 0
						posOpen = false
						break
					}
				}

				curPnL := calculateLeveragedPnLPct(side, entryPrice, markPrice, abLeverage)
				priceMove := calculateUnleveragedPnLPct(side, entryPrice, markPrice)
				action, _ := choosePositionProtectionAction(state, curPnL, priceMove)
				lastAction = action
				barsHeld++

				closeRatio := 0.0
				switch action {
				case protectionNone:
					continue
				case protectionTP0:
					closeRatio = protectorTP0CloseRatio
				case protectionTP1:
					closeRatio = protectorTP1CloseRatio
				case protectionTP2:
					closeRatio = protectorTP2CloseRatio
				case protectionGivebackClose, protectionTrailClose:
					if staged {
						closeRatio = protectorGivebackCloseRatio // P0-3: staged
					} else {
						closeRatio = 1.0 // baseline: liquidate all
					}
				case protectionHardLossClose:
					closeRatio = 1.0
				}

				// Fill at close with adverse slippage.
				fillPrice := markPrice
				if side == "long" {
					fillPrice = markPrice * (1 - abSlippageBps/10000)
				} else {
					fillPrice = markPrice * (1 + abSlippageBps/10000)
				}
				closeQty := remaining * closeRatio
				if closeQty <= 0 {
					continue
				}
				// Min-notional guard mirrors protectionCloseQuantity: dust remainder closes all.
				if closeQty*fillPrice < protectorDefaultMinCloseNotional || (remaining-closeQty)*fillPrice < protectorDefaultMinCloseNotional {
					closeQty = remaining
				}
				pnlPct := calculateLeveragedPnLPct(side, entryPrice, fillPrice, abLeverage)
				realized += closeQty * entryPrice * pnlPct / 100 / float64(abLeverage)
				realized -= closeQty * fillPrice * abFeeBps / 10000
				remaining -= closeQty

				if remaining <= 1e-12 || closeRatio >= 1.0 {
					posOpen = false
					break
				}
				// Partial close: rebuild the breakeven stop for the remainder,
				// mirroring rebuildProtectionStops (tight buffer for TP
				// partials, wider buffer for staged giveback in the new variant).
				if side == "long" {
					stopPrice = entryPrice * (1 + protectorBreakevenBufferPct)
				} else {
					stopPrice = entryPrice * (1 - protectorBreakevenBufferPct)
				}
				if staged && (action == protectionGivebackClose || action == protectionTrailClose) {
					if side == "long" {
						stopPrice = entryPrice * (1 + protectorGivebackBreakevenBuffer)
					} else {
						stopPrice = entryPrice * (1 - protectorGivebackBreakevenBuffer)
					}
				}
			}
			protectionNow = time.Now // restore real clock

			// Force-close anything left at the end of data.
			if posOpen && remaining > 0 {
				last := klines[len(klines)-1].Close
				pnlPct := calculateLeveragedPnLPct(side, entryPrice, last, abLeverage)
				realized += remaining * entryPrice * pnlPct / 100 / float64(abLeverage)
				realized -= remaining * last * abFeeBps / 10000
			}

			metrics.record(realized, barsHeld, lastAction)
			equity += realized
			if equity > peakEquity {
				peakEquity = equity
			}
			if dd := peakEquity - equity; dd > metrics.maxDD {
				metrics.maxDD = dd
			}
			i += barsHeld + 24 // cooldown before next entry
		}
	}

	fmt.Printf("\n[protector-ab variant=%s] %s\n", variant, metrics.report())
}
