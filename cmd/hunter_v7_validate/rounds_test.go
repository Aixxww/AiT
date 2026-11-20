package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/provider/local"
)

// TA-2: failure classification, retry/backoff, round skip, exit codes and
// rejected/ artifacts.

func TestClassifyRoundError(t *testing.T) {
	cases := []struct {
		err   error
		class failureClass
		sig   string
	}{
		{transientError(errors.New("fetch snapshot failed: EOF")), failTransient, ""},
		{fmt.Errorf("wrapped: %w", transientError(errors.New("x"))), failTransient, ""},
		{contractError("missing_field:price_context", errors.New("validation contract rejected")), failContract, "missing_field:price_context"},
		{fatalError(errors.New("load strategy config failed")), failFatal, ""},
		{errors.New("unclassified"), failTransient, ""}, // unknown errors are retried, never passed
	}
	for _, c := range cases {
		class, sig := classifyRoundError(c.err)
		if class != c.class || sig != c.sig {
			t.Errorf("%v: got %s/%q, want %s/%q", c.err, class, sig, c.class, c.sig)
		}
	}
}

// The Tier A failure (2 missing fields per T2 signal) has one signature
// whatever the symbols or the number of signals.
func TestContractSignatureIgnoresSymbols(t *testing.T) {
	mk := func(symbols ...string) validationReport {
		r := validationReport{FormatCheck: formatCheck{JSONMarshalOK: true, JSONUnmarshalOK: true}}
		for _, s := range symbols {
			r.Issues = append(r.Issues,
				issue{Symbol: s, Code: "missing_json_field", Detail: "price_context"},
				issue{Symbol: s, Code: "missing_json_field", Detail: "derivatives_context"},
				issue{Symbol: s, Code: "missing_required_confirmations"},
				issue{Symbol: s, Code: "universe_coverage_low"}) // not a contract issue
		}
		r.FormatCheck.MissingFieldCount = 2 * len(symbols)
		return r
	}
	a, b := contractSignature(mk("CRVUSDT")), contractSignature(mk("FETUSDT", "ZILUSDT", "SKYUSDT"))
	want := "gap:missing_required_confirmations,missing_field:derivatives_context,missing_field:price_context"
	if a != want || b != want {
		t.Fatalf("signatures %q / %q, want %q", a, b, want)
	}
	other := validationReport{FormatCheck: formatCheck{JSONMarshalOK: true, JSONUnmarshalOK: true},
		Issues: []issue{{Symbol: "X", Code: "missing_entry_mode"}}}
	if contractSignature(other) == want {
		t.Fatal("a different defect must have a different signature")
	}
}

type scriptedAttempts struct {
	errs  []error // per attempt; nil = accepted; last repeats
	calls int
}

func (s *scriptedAttempts) fn(round, attempt int) (validationReport, error) {
	i := s.calls
	if i >= len(s.errs) {
		i = len(s.errs) - 1
	}
	s.calls++
	return validationReport{Round: round}, s.errs[i]
}

type sleepRecorder struct{ got []time.Duration }

func (r *sleepRecorder) sleep(d time.Duration) { r.got = append(r.got, d) }

func TestTransientRetryBackoffSequence(t *testing.T) {
	att := &scriptedAttempts{errs: []error{transientError(errors.New("EOF"))}}
	sl := &sleepRecorder{}
	res := runRoundWithPolicy(1, defaultRoundPolicy(), att.fn, sl.sleep)
	if res.status != roundTransientExhaust || res.attempts != 4 {
		t.Fatalf("status %s attempts %d, want transient_exhausted after 4", res.status, res.attempts)
	}
	if want := []time.Duration{10 * time.Second, 30 * time.Second, 60 * time.Second}; !reflect.DeepEqual(sl.got, want) {
		t.Fatalf("backoffs %v, want %v", sl.got, want)
	}

	att = &scriptedAttempts{errs: []error{transientError(errors.New("EOF")), nil}}
	sl = &sleepRecorder{}
	if res := runRoundWithPolicy(1, defaultRoundPolicy(), att.fn, sl.sleep); res.status != roundAccepted || res.attempts != 2 || !reflect.DeepEqual(sl.got, []time.Duration{10 * time.Second}) {
		t.Fatalf("transient then success: %s attempts %d sleeps %v", res.status, res.attempts, sl.got)
	}
}

