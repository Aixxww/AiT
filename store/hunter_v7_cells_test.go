package store

import (
	"math"
	"math/rand"
	"testing"
	"time"
)

func cellRec(id int64, class, band, regime, source, outcome string, r *float64) HunterV7SignalRecord {
	return HunterV7SignalRecord{ID: id, Timestamp: time.Date(2026, 11, 10, 0, 0, int(id), 0, time.UTC),
		SignalClass: class, RRBand: band, MarketRegime: regime, OutcomeSource: source,
		FeeNetOutcome: outcome, FeeNetR: r, CellStatus: V7CellStatusProbation}
}

func fp(v float64) *float64 { return &v }

func TestU12CellRuleDefaults(t *testing.T) {
	d := EffectiveV7CellRule(nil)
	if d.MinSamples != 30 || d.CILevel != 0.90 || d.BootstrapResamples != 5000 || d.Seed != 20261118 || d.Quantiles != 5 {
		t.Fatalf("defaults = %+v", d)
	}
	if c := EffectiveV7CellRule(&V7CellRuleConfig{MinSamples: 20, CILevel: 0.95}); c.MinSamples != 20 || c.CILevel != 0.95 {
		t.Fatalf("override = %+v", c)
	}
	if c := EffectiveV7CellRule(&V7CellRuleConfig{CILevel: 1.5}); c.CILevel != 0.90 {
		t.Fatal("an out-of-range CI level must fall back to the default")
	}
}

// The shared bootstrap reproduces the U7 inline computation draw for draw.
func TestU12BootstrapMatchesU7Inline(t *testing.T) {
	xs := []float64{-1, -1, 0.8, 1.2, -1, 2.0, 0.4, -1, 1.1}
	n, ci := 5000, 0.90
	ref := rand.New(rand.NewSource(7))
	means := make([]float64, n)
	for s := range means {
		sum := 0.0
		for range xs {
			sum += xs[ref.Intn(len(xs))]
		}
		means[s] = sum / float64(len(xs))
	}
	sortFloats(means)
	tail := (1 - ci) / 2
	wantLo, wantHi := means[int(math.Round(float64(n)*tail))], means[int(math.Round(float64(n)*(1-tail)))-1]
	lo, hi, ok := V7BootstrapMeanCI(xs, n, ci, rand.New(rand.NewSource(7)))
	if !ok || lo != wantLo || hi != wantHi {
		t.Fatalf("got [%v, %v], want [%v, %v]", lo, hi, wantLo, wantHi)
	}
	if _, _, ok := V7BootstrapMeanCI(nil, n, ci, rand.New(rand.NewSource(1))); ok {
		t.Fatal("empty sample must report !ok")
	}
}

func sortFloats(xs []float64) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j] < xs[j-1]; j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
}

func TestU12EvaluateCellStatus(t *testing.T) {
	for _, c := range []struct {
		n      int
		lo, hi float64
		want   string
	}{
		{30, 0.01, 0.5, V7CellStatusEarned},
		{29, 0.01, 0.5, V7CellStatusProbation}, // n < N_min
		{30, -0.5, -0.01, V7CellStatusBlocked},
		{30, -0.1, 0.2, V7CellStatusProbation},
		{30, 0, 0.2, V7CellStatusProbation}, // lower bound must be > 0
	} {
		if got := V7EvaluateCellStatus(c.n, c.lo, c.hi, true, 30); got != c.want {
			t.Errorf("%+v -> %s", c, got)
		}
	}
	if V7EvaluateCellStatus(40, 1, 2, false, 30) != V7CellStatusProbation {
		t.Fatal("no CI must be probation")
	}
}

