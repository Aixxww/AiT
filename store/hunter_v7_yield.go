package store

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// U13: regime-conditional yield target (redesign plan §6.2 "Yield target",
// OD-20). Owner decisions (U13):
//   - regime = the scanner's BTC RV24 tier (spec §6.1): LOW < 0.25,
//     MID < 0.35, HIGH >= 0.35;
//   - RV24 is recomputed from public BTCUSDT futures klines;
//   - the target replaces the fixed G3 (admitted >= 170) of t2_m2_eval and
//     is shown read-only, per regime, on the U12 cell dashboard;
//   - target rate = the 90% Poisson lower bound of the measured rate.
// Nothing here gates live admission.

// RV24 tiers.
const (
	V7RV24TierLow  = "LOW"
	V7RV24TierMid  = "MID"
	V7RV24TierHigh = "HIGH"
	// V7RV24TierNone marks a time with too few bars (the scanner fails
	// open there and writes no tier).
	V7RV24TierNone = "none"
)

// V7RV24Tiers lists the tiers in display order.
var V7RV24Tiers = []string{V7RV24TierLow, V7RV24TierMid, V7RV24TierHigh}

// Yield stages. The t2_m2_eval stage name carries its RR floor.
const (
	V7YieldStageT2Triggered = "t2_triggered"
)

// V7YieldStageM2Admitted is the stage name of the structure-admitted
// events of t2_m2_eval at an RR floor.
func V7YieldStageM2Admitted(rrMin float64) string {
	return fmt.Sprintf("m2v2_admitted_rr%.2f", rrMin)
}

// V7YieldTargetConfig is HunterConfig.V7YieldTarget (json v7_yield_target).
// Zero values mean the defaults below.
type V7YieldTargetConfig struct {
	// RV24 rule (scanner spec §6.1): Bars 1h klines ending with the
	// forming bar, the population std of the last Returns log returns
	// x100; fewer than MinBars bars = no tier.
	RV24Bars    int     `json:"rv24_bars,omitempty"`
	RV24Returns int     `json:"rv24_returns,omitempty"`
	RV24MinBars int     `json:"rv24_min_bars,omitempty"`
	RV24LowMax  float64 `json:"rv24_low_max,omitempty"`
	RV24MidMax  float64 `json:"rv24_mid_max,omitempty"`
	// RoundMinutes is the length of one round of wall time; rates are
	// events per round in a tier.
	RoundMinutes int `json:"round_minutes,omitempty"`
	// CILevel is the two-sided exact Poisson interval level; the target
	// is its lower bound.
	CILevel float64 `json:"ci_level,omitempty"`
	// Targets is stage -> tier -> target rate (events per round). An
	// entry replaces the default for that stage and tier.
	Targets map[string]map[string]float64 `json:"targets,omitempty"`
}

// Defaults. The RV24 constants are the scanner's (spec §6.1). The rounds
// are the plan's 5-minute rounds (§6.2: 48 days x 288).
const (
	DefaultV7RV24Bars         = 30
	DefaultV7RV24Returns      = 24
	DefaultV7RV24MinBars      = 25
	DefaultV7RV24LowMax       = 0.25
	DefaultV7RV24MidMax       = 0.35
	DefaultV7YieldRoundMinute = 5
	DefaultV7YieldCILevel     = 0.90
)

// DefaultV7YieldTargets are the U13 measured targets: the 90% Poisson lower
// bound of each stage's rate per tier over the 48-day study window
// (2026-09-28 to 2026-11-15, 13,824 rounds, 694 T2 events), with RV24
// recomputed from BTCUSDT futures klines (cmd/t2_u13_yield; record in
// docs/t2-u13-yield-target.md). Events per 5-minute round.
func DefaultV7YieldTargets() map[string]map[string]float64 {
	return map[string]map[string]float64{
		V7YieldStageT2Triggered: {
			V7RV24TierLow:  defaultV7YieldTriggeredLow,
			V7RV24TierMid:  defaultV7YieldTriggeredMid,
			V7RV24TierHigh: defaultV7YieldTriggeredHigh,
		},
		V7YieldStageM2Admitted(1.2): {
			V7RV24TierLow:  defaultV7YieldRR12Low,
			V7RV24TierMid:  defaultV7YieldRR12Mid,
			V7RV24TierHigh: defaultV7YieldRR12High,
		},
		V7YieldStageM2Admitted(1.5): {
			V7RV24TierLow:  defaultV7YieldRR15Low,
			V7RV24TierMid:  defaultV7YieldRR15Mid,
			V7RV24TierHigh: defaultV7YieldRR15High,
		},
	}
}

