package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/store"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var t0 = time.Date(2026, 11, 20, 10, 3, 20, 0, time.UTC) // signal time (mid-minute)

// fakeBars returns 1m bars from 10:00 for n minutes at a flat 100, with
// per-minute overrides (minutes after 10:00).
func fakeBars(n int, over map[int]bar) []bar {
	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	out := make([]bar, 0, n)
	for i := 0; i < n; i++ {
		b := bar{OpenMs: base.Add(time.Duration(i) * time.Minute).UnixMilli(), Open: 100, High: 100.1, Low: 99.9, Close: 100}
		if o, ok := over[i]; ok {
			o.OpenMs = b.OpenMs
			b = o
		}
		out = append(out, b)
	}
	return out
}

func memStore(bars []bar) *klineStore {
	ks := newKlineStore("", true, nil)
	day := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	ks.mem["AUSDT|"+day.Format("2006-01-02")] = bars
	return ks
}

func longRow() inputRow {
	return inputRow{ID: 1, Time: t0, Symbol: "AUSDT", Direction: "LONG", Setup: "ignition_early_long", CanonSetup: "ignition_early_long",
		Tier: "WATCH", Stack: "t2", Class: "TRIGGER", RRBand: "1.2_1.5", Regime: "trend_down", OutcomeSource: store.V7OutcomeSourceWatchAudit,
		Entry: 100, Stop: 98, TP1: 103, Trigger: 99.5}
}

var wc2h = walkConfig{Horizon: 2 * time.Hour, CostPct: store.DefaultV7FeeNetCostPct}
var later = t0.Add(24 * time.Hour)

// The replay is the live OD-2 walk: anchor on the close of the first bar
// opening after the signal minute (10:04 for 10:03:20), never scoring the
// anchor bar; first touch of stop or TP1 after it; stop first on a bar
// touching both; R(TP1) from V7FeeNetRRAtTP1 with the cost; drift from the
// trigger price.
func TestWalkMatchesLiveRules(t *testing.T) {
	ks := memStore(fakeBars(200, map[int]bar{
		3:  {Open: 100, High: 104, Low: 97, Close: 100},     // signal minute: ignored
		4:  {Open: 100, High: 104, Low: 97, Close: 100.2},   // anchor bar: E = 100.2, not scored
		10: {Open: 100, High: 103.5, Low: 99.9, Close: 103}, // first TP1 touch
	}))
	res := walkRow(longRow(), wc2h, ks, later)
	wantR, _ := store.V7FeeNetRRAtTP1(100.2, 98, 103, store.DefaultV7FeeNetCostPct)
	if res.Status != "walked" || res.Outcome != store.V7FeeNetTP1 || res.Entry != 100.2 || res.R == nil || abs(*res.R-wantR) > 1e-12 {
		t.Fatalf("TP1 walk: %+v (want R %v)", res, wantR)
	}
	if res.DriftPct == nil || abs(*res.DriftPct-(100.2-99.5)/99.5*100) > 1e-9 {
		t.Fatalf("drift %v", res.DriftPct)
	}

	// Same bar touches both: stop first, R = -1.
	ks = memStore(fakeBars(200, map[int]bar{10: {Open: 100, High: 103.5, Low: 97.5, Close: 100}}))
	res = walkRow(longRow(), wc2h, ks, later)
	if res.Outcome != store.V7FeeNetStop || res.R == nil || *res.R != -1 {
		t.Fatalf("both touched: %+v", res)
	}

	// SHORT mirror: TP1 below, stop above.
	short := longRow()
	short.Direction, short.Stop, short.TP1, short.Trigger = "SHORT", 102, 97, 100.5
	ks = memStore(fakeBars(200, map[int]bar{20: {Open: 100, High: 100.1, Low: 96.9, Close: 97}}))
	res = walkRow(short, wc2h, ks, later)
	wantR, _ = store.V7FeeNetRRAtTP1(100, 102, 97, store.DefaultV7FeeNetCostPct)
	// Drift is signed in the trade direction: a short entered 0.5 below its
	// trigger has drift -(100-100.5)/100.5*100 = +0.4975%.
	if res.Outcome != store.V7FeeNetTP1 || abs(*res.R-wantR) > 1e-12 || abs(*res.DriftPct-(-(100-100.5)/100.5*100)) > 1e-9 {
		t.Fatalf("short TP1: %+v R %v drift %v want %v", res, *res.R, *res.DriftPct, wantR)
	}

	// Nothing touched within the horizon: none, R 0 (bars reach past it).
	ks = memStore(fakeBars(200, nil))
	res = walkRow(longRow(), wc2h, ks, later)
	if res.Status != "walked" || res.Outcome != store.V7FeeNetNone || res.R == nil || *res.R != 0 {
		t.Fatalf("none: %+v", res)
	}

	// A touch after the horizon does not count.
	ks = memStore(fakeBars(200, map[int]bar{3 + 121: {Open: 100, High: 110, Low: 99.9, Close: 100}}))
	if res = walkRow(longRow(), wc2h, ks, later); res.Outcome != store.V7FeeNetNone {
		t.Fatalf("touch after the horizon: %+v", res)
	}

	// Degenerate entry: anchor close already through TP1 -> no R.
	ks = memStore(fakeBars(200, map[int]bar{4: {Open: 100, High: 103.2, Low: 99.9, Close: 103.1}}))
	if res = walkRow(longRow(), wc2h, ks, later); res.Outcome != store.V7FeeNetDegenerateTP1 || res.R != nil {
		t.Fatalf("degenerate tp1: %+v", res)
	}
}

