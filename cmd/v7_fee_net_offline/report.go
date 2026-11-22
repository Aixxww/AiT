package main

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/Aixxww/AiT/store"
)

// result is one replayed row.
type result struct {
	inputRow
	Walk  walkResult `json:"walk"`
	Dedup bool       `json:"kept_in_dedup"` // first row of its thesis window (see dedupRows)
}

// dedupRows marks the first row of each thesis (symbol, canonical setup,
// direction) per window: a row is dropped when a kept row of the same
// thesis started less than window earlier. Consecutive rounds re-emit the
// same signal every few minutes, so their fee-net walks overlap almost
// entirely; the deduplicated sample is closer to independent draws.
// window <= 0 keeps every row. results must be sorted by time.
func dedupRows(results []result, window time.Duration) {
	last := map[string]time.Time{}
	for i := range results {
		r := &results[i]
		if window <= 0 {
			r.Dedup = true
			continue
		}
		key := r.Symbol + "|" + r.CanonSetup + "|" + r.Direction
		if t, ok := last[key]; ok && r.Time.Sub(t) < window {
			continue
		}
		last[key] = r.Time
		r.Dedup = true
	}
}

// groupStats is one table row under the cell rule.
type groupStats struct {
	Key      string         `json:"key"`
	Rows     int            `json:"rows"`
	N        int            `json:"n"` // rows with a fee-net R (tp1 / stop / none)
	TP1      int            `json:"tp1"`
	Stop     int            `json:"stop"`
	None     int            `json:"none"`
	NoR      map[string]int `json:"walked_without_r,omitempty"` // degenerate_* / no_anchor
	Skipped  map[string]int `json:"skipped,omitempty"`
	TP1Rate  float64        `json:"tp1_first_rate"`
	MeanR    float64        `json:"mean_fee_net_r"`
	CILo     float64        `json:"ci_lo"`
	CIHi     float64        `json:"ci_hi"`
	CellRule string         `json:"cell_rule_status"` // earned / probation / blocked (store.V7EvaluateCellStatus)
	SumR     float64        `json:"-"`
	rs       []float64
}

type grouping struct {
	Name string
	Key  func(r result) (string, bool)
}

func groupings() []grouping {
	isShortIgnition := func(r result) bool {
		return r.CanonSetup == "ignition_early_short"
	}
	return []grouping{
		{"tier", func(r result) (string, bool) { return r.Tier, true }}, // assembled tier (execution_tier)
		{"prompt-final tier", func(r result) (string, bool) { return promptTierKey(r.PromptTier), true }},
		{"assembled tier x prompt-final tier", func(r result) (string, bool) { return r.Tier + " -> " + promptTierKey(r.PromptTier), true }},
		{"tier x class", func(r result) (string, bool) { return r.Tier + " | " + r.Class, true }},
		{"tier x class x rr_band", func(r result) (string, bool) { return r.Tier + " | " + r.Class + " | " + r.RRBand, true }},
		{"class x rr_band (all tiers)", func(r result) (string, bool) { return r.Class + " | " + r.RRBand, true }},
		{"WATCH x tier_reason", func(r result) (string, bool) { return r.TierReason, r.Tier == "WATCH" }},
		{"setup x direction x tier", func(r result) (string, bool) {
			return r.Stack + " | " + r.CanonSetup + " | " + r.Direction + " | " + r.Tier, true
		}},
		{"short ignition", func(r result) (string, bool) { return "ignition_early_short | all", isShortIgnition(r) }},
		{"short ignition x tier", func(r result) (string, bool) { return r.Tier + " | " + r.TierReason, isShortIgnition(r) }},
		{"short ignition x rr_band", func(r result) (string, bool) { return r.RRBand, isShortIgnition(r) }},
		{"market_regime", func(r result) (string, bool) { return r.Regime, true }},
	}
}

// promptTierKey names an empty prompt-final tier (fix 12): the row was not
// shown in a prompt, or was written before the column existed / by a path
// that does not record it.
func promptTierKey(t string) string {
	if t == "" {
		return "not_in_prompt_or_unrecorded"
	}
	return t
}