// EffectiveV7YieldTarget returns the config with defaults filled in.
func EffectiveV7YieldTarget(c *V7YieldTargetConfig) V7YieldTargetConfig {
	out := V7YieldTargetConfig{}
	if c != nil {
		out = *c
	}
	if out.RV24Bars <= 0 {
		out.RV24Bars = DefaultV7RV24Bars
	}
	if out.RV24Returns <= 0 {
		out.RV24Returns = DefaultV7RV24Returns
	}
	if out.RV24MinBars <= 0 {
		out.RV24MinBars = DefaultV7RV24MinBars
	}
	if out.RV24LowMax <= 0 {
		out.RV24LowMax = DefaultV7RV24LowMax
	}
	if out.RV24MidMax <= 0 {
		out.RV24MidMax = DefaultV7RV24MidMax
	}
	if out.RoundMinutes <= 0 {
		out.RoundMinutes = DefaultV7YieldRoundMinute
	}
	if out.CILevel <= 0 || out.CILevel >= 1 {
		out.CILevel = DefaultV7YieldCILevel
	}
	targets := DefaultV7YieldTargets()
	for stage, tiers := range out.Targets {
		if targets[stage] == nil {
			targets[stage] = map[string]float64{}
		}
		for tier, v := range tiers {
			targets[stage][tier] = v
		}
	}
	out.Targets = targets
	return out
}

// Target returns a stage's target rate for a tier.
func (c V7YieldTargetConfig) Target(stage, tier string) (float64, bool) {
	v, ok := c.Targets[stage][tier]
	return v, ok
}

// RoundMs is the round length in milliseconds.
func (c V7YieldTargetConfig) RoundMs() int64 { return int64(c.RoundMinutes) * 60_000 }

// RV24Tier maps an RV24 value to its tier.
func (c V7YieldTargetConfig) RV24Tier(rv float64) string {
	switch {
	case rv < c.RV24LowMax:
		return V7RV24TierLow
	case rv < c.RV24MidMax:
		return V7RV24TierMid
	default:
		return V7RV24TierHigh
	}
}

// V7PriceBar is an OHLC-close bar by open time (ms).
type V7PriceBar struct {
	OpenMs int64
	Close  float64
}

// V7RV24Series computes RV24 at times from BTCUSDT bars. The scanner reads
// the latest RV24Bars 1h klines, whose last one is the bar still forming,
// so at time t the series is the closed 1h bars (close time <= t) plus the
// forming bar priced at the close of the latest closed bar of the finer
// series (fine, e.g. 5m) at t. Both series must be sorted by open time.
type V7RV24Series struct {
	cfg      V7YieldTargetConfig
	h1       []V7PriceBar
	fine     []V7PriceBar
	fineMs   int64
	hourMs   int64
	minStart int64
}

// NewV7RV24Series builds the calculator; fineMs is the finer bar length.
func NewV7RV24Series(cfg V7YieldTargetConfig, h1, fine []V7PriceBar, fineMs int64) *V7RV24Series {
	s := &V7RV24Series{cfg: cfg, h1: h1, fine: fine, fineMs: fineMs, hourMs: 3_600_000}
	sort.Slice(s.h1, func(i, j int) bool { return s.h1[i].OpenMs < s.h1[j].OpenMs })
	sort.Slice(s.fine, func(i, j int) bool { return s.fine[i].OpenMs < s.fine[j].OpenMs })
	return s
}