func TestWalkSkips(t *testing.T) {
	ks := memStore(fakeBars(200, nil))
	r := longRow()
	r.TP1 = 0
	if res := walkRow(r, wc2h, ks, later); res.Status != skipNoGeometry {
		t.Fatalf("no TP1: %+v", res)
	}
	if res := walkRow(longRow(), wc2h, ks, t0.Add(time.Hour)); res.Status != skipHorizonOpen {
		t.Fatalf("horizon not over: %+v", res)
	}
	// Bars stop after 30 minutes and nothing was touched: the live code
	// would close the walk as "none" on wall-clock time, so the replay
	// rejects it as incomplete instead.
	ks = memStore(fakeBars(34, nil))
	if res := walkRow(longRow(), wc2h, ks, later); res.Status != skipIncompleteKlines {
		t.Fatalf("short window: %+v", res)
	}
	// ...but a touch inside the bars that exist is a real outcome.
	ks = memStore(fakeBars(34, map[int]bar{20: {Open: 100, High: 100.1, Low: 97, Close: 98}}))
	if res := walkRow(longRow(), wc2h, ks, later); res.Status != "walked" || res.Outcome != store.V7FeeNetStop {
		t.Fatalf("stop inside a short window: %+v", res)
	}
	// Offline with no cache: klines error.
	if res := walkRow(longRow(), wc2h, newKlineStore(t.TempDir(), true, nil), later); res.Status != skipKlinesError {
		t.Fatalf("no klines: %+v", res)
	}
}

func TestDedupRows(t *testing.T) {
	mk := func(sym string, min int) result {
		r := longRow()
		r.Symbol, r.Time = sym, t0.Add(time.Duration(min)*time.Minute)
		return result{inputRow: r}
	}
	rs := []result{mk("A", 0), mk("A", 6), mk("B", 6), mk("A", 119), mk("A", 120), mk("A", 125), mk("A", 300)}
	dedupRows(rs, 2*time.Hour)
	var got []string
	for _, r := range rs {
		got = append(got, fmt.Sprintf("%s%d:%v", r.Symbol, int(r.Time.Sub(t0).Minutes()), r.Dedup))
	}
	want := "[A0:true A6:false B6:true A119:false A120:true A125:false A300:true]"
	if fmt.Sprint(got) != want {
		t.Fatalf("dedup %v, want %s", got, want)
	}
	dedupRows(rs, -1)
	for _, r := range rs {
		if !r.Dedup {
			t.Fatal("window <= 0 keeps every row")
		}
	}
}

