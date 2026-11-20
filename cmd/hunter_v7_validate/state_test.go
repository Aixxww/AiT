package main

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/datafetch"
	"github.com/Aixxww/AiT/provider/local"
)

// TA-3: run state across harness restarts.

func newStateOpts(dir string, rounds int, resume bool) validationOptions {
	return validationOptions{
		outDir:     dir,
		rounds:     rounds,
		runID:      "tierA",
		resume:     resume,
		watchState: local.NewV7SignalStateManager(),
		t2Store:    local.NewV7T2EventStore(),
	}
}

func withFakeHarness(t *testing.T, fn func(opts validationOptions, round int) (validationReport, error)) {
	t.Helper()
	origRun, origSleep := runValidationFn, sleepFn
	t.Cleanup(func() { runValidationFn, sleepFn = origRun, origSleep })
	sleepFn = func(time.Duration) {}
	runValidationFn = fn
}

func TestHarnessStateSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state-x.json")
	opts := newStateOpts(dir, 10, false)
	opts.t2Store.Upsert(&local.V7T2Signal{Symbol: "AUSDT", Direction: "LONG", SetupType: "ignition_early_long", Status: "WATCH",
		Score: 70, Trigger: 100, Entry: 100, Stop: 99, Target1: 101.8, Target2: 103}, 100, time.Now())
	st, err := snapshotHarnessState(opts, 4, 3, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := saveHarnessState(path, st); err != nil {
		t.Fatal(err)
	}
	if tmps, _ := filepath.Glob(filepath.Join(dir, "*.tmp-*")); len(tmps) != 0 {
		t.Fatalf("temp files left behind: %v", tmps)
	}
	got, err := loadHarnessState(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.RunID != "tierA" || got.LastRound != 4 || got.NextCycleNumber != 5 || got.AcceptedRounds != 3 || got.RejectedRounds != 1 || got.PlannedRounds != 10 {
		t.Fatalf("state %+v", got)
	}
	st2 := local.NewV7T2EventStore()
	if err := st2.LoadState(got.T2State); err != nil || st2.Len() != 1 {
		t.Fatalf("t2 state not restored: len=%d err=%v", st2.Len(), err)
	}
}

func TestPrepareRunStateRules(t *testing.T) {
	dir := t.TempDir()
	// No run id: stateless, round 1; --resume needs a run id.
	o := newStateOpts(dir, 3, false)
	o.runID = ""
	if first, _, err := prepareRunState(&o); err != nil || first != 1 {
		t.Fatalf("stateless: first=%d err=%v", first, err)
	}
	o.resume = true
	if _, _, err := prepareRunState(&o); err == nil {
		t.Fatal("--resume without --run-id accepted")
	}
	// Resume without a state file.
	o = newStateOpts(dir, 3, true)
	if _, _, err := prepareRunState(&o); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing state file: %v", err)
	}
	// Existing state without --resume is refused (would reuse cycle numbers).
	o = newStateOpts(dir, 3, false)
	st, _ := snapshotHarnessState(o, 2, 2, 0)
	if err := saveHarnessState(defaultStateFile(dir, "tierA"), st); err != nil {
		t.Fatal(err)
	}
	o = newStateOpts(dir, 3, false)
	if _, _, err := prepareRunState(&o); err == nil || !strings.Contains(err.Error(), "reuse cycle numbers") {
		t.Fatalf("existing state without --resume: %v", err)
	}
	// Wrong run id in the file.
	o = newStateOpts(dir, 3, true)
	o.stateFile = defaultStateFile(dir, "tierA")
	o.runID = "other"
	if _, _, err := prepareRunState(&o); err == nil || !strings.Contains(err.Error(), "belongs to run") {
		t.Fatalf("run id mismatch: %v", err)
	}
	// Different harness sha is refused unless allowed.
	st.HarnessGitSHA = "0000000"
	if err := saveHarnessState(defaultStateFile(dir, "tierA"), st); err != nil {
		t.Fatal(err)
	}
	o = newStateOpts(dir, 3, true)
	if _, _, err := prepareRunState(&o); err == nil || !strings.Contains(err.Error(), "sha-change") {
		t.Fatalf("sha mismatch: %v", err)
	}
	o = newStateOpts(dir, 3, true)
	o.resumeAllowSHAChange = true
	if first, _, err := prepareRunState(&o); err != nil || first != 3 {
		t.Fatalf("allowed sha change: first=%d err=%v", first, err)
	}
}