// At returns RV24 (percent) at t, and false when there are too few bars.
func (s *V7RV24Series) At(tMs int64) (float64, bool) {
	// Closed 1h bars: open + 1h <= t.
	nClosed := sort.Search(len(s.h1), func(i int) bool { return s.h1[i].OpenMs+s.hourMs > tMs })
	// Forming bar price: latest fine bar closed at t.
	nFine := sort.Search(len(s.fine), func(i int) bool { return s.fine[i].OpenMs+s.fineMs > tMs })
	if nFine == 0 {
		return 0, false
	}
	formingClose := s.fine[nFine-1].Close
	take := s.cfg.RV24Bars - 1
	start := nClosed - take
	if start < 0 {
		start = 0
	}
	closes := make([]float64, 0, s.cfg.RV24Bars)
	for _, b := range s.h1[start:nClosed] {
		closes = append(closes, b.Close)
	}
	closes = append(closes, formingClose)
	if len(closes) < s.cfg.RV24MinBars {
		return 0, false
	}
	return V7RV24FromCloses(closes, s.cfg.RV24Returns), true
}

// Tier returns the RV24 tier at t (V7RV24TierNone without enough bars).
func (s *V7RV24Series) Tier(tMs int64) string {
	rv, ok := s.At(tMs)
	if !ok {
		return V7RV24TierNone
	}
	return s.cfg.RV24Tier(rv)
}

// V7RV24FromCloses is the population std of the last n log returns of
// closes, x100 (scanner btc_trailing_rv24).
func V7RV24FromCloses(closes []float64, n int) float64 {
	var rets []float64
	for i := 1; i < len(closes); i++ {
		if closes[i-1] > 0 && closes[i] > 0 {
			rets = append(rets, math.Log(closes[i]/closes[i-1]))
		}
	}
	if len(rets) > n {
		rets = rets[len(rets)-n:]
	}
	if len(rets) == 0 {
		return 0
	}
	mean := 0.0
	for _, r := range rets {
		mean += r
	}
	mean /= float64(len(rets))
	v := 0.0
	for _, r := range rets {
		v += (r - mean) * (r - mean)
	}
	return math.Sqrt(v/float64(len(rets))) * 100
}

// V7RoundsPerTier counts the rounds of [from, to) by RV24 tier. A round at
// time t covers [t, t+round); its tier is the tier at t.
func (s *V7RV24Series) RoundsPerTier(fromMs, toMs int64) map[string]int {
	out := map[string]int{}
	step := s.cfg.RoundMs()
	start := (fromMs + step - 1) / step * step
	for t := start; t < toMs; t += step {
		out[s.Tier(t)]++
	}
	return out
}

// V7PoissonInterval is the exact (Garwood) two-sided Poisson interval of a
// count k at level ci: lower solves P(X >= k; lo) = (1-ci)/2 (0 for k = 0),
// upper solves P(X <= k; hi) = (1-ci)/2.
func V7PoissonInterval(k int, ci float64) (lo, hi float64) {
	a := (1 - ci) / 2
	if k > 0 {
		lo = v7Bisect(func(l float64) float64 { return 1 - v7PoissonCDF(k-1, l) - a }, 0, float64(k)+1)
	}
	hi = v7Bisect(func(l float64) float64 { return a - v7PoissonCDF(k, l) }, float64(k), 2*float64(k)+20)
	return lo, hi
}

// v7PoissonCDF is P(X <= k) for X ~ Poisson(lambda).
func v7PoissonCDF(k int, lambda float64) float64 {
	if lambda <= 0 {
		return 1
	}
	sum := 0.0
	for i := 0; i <= k; i++ {
		lg, _ := math.Lgamma(float64(i) + 1)
		sum += math.Exp(float64(i)*math.Log(lambda) - lambda - lg)
	}
	if sum > 1 {
		return 1
	}
	return sum
}

