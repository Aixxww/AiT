package main

import (
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"
)

// TA-2 (docs/tier-a-improvement-plan-20261120.md): every round failure is
// classified, and the classification decides what happens next.
//
//   - transient: transport or I/O trouble (snapshot fetch, snapshot quality,
//     artifact writes, DB persistence). Retried after 10 s, 30 s and 60 s.
//     If all retries fail the run stops with exit code 2 (resumable; the
//     supervisor restarts it with --resume) instead of skipping the round,
//     so a network outage never silently removes rounds from the evidence.
//   - contract: the validation contract rejected the round's data
//     (reportAcceptanceError). A contract failure never counts as a pass.
//     The round is retried once; when the same contract signature repeats,
//     the round is rejected and skipped, and its artifacts are written to
//     <out-dir>/rejected/. Repeated deterministic rejections trip the run
//     breaker: exit code 3 (stop; restarting cannot help).
//   - fatal: configuration or programming errors (strategy load, report
//     marshal, golden dump). Exit code 1.

const (
	exitOK            = 0
	exitFatal         = 1
	exitTransient     = 2
	exitDeterministic = 3
)

type failureClass string

const (
	failTransient failureClass = "transient"
	failContract  failureClass = "contract"
	failFatal     failureClass = "fatal"
)

// roundError carries the class (and, for contract failures, the signature)
// of a round failure.
type roundError struct {
	class     failureClass
	signature string
	err       error
}

func (e *roundError) Error() string { return e.err.Error() }
func (e *roundError) Unwrap() error { return e.err }

func transientError(err error) error { return &roundError{class: failTransient, err: err} }
func fatalError(err error) error     { return &roundError{class: failFatal, err: err} }
func contractError(signature string, err error) error {
	return &roundError{class: failContract, signature: signature, err: err}
}

// classifyRoundError returns the class and signature of a round failure.
// Unclassified errors are treated as transient: they are retried with
// backoff and, if they persist, end the run with the resumable exit code.
func classifyRoundError(err error) (failureClass, string) {
	var re *roundError
	if errors.As(err, &re) {
		return re.class, re.signature
	}
	return failTransient, ""
}

// contractSignature identifies a contract failure without per-symbol
// detail, so the same defect on different symbols has the same signature:
// the sorted set of missing JSON fields, executable-gap codes and failed
// prompt checks.
func contractSignature(report validationReport) string {
	set := map[string]bool{}
	check := report.FormatCheck
	if !check.JSONMarshalOK {
		set["json_marshal_failed"] = true
	}
	if !check.JSONUnmarshalOK {
		set["json_unmarshal_failed"] = true
	}
	for _, is := range report.Issues {
		switch is.Code {
		case "missing_json_field":
			set["missing_field:"+is.Detail] = true
		case "empty_identity_field", "empty_reason_codes", "missing_required_confirmations", "missing_entry_mode",
			"missing_invalidation", "missing_targets", "missing_price_context", "missing_derivatives_context":
			set["gap:"+is.Code] = true
		}
	}
	if len(set) == 0 && reportOpenReviewCount(report) > 0 {
		set["prompt_missing_v7_fields"] = true
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return "contract:unspecified"
	}
	return strings.Join(keys, ",")
}

// roundPolicy holds the per-round retry rules.
type roundPolicy struct {
	transientBackoffs  []time.Duration // delay before each transient retry
	contractRetryDelay time.Duration   // delay before re-running a contract-rejected round
	contractRepeats    int             // same signature this many times -> round rejected
	maxContractTries   int             // cap when signatures keep changing
}

func defaultRoundPolicy() roundPolicy {
	return roundPolicy{
		transientBackoffs:  []time.Duration{10 * time.Second, 30 * time.Second, 60 * time.Second},
		contractRetryDelay: 10 * time.Second,
		contractRepeats:    2,
		maxContractTries:   3,
	}
}

type roundStatus string

const (
	roundAccepted         roundStatus = "accepted"
	roundRejectedContract roundStatus = "rejected_contract"
	roundTransientExhaust roundStatus = "transient_exhausted"
	roundFatal            roundStatus = "fatal"
)

type roundResult struct {
	status    roundStatus
	report    validationReport
	attempts  int
	signature string
	err       error
}

// attemptFunc runs one attempt of a round.
type attemptFunc func(round, attempt int) (validationReport, error)

