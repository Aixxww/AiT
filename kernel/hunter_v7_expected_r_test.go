package kernel

import (
	"math"
	"testing"
	"time"

	local "github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

func erRow(class, setup, dir, band, outcome string, rwin float64, t int) store.V7CalibrationOutcome {
	return store.V7CalibrationOutcome{Time: time.Unix(int64(t), 0), SetupType: setup, Direction: dir, Class: class,
		Family: store.V7Family(setup), RRBand: band, Outcome: outcome, RWin: rwin}
}

// Hand-computed shrinkage: global 4 rows (2 tp1, 1 stop), class A 3 rows,
// cell a|LONG|b 2 rows (2 tp1), k = 1.
func TestExpectedRShrinkageAndEstimate(t *testing.T) {
	rows := []store.V7CalibrationOutcome{
		erRow("A", "a_long", "LONG", "b", store.V7FeeNetTP1, 1.5, 1),
		erRow("A", "a_long", "LONG", "b", store.V7FeeNetTP1, 1.5, 2),
		erRow("A", "x_short", "SHORT", "b", store.V7FeeNetStop, 1.5, 3),
		erRow("B", "y_long", "LONG", "b", store.V7FeeNetNone, 1.5, 4),
	}
	cfg := store.EffectiveV7ExpectedR(nil)
	cal := FitHunterV7ExpectedR(rows, 1, cfg, time.Unix(10, 0))
	gT, gS := 0.5, 0.25
	cT, cS := (2+gT)/4, (1+gS)/4
	wT, wS := (2+cT)/3, (0+cS)/3
	pT, pS, nEff := cal.Rates(HunterV7ERInput{Class: "A", Family: "a", Direction: "LONG", RRBand: "b"})
	if math.Abs(pT-wT) > 1e-12 || math.Abs(pS-wS) > 1e-12 || nEff != 3 {
		t.Fatalf("rates = %v/%v/%v, want %v/%v/3", pT, pS, nEff, wT, wS)
	}
	er, lb, ok := cal.Estimate(HunterV7ERInput{Class: "A", Family: "a", Direction: "LONG", RRBand: "b", RWin: 2})
	m := wT*2 - wS
	v := wT*4 + wS - m*m
	z := math.Sqrt2 * math.Erfinv(0.8)
	if !ok || math.Abs(er-m) > 1e-12 || math.Abs(lb-(m-z*math.Sqrt(v/3))) > 1e-12 {
		t.Fatalf("estimate = %v/%v", er, lb)
	}
	if math.Abs(z-1.2815515655446004) > 1e-9 {
		t.Fatalf("z(0.90) = %v", z)
	}
	// Unseen cell falls back to its class, unseen class to global.
	if p, _, _ := cal.Rates(HunterV7ERInput{Class: "A", Family: "zz", Direction: "LONG", RRBand: "b"}); math.Abs(p-cT) > 1e-12 {
		t.Fatalf("unseen cell = %v, want class %v", p, cT)
	}
	if p, _, _ := cal.Rates(HunterV7ERInput{Class: "Q", Family: "zz", RRBand: "b"}); math.Abs(p-((0+gT)/1)) > 1e-12 {
		t.Fatalf("unseen class = %v", p)
	}
	// Pooled directions merge the cell.
	cfg.PoolDirections = true
	if FitHunterV7ExpectedR(rows, 1, cfg, time.Time{}).Cell["a|b"].N != 2 {
		t.Fatalf("pooled cell")
	}
}

// k selection prefers strong shrinkage when cells are pure noise and weak
// shrinkage when cells differ systematically.
func TestExpectedRChooseK(t *testing.T) {
	cfg := store.EffectiveV7ExpectedR(nil)
	var sep [3][]store.V7CalibrationOutcome
	for f := 0; f < 3; f++ {
		for i := 0; i < 40; i++ {
			sep[f] = append(sep[f], erRow("A", "w_long", "LONG", "b", store.V7FeeNetTP1, 1.5, f*100+i))
			sep[f] = append(sep[f], erRow("A", "l_long", "LONG", "b", store.V7FeeNetStop, 1.5, f*100+i))
		}
	}
	if k := HunterV7ERChooseK(sep[:], cfg); k != cfg.KGrid[0] {
		t.Fatalf("separated cells: k = %v, want the smallest grid value", k)
	}
	// Many small cells with the same true rate (deterministic pseudo-random
	// outcomes): cell differences are noise, so strong shrinkage wins.
	var noise [3][]store.V7CalibrationOutcome
	x := uint32(12345)
	for f := 0; f < 3; f++ {
		for c := 0; c < 30; c++ {
			for i := 0; i < 3; i++ {
				x = x*1664525 + 1013904223
				o := store.V7FeeNetStop
				if x>>16%2 == 0 {
					o = store.V7FeeNetTP1
				}
				noise[f] = append(noise[f], erRow("A", "c"+string(rune('a'+c%26))+string(rune('a'+c/26))+"_long", "LONG", "b", o, 1, f*1000+c*10+i))
			}
		}
	}
	if k := HunterV7ERChooseK(noise[:], cfg); k < 20 {
		t.Fatalf("noise cells: k = %v, want strong shrinkage", k)
	}
}

func TestExpectedRRuntimeFitAndFolds(t *testing.T) {
	cfg := store.EffectiveV7ExpectedR(nil)
	var rows []store.V7CalibrationOutcome
	for i := 0; i < cfg.MinRows-1; i++ {
		rows = append(rows, erRow("A", "a_long", "LONG", "b", store.V7FeeNetTP1, 1.2, 60-i))
	}
	if FitHunterV7ExpectedRFromOutcomes(rows, cfg, time.Unix(100, 0)) != nil {
		t.Fatalf("below MinRows there is no calibration")
	}
	rows = append(rows, erRow("A", "a_long", "LONG", "b", store.V7FeeNetStop, 1.2, 0))
	cal := FitHunterV7ExpectedRFromOutcomes(rows, cfg, time.Unix(100, 0))
	if cal == nil || cal.Global.N != cfg.MinRows || cal.Version == "" {
		t.Fatalf("runtime fit = %+v", cal)
	}
	f := HunterV7ERFolds(rows)
	if len(f[0])+len(f[1])+len(f[2]) != len(rows) || f[0][0].Time.Unix() != 0 {
		t.Fatalf("folds must be time tertiles")
	}
}

// The shadow stamp changes only the E[R] fields: tiers, reasons and risk
// stamps are identical with and without a calibration.
func TestExpectedRStampIsShadowOnly(t *testing.T) {
	sigs := []local.V7SignalOutput{
		t2TagSignal("ignition_early_long", local.V7DirLong, "TRIGGERED"),
		t2TagSignal("momentum_continuation_short", local.V7DirShort, "ARMED"),
	}
	sigs[1].Symbol = "BUSDT"
	cfg := &store.StrategyConfig{}
	cfg.CoinSource.SourceType = "hunter_v7"
	cfg.CoinSource.Hunter = &store.HunterConfig{}
	e := NewStrategyEngine(cfg)
	before := e.hunterV7SignalsToCandidateCoins(sigs, "BOTH")
	rows := []store.V7CalibrationOutcome{}
	for i := 0; i < 40; i++ {
		o := store.V7FeeNetTP1
		if i%3 == 0 {
			o = store.V7FeeNetStop
		}
		rows = append(rows, erRow(store.V7ClassTrigger, "ignition_early_long", "LONG", "1.2_1.5", o, 1.4, i))
	}
	cal := FitHunterV7ExpectedRFromOutcomes(rows, store.EffectiveV7ExpectedR(nil), time.Unix(1000, 0))
	e.SetV7ExpectedRCalibration(cal)
	after := e.hunterV7SignalsToCandidateCoins(sigs, "BOTH")
	for i := range before {
		b, a := before[i], after[i]
		if b.V7ExpectedR != nil || a.V7ExpectedR == nil || a.V7CalibrationVersion != cal.Version {
			t.Fatalf("stamp %d: before %v after %v %q", i, b.V7ExpectedR, a.V7ExpectedR, a.V7CalibrationVersion)
		}
		a.V7ExpectedR, a.V7ExpectedRLower, a.V7CalibrationVersion = nil, nil, ""
		if a.V7ExecutionTier != b.V7ExecutionTier || a.V7TierReason != b.V7TierReason || a.V7RequiredRR != b.V7RequiredRR ||
			a.V7RequiredMinStop != b.V7RequiredMinStop || a.V7RequiredMaxTP != b.V7RequiredMaxTP || a.V7Status != b.V7Status {
			t.Fatalf("candidate %d changed beyond the E[R] fields", i)
		}
	}
	// Records carry the estimate and the calibration version.
	recs := BuildHunterV7SignalRecordsTagged(sigs, after, HunterV7RecordTagOptionsFromConfig(cfg))
	db := BuildHunterV7SignalDBRecords(1, recs, time.Now(), "ACTIVE")
	if db[0].ExpectedR == nil || db[0].ExpectedRLower == nil || *db[0].ExpectedRLower > *db[0].ExpectedR || db[0].CalibrationVersion != cal.Version {
		t.Fatalf("record = %v/%v %q", db[0].ExpectedR, db[0].ExpectedRLower, db[0].CalibrationVersion)
	}
	// The LLM payload contract never carries it.
	if c := buildHunterV7ExecutionContract(after[0]); c.MinRR != after[0].V7RequiredRR {
		t.Fatalf("contract changed")
	}
	e.SetV7ExpectedRCalibration(nil)
	if e.hunterV7SignalsToCandidateCoins(sigs, "BOTH")[0].V7ExpectedR != nil {
		t.Fatalf("cleared calibration must stop stamping")
	}
}
