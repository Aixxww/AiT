package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/store"
	aittrader "github.com/Aixxww/AiT/trader"
)

// Fix 09: the outcome tracker across a resume.

func newResumeTracker(st *store.Store) *aittrader.SignalOutcomeTracker {
	tr := aittrader.NewSignalOutcomeTracker(&aittrader.TrackerConfig{
		PollInterval: time.Minute, TimeoutDuration: 2 * time.Hour, MaxTracked: 500, SnapshotLimit: 180,
		ActiveOutcomeInterval: time.Second, EnableDynamicStop: true, TrackPartialTargets: true,
		FeeNetCostPct: store.DefaultV7FeeNetCostPct,
	}, nil)
	tr.SetOutcomeCallback(func(o aittrader.TrackedOutcome) {
		_ = st.HunterV7Signal().UpdateTrackOutcome(o.RecordID, validationTrackUpdate(o))
	})
	return tr
}

// flatBars are 1m candles (close-time stamped, as the live sources) from
// the minute of from to the minute of to, flat at 100 with overrides keyed
// by minutes after from's minute.
func flatBars(from, to time.Time, over map[int][3]float64) []aittrader.TrackedCandle {
	var out []aittrader.TrackedCandle
	start := from.Truncate(time.Minute)
	for i := 0; !start.Add(time.Duration(i) * time.Minute).After(to); i++ {
		h, l, c := 100.1, 99.9, 100.0
		if o, ok := over[i]; ok {
			h, l, c = o[0], o[1], o[2]
		}
		out = append(out, aittrader.TrackedCandle{T: start.Add(time.Duration(i+1)*time.Minute - time.Millisecond), Open: 100, High: h, Low: l, Close: c})
	}
	return out
}

func resumeRow(sym string, ts time.Time, status string) store.HunterV7SignalRecord {
	trig := 99.9
	return store.HunterV7SignalRecord{Timestamp: ts, Symbol: sym, Direction: "LONG", SetupType: "ignition_early_long",
		ExecutionTier: "REVIEWABLE", EntryZoneLower: 99.9, EntryZoneUpper: 100.1, InvalidationPrice: 98, TP0Price: 101,
		TP1Price: 103, TP2Price: 105, TriggerPrice: &trig, OutcomeSource: store.V7OutcomeSourcePromptOpenReview,
		TrackStatus: status, TrackMFE: 1.5}
}