// The tables use the dashboard cell rule: n >= 30 and the 90% CI above 0 =
// earned; below 0 = blocked; else probation. Rows without R are listed,
// not scored.
func TestTablesUseCellRule(t *testing.T) {
	rule := store.EffectiveV7CellRule(nil)
	var rs []result
	add := func(tier, reason, outcome string, r *float64, n int) {
		for i := 0; i < n; i++ {
			in := longRow()
			in.Tier, in.TierReason = tier, reason
			rs = append(rs, result{inputRow: in, Walk: walkResult{Status: "walked", Outcome: outcome, R: r}, Dedup: i%2 == 0})
		}
	}
	win, loss := 1.4, -1.0
	add("REVIEWABLE", "t2_impulse_triggered", store.V7FeeNetTP1, &win, 30)
	add("WATCH", "t2_awaiting_trigger", store.V7FeeNetStop, &loss, 40)
	add("WATCH", "t2_awaiting_trigger", store.V7FeeNetDegenerateTP1, nil, 3)
	rs = append(rs, result{inputRow: longRow(), Walk: walkResult{Status: skipNoGeometry}})
	tables := buildTables(rs, false, rule)
	byKey := map[string]groupStats{}
	for _, g := range tables["tier"] {
		byKey[g.Key] = g
	}
	rv, wa := byKey["REVIEWABLE"], byKey["WATCH"]
	if rv.N != 30 || rv.CellRule != store.V7CellStatusEarned || rv.TP1 != 30 || abs(rv.MeanR-1.4) > 1e-12 {
		t.Fatalf("REVIEWABLE %+v", rv)
	}
	if wa.N != 40 || wa.Rows != 44 || wa.CellRule != store.V7CellStatusBlocked || wa.NoR[store.V7FeeNetDegenerateTP1] != 3 || wa.Skipped[skipNoGeometry] != 1 {
		t.Fatalf("WATCH %+v", wa)
	}
	// The dedup tables use only kept rows: 15 REVIEWABLE < 30 -> probation.
	for _, g := range buildTables(rs, true, rule)["tier"] {
		if g.Key == "REVIEWABLE" && (g.N != 15 || g.CellRule != store.V7CellStatusProbation) {
			t.Fatalf("dedup REVIEWABLE %+v", g)
		}
	}
	// Short-ignition groupings select by canonical setup only.
	si := longRow()
	si.Setup, si.CanonSetup, si.Direction = "t2_ignition_early_short", "ignition_early_short", "SHORT"
	only := buildTables([]result{{inputRow: si, Walk: walkResult{Status: "walked", Outcome: "stop", R: &loss}}}, false, rule)
	if len(only["short ignition"]) != 1 || only["short ignition"][0].N != 1 || len(buildTables(rs, false, rule)["short ignition"]) != 0 {
		t.Fatal("short ignition grouping")
	}
}

func TestKlineCache(t *testing.T) {
	dir := t.TempDir()
	calls := 0
	fetch := func(symbol string, start, end time.Time) ([]bar, error) {
		calls++
		var out []bar
		for m := start; m.Before(end); m = m.Add(time.Minute) {
			out = append(out, bar{OpenMs: m.UnixMilli(), Open: 1, High: 1, Low: 1, Close: 1})
		}
		return out, nil
	}
	ks := newKlineStore(dir, false, fetch)
	ks.now = func() time.Time { return later.Add(48 * time.Hour) }
	bars, err := ks.window("AUSDT", t0, t0.Add(2*time.Hour))
	if err != nil || len(bars) != 120 || calls != 1 { // opens 10:04 .. 12:03
		t.Fatalf("bars %d calls %d err %v", len(bars), calls, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AUSDT", "2026-11-20.json")); err != nil {
		t.Fatal("complete day must be cached")
	}
	off := newKlineStore(dir, true, nil)
	if bars, err := off.window("AUSDT", t0, t0.Add(2*time.Hour)); err != nil || len(bars) != 120 || off.Cached != 1 {
		t.Fatalf("offline from cache: %d %v", len(bars), err)
	}
	// A day that has not ended is fetched but not cached.
	ks2 := newKlineStore(t.TempDir(), false, fetch)
	ks2.now = func() time.Time { return t0.Add(3 * time.Hour) }
	if _, err := ks2.window("AUSDT", t0, t0.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(ks2.dir, "AUSDT", "2026-11-20.json")); err == nil {
		t.Fatal("an unfinished day must not be cached")
	}
}

// newTestDB writes rows to a fresh SQLite file with the store schema.
func newTestDB(t *testing.T, recs []store.HunterV7SignalRecord) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&store.HunterV7SignalRecord{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE strategies (id TEXT PRIMARY KEY, config TEXT)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO strategies (id, config) VALUES (?, ?)", "s1",
		`{"coin_source":{"hunter_config":{"v7_outcome_tags":{"fee_net_cost_pct":0.002}}}}`).Error; err != nil {
		t.Fatal(err)
	}
	if len(recs) > 0 {
		if err := db.Create(&recs).Error; err != nil {
			t.Fatal(err)
		}
	}
	sqlDB, _ := db.DB()
	sqlDB.Close()
	return path
}

