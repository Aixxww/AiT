package kernel

// Unit tests for P0-2a: RR validation must use the real entry price.
// The legacy formula (SL + 0.2*(TP-SL)) always yields exactly 4.0, making the
// RR>=3.0 gate dead code. These tests pin the fixed behavior.
import "testing"

func TestValidateDecisionRRUsesRealPrice(t *testing.T) {
	mk := func(sl, tp float64) *Decision {
		return &Decision{
			Symbol:          "BTCUSDT",
			Action:          "open_long",
			Leverage:        10,
			PositionSizeUSD: 100,
			StopLoss:        sl,
			TakeProfit:      tp,
			Confidence:      80,
			Reasoning:       "test",
		}
	}
	priceMap := map[string]float64{"BTCUSDT": 100.0}

	// Real RR at market 100: risk 2% (SL 98), reward 4% (TP 104) -> RR=2.0 < 3.0 -> reject.
	// (Legacy formula would compute exactly 4.0 and pass.)
	d := mk(98, 104)
	if err := validateDecision(d, 10000, 20, 10, 10.0, 1.5, nil, priceMap); err == nil {
		t.Errorf("expected rejection for real RR=2.0 (< 3.0), got pass (P0-2a regression)")
	}

	// Real RR: risk 2% (SL 98), reward 8% (TP 108) -> RR=4.0 >= 3.0 -> pass.
	d = mk(98, 108)
	if err := validateDecision(d, 10000, 20, 10, 10.0, 1.5, nil, priceMap); err != nil {
		t.Errorf("expected pass for real RR=4.0, got %v", err)
	}

	// Trigger price takes precedence over market price.
	d = mk(98, 104)
	d.Trigger = &DecisionTrigger{TriggerPrice: 90} // entry 90: risk 8.9%, reward 15.6% -> RR 1.75
	if err := validateDecision(d, 10000, 20, 10, 10.0, 1.5, nil, priceMap); err == nil {
		t.Errorf("expected rejection using trigger price as entry (RR~1.75), got pass")
	}

	// No price map: legacy fallback keeps old behavior (pass, RR fabricated 4.0).
	d = mk(98, 104)
	if err := validateDecision(d, 10000, 20, 10, 10.0, 1.5, nil, nil); err != nil {
		t.Errorf("legacy fallback without priceMap should pass as before, got %v", err)
	}
}