func openTestStore(t *testing.T, path string) *store.Store {
	t.Helper()
	st, err := store.New(path)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func rowByID(t *testing.T, st *store.Store, id int64) store.HunterV7SignalRecord {
	t.Helper()
	var r store.HunterV7SignalRecord
	if err := st.GormDB().First(&r, id).Error; err != nil {
		t.Fatal(err)
	}
	return r
}

func TestStateSavesTrackedActiveIDs(t *testing.T) {
	st := openTestStore(t, filepath.Join(t.TempDir(), "v.db"))
	defer st.Close()
	opts := newStateOpts(t.TempDir(), 3, false)
	s, err := snapshotHarnessState(opts, 1, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(s)
	if !strings.Contains(string(b), `"tracked_active_ids":[]`) {
		t.Fatalf("no tracker / none active must save [] (not null): %s", b)
	}
	tr := newResumeTracker(st)
	now := time.Now().UTC()
	tr.RegisterWithMeta(aittrader.TrackedSignalMeta{}, 7, "BUSDT", "LONG", "x", "REVIEWABLE", 100, 98, 0, 103, 0, now)
	tr.RegisterWithMeta(aittrader.TrackedSignalMeta{}, 3, "AUSDT", "LONG", "x", "REVIEWABLE", 100, 98, 0, 103, 0, now)
	opts.outcomeTracker = tr
	if s, _ = snapshotHarnessState(opts, 1, 1, 0); !reflect.DeepEqual(s.TrackedActiveIDs, []int64{3, 7}) {
		t.Fatalf("ids %v, want [3 7]", s.TrackedActiveIDs)
	}
	// A state file written before fix 09 has no field: nil, reported.
	var old harnessState
	_ = json.Unmarshal([]byte(`{"schema_version":1,"run_id":"x"}`), &old)
	sum, err := restoreTrackedOutcomes(validationOptions{store: st, outcomeTracker: tr}, old, now, nil)
	if err != nil || old.TrackedActiveIDs != nil || !strings.Contains(sum.Note, "before fix 09") {
		t.Fatalf("old state: %+v %v", sum, err)
	}
}

// Every branch of the resume decision, against a real store.
func TestRestoreTrackedOutcomes(t *testing.T) {
	st := openTestStore(t, filepath.Join(t.TempDir(), "v.db"))
	defer st.Close()
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	rows := []store.HunterV7SignalRecord{
		resumeRow("AUSDT", now.Add(-30*time.Minute), "ACTIVE"),  // 0 re-register, stop hit during the gap
		resumeRow("BUSDT", now.Add(-3*time.Hour), "ACTIVE"),     // 1 too old -> orphan
		resumeRow("CUSDT", now.Add(-20*time.Minute), "ACTIVE"),  // 2 backfill starts late -> orphan
		resumeRow("DUSDT", now.Add(-40*time.Minute), "WIN_TP1"), // 3 already final
		resumeRow("EUSDT", now.Add(-40*time.Minute), "ACTIVE"),  // 4 re-register, still open
		resumeRow("FUSDT", now.Add(-10*time.Minute), "ACTIVE"),  // 5 not in the saved ids (another run)
		resumeRow("GUSDT", now.Add(-116*time.Minute), "ACTIVE"), // 6 just past the limit -> orphan
	}
	if err := st.HunterV7Signal().CreateBatch(rows); err != nil {
		t.Fatal(err)
	}
	ids := func(idx ...int) []int64 {
		var out []int64
		for _, i := range idx {
			out = append(out, rows[i].ID)
		}
		return out
	}
	saved := append(ids(0, 1, 2, 3, 4, 6), 99999)
	var fetched []string
	backfill := func(symbol string, from, to time.Time) []aittrader.TrackedCandle {
		fetched = append(fetched, symbol)
		switch symbol {
		case "AUSDT":
			return flatBars(from, to, map[int][3]float64{12: {100.1, 97.5, 98}}) // stop at minute 12
		case "CUSDT":
			return flatBars(from.Add(10*time.Minute), to, nil) // first 10 minutes missing
		default:
			return flatBars(from, to, nil)
		}
	}
	tr := newResumeTracker(st)
	tr.SetCandleBackfillSource(func(string, time.Time, time.Time) []aittrader.TrackedCandle { return nil })
	opts := validationOptions{store: st, outcomeTracker: tr}
	sum, err := restoreTrackedOutcomes(opts, harnessState{TrackedActiveIDs: saved}, now, backfill)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sum.Reregistered, ids(0, 4)) || !reflect.DeepEqual(sum.Orphaned, ids(1, 2, 6)) ||
		sum.AlreadyFinal != 1 || sum.Missing != 1 || sum.SavedActive != 7 {
		t.Fatalf("summary %+v", sum)
	}
	if sum.OrphanReasons["older_than_backfill"] != 2 || sum.OrphanReasons["backfill_incomplete"] != 1 {
		t.Fatalf("orphan reasons %v", sum.OrphanReasons)
	}
	if reflect.DeepEqual(fetched, []string{}) || strings.Contains(strings.Join(fetched, ","), "BUSDT") || strings.Contains(strings.Join(fetched, ","), "GUSDT") {
		t.Fatalf("rows past the limit must not be fetched: %v", fetched)
	}
	// Re-registered and walked over the gap: the stop is found.
	a := rowByID(t, st, rows[0].ID)
	if a.TrackStatus != string(aittrader.TrackedStop) || a.FeeNetOutcome != store.V7FeeNetStop {
		t.Fatalf("AUSDT after resume: status %q fee-net %q", a.TrackStatus, a.FeeNetOutcome)
	}
	// Still open: tracked again, so the next state save lists it.
	if got := trackedActiveIDs(tr); !reflect.DeepEqual(got, ids(4)) {
		t.Fatalf("tracker ACTIVE after resume %v, want %v", got, ids(4))
	}
	// Orphaned: status + exit time only; the other track values stay.
	b := rowByID(t, st, rows[1].ID)
	if b.TrackStatus != trackRestartOrphaned || b.TrackExitTime == nil || b.TrackMFE != 1.5 {
		t.Fatalf("orphan row %+v", b)
	}
	if d := rowByID(t, st, rows[3].ID); d.TrackStatus != "WIN_TP1" || d.TrackExitTime != nil {
		t.Fatalf("final row touched: %+v", d)
	}
	if f := rowByID(t, st, rows[5].ID); f.TrackStatus != "ACTIVE" {
		t.Fatalf("a row not saved by this run must be left alone: %+v", f)
	}
}

// The point of re-registration: the outcome equals an uninterrupted run's.
func TestResumeOutcomeEqualsUninterrupted(t *testing.T) {
	ts := time.Date(2026, 11, 22, 10, 0, 20, 0, time.UTC)
	now := ts.Add(100 * time.Minute)
	over := map[int][3]float64{5: {101.2, 99.9, 101}, 30: {103.4, 100.5, 103}} // TP0 then TP1
	bars := flatBars(ts, now, over)

	run := func(resume bool) store.HunterV7SignalRecord {
		st := openTestStore(t, filepath.Join(t.TempDir(), "v.db"))
		defer st.Close()
		row := resumeRow("AUSDT", ts, "ACTIVE")
		recs := []store.HunterV7SignalRecord{row}
		if err := st.HunterV7Signal().CreateBatch(recs); err != nil {
			t.Fatal(err)
		}
		tr := newResumeTracker(st)
		if !resume {
			tr.SetCandleHistorySource(func(_ string, since time.Time) []aittrader.TrackedCandle {
				var out []aittrader.TrackedCandle
				for _, c := range bars {
					if c.T.After(since) {
						out = append(out, c)
					}
				}
				return out
			})
			tr.RegisterWithMeta(aittrader.TrackedSignalMeta{OutcomeSource: row.OutcomeSource, TriggerPrice: *row.TriggerPrice},
				recs[0].ID, row.Symbol, row.Direction, row.SetupType, row.ExecutionTier, resumeEntryPrice(row), row.InvalidationPrice, row.TP0Price, row.TP1Price, row.TP2Price, ts)
			tr.TickNow()
		} else {
			if _, err := restoreTrackedOutcomes(validationOptions{store: st, outcomeTracker: tr}, harnessState{TrackedActiveIDs: []int64{recs[0].ID}}, now,
				func(string, time.Time, time.Time) []aittrader.TrackedCandle { return bars }); err != nil {
				t.Fatal(err)
			}
		}
		return rowByID(t, st, recs[0].ID)
	}
	a, b := run(false), run(true)
	if a.TrackStatus == "ACTIVE" || a.TrackStatus != b.TrackStatus || a.TrackPnLPct != b.TrackPnLPct || a.TrackExitPrice != b.TrackExitPrice ||
		a.TrackMFE != b.TrackMFE || a.FeeNetOutcome != b.FeeNetOutcome || !reflect.DeepEqual(a.FeeNetR, b.FeeNetR) {
		t.Fatalf("uninterrupted %s pnl %v exit %v fee-net %s\nresumed       %s pnl %v exit %v fee-net %s",
			a.TrackStatus, a.TrackPnLPct, a.TrackExitPrice, a.FeeNetOutcome, b.TrackStatus, b.TrackPnLPct, b.TrackExitPrice, b.FeeNetOutcome)
	}
}

// End to end through runHarness: process 1 tracks rows and stops with exit
// 2; process 2 (a fresh tracker, as after a VM restart) resumes, settles
// them and reports it in the run summary.
func TestRunHarnessResumeSettlesTrackedRows(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "v.db")
	signalAt := time.Now().UTC().Add(-20 * time.Minute)
	oldAt := time.Now().UTC().Add(-4 * time.Hour)
	var ids []int64
	outage := true
	withFakeHarness(t, func(opts validationOptions, round int) (validationReport, error) {
		if round == 2 && outage {
			return validationReport{}, transientError(errors.New("fetch snapshot failed: EOF"))
		}
		if round == 1 {
			recs := []store.HunterV7SignalRecord{resumeRow("AUSDT", signalAt, "ACTIVE"), resumeRow("BUSDT", oldAt, "ACTIVE")}
			if err := opts.store.HunterV7Signal().CreateBatch(recs); err != nil {
				return validationReport{}, fatalError(err)
			}
			for _, r := range recs {
				ids = append(ids, r.ID)
				opts.outcomeTracker.RegisterWithMeta(aittrader.TrackedSignalMeta{OutcomeSource: r.OutcomeSource}, r.ID, r.Symbol, r.Direction,
					r.SetupType, r.ExecutionTier, 100, r.InvalidationPrice, r.TP0Price, r.TP1Price, r.TP2Price, r.Timestamp)
			}
		}
		return validationReport{Round: round, Accepted: true}, nil
	})
	mk := func(resume bool) validationOptions {
		o := newStateOpts(dir, 3, resume)
		o.store = openTestStore(t, dbPath)
		o.outcomeTracker = newResumeTracker(o.store)
		o.backfillCandles = func(_ string, from, to time.Time) []aittrader.TrackedCandle { return flatBars(from, to, nil) }
		return o
	}
	if code := runHarness(mk(false), 3); code != exitTransient {
		t.Fatalf("first process exit %d, want 2", code)
	}
	st, err := loadHarnessState(defaultStateFile(dir, "tierA"))
	if err != nil || !reflect.DeepEqual(st.TrackedActiveIDs, ids) {
		t.Fatalf("saved ids %v, want %v (err %v)", st.TrackedActiveIDs, ids, err)
	}
	outage = false
	if code := runHarness(mk(true), 3); code != exitOK {
		t.Fatalf("resumed process exit %d, want 0", code)
	}
	check := openTestStore(t, dbPath)
	defer check.Close()
	if a := rowByID(t, check, ids[0]); a.TrackStatus != "ACTIVE" {
		t.Fatalf("fresh row: %q, want ACTIVE (re-registered, still open)", a.TrackStatus)
	}
	if b := rowByID(t, check, ids[1]); b.TrackStatus != trackRestartOrphaned {
		t.Fatalf("old row: %q, want %s", b.TrackStatus, trackRestartOrphaned)
	}
	st, _ = loadHarnessState(defaultStateFile(dir, "tierA"))
	if !reflect.DeepEqual(st.TrackedActiveIDs, ids[:1]) {
		t.Fatalf("after resume the re-registered row is saved again: %v", st.TrackedActiveIDs)
	}
	sums, _ := filepath.Glob(filepath.Join(dir, "hunter-v7-validation-run-summary-*.json"))
	found := false
	for _, p := range sums {
		b, _ := os.ReadFile(p)
		var s validationRunSummary
		if json.Unmarshal(b, &s) == nil && s.ResumeTracking != nil {
			found = len(s.ResumeTracking.Reregistered) == 1 && reflect.DeepEqual(s.ResumeTracking.Orphaned, ids[1:])
		}
	}
	if !found {
		t.Fatal("run summary lacks resume_tracking")
	}
}