func TestContractSameSignatureTwiceRejectsRound(t *testing.T) {
	sigErr := contractError("missing_field:price_context", errors.New("validation contract rejected: missing_fields=2"))
	att := &scriptedAttempts{errs: []error{sigErr}}
	sl := &sleepRecorder{}
	res := runRoundWithPolicy(1, defaultRoundPolicy(), att.fn, sl.sleep)
	if res.status != roundRejectedContract || res.attempts != 2 || res.signature != "missing_field:price_context" {
		t.Fatalf("got %s attempts %d sig %q, want rejected_contract after 2", res.status, res.attempts, res.signature)
	}
	if len(sl.got) != 1 {
		t.Fatalf("one retry delay expected, got %v", sl.got)
	}
	// Different signature then accepted: a changed market is allowed one more try.
	att = &scriptedAttempts{errs: []error{contractError("a", errors.New("x")), nil}}
	if res := runRoundWithPolicy(1, defaultRoundPolicy(), att.fn, (&sleepRecorder{}).sleep); res.status != roundAccepted {
		t.Fatalf("contract then accepted: %s", res.status)
	}
	// Signatures that keep changing are capped.
	att = &scriptedAttempts{errs: []error{contractError("a", errors.New("x")), contractError("b", errors.New("x")), contractError("c", errors.New("x"))}}
	if res := runRoundWithPolicy(1, defaultRoundPolicy(), att.fn, (&sleepRecorder{}).sleep); res.status != roundRejectedContract || res.attempts != 3 {
		t.Fatalf("changing signatures: %s after %d", res.status, res.attempts)
	}
}

// Exit code 3: the same contract signature on 3 consecutive rounds stops
// the run. Contract-rejected rounds are never reported as accepted.
func TestDeterministicFailureExitCode3(t *testing.T) {
	att := &scriptedAttempts{errs: []error{contractError("missing_field:price_context", errors.New("rejected"))}}
	out := runRoundsWithPolicy(1, 10, time.Minute, defaultRoundPolicy(), newRunBreaker(3), att.fn, (&sleepRecorder{}).sleep, nil)
	if out.exitCode != exitDeterministic {
		t.Fatalf("exit %d (%s), want 3", out.exitCode, out.exitReason)
	}
	if len(out.reports) != 0 || len(out.rejected) != 3 || out.lastRound != 3 {
		t.Fatalf("reports %d rejected %d lastRound %d, want 0/3/3", len(out.reports), len(out.rejected), out.lastRound)
	}
	if att.calls != 6 {
		t.Fatalf("attempts %d, want 6 (2 per round); no round after the breaker", att.calls)
	}
}

// A single rejected round is skipped and the run continues.
func TestContractRejectedRoundIsSkippedAndRunContinues(t *testing.T) {
	calls := 0
	fn := func(round, attempt int) (validationReport, error) {
		calls++
		if round == 2 {
			return validationReport{Round: round}, contractError("gap:missing_entry_mode", errors.New("rejected"))
		}
		return validationReport{Round: round, Accepted: true}, nil
	}
	var done []int
	out := runRoundsWithPolicy(1, 3, time.Second, defaultRoundPolicy(), newRunBreaker(3), fn, (&sleepRecorder{}).sleep,
		func(round int, res roundResult) error { done = append(done, round); return nil })
	if out.exitCode != exitOK || len(out.reports) != 2 || len(out.rejected) != 1 || out.rejected[0].Round != 2 {
		t.Fatalf("exit %d reports %d rejected %+v", out.exitCode, len(out.reports), out.rejected)
	}
	if !reflect.DeepEqual(done, []int{1, 2, 3}) {
		t.Fatalf("onRoundDone rounds %v", done)
	}
	for _, r := range out.reports {
		if r.Round == 2 {
			t.Fatal("a contract-rejected round must never be reported as accepted")
		}
	}
}

// Exit code 2: a round whose transient retries all fail stops the run
// (resumable) instead of skipping the round.
func TestRepeatedTransientFailureExitCode2(t *testing.T) {
	fn := func(round, attempt int) (validationReport, error) {
		if round == 2 {
			return validationReport{}, transientError(errors.New("dial tcp: connection refused"))
		}
		return validationReport{Round: round, Accepted: true}, nil
	}
	sl := &sleepRecorder{}
	out := runRoundsWithPolicy(1, 5, 3*time.Minute, defaultRoundPolicy(), newRunBreaker(3), fn, sl.sleep, nil)
	if out.exitCode != exitTransient || out.lastRound != 1 || len(out.reports) != 1 || len(out.rejected) != 0 {
		t.Fatalf("exit %d lastRound %d reports %d rejected %d, want 2/1/1/0", out.exitCode, out.lastRound, len(out.reports), len(out.rejected))
	}
	want := []time.Duration{3 * time.Minute, 10 * time.Second, 30 * time.Second, 60 * time.Second}
	if !reflect.DeepEqual(sl.got, want) {
		t.Fatalf("sleeps %v, want %v", sl.got, want)
	}
}

func TestFatalFailureExitCode1(t *testing.T) {
	att := &scriptedAttempts{errs: []error{fatalError(errors.New("load strategy config failed"))}}
	out := runRoundsWithPolicy(1, 3, time.Second, defaultRoundPolicy(), newRunBreaker(3), att.fn, (&sleepRecorder{}).sleep, nil)
	if out.exitCode != exitFatal || att.calls != 1 {
		t.Fatalf("exit %d calls %d, want 1/1", out.exitCode, att.calls)
	}
}