// buildTables groups the sample under each grouping and applies the cell
// rule (store.EffectiveV7CellRule: n >= MinSamples and the bootstrap CI of
// mean fee-net R entirely above / below 0), with a fresh generator seeded
// with the rule's seed per row, as store.BuildV7CellTables does.
func buildTables(results []result, dedupOnly bool, rule store.V7CellRuleConfig) map[string][]groupStats {
	out := map[string][]groupStats{}
	for _, g := range groupings() {
		acc := map[string]*groupStats{}
		for _, r := range results {
			if dedupOnly && !r.Dedup {
				continue
			}
			key, ok := g.Key(r)
			if !ok {
				continue
			}
			s := acc[key]
			if s == nil {
				s = &groupStats{Key: key, NoR: map[string]int{}, Skipped: map[string]int{}}
				acc[key] = s
			}
			s.Rows++
			switch {
			case r.Walk.Status != "walked":
				s.Skipped[r.Walk.Status]++
			case r.Walk.R == nil:
				s.NoR[r.Walk.Outcome]++
			default:
				s.N++
				s.rs = append(s.rs, *r.Walk.R)
				s.SumR += *r.Walk.R
				switch r.Walk.Outcome {
				case store.V7FeeNetTP1:
					s.TP1++
				case store.V7FeeNetStop:
					s.Stop++
				case store.V7FeeNetNone:
					s.None++
				}
			}
		}
		rows := make([]groupStats, 0, len(acc))
		for _, s := range acc {
			if s.N > 0 {
				s.MeanR = s.SumR / float64(s.N)
				s.TP1Rate = float64(s.TP1) / float64(s.N)
			}
			lo, hi, ciOK := store.V7BootstrapMeanCI(s.rs, rule.BootstrapResamples, rule.CILevel, rand.New(rand.NewSource(rule.Seed)))
			s.CILo, s.CIHi = lo, hi
			s.CellRule = store.V7EvaluateCellStatus(s.N, lo, hi, ciOK, rule.MinSamples)
			if len(s.NoR) == 0 {
				s.NoR = nil
			}
			if len(s.Skipped) == 0 {
				s.Skipped = nil
			}
			s.rs = nil
			rows = append(rows, *s)
		}
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].N != rows[j].N {
				return rows[i].N > rows[j].N
			}
			return rows[i].Key < rows[j].Key
		})
		out[g.Name] = rows
	}
	return out
}

// agreement compares the replay with fee-net outcomes already stored by
// the live / harness tracker (rows it tracked).
type agreement struct {
	Compared      int      `json:"compared"`
	SameOutcome   int      `json:"same_outcome"`
	SameR         int      `json:"same_r"` // |dR| <= 1e-6, or both without R
	Mismatches    []string `json:"mismatches,omitempty"`
	StoredNoWalk  int      `json:"stored_but_not_walked"`
	MismatchLimit int      `json:"-"`
}

func compareStored(results []result) agreement {
	a := agreement{MismatchLimit: 50}
	for _, r := range results {
		if r.StoredOutcome == "" {
			continue
		}
		if r.Walk.Status != "walked" {
			a.StoredNoWalk++
			continue
		}
		a.Compared++
		if r.StoredOutcome == r.Walk.Outcome {
			a.SameOutcome++
		}
		switch {
		case r.StoredR == nil && r.Walk.R == nil:
			a.SameR++
		case r.StoredR != nil && r.Walk.R != nil && abs(*r.StoredR-*r.Walk.R) <= 1e-6:
			a.SameR++
		}
		if r.StoredOutcome != r.Walk.Outcome && len(a.Mismatches) < a.MismatchLimit {
			a.Mismatches = append(a.Mismatches, fmt.Sprintf("id %d %s %s: stored %s, replay %s", r.ID, r.Symbol, r.Time.UTC().Format(time.RFC3339), r.StoredOutcome, r.Walk.Outcome))
		}
	}
	return a
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// markdownTable renders one grouping.
func markdownTable(title string, rows []groupStats, max int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "#### %s\n\n", title)
	if len(rows) == 0 {
		b.WriteString("(no rows)\n\n")
		return b.String()
	}
	b.WriteString("| Key | Rows | n (with R) | TP1 / stop / none | TP1-first | Mean fee-net R | 90% CI | Cell rule | Not scored |\n")
	b.WriteString("|---|---:|---:|---|---:|---:|---|---|---|\n")
	for i, s := range rows {
		if max > 0 && i >= max {
			fmt.Fprintf(&b, "| … %d more rows in summary.json | | | | | | | | |\n", len(rows)-max)
			break
		}
		fmt.Fprintf(&b, "| `%s` | %d | %d | %d / %d / %d | %.0f%% | %+.3f | [%+.3f, %+.3f] | %s | %s |\n",
			mdCell(s.Key), s.Rows, s.N, s.TP1, s.Stop, s.None, s.TP1Rate*100, s.MeanR, s.CILo, s.CIHi, s.CellRule, notScored(s))
	}
	b.WriteString("\n")
	return b.String()
}

func notScored(s groupStats) string {
	var parts []string
	for _, m := range []map[string]int{s.NoR, s.Skipped} {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s %d", k, m[k]))
		}
	}
	return strings.Join(parts, ", ")
}