// runRoundWithPolicy runs attempts of one round until it is accepted,
// rejected by the contract rule, exhausts its transient retries, or fails
// fatally.
func runRoundWithPolicy(round int, p roundPolicy, attempt attemptFunc, sleep func(time.Duration)) roundResult {
	transientRetries := 0
	contractTries := 0
	seen := map[string]int{}
	for n := 1; ; n++ {
		report, err := attempt(round, n)
		if err == nil {
			return roundResult{status: roundAccepted, report: report, attempts: n}
		}
		class, sig := classifyRoundError(err)
		switch class {
		case failFatal:
			return roundResult{status: roundFatal, attempts: n, err: err}
		case failContract:
			contractTries++
			seen[sig]++
			if seen[sig] >= p.contractRepeats || contractTries >= p.maxContractTries {
				return roundResult{status: roundRejectedContract, report: report, attempts: n, signature: sig, err: err}
			}
			log.Printf("validation round %d attempt %d failed (contract, signature %s): %v; retrying in %s", round, n, sig, err, p.contractRetryDelay)
			sleep(p.contractRetryDelay)
		default:
			if transientRetries >= len(p.transientBackoffs) {
				return roundResult{status: roundTransientExhaust, attempts: n, err: err}
			}
			d := p.transientBackoffs[transientRetries]
			transientRetries++
			log.Printf("validation round %d attempt %d failed (transient): %v; retrying in %s", round, n, err, d)
			sleep(d)
		}
	}
}

// runBreaker trips on deterministic contract rejections across rounds.
type runBreaker struct {
	consecutiveLimit int    // same signature on this many consecutive rounds
	windowSize       int    // ... or more than half of the last windowSize rounds rejected
	lastSig          string // signature of the previous round (accepted rounds reset it)
	consecutive      int
	window           []bool // true = contract-rejected
}

func newRunBreaker(consecutiveLimit int) *runBreaker {
	if consecutiveLimit <= 0 {
		consecutiveLimit = 3
	}
	return &runBreaker{consecutiveLimit: consecutiveLimit, windowSize: 10}
}

// observe records one round and reports whether the breaker trips.
func (b *runBreaker) observe(rejected bool, sig string) (bool, string) {
	b.window = append(b.window, rejected)
	if len(b.window) > b.windowSize {
		b.window = b.window[1:]
	}
	if !rejected {
		b.lastSig, b.consecutive = "", 0
		return false, ""
	}
	if sig == b.lastSig {
		b.consecutive++
	} else {
		b.lastSig, b.consecutive = sig, 1
	}
	if b.consecutive >= b.consecutiveLimit {
		return true, fmt.Sprintf("same contract signature on %d consecutive rounds: %s", b.consecutive, sig)
	}
	if len(b.window) == b.windowSize {
		n := 0
		for _, r := range b.window {
			if r {
				n++
			}
		}
		if n*2 > b.windowSize {
			return true, fmt.Sprintf("%d of the last %d rounds contract-rejected", n, b.windowSize)
		}
	}
	return false, ""
}

// rejectedRound records a skipped round in the run summary.
type rejectedRound struct {
	Round     int    `json:"round"`
	Class     string `json:"class"`
	Signature string `json:"signature"`
	Attempts  int    `json:"attempts"`
	Error     string `json:"error"`
}

// runOutcome is the result of the whole round loop.
type runOutcome struct {
	reports    []validationReport
	rejected   []rejectedRound
	lastRound  int // last round that completed (accepted or rejected)
	exitCode   int
	exitReason string
}

// runRoundsWithPolicy runs rounds first..last. onRoundDone is called after
// every completed round (accepted or rejected) and may stop the run with an
// error (treated as fatal).
func runRoundsWithPolicy(first, last int, interval time.Duration, p roundPolicy, breaker *runBreaker,
	attempt attemptFunc, sleep func(time.Duration), onRoundDone func(round int, res roundResult) error) runOutcome {
	out := runOutcome{lastRound: first - 1}
	for round := first; round <= last; round++ {
		if last > 1 {
			fmt.Printf("Hunter v7 validation round %d/%d\n", round, last)
		}
		res := runRoundWithPolicy(round, p, attempt, sleep)
		switch res.status {
		case roundFatal:
			out.exitCode, out.exitReason = exitFatal, fmt.Sprintf("round %d fatal: %v", round, res.err)
			return out
		case roundTransientExhaust:
			out.exitCode, out.exitReason = exitTransient, fmt.Sprintf("round %d transient failure after %d attempts: %v", round, res.attempts, res.err)
			return out
		case roundRejectedContract:
			log.Printf("validation round %d rejected by the contract after %d attempts (signature %s); round skipped", round, res.attempts, res.signature)
			out.rejected = append(out.rejected, rejectedRound{Round: round, Class: string(failContract), Signature: res.signature, Attempts: res.attempts, Error: res.err.Error()})
		case roundAccepted:
			out.reports = append(out.reports, res.report)
		}
		out.lastRound = round
		if onRoundDone != nil {
			if err := onRoundDone(round, res); err != nil {
				out.exitCode, out.exitReason = exitFatal, fmt.Sprintf("after round %d: %v", round, err)
				return out
			}
		}
		if tripped, why := breaker.observe(res.status == roundRejectedContract, res.signature); tripped {
			out.exitCode, out.exitReason = exitDeterministic, "deterministic contract failure: "+why
			return out
		}
		if round < last {
			fmt.Printf("Sleeping %s before next Binance REST validation round\n", interval)
			sleep(interval)
		}
	}
	return out
}