func TestU12CellTables(t *testing.T) {
	cfg := EffectiveV7CellRule(&V7CellRuleConfig{MinSamples: 3, BootstrapResamples: 500})
	recs := []HunterV7SignalRecord{
		cellRec(1, "TRIGGER", "1.2_1.5", "trend_up", V7OutcomeSourcePromptOpenReview, V7FeeNetTP1, fp(1.3)),
		cellRec(2, "TRIGGER", "1.2_1.5", "trend_up", V7OutcomeSourcePromptOpenReview, V7FeeNetTP1, fp(1.2)),
		cellRec(3, "TRIGGER", "1.2_1.5", "", V7OutcomeSourcePromptOpenReview, V7FeeNetTP1, fp(1.4)),
		cellRec(4, "TRIGGER", "lt_1.0", "trend_up", V7OutcomeSourcePromptOpenReview, V7FeeNetStop, fp(-1)),
		cellRec(5, "", "", "", V7OutcomeSourcePromptOpenReview, V7FeeNetNone, fp(0.1)),
		cellRec(6, "IMPULSE", "1.5_2.0", "chop", V7OutcomeSourceShadow, V7FeeNetStop, fp(-1)),
		cellRec(7, "IMPULSE", "1.5_2.0", "chop", V7OutcomeSourcePromptOpenReview, V7FeeNetDegenerateStop, nil),
	}
	tab := BuildV7CellTables(recs, V7CellSourceDefault, cfg)
	if tab.Rows != 5 || len(tab.Classes) != 2 {
		t.Fatalf("rows=%d classes=%+v", tab.Rows, tab.Classes)
	}
	trig := tab.Classes[0]
	if trig.Class != "TRIGGER" || trig.N != 4 || trig.TP1 != 3 || trig.Stop != 1 || math.Abs(trig.MeanR-0.725) > 1e-12 || trig.TP1Rate != 0.75 {
		t.Fatalf("TRIGGER class = %+v", trig)
	}
	if tab.Classes[1].Class != V7ClassUnassigned || tab.Classes[1].None != 1 {
		t.Fatalf("unassigned = %+v", tab.Classes[1])
	}
	// Bands sort by edge: lt_1.0 before 1.2_1.5.
	if tab.Cells[0].RRBand != "lt_1.0" || tab.Cells[1].RRBand != "1.2_1.5" || tab.Cells[2].RRBand != V7BandUnknown {
		t.Fatalf("cell order = %+v", tab.Cells)
	}
	c := tab.Cells[1]
	if c.N != 3 || c.EvaluatedStatus != V7CellStatusEarned || c.CILo <= 0 || len(c.StampedStatus) != 1 || c.StampedStatus[0] != "probation" {
		t.Fatalf("earned cell = %+v", c)
	}
	var regimes []string
	for _, r := range tab.CellRegimes {
		if r.Class == "TRIGGER" && r.RRBand == "1.2_1.5" {
			regimes = append(regimes, r.Regime)
		}
	}
	if len(regimes) != 2 || regimes[0] != "trend_up" || regimes[1] != V7CellRegimeUnknown {
		t.Fatalf("regimes = %v", regimes)
	}
	if all := BuildV7CellTables(recs, V7CellSourceAll, cfg); all.Rows != 6 {
		t.Fatalf("all sources rows = %d", all.Rows)
	}
	if sh := BuildV7CellTables(recs, V7OutcomeSourceShadow, cfg); sh.Rows != 1 || sh.Classes[0].Class != "IMPULSE" {
		t.Fatalf("shadow = %+v", sh)
	}
	if ex := V7CellExclusions(recs, V7CellSourceDefault); ex[V7FeeNetDegenerateStop] != 1 || len(ex) != 1 {
		t.Fatalf("exclusions = %v", ex)
	}
	// A row's CI does not depend on the other rows.
	one := BuildV7CellTables(recs[:3], V7CellSourceDefault, cfg)
	if one.Cells[0].CILo != c.CILo || one.Cells[0].CIHi != c.CIHi {
		t.Fatal("CI must be independent of other rows")
	}
	if empty := BuildV7CellTables(nil, V7CellSourceDefault, cfg); empty.Classes == nil || empty.Cells == nil || empty.CellRegimes == nil {
		t.Fatal("empty tables must be non-nil lists")
	}
}

func TestU12CalibrationDrift(t *testing.T) {
	cfg := EffectiveV7CellRule(&V7CellRuleConfig{Quantiles: 2, BootstrapResamples: 500})
	day := func(d int) time.Time { return time.Date(2026, 11, d, 0, 0, 0, 0, time.UTC) }
	prevFrom, from, to := day(1), day(8), day(15)
	mk := func(id int64, at time.Time, ver string, er, r float64) HunterV7SignalRecord {
		rec := cellRec(id, "TRIGGER", "1.2_1.5", "", V7OutcomeSourcePromptOpenReview, V7FeeNetTP1, fp(r))
		rec.Timestamp, rec.CalibrationVersion, rec.ExpectedR = at, ver, fp(er)
		return rec
	}
	var recs []HunterV7SignalRecord
	// v1: predictions far above realized in both windows -> drift.
	for i := 0; i < 6; i++ {
		recs = append(recs, mk(int64(i), day(2).Add(time.Duration(i)*time.Hour), "v1", 2+float64(i)*0.1, -1+float64(i%2)*0.1))
		recs = append(recs, mk(int64(100+i), day(9).Add(time.Duration(i)*time.Hour), "v1", 2+float64(i)*0.1, -1+float64(i%2)*0.1))
	}
	// v2: well calibrated in the current window, absent from the previous.
	for i := 0; i < 6; i++ {
		r := []float64{-1, 1}[i%2]
		recs = append(recs, mk(int64(200+i), day(10).Add(time.Duration(i)*time.Hour), "v2", 0, r))
	}
	// Outside both windows, and no expected_r: ignored.
	recs = append(recs, mk(300, day(20), "v1", 5, -1))
	noER := cellRec(301, "TRIGGER", "1.2_1.5", "", V7OutcomeSourcePromptOpenReview, V7FeeNetTP1, fp(1))
	noER.Timestamp, noER.CalibrationVersion = day(9), "v1"
	recs = append(recs, noER)

	d := BuildV7CalibrationDrift(recs, prevFrom, from, to, V7CellSourceDefault, cfg)
	if len(d) != 2 || d[0].Version != "v1" || d[1].Version != "v2" {
		t.Fatalf("versions = %+v", d)
	}
	v1, v2 := d[0], d[1]
	if v1.Current.N != 6 || v1.Previous.N != 6 || !v1.Drift || v1.Current.Outside != 2 || len(v1.Current.Quantiles) != 2 {
		t.Fatalf("v1 = %+v", v1)
	}
	if v1.Current.Quantiles[0].PredMean >= v1.Current.Quantiles[1].PredMean {
		t.Fatal("quantiles must be ordered by expected_r")
	}
	if v2.Drift || v2.Previous.Sufficient || !v2.Current.Sufficient {
		t.Fatalf("v2 = %+v", v2)
	}
	if none := BuildV7CalibrationDrift(recs, prevFrom, from, to, V7OutcomeSourceShadow, cfg); len(none) != 0 {
		t.Fatalf("source filter: %+v", none)
	}
}