// Resume continues cycle numbering: a run interrupted by a transient outage
// (exit 2) and resumed covers every cycle exactly once. A contract-rejected
// round is not re-run on resume.
func TestResumeContinuesCycleNumbersWithoutDuplicates(t *testing.T) {
	dir := t.TempDir()
	var persisted []int // cycle numbers of rows written to the "DB"
	outage := true
	withFakeHarness(t, func(opts validationOptions, round int) (validationReport, error) {
		if round == 3 {
			return validationReport{Round: round}, contractError("gap:missing_entry_mode", errors.New("rejected"))
		}
		if round == 5 && outage {
			return validationReport{}, transientError(errors.New("fetch snapshot failed: EOF"))
		}
		persisted = append(persisted, round) // cfg.CycleNumber = round
		return validationReport{Round: round, Accepted: true}, nil
	})
	if code := runHarness(newStateOpts(dir, 8, false), 3); code != exitTransient {
		t.Fatalf("first process exit %d, want 2", code)
	}
	st, err := loadHarnessState(defaultStateFile(dir, "tierA"))
	if err != nil || st.LastRound != 4 || st.NextCycleNumber != 5 || st.AcceptedRounds != 3 || st.RejectedRounds != 1 {
		t.Fatalf("state after outage: %+v err=%v", st, err)
	}
	// A restart without --resume must not start again from cycle 1.
	if code := runHarness(newStateOpts(dir, 8, false), 3); code != exitFatal {
		t.Fatalf("restart without --resume: exit %d, want 1 (refused)", code)
	}
	outage = false
	if code := runHarness(newStateOpts(dir, 8, true), 3); code != exitOK {
		t.Fatalf("resumed process exit %d, want 0", code)
	}
	if want := []int{1, 2, 4, 5, 6, 7, 8}; !reflect.DeepEqual(persisted, want) {
		t.Fatalf("persisted cycles %v, want %v (no cycle reused, rejected round 3 not re-run)", persisted, want)
	}
	st, _ = loadHarnessState(defaultStateFile(dir, "tierA"))
	if st.LastRound != 8 || st.AcceptedRounds != 7 || st.RejectedRounds != 1 {
		t.Fatalf("final state %+v", st)
	}
	// Resuming a finished run does nothing.
	if code := runHarness(newStateOpts(dir, 8, true), 3); code != exitOK || len(persisted) != 7 {
		t.Fatalf("resume of a finished run: exit %d persisted %v", code, persisted)
	}
}

// The design-standard gap 7 fix: a WATCH event created before a restart is
// still known after it, so the next round's price reaching the entry makes
// it TRIGGERED. Failed attempts never leak events into the saved store.
func TestWatchProgressSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	t0 := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	watch := func() *local.V7T2Signal {
		return &local.V7T2Signal{Symbol: "AUSDT", Direction: "LONG", SetupType: "ignition_early_long", Status: "WATCH",
			Score: 70, Trigger: 100, Entry: 100, Stop: 99, Target1: 101.8, Target2: 103}
	}
	cfg := &local.V7T2Config{Enabled: true}
	atPrice := func(p float64) *datafetch.Snapshot {
		return &datafetch.Snapshot{Symbols: map[string]*datafetch.SymbolSnapshot{"AUSDT": {Symbol: "AUSDT", Price: p}}}
	}

	// Process 1: round 1 creates the WATCH event (price below entry).
	// Round 2: attempt 1 creates a stray event and fails, attempt 2 is
	// accepted, so the state saved after round 2 must not hold the stray.
	// Round 3 fails transiently on every attempt: exit 2.
	withFakeHarness(t, func(opts validationOptions, round int) (validationReport, error) {
		switch {
		case round == 1:
			if r := opts.t2Store.Upsert(watch(), 99.5, t0); !r.Created || r.Event.Status != "WATCH" {
				t.Fatalf("watch event not created: %+v", r)
			}
			local.RunV7T2Module(atPrice(99.5), cfg, opts.t2Store, nil, "", t0.Add(time.Minute))
			return validationReport{Round: round, Accepted: true}, nil
		case round == 2 && opts.attempt == 1:
			stray := watch()
			stray.Symbol = "STRAYUSDT"
			opts.t2Store.Upsert(stray, 100, t0.Add(3*time.Minute))
			return validationReport{}, transientError(errors.New("read: connection reset by peer"))
		case round == 2:
			return validationReport{Round: round, Accepted: true}, nil
		default:
			return validationReport{}, transientError(errors.New("dial tcp: connection refused"))
		}
	})
	if code := runHarness(newStateOpts(dir, 4, false), 3); code != exitTransient {
		t.Fatalf("process 1 exit %d, want 2", code)
	}

	// Process 2: fresh stores, --resume. Round 3 sees the prior WATCH event
	// and the price reaching the entry triggers it.
	var out []local.V7SignalOutput
	strays := -1
	withFakeHarness(t, func(opts validationOptions, round int) (validationReport, error) {
		if round == 3 {
			b, _ := opts.t2Store.MarshalState()
			strays = strings.Count(string(b), "STRAYUSDT")
			out = local.RunV7T2Module(atPrice(100), cfg, opts.t2Store, nil, "", t0.Add(6*time.Minute))
		}
		return validationReport{Round: round, Accepted: true}, nil
	})
	if code := runHarness(newStateOpts(dir, 4, true), 3); code != exitOK {
		t.Fatalf("process 2 exit %d, want 0", code)
	}
	if strays != 0 {
		t.Fatalf("failed attempts leaked %d stray events into the saved store", strays)
	}
	if len(out) != 1 || out[0].Symbol != "AUSDT" || out[0].LifecycleState != "TRIGGERED" {
		t.Fatalf("after restart: %+v, want AUSDT TRIGGERED", out)
	}
	// Control: without the saved state, the same scan knows no event.
	if ctl := local.RunV7T2Module(atPrice(100), cfg, local.NewV7T2EventStore(), nil, "", t0.Add(6*time.Minute)); len(ctl) != 0 {
		t.Fatalf("control (fresh store) emitted %+v", ctl)
	}
	st, _ := loadHarnessState(defaultStateFile(dir, "tierA"))
	var ts struct {
		Events []struct {
			Symbol, Status string
			TriggeredAt    *time.Time
		} `json:"events"`
	}
	if err := json.Unmarshal(st.T2State, &ts); err != nil || len(ts.Events) != 1 || ts.Events[0].Status != "TRIGGERED" || ts.Events[0].TriggeredAt == nil {
		t.Fatalf("saved T2 state after the restart: %+v err=%v", ts, err)
	}
}
