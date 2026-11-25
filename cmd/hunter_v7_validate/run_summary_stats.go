package main

// #107 / AUD-E4: per-round averages, tier shares and the sampling-rule
// down-market label in the run summary. Reporting only: nothing here feeds
// validation, tiering or exit codes. Every figure is derivable from the
// round rows emitted in the same summary JSON.

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// sampleLabelDownMarketOnly is the sampling-rule §4 mark.
const sampleLabelDownMarketOnly = "DOWN-MARKET-ONLY"

// Sampling-rule §2 buckets (docs/hunter-v7-validation-sampling-rule.md).
const (
	marketBucketDown         = "down"
	marketBucketUp           = "up"
	marketBucketNonDir       = "non_directional"
	marketBucketNonDirFailed = "non_directional_cross_check_failed" // non-directional regime with btc_24h <= -2%
	marketBucketUnclassified = "unclassified"
)

// Sampling-rule §2 cross-check and §3 step 4 coverage thresholds.
const (
	coverageNonDirMinBTC24h    = -2.0 // a non-directional round counts as non-down only above this btc_24h (%)
	coverageMinNonDownSharePct = 30.0
	coverageMinNonDownEpisodes = 2
	coverageMinNonDownUTCDays  = 2
)

// validationTierBreakdown is the prompt-final tier split of one round (or
// the sum over rounds). Counts come from the prompt's Tier Summary line
// (AIRecognition.PromptTierCounts); NotInPrompt is the rest of the signals
// (signal_count - E - R - W - Rj, floored at 0), so the five counts sum to
// signal_count unless TierCountExceedsSignals. Shares are percent of
// signal_count.
type validationTierBreakdown struct {
	SignalCount             int     `json:"signal_count"`
	Executable              int     `json:"executable"`
	Reviewable              int     `json:"reviewable"`
	Watch                   int     `json:"watch"`
	Rejected                int     `json:"rejected"`
	NotInPrompt             int     `json:"not_in_prompt"`
	ExecutableSharePct      float64 `json:"executable_share_pct"`
	ReviewableSharePct      float64 `json:"reviewable_share_pct"`
	WatchSharePct           float64 `json:"watch_share_pct"`
	RejectedSharePct        float64 `json:"rejected_share_pct"`
	NotInPromptSharePct     float64 `json:"not_in_prompt_share_pct"`
	TierCountExceedsSignals bool    `json:"tier_count_exceeds_signals,omitempty"`
}

func newTierBreakdown(counts map[string]int, signalCount int) validationTierBreakdown {
	t := validationTierBreakdown{
		SignalCount: signalCount,
		Executable:  counts["EXECUTABLE"],
		Reviewable:  counts["REVIEWABLE"],
		Watch:       counts["WATCH"],
		Rejected:    counts["REJECTED"],
	}
	t.finish()
	return t
}

func (t *validationTierBreakdown) add(o validationTierBreakdown) {
	t.SignalCount += o.SignalCount
	t.Executable += o.Executable
	t.Reviewable += o.Reviewable
	t.Watch += o.Watch
	t.Rejected += o.Rejected
}

// finish derives NotInPrompt and the shares from the counts.
func (t *validationTierBreakdown) finish() {
	tiered := t.Executable + t.Reviewable + t.Watch + t.Rejected
	t.NotInPrompt, t.TierCountExceedsSignals = 0, false
	if rest := t.SignalCount - tiered; rest > 0 {
		t.NotInPrompt = rest
	} else if rest < 0 {
		t.TierCountExceedsSignals = true
	}
	t.ExecutableSharePct = pct(t.Executable, t.SignalCount)
	t.ReviewableSharePct = pct(t.Reviewable, t.SignalCount)
	t.WatchSharePct = pct(t.Watch, t.SignalCount)
	t.RejectedSharePct = pct(t.Rejected, t.SignalCount)
	t.NotInPromptSharePct = pct(t.NotInPrompt, t.SignalCount)
}

// marketBucketForRegime maps a round's detected regime to its sampling-rule
// §2 bucket. trend_continuation is not in the rule's table; the regime type
// documents that consumers treat it as compression (#74), so it is
// non-directional here, subject to the same btc_24h cross-check. Any other
// unknown or empty regime is unclassified (never non-down).
func marketBucketForRegime(regime string, btc24h float64) string {
	switch strings.ToLower(strings.TrimSpace(regime)) {
	case "panic_dump", "market_pullback", "trend_down":
		return marketBucketDown
	case "mania_pump", "trend_up":
		return marketBucketUp
	case "range", "compression", "rotation", "trend_continuation":
		if btc24h > coverageNonDirMinBTC24h {
			return marketBucketNonDir
		}
		return marketBucketNonDirFailed
	default:
		return marketBucketUnclassified
	}
}