func TestBreakerWindowRule(t *testing.T) {
	b := newRunBreaker(3)
	// Alternating signatures never reach 3 consecutive; 6 of 10 rejected trips.
	seq := []struct {
		rej bool
		sig string
	}{{true, "a"}, {true, "b"}, {false, ""}, {true, "a"}, {true, "b"}, {false, ""}, {true, "a"}, {true, "b"}, {false, ""}, {false, ""}}
	for i, s := range seq {
		if tripped, why := b.observe(s.rej, s.sig); tripped {
			t.Fatalf("tripped early at %d: %s", i, why)
		}
	}
	if tripped, _ := b.observe(true, "c"); !tripped { // window now holds 6 of 10 rejected
		t.Fatal("breaker must trip when more than half of the last 10 rounds were rejected")
	}
}

// Rejected attempts' artifacts land in <out-dir>/rejected/ with the attempt
// number and accepted=false; accepted artifacts stay in <out-dir>.
func TestRejectedArtifactsLandInRejectedDir(t *testing.T) {
	dir := t.TempDir()
	opts := validationOptions{outDir: dir, attempt: 2}
	rep := validationReport{Round: 7, Accepted: false, RejectedReason: "validation contract rejected: executable_field_gaps=1"}
	pp, raw, md, err := writeRoundArtifacts(opts, rep, "prompt", "20261120-120000-r07", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{pp, raw, md} {
		if filepath.Dir(p) != filepath.Join(dir, "rejected") || !strings.Contains(filepath.Base(p), "-r07-a2") {
			t.Fatalf("rejected artifact path %s", p)
		}
		if _, err := os.Stat(p); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := os.ReadFile(raw)
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["accepted"] != false || got["rejected_reason"] == nil || got["prompt_preview_path"] != pp {
		t.Fatalf("rejected raw JSON: accepted=%v reason=%v prompt=%v", got["accepted"], got["rejected_reason"], got["prompt_preview_path"])
	}
	rep.Accepted, rep.RejectedReason = true, ""
	_, raw, _, err = writeRoundArtifacts(opts, rep, "prompt", "20261120-120000-r08", false)
	if err != nil || filepath.Dir(raw) != dir {
		t.Fatalf("accepted artifact %s err %v", raw, err)
	}
}

// End to end through runHarness: deterministic failure -> exit 3 and a run
// summary recording the rejected rounds and the exit reason; the fake
// attempt writes its artifacts like runValidation's contract branch.
func TestRunHarnessExitCodesAndSummary(t *testing.T) {
	origRun, origSleep := runValidationFn, sleepFn
	defer func() { runValidationFn, sleepFn = origRun, origSleep }()
	sleepFn = func(time.Duration) {}

	dir := t.TempDir()
	runValidationFn = func(opts validationOptions, round int) (validationReport, error) {
		rep := validationReport{Round: round, RejectedReason: "validation contract rejected: missing_fields=2"}
		if _, _, _, err := writeRoundArtifacts(opts, rep, "p", fmt.Sprintf("x-r%02d", round), true); err != nil {
			t.Fatal(err)
		}
		return rep, contractError("missing_field:price_context", errors.New(rep.RejectedReason))
	}
	opts := validationOptions{outDir: dir, rounds: 20, watchState: newTestWatchState()}
	if code := runHarness(opts, 3); code != exitDeterministic {
		t.Fatalf("exit %d, want 3", code)
	}
	rejected, _ := filepath.Glob(filepath.Join(dir, "rejected", "*-raw-*.json"))
	if len(rejected) != 6 { // 3 rounds x 2 attempts
		t.Fatalf("rejected raw artifacts %d, want 6", len(rejected))
	}
	accepted, _ := filepath.Glob(filepath.Join(dir, "hunter-v7-live-validation-raw-*.json"))
	if len(accepted) != 0 {
		t.Fatalf("no accepted artifacts expected, got %v", accepted)
	}
	sums, _ := filepath.Glob(filepath.Join(dir, "hunter-v7-validation-run-summary-*.json"))
	if len(sums) != 1 {
		t.Fatalf("run summaries %v", sums)
	}
	b, _ := os.ReadFile(sums[0])
	var sum validationRunSummary
	if err := json.Unmarshal(b, &sum); err != nil {
		t.Fatal(err)
	}
	if sum.ExitCode != exitDeterministic || len(sum.RejectedRounds) != 3 || sum.Rounds != 0 {
		t.Fatalf("summary exit %d rejected %d rounds %d", sum.ExitCode, len(sum.RejectedRounds), sum.Rounds)
	}

	// Transient outage -> exit 2.
	dir2 := t.TempDir()
	runValidationFn = func(opts validationOptions, round int) (validationReport, error) {
		return validationReport{}, transientError(errors.New("fetch snapshot failed: EOF"))
	}
	if code := runHarness(validationOptions{outDir: dir2, rounds: 3, watchState: newTestWatchState()}, 3); code != exitTransient {
		t.Fatalf("exit %d, want 2", code)
	}
}

func newTestWatchState() *local.V7SignalStateManager { return local.NewV7SignalStateManager() }
