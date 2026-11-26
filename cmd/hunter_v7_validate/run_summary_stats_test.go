package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// e4Report is a fixture round: regime, btc_24h, UTC time, signal count,
// prompt tier counts and the degraded flag.
func e4Report(round int, regime string, btc24h float64, at string, signals, e, r, w, rj int, degraded bool) validationReport {
	rep := validationReport{Round: round, GeneratedAt: at, GeneratedAtUTC: at}
	rep.Snapshot.Regime, rep.Snapshot.BTC24h, rep.Snapshot.Degraded = regime, btc24h, degraded
	rep.OpportunityCover.SignalCount = signals
	rep.AIRecognition.PromptTierCounts = map[string]int{"EXECUTABLE": e, "REVIEWABLE": r, "WATCH": w, "REJECTED": rj}
	return rep
}

func e4DownOnlyFixture() []validationReport {
	return []validationReport{
		e4Report(1, "trend_down", -3.1, "2026-11-20T01:00:00Z", 10, 0, 1, 8, 1, false),
		e4Report(2, "market_pullback", -5.6, "2026-11-20T02:00:00Z", 12, 1, 0, 11, 0, false),
		e4Report(3, "compression", -2.4, "2026-11-20T03:00:00Z", 8, 0, 0, 7, 0, false), // fails the -2% cross-check; 1 not in prompt
		e4Report(4, "range", 0.5, "2026-11-20T04:00:00Z", 6, 0, 2, 4, 0, true),         // degraded: excluded
	}
}

func e4MixedFixture() []validationReport {
	return []validationReport{
		e4Report(1, "trend_down", -3.0, "2026-11-20T01:00:00Z", 10, 0, 1, 9, 0, false),
		e4Report(2, "trend_up", 2.0, "2026-11-20T02:00:00Z", 10, 1, 1, 8, 0, false),
		e4Report(3, "trend_down", -4.0, "2026-11-20T03:00:00Z", 10, 0, 0, 10, 0, false),
		e4Report(4, "range", -1.0, "2026-11-21T01:00:00Z", 10, 0, 2, 8, 0, false),
		e4Report(5, "mixed", 0.0, "2026-11-21T02:00:00Z", 10, 0, 0, 10, 0, false),
	}
}

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// Per-round averages, overall and valid-only.
func TestE4_PerRoundAverages(t *testing.T) {
	s := buildValidationRunSummary(e4DownOnlyFixture(), time.Now())
	// all: signals 36 / 4 rounds; E+R = 1+1+0+2 = 4 / 4
	if !approx(s.MeanSignalsPerRound, 9) || !approx(s.MeanOpenReviewPerRound, 1) {
		t.Fatalf("all means: %v %v", s.MeanSignalsPerRound, s.MeanOpenReviewPerRound)
	}
	// valid: rounds 1-3, signals 30 / 3, E+R 2 / 3
	if s.ValidRounds != 3 || !approx(s.ValidMeanSignalsPerRound, 10) || !approx(s.ValidMeanOpenReviewPerRound, 2.0/3) {
		t.Fatalf("valid means: rounds %d %v %v", s.ValidRounds, s.ValidMeanSignalsPerRound, s.ValidMeanOpenReviewPerRound)
	}
	// Existing fields keep their values.
	if s.SignalCount != 36 || s.OpenReviewCount != 4 || s.ValidOpenReviewCount != 2 {
		t.Fatalf("existing totals changed: %+v", s)
	}
	if empty := buildValidationRunSummary(nil, time.Now()); empty.MeanSignalsPerRound != 0 || empty.ValidMeanOpenReviewPerRound != 0 {
		t.Fatalf("no rounds must give zero means: %+v", empty)
	}
}