func isNonDownBucket(bucket string) bool {
	return bucket == marketBucketUp || bucket == marketBucketNonDir
}

func utcDay(rfc3339 string) string {
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}

// validationMarketCoverage is the sampling-rule §3 round-level coverage
// test over accepted, non-degraded rounds (rejected rounds are not in the
// summary's round rows; degraded rows are skipped).
type validationMarketCoverage struct {
	Rule                           string   `json:"rule"`
	CountedRounds                  int      `json:"counted_rounds"`
	Down                           int      `json:"down"`
	Up                             int      `json:"up"`
	NonDirectional                 int      `json:"non_directional"`
	NonDirectionalCrossCheckFailed int      `json:"non_directional_cross_check_failed"`
	Unclassified                   int      `json:"unclassified"`
	NonDownRounds                  int      `json:"non_down_rounds"`
	NonDownSharePct                float64  `json:"non_down_share_pct"`
	NonDownEpisodes                int      `json:"non_down_episodes"`
	NonDownUTCDays                 int      `json:"non_down_utc_days"`
	NonDownRoundsWithoutUTCDay     int      `json:"non_down_rounds_without_utc_day,omitempty"`
	DownMarketOnly                 bool     `json:"down_market_only"`
	FailedChecks                   []string `json:"failed_checks,omitempty"`
	NotCheckedHere                 []string `json:"not_checked_here"`
}

const marketCoverageRule = "docs/hunter-v7-validation-sampling-rule.md §2-§4: counted = accepted non-degraded rounds; " +
	"non-down = up + non_directional (btc_24h > -2%); mixed/unknown never non-down; " +
	"the §4 label applies unless non-down >= 30% of counted rounds, from >= 2 episodes (runs of consecutive counted rounds in one bucket), on >= 2 UTC days"

func buildMarketCoverage(rounds []validationRoundSummary) validationMarketCoverage {
	c := validationMarketCoverage{
		Rule: marketCoverageRule,
		NotCheckedHere: []string{
			"§3 step 4 cell floor: non-down n >= 30 per decision cell in the deduplicated sample (cmd/v7_fee_net_offline market_regime table)",
			"§3 step 5 comparable settings: see effective_flags.rounds_with_different_settings",
		},
	}
	ordered := append([]validationRoundSummary(nil), rounds...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Round < ordered[j].Round })
	days := map[string]bool{}
	prevBucket := ""
	for _, r := range ordered {
		if r.Degraded {
			continue // excluded; it does not break an episode either
		}
		c.CountedRounds++
		switch r.MarketBucket {
		case marketBucketDown:
			c.Down++
		case marketBucketUp:
			c.Up++
		case marketBucketNonDir:
			c.NonDirectional++
		case marketBucketNonDirFailed:
			c.NonDirectionalCrossCheckFailed++
		default:
			c.Unclassified++
		}
		if isNonDownBucket(r.MarketBucket) {
			c.NonDownRounds++
			if r.MarketBucket != prevBucket {
				c.NonDownEpisodes++
			}
			if r.UTCDay != "" {
				days[r.UTCDay] = true
			} else {
				c.NonDownRoundsWithoutUTCDay++
			}
		}
		prevBucket = r.MarketBucket
	}
	c.NonDownUTCDays = len(days)
	c.NonDownSharePct = pct(c.NonDownRounds, c.CountedRounds)
	if c.CountedRounds == 0 {
		c.FailedChecks = append(c.FailedChecks, "no counted rounds")
	}
	if c.NonDownSharePct < coverageMinNonDownSharePct {
		c.FailedChecks = append(c.FailedChecks, fmt.Sprintf("non-down share %.1f%% < %.0f%%", c.NonDownSharePct, coverageMinNonDownSharePct))
	}
	if c.NonDownEpisodes < coverageMinNonDownEpisodes {
		c.FailedChecks = append(c.FailedChecks, fmt.Sprintf("non-down episodes %d < %d", c.NonDownEpisodes, coverageMinNonDownEpisodes))
	}
	if c.NonDownUTCDays < coverageMinNonDownUTCDays {
		c.FailedChecks = append(c.FailedChecks, fmt.Sprintf("non-down UTC days %d < %d", c.NonDownUTCDays, coverageMinNonDownUTCDays))
	}
	c.DownMarketOnly = len(c.FailedChecks) > 0
	return c
}

