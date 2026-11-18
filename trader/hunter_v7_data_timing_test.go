package trader

import (
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/store"
)

func TestNextAlignedScan(t *testing.T) {
	at := func(h, m, s int) time.Time { return time.Date(2026, 11, 18, h, m, s, 0, time.UTC) }
	cases := []struct{ now, want time.Time }{
		{at(10, 0, 10), at(10, 0, 30)}, // before this bar's settle point
		{at(10, 0, 30), at(10, 5, 30)}, // exactly at it: the next one
		{at(10, 3, 0), at(10, 5, 30)},
		{at(10, 59, 59), at(11, 0, 30)},
	}
	for _, c := range cases {
		if got := nextAlignedScan(c.now, 5, 30*time.Second); !got.Equal(c.want) {
			t.Errorf("now %s: next %s, want %s", c.now.Format("15:04:05"), got.Format("15:04:05"), c.want.Format("15:04:05"))
		}
	}
}

func TestScanChannelDefaultIsWallClock(t *testing.T) {
	at := t2StopTrader() // hunter_v7
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	c, release := at.scanChannel(ticker, false, time.Now())
	release()
	if c != ticker.C {
		t.Fatalf("default must wait on the ScanInterval ticker")
	}
	at.config.StrategyConfig.CoinSource.Hunter = &store.HunterConfig{V7DataTiming: &store.V7DataTimingConfig{CycleAlignment: store.V7CycleAligned, SettleSeconds: 1}}
	if c, release := at.scanChannel(ticker, true, time.Now()); c != ticker.C {
		release()
		t.Fatalf("grid strategies keep the ticker")
	}
	now := time.Now()
	c, release = at.scanChannel(ticker, false, now)
	defer release()
	if c == ticker.C {
		t.Fatalf("aligned mode must use its own timer")
	}
}

func TestSnapshotBudgetGate(t *testing.T) {
	at := t2StopTrader()
	cand := &kernel.CandidateCoin{V7SetupType: "momentum_continuation_long", V7SignalClass: store.V7ClassImpulse}
	cand.V7DataFreshness.PriceAgeMs = 90_000
	d := &kernel.Decision{Symbol: "AUSDT", Action: "open_long"}
	if err := at.validateHunterV7SnapshotBudget(cand, d); err != nil {
		t.Fatalf("default gate is off: %v", err)
	}
	at.config.StrategyConfig.CoinSource.Hunter = &store.HunterConfig{V7DataTiming: &store.V7DataTimingConfig{FreshnessBudgetSeconds: map[string]float64{store.V7ClassImpulse: 60}}}
	if err := at.validateHunterV7SnapshotBudget(cand, d); err == nil || !strings.Contains(err.Error(), "snapshot_age_budget") {
		t.Fatalf("90 s over a 60 s IMPULSE budget must be rejected, got %v", err)
	}
	cand.V7DataFreshness.PriceAgeMs = 30_000
	if err := at.validateHunterV7SnapshotBudget(cand, d); err != nil {
		t.Fatalf("30 s is inside the budget: %v", err)
	}
	trig := &kernel.CandidateCoin{V7SignalClass: store.V7ClassTrigger}
	trig.V7DataFreshness.PriceAgeMs = 900_000
	if err := at.validateHunterV7SnapshotBudget(trig, d); err != nil {
		t.Fatalf("a class without a budget is not gated: %v", err)
	}
	if err := at.validateHunterV7SnapshotBudget(&kernel.CandidateCoin{V7SignalClass: store.V7ClassImpulse}, d); err != nil {
		t.Fatalf("unknown age passes: %v", err)
	}
}