// Tier counts sum to the signal counts, per round and in the totals; the
// shares follow the counts.
func TestE4_TierSharesSumToSignals(t *testing.T) {
	s := buildValidationRunSummary(e4DownOnlyFixture(), time.Now())
	sum := func(b validationTierBreakdown) int {
		return b.Executable + b.Reviewable + b.Watch + b.Rejected + b.NotInPrompt
	}
	shareSum := func(b validationTierBreakdown) float64 {
		return b.ExecutableSharePct + b.ReviewableSharePct + b.WatchSharePct + b.RejectedSharePct + b.NotInPromptSharePct
	}
	for _, r := range s.Round {
		if sum(r.Tiers) != r.SignalCount || r.Tiers.SignalCount != r.SignalCount || !approx(shareSum(r.Tiers), 100) {
			t.Fatalf("round %d tiers %+v do not sum to %d", r.Round, r.Tiers, r.SignalCount)
		}
		if r.Tiers.Executable+r.Tiers.Reviewable != r.OpenReview {
			t.Fatalf("round %d: E+R %d != open_review %d", r.Round, r.Tiers.Executable+r.Tiers.Reviewable, r.OpenReview)
		}
	}
	if s.Round[2].Tiers.NotInPrompt != 1 {
		t.Fatalf("round 3 has 8 signals and 7 tiered: not_in_prompt %+v", s.Round[2].Tiers)
	}
	if sum(s.Tiers) != s.SignalCount || sum(s.ValidTiers) != s.ValidSignalCount || !approx(shareSum(s.Tiers), 100) {
		t.Fatalf("totals: %+v / %+v", s.Tiers, s.ValidTiers)
	}
	// all WATCH 8+11+7+4 = 30 of 36; valid WATCH 26 of 30.
	if s.Tiers.Watch != 30 || !approx(s.Tiers.WatchSharePct, 30.0/36*100) || s.ValidTiers.Watch != 26 || !approx(s.ValidTiers.WatchSharePct, 26.0/30*100) {
		t.Fatalf("watch: %+v / %+v", s.Tiers, s.ValidTiers)
	}
	// Tier counts above the signal count are flagged, never hidden.
	over := newTierBreakdown(map[string]int{"WATCH": 5}, 3)
	if !over.TierCountExceedsSignals || over.NotInPrompt != 0 {
		t.Fatalf("over-count: %+v", over)
	}
}

// DOWN-MARKET-ONLY is set for a down-market-only run and absent for a
// mixed-regime run that passes the round-level coverage test.
func TestE4_DownMarketLabel(t *testing.T) {
	down := buildValidationRunSummary(e4DownOnlyFixture(), time.Now())
	c := down.MarketCoverage
	if down.SampleLabel != sampleLabelDownMarketOnly || !c.DownMarketOnly {
		t.Fatalf("down-only run must be labelled: %+v", c)
	}
	if c.CountedRounds != 3 || c.Down != 2 || c.NonDirectionalCrossCheckFailed != 1 || c.NonDownRounds != 0 {
		t.Fatalf("down-only buckets: %+v", c)
	}
	md := formatRunSummaryMarkdown(down, "x.json")
	lines := strings.Split(md, "\n")
	if !strings.HasPrefix(lines[0], "# DOWN-MARKET-ONLY ") || !strings.HasPrefix(lines[2], "> DOWN-MARKET-ONLY: counted rounds 3: down 2") {
		t.Fatalf("title and first line must carry the label and bucket counts:\n%s", md)
	}

	mixed := buildValidationRunSummary(e4MixedFixture(), time.Now())
	c = mixed.MarketCoverage
	// non-down: round 2 (up, day 20) and round 4 (range -1%, day 21): 2 of 5 = 40%, 2 episodes, 2 days.
	if mixed.SampleLabel != "" || c.DownMarketOnly || c.NonDownRounds != 2 || c.NonDownEpisodes != 2 || c.NonDownUTCDays != 2 || c.Unclassified != 1 || !approx(c.NonDownSharePct, 40) {
		t.Fatalf("mixed run must pass the round-level test: label %q %+v", mixed.SampleLabel, c)
	}
	md = formatRunSummaryMarkdown(mixed, "x.json")
	if strings.Contains(md, "DOWN-MARKET-ONLY") || !strings.Contains(strings.Split(md, "\n")[2], "cell floor not checked here") {
		t.Fatalf("mixed run markdown:\n%s", md)
	}
}