// applyRunSummaryStats fills the #107 fields from the round rows and totals
// buildValidationRunSummary has already set.
func applyRunSummaryStats(s *validationRunSummary) {
	if s.Rounds > 0 {
		s.MeanSignalsPerRound = float64(s.SignalCount) / float64(s.Rounds)
		s.MeanOpenReviewPerRound = float64(s.OpenReviewCount) / float64(s.Rounds)
	}
	if s.ValidRounds > 0 {
		s.ValidMeanSignalsPerRound = float64(s.ValidSignalCount) / float64(s.ValidRounds)
		s.ValidMeanOpenReviewPerRound = float64(s.ValidOpenReviewCount) / float64(s.ValidRounds)
	}
	s.Tiers, s.ValidTiers = validationTierBreakdown{}, validationTierBreakdown{}
	for _, r := range s.Round {
		s.Tiers.add(r.Tiers)
		if !r.Degraded {
			s.ValidTiers.add(r.Tiers)
		}
	}
	s.Tiers.finish()
	s.ValidTiers.finish()
	s.MarketCoverage = buildMarketCoverage(s.Round)
	s.SampleLabel = ""
	if s.MarketCoverage.DownMarketOnly {
		s.SampleLabel = sampleLabelDownMarketOnly
	}
}

// marketCoverageLine is the markdown first line: the label (when set) and
// the §3 bucket counts.
func marketCoverageLine(s validationRunSummary) string {
	c := s.MarketCoverage
	counts := fmt.Sprintf("counted rounds %d: down %d, up %d, non_directional %d, non_directional_cross_check_failed %d, unclassified %d; non-down %d (%.1f%%), %d episodes, %d UTC days",
		c.CountedRounds, c.Down, c.Up, c.NonDirectional, c.NonDirectionalCrossCheckFailed, c.Unclassified,
		c.NonDownRounds, c.NonDownSharePct, c.NonDownEpisodes, c.NonDownUTCDays)
	if s.SampleLabel != "" {
		return fmt.Sprintf("%s: %s (failed: %s)", s.SampleLabel, counts, strings.Join(c.FailedChecks, "; "))
	}
	return "Round-level non-down coverage passed (cell floor not checked here): " + counts
}

func tierSharesMarkdown(s validationRunSummary) string {
	var sb strings.Builder
	sb.WriteString("### Prompt-final tier shares (of signals)\n\n")
	sb.WriteString("| Scope | Signals | EXECUTABLE | REVIEWABLE | WATCH | REJECTED | Not in prompt |\n|---|---:|---:|---:|---:|---:|---:|\n")
	for _, row := range []struct {
		name string
		t    validationTierBreakdown
	}{{"all", s.Tiers}, {"valid", s.ValidTiers}} {
		sb.WriteString(fmt.Sprintf("| %s | %d | %d (%.1f%%) | %d (%.1f%%) | %d (%.1f%%) | %d (%.1f%%) | %d (%.1f%%) |\n",
			row.name, row.t.SignalCount, row.t.Executable, row.t.ExecutableSharePct, row.t.Reviewable, row.t.ReviewableSharePct,
			row.t.Watch, row.t.WatchSharePct, row.t.Rejected, row.t.RejectedSharePct, row.t.NotInPrompt, row.t.NotInPromptSharePct))
	}
	if s.Tiers.TierCountExceedsSignals {
		sb.WriteString("\n> WARNING: prompt tier counts exceed the signal count in at least one round; shares do not sum to 100%.\n")
	}
	sb.WriteString("\n")
	return sb.String()
}

func marketCoverageMarkdown(c validationMarketCoverage) string {
	var sb strings.Builder
	sb.WriteString("### Market coverage (sampling rule §3)\n\n")
	sb.WriteString("| Bucket | Rounds |\n|---|---:|\n")
	sb.WriteString(fmt.Sprintf("| down | %d |\n| up | %d |\n| non_directional | %d |\n| non_directional_cross_check_failed | %d |\n| unclassified | %d |\n| **counted** | %d |\n\n",
		c.Down, c.Up, c.NonDirectional, c.NonDirectionalCrossCheckFailed, c.Unclassified, c.CountedRounds))
	sb.WriteString(fmt.Sprintf("> Rule: %s.\n", c.Rule))
	for _, n := range c.NotCheckedHere {
		sb.WriteString(fmt.Sprintf("> Not checked here: %s.\n", n))
	}
	sb.WriteString("\n")
	return sb.String()
}