func fileHash(t *testing.T, path string) [32]byte {
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(b)
}

// The database is opened read-only: writes fail, and the file is
// byte-identical after a full replay.
func TestReadOnlyAndEndToEnd(t *testing.T) {
	trig := 99.5
	storedR := -1.0
	recs := []store.HunterV7SignalRecord{
		{Timestamp: t0, Symbol: "AUSDT", Direction: "LONG", SetupType: "ignition_early_long", ExecutionTier: "REVIEWABLE", TierReason: "t2_trigger_triggered",
			EntryZoneLower: 99.8, EntryZoneUpper: 100.2, InvalidationPrice: 98, TP1Price: 103, TriggerPrice: &trig, Stack: "t2", SignalClass: "TRIGGER",
			RRBand: "1.2_1.5", MarketRegime: "trend_down", OutcomeSource: store.V7OutcomeSourcePromptOpenReview, FeeNetOutcome: store.V7FeeNetStop, FeeNetR: &storedR},
		{Timestamp: t0.Add(5 * time.Minute), Symbol: "AUSDT", Direction: "SHORT", SetupType: "t2_ignition_early_short", ExecutionTier: "WATCH", TierReason: "class_shadow_only",
			EntryZoneLower: 99.8, EntryZoneUpper: 100.2, InvalidationPrice: 102, TP1Price: 97, Stack: "t2", MarketRegime: "trend_down", OutcomeSource: store.V7OutcomeSourceShadow},
		{Timestamp: t0.Add(6 * time.Minute), Symbol: "AUSDT", Direction: "LONG", SetupType: "trend_breakout_long", ExecutionTier: "", InvalidationPrice: 98, TP1Price: 103},
	}
	path := newTestDB(t, recs)
	before := fileHash(t, path)

	db, err := openReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("DELETE FROM hunter_v7_signal_records").Error; err == nil {
		t.Fatal("a write must fail on the read-only connection")
	}
	if err := db.Exec("CREATE TABLE x (a INT)").Error; err == nil {
		t.Fatal("DDL must fail on the read-only connection")
	}
	// Each read-only layer holds on its own: query_only is on, and with it
	// switched off the mode=ro connection still refuses writes.
	var qo int
	if err := db.Raw("PRAGMA query_only").Scan(&qo).Error; err != nil || qo != 1 {
		t.Fatalf("query_only = %d (%v), want 1", qo, err)
	}
	if err := db.Exec("PRAGMA query_only = OFF").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("DELETE FROM hunter_v7_signal_records").Error; err == nil {
		t.Fatal("mode=ro must refuse writes even with query_only off")
	}
	cost, from, err := resolveCost(db, 0, "s1")
	if err != nil || cost != 0.002 || from != "strategy s1" {
		t.Fatalf("cost %v %q %v", cost, from, err)
	}
	if c, _, _ := resolveCost(db, 0, ""); c != store.DefaultV7FeeNetCostPct {
		t.Fatalf("default cost %v", c)
	}
	loaded, err := loadRecords(db, rowFilter{})
	if err != nil || len(loaded) != 2 {
		t.Fatalf("loaded %d rows (router-filtered rows excluded by default), err %v", len(loaded), err)
	}

	// Stop for the long at minute 20; TP1 for the short at minute 30.
	ks := memStore(fakeBars(200, map[int]bar{
		20: {Open: 100, High: 100.1, Low: 97.9, Close: 98.5},
		30: {Open: 100, High: 100.1, Low: 96.9, Close: 97.2},
	}))
	results := replay(loaded, walkConfig{Horizon: 2 * time.Hour, CostPct: cost}, ks, false, 2*time.Hour, later)
	if results[0].Walk.Outcome != store.V7FeeNetStop || results[1].Walk.Outcome != store.V7FeeNetTP1 {
		t.Fatalf("outcomes %+v / %+v", results[0].Walk, results[1].Walk)
	}
	// The strategy's cost (0.002) reaches the live walk's R(TP1).
	if wantR, _ := store.V7FeeNetRRAtTP1(results[1].Walk.Entry, 102, 97, 0.002); abs(*results[1].Walk.R-wantR) > 1e-12 {
		t.Fatalf("TP1 R %v, want %v at cost 0.002", *results[1].Walk.R, wantR)
	}
	if results[1].Class != store.V7ClassTrigger || results[1].ClassSource != "registry" || results[1].Admission != store.V7AdmissionShadowOnly {
		t.Fatalf("short ignition class from the registry: %+v", results[1].inputRow)
	}
	if results[0].Entry != 100 {
		t.Fatalf("registered entry %v, want the zone mid", results[0].Entry)
	}
	sum := buildSummary(results, walkConfig{Horizon: 2 * time.Hour, CostPct: cost}, "strategy s1", 2*time.Hour, store.EffectiveV7CellRule(nil), ks, path, rowFilter{})
	if sum.Agreement.Compared != 1 || sum.Agreement.SameOutcome != 1 || sum.Agreement.SameR != 1 {
		t.Fatalf("agreement %+v", sum.Agreement)
	}
	if len(sum.TablesDedup["short ignition"]) != 1 || sum.Regimes["trend_down"] != 2 {
		t.Fatalf("summary tables %+v regimes %v", sum.TablesDedup["short ignition"], sum.Regimes)
	}
	out := t.TempDir()
	if err := writeOutputs(out, results, sum); err != nil {
		t.Fatal(err)
	}
	md, _ := os.ReadFile(filepath.Join(out, "summary.md"))
	for _, want := range []string{"(a) Is WATCH too strict?", "(b) Should short ignition stay shadow-only?", "trend_down 2", "Conclusions hold only for these regimes"} {
		if !strings.Contains(string(md), want) {
			t.Fatalf("summary.md lacks %q", want)
		}
	}
	sqlDB, _ := db.DB()
	sqlDB.Close()
	if fileHash(t, path) != before {
		t.Fatal("the database file changed")
	}
}