// Each coverage condition alone sets the label.
func TestE4_CoverageConditions(t *testing.T) {
	// One episode: up, up on two days, 2 of 3 rounds non-down.
	oneEpisode := []validationReport{
		e4Report(1, "trend_up", 1, "2026-11-20T23:00:00Z", 1, 0, 0, 1, 0, false),
		e4Report(2, "trend_up", 1, "2026-11-21T01:00:00Z", 1, 0, 0, 1, 0, false),
		e4Report(3, "trend_down", -3, "2026-11-21T02:00:00Z", 1, 0, 0, 1, 0, false),
	}
	if c := buildValidationRunSummary(oneEpisode, time.Now()).MarketCoverage; !c.DownMarketOnly || c.NonDownEpisodes != 1 {
		t.Fatalf("one episode: %+v", c)
	}
	// Two episodes on one UTC day.
	oneDay := []validationReport{
		e4Report(1, "trend_up", 1, "2026-11-20T01:00:00Z", 1, 0, 0, 1, 0, false),
		e4Report(2, "trend_down", -3, "2026-11-20T02:00:00Z", 1, 0, 0, 1, 0, false),
		e4Report(3, "rotation", 0, "2026-11-20T03:00:00Z", 1, 0, 0, 1, 0, false),
	}
	if c := buildValidationRunSummary(oneDay, time.Now()).MarketCoverage; !c.DownMarketOnly || c.NonDownEpisodes != 2 || c.NonDownUTCDays != 1 {
		t.Fatalf("one day: %+v", c)
	}
	// Share below 30%: 2 non-down of 7.
	low := []validationReport{
		e4Report(1, "trend_up", 1, "2026-11-20T01:00:00Z", 1, 0, 0, 1, 0, false),
		e4Report(2, "trend_down", -3, "2026-11-20T02:00:00Z", 1, 0, 0, 1, 0, false),
		e4Report(3, "trend_down", -3, "2026-11-20T03:00:00Z", 1, 0, 0, 1, 0, false),
		e4Report(4, "trend_down", -3, "2026-11-20T04:00:00Z", 1, 0, 0, 1, 0, false),
		e4Report(5, "trend_down", -3, "2026-11-20T05:00:00Z", 1, 0, 0, 1, 0, false),
		e4Report(6, "trend_down", -3, "2026-11-20T06:00:00Z", 1, 0, 0, 1, 0, false),
		e4Report(7, "range", 0, "2026-11-21T01:00:00Z", 1, 0, 0, 1, 0, false),
	}
	if c := buildValidationRunSummary(low, time.Now()).MarketCoverage; !c.DownMarketOnly || len(c.FailedChecks) != 1 || !strings.Contains(c.FailedChecks[0], "non-down share") {
		t.Fatalf("low share: %+v", c)
	}
	// Buckets: mixed, empty and trend_continuation.
	if marketBucketForRegime("mixed", 5) != marketBucketUnclassified || marketBucketForRegime("", 5) != marketBucketUnclassified ||
		marketBucketForRegime("trend_continuation", 0) != marketBucketNonDir || marketBucketForRegime("trend_continuation", -2) != marketBucketNonDirFailed {
		t.Fatal("bucket mapping")
	}
}

// §4 recomputability: the emitted summary JSON alone gives mean E+R per
// round and the WATCH share. The fixture summary is written through the
// real writer (same file names) and read back as plain JSON.
func TestE4_HeadlineFiguresRecomputableFromSummaryJSON(t *testing.T) {
	dir := t.TempDir()
	opts := validationOptions{strategyID: "fixture", rounds: 4, outDir: dir}
	if err := writeValidationRunSummary(opts, runOutcome{reports: e4DownOnlyFixture()}); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "hunter-v7-validation-run-summary-*.json"))
	mds, _ := filepath.Glob(filepath.Join(dir, "hunter-v7-validation-run-summary-*.md"))
	if len(files) != 1 || len(mds) != 1 {
		t.Fatalf("summary files: %v %v", files, mds)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		SampleLabel string `json:"sample_label"`
		Round       []struct {
			Degraded    bool `json:"degraded"`
			SignalCount int  `json:"signal_count"`
			Tiers       struct {
				Executable int `json:"executable"`
				Reviewable int `json:"reviewable"`
				Watch      int `json:"watch"`
			} `json:"tiers"`
		} `json:"round"`
		ValidMeanOpenReviewPerRound float64 `json:"valid_mean_open_review_per_round"`
		ValidTiers                  struct {
			WatchSharePct float64 `json:"watch_share_pct"`
		} `json:"valid_tiers"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	rounds, er, watch, signals := 0, 0, 0, 0
	for _, r := range doc.Round {
		if r.Degraded {
			continue
		}
		rounds++
		er += r.Tiers.Executable + r.Tiers.Reviewable
		watch += r.Tiers.Watch
		signals += r.SignalCount
	}
	if !approx(float64(er)/float64(rounds), doc.ValidMeanOpenReviewPerRound) || !approx(float64(watch)/float64(signals)*100, doc.ValidTiers.WatchSharePct) {
		t.Fatalf("recomputed E+R/round %v WATCH %v vs emitted %v %v", float64(er)/float64(rounds), float64(watch)/float64(signals)*100,
			doc.ValidMeanOpenReviewPerRound, doc.ValidTiers.WatchSharePct)
	}
	if doc.SampleLabel != sampleLabelDownMarketOnly {
		t.Fatalf("label %q", doc.SampleLabel)
	}
	if os.Getenv("E4_PRINT_FIXTURE") != "" {
		md, _ := os.ReadFile(mds[0])
		t.Logf("JSON:\n%s\nMD:\n%s", raw, md)
	}
}
