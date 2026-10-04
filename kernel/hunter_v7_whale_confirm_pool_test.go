package kernel

// Unit tests for P1-4B whale_flow shadow confirmation pool.
import (
	"testing"
	"time"

	"github.com/Aixxww/AiT/provider/local"
)

func whaleShadowTestCoin(taker, oi1h, zonePos float64) CandidateCoin {
	return CandidateCoin{
		Symbol:    "TESTUSDT",
		Direction: "LONG",
		V7SetupType: "whale_flow_reversal",
		V7DerivativesCtx: &local.V7DerivativesContext{
			TakerBuy15m: taker,
			OIChange1h:  oi1h,
		},
		V7ConfirmSummary: &local.V7ConfirmationSummary{EntryZonePosition: zonePos},
		V7EntryZone:      local.V7PriceZone{Lower: 99.0, Upper: 101.0},
	}
}

func TestWhaleShadowPoolWatchConfirm(t *testing.T) {
	ResetWhaleShadowPool()

	// Round 1: taker 0.52 (< 0.56), OI +6 accumulating, zone 40 -> watch.
	whaleShadowPoolObserve(whaleShadowTestCoin(0.52, 6.0, 40), 100.0)
	w, c, e, iv := WhaleShadowPoolStats()
	if w != 1 {
		t.Fatalf("expected 1 watched, got %d", w)
	}

	// Round 2: taker recovered to 0.55, price held above band low -> confirmed.
	whaleShadowPoolObserve(whaleShadowTestCoin(0.55, 7.0, 38), 100.5)
	_, c, _, _ = WhaleShadowPoolStats()
	if c != 1 {
		t.Fatalf("expected 1 confirmed, got %d", c)
	}
	_ = e
	_ = iv
}

func TestWhaleShadowPoolInvalidate(t *testing.T) {
	ResetWhaleShadowPool()

	whaleShadowPoolObserve(whaleShadowTestCoin(0.52, 6.0, 40), 100.0)
	// Price breaks below entry band low (99.0) -> invalidated.
	whaleShadowPoolObserve(whaleShadowTestCoin(0.50, 6.5, 42), 98.5)
	_, _, _, iv := WhaleShadowPoolStats()
	if iv != 1 {
		t.Fatalf("expected 1 invalidated, got %d", iv)
	}
}

func TestWhaleShadowPoolNoEntryConditions(t *testing.T) {
	ResetWhaleShadowPool()

	// Taker too low (noise).
	whaleShadowPoolObserve(whaleShadowTestCoin(0.40, 6.0, 40), 100.0)
	// Taker already above gate (no need to watch).
	whaleShadowPoolObserve(whaleShadowTestCoin(0.60, 6.0, 40), 100.0)
	// OI not accumulating.
	whaleShadowPoolObserve(whaleShadowTestCoin(0.52, 2.0, 40), 100.0)
	// Zone too high.
	whaleShadowPoolObserve(whaleShadowTestCoin(0.52, 6.0, 60), 100.0)
	// Wrong setup.
	c := whaleShadowTestCoin(0.52, 6.0, 40)
	c.V7SetupType = "leader_momentum_long"
	whaleShadowPoolObserve(c, 100.0)

	w, _, _, _ := WhaleShadowPoolStats()
	if w != 0 {
		t.Fatalf("expected 0 watched, got %d", w)
	}
}

func TestWhaleShadowPoolExpiry(t *testing.T) {
	ResetWhaleShadowPool()

	whaleShadowPoolObserve(whaleShadowTestCoin(0.52, 6.0, 40), 100.0)
	// Force expiry by backdating the entry.
	globalWhaleShadowPool.mu.Lock()
	for _, entry := range globalWhaleShadowPool.entries {
		entry.AddedAt = time.Now().Add(-whaleShadowPoolTTL - time.Minute)
	}
	globalWhaleShadowPool.mu.Unlock()

	whaleShadowPoolObserve(whaleShadowTestCoin(0.55, 7.0, 38), 100.5)
	_, _, e, _ := WhaleShadowPoolStats()
	if e != 1 {
		t.Fatalf("expected 1 expired, got %d", e)
	}
}
