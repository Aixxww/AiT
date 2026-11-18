package trader

import (
	"testing"
	"time"

	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/store"
)

func TestHunterV7ERRefresherCadence(t *testing.T) {
	cfg := store.EffectiveV7ExpectedR(&store.V7ExpectedRConfig{Enabled: true})
	var rows []store.V7CalibrationOutcome
	add := func(n int, t0 time.Time) {
		for i := 0; i < n; i++ {
			rows = append(rows, store.V7CalibrationOutcome{Time: t0.Add(time.Duration(i) * time.Minute), SetupType: "ignition_early_long",
				Direction: "LONG", Class: store.V7ClassTrigger, Family: "ignition_early", RRBand: "1.2_1.5", Outcome: store.V7FeeNetTP1, RWin: 1.4})
		}
	}
	installs := 0
	var got *kernel.HunterV7ERCalibration
	r := newHunterV7ERRefresher(cfg, 0.0015, func(time.Time) ([]store.V7CalibrationOutcome, error) { return rows, nil },
		func(c *kernel.HunterV7ERCalibration) { installs++; got = c })
	t0 := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	add(cfg.MinRows-1, t0)
	r.maybeRefit(t0)
	if installs != 0 {
		t.Fatalf("below MinRows must not install")
	}
	add(1, t0)
	r.maybeRefit(t0.Add(time.Minute)) // inside the check interval: skipped
	if installs != 0 {
		t.Fatalf("check interval not honoured")
	}
	r.maybeRefit(t0.Add(61 * time.Minute))
	if installs != 1 || got == nil || got.Global.N != cfg.MinRows {
		t.Fatalf("first fit: installs=%d", installs)
	}
	add(cfg.RefitMinNewOutcomes-1, t0)
	r.maybeRefit(t0.Add(130 * time.Minute))
	if installs != 1 {
		t.Fatalf("refit before %d new outcomes in a cell", cfg.RefitMinNewOutcomes)
	}
	add(1, t0)
	r.maybeRefit(t0.Add(200 * time.Minute))
	if installs != 2 {
		t.Fatalf("refit at %d new outcomes in a cell", cfg.RefitMinNewOutcomes)
	}
	r.maybeRefit(t0.Add(time.Duration(cfg.RefitMaxAgeHours)*time.Hour + 300*time.Minute))
	if installs != 3 {
		t.Fatalf("refit after the max age")
	}
	var nilR *hunterV7ERRefresher
	nilR.maybeRefit(t0) // disabled: no-op
}