// v7Bisect finds the root of an increasing f on [lo, hi].
func v7Bisect(f func(float64) float64, lo, hi float64) float64 {
	for f(hi) < 0 {
		hi *= 2
	}
	for i := 0; i < 200 && hi-lo > 1e-12*math.Max(1, hi); i++ {
		mid := (lo + hi) / 2
		if f(mid) < 0 {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
}

// V7YieldRow is one stage x tier yield row.
type V7YieldRow struct {
	Stage  string  `json:"stage"`
	Tier   string  `json:"rv24_tier"`
	Rounds int     `json:"rounds"`
	Events int     `json:"events"`
	Rate   float64 `json:"rate_per_round"`
	CILo   float64 `json:"rate_ci_lo"`
	CIHi   float64 `json:"rate_ci_hi"`
	// Target is the configured target rate; Expected = Target x Rounds.
	Target   *float64 `json:"target_rate,omitempty"`
	Expected *float64 `json:"expected_events,omitempty"`
	// BelowTarget: the rate's upper bound is under the target.
	BelowTarget bool `json:"below_target"`
}

// BuildV7YieldRow fills one row from a count and its rounds.
func BuildV7YieldRow(stage, tier string, events, rounds int, cfg V7YieldTargetConfig) V7YieldRow {
	row := V7YieldRow{Stage: stage, Tier: tier, Rounds: rounds, Events: events}
	if rounds > 0 {
		lo, hi := V7PoissonInterval(events, cfg.CILevel)
		row.Rate = float64(events) / float64(rounds)
		row.CILo, row.CIHi = lo/float64(rounds), hi/float64(rounds)
	}
	if t, ok := cfg.Target(stage, tier); ok {
		exp := t * float64(rounds)
		row.Target, row.Expected = &t, &exp
		row.BelowTarget = rounds > 0 && row.CIHi < t
	}
	return row
}

// V7YieldThreshold is the regime-conditional G3 threshold of a stage: the
// sum over tiers of rounds x target. ok is false when a tier with rounds
// has no target.
func V7YieldThreshold(stage string, rounds map[string]int, cfg V7YieldTargetConfig) (float64, bool) {
	sum := 0.0
	for _, tier := range V7RV24Tiers {
		n := rounds[tier]
		if n == 0 {
			continue
		}
		t, ok := cfg.Target(stage, tier)
		if !ok {
			return 0, false
		}
		sum += t * float64(n)
	}
	return sum, true
}

// V7YieldWindowUTC is the study window of the defaults (48 days).
var V7YieldWindowUTC = [2]time.Time{
	time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC),
	time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC),
}

// V7YieldEvent is one deduplicated live T2 triggered event: the first
// record of its key (symbol, canonical setup, direction, trigger price) in
// the window.
type V7YieldEvent struct {
	Key  string    `json:"key"`
	Time time.Time `json:"time"`
}

// V7T2TriggeredEvents deduplicates T2 records at lifecycle TRIGGERED into
// events. A TRIGGERED event surfaces in several cycles; its first record
// time is the event time.
func V7T2TriggeredEvents(records []HunterV7SignalRecord) []V7YieldEvent {
	first := map[string]time.Time{}
	for _, r := range records {
		if r.Stack != V7StackT2 || r.LifecycleStateAtDecision != V7T2StateTriggered {
			continue
		}
		trig := 0.0
		if r.TriggerPrice != nil {
			trig = *r.TriggerPrice
		}
		key := fmt.Sprintf("%s|%s|%s|%g", r.Symbol, V7CanonicalSetupType(r.SetupType), r.Direction, trig)
		if t, ok := first[key]; !ok || r.Timestamp.Before(t) {
			first[key] = r.Timestamp
		}
	}
	out := make([]V7YieldEvent, 0, len(first))
	for k, t := range first {
		out = append(out, V7YieldEvent{Key: k, Time: t})
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Time.Equal(out[j].Time) {
			return out[i].Time.Before(out[j].Time)
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// T2YieldSource returns the T2 records of [from, to] with the columns the
// live yield table reads.
func (s *HunterV7SignalStore) T2YieldSource(from, to time.Time) ([]HunterV7SignalRecord, error) {
	var recs []HunterV7SignalRecord
	err := s.db.Select("id", "timestamp", "symbol", "setup_type", "direction", "stack", "lifecycle_state_at_decision", "trigger_price").
		Where("timestamp >= ? AND timestamp <= ? AND stack = ?", from, to, V7StackT2).
		Order("timestamp, id").Find(&recs).Error
	return recs, err
}