// The class x rr_band table equals the dashboard's (store.BuildV7CellTables)
// on the same rows: same n, mean, CI and status.
func TestTablesMatchDashboardCells(t *testing.T) {
	rule := store.EffectiveV7CellRule(nil)
	var rs []result
	var recs []store.HunterV7SignalRecord
	vals := []float64{1.3, -1, -1, 0, 1.1, -1, 2.0, 0, -1, 1.25}
	for i := 0; i < 40; i++ {
		v := vals[i%len(vals)]
		in := longRow()
		in.Class, in.RRBand = "TRIGGER", "1.2_1.5"
		if i%3 == 0 {
			in.RRBand = "1.5_2.0"
		}
		r := v
		rs = append(rs, result{inputRow: in, Walk: walkResult{Status: "walked", Outcome: store.V7FeeNetStop, R: &r}})
		recs = append(recs, store.HunterV7SignalRecord{SignalClass: in.Class, RRBand: in.RRBand, FeeNetR: &r, FeeNetOutcome: store.V7FeeNetStop,
			OutcomeSource: store.V7OutcomeSourcePromptOpenReview})
	}
	mine := map[string]groupStats{}
	for _, g := range buildTables(rs, false, rule)["class x rr_band (all tiers)"] {
		mine[g.Key] = g
	}
	dash := store.BuildV7CellTables(recs, store.V7OutcomeSourcePromptOpenReview, rule)
	if len(dash.Cells) != 2 {
		t.Fatalf("dashboard cells %d", len(dash.Cells))
	}
	for _, c := range dash.Cells {
		g := mine[c.Class+" | "+c.RRBand]
		if g.N != c.N || abs(g.MeanR-c.MeanR) > 1e-12 || g.CILo != c.CILo || g.CIHi != c.CIHi || g.CellRule != c.EvaluatedStatus {
			t.Fatalf("cell %s|%s: tool %+v, dashboard %+v", c.Class, c.RRBand, g, c)
		}
	}
}
