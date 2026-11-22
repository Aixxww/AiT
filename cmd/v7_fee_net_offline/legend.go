package main

import (
	"fmt"
	"strings"

	"github.com/Aixxww/AiT/store"
)

// Fix 13 (P2-13): what every label, table and column of this tool's output
// means. The legend is written at the top of summary.md and as "legend" in
// summary.json, so a reader of the output finds it there. It records
// meanings only; nothing here changes a number, a grouping or a tier.
//
// Keep it in step with the code: TestLegendCoversOutput fails when a
// grouping, a walk status or an outcome label has no entry.

// legendEntry is one defined term.
type legendEntry struct {
	Term    string `json:"term"`
	Meaning string `json:"meaning"`
}

// legendSection is one group of terms.
type legendSection struct {
	Title   string        `json:"title"`
	Entries []legendEntry `json:"entries"`
}

// tableLegend documents one table (one grouping in groupings()).
type tableLegend struct {
	Grouping string `json:"grouping"`     // key in tables_dedup / tables_all_rows
	Title    string `json:"title"`        // summary.md heading ("" = summary.json only)
	Layer    string `json:"tier_layer"`   // which tier layer the key uses
	Key      string `json:"key"`          // what the Key column holds
	Rows     string `json:"rows_counted"` // which replayed rows the table counts
}

// outputLegend is the legend. Table entries follow groupings() order.
func outputLegend() []legendSection {
	return []legendSection{
		{Title: "Tier layers", Entries: []legendEntry{
			{"module status", "the signal's own status from the module that produced it (stored as status). No table in this tool groups by it."},
			{"assembled tier", "the kernel tier at scoring time (stored as execution_tier / tier_reason). It is also the tier the outcome tracker registers. Tables titled \"assembled\", and the summary.json groupings tier, tier x class, tier x class x rr_band, WATCH x tier_reason, setup x direction x tier and short ignition x tier, use this layer."},
			{"prompt-final tier", "the tier after the prompt-time reprice, live confirmation and readiness re-check, i.e. what the LLM saw (stored as prompt_final_tier / prompt_final_tier_reason, fix 12). Only the prompt-final tables use it. A row's prompt-final tier can be higher (promoted), lower (demoted) or equal to its assembled tier."},
			{"not_in_prompt_or_unrecorded", "the key for a row whose prompt_final_tier is empty. Either the signal was not shown in that round's prompt (not a prompt candidate, a held position, or no market data for the symbol), or the row was written without the column: before fix 12, by the live engine (it writes rows before the prompt is built), or by any other path that does not record it. It is never merged into a tier."},
		}},
		{Title: "Which rows are replayed", Entries: []legendEntry{
			{"rows", "hunter_v7_signal_records rows selected by -from / -to / -min-id / -max-id / -tiers / -setups. Without -tiers, rows with an empty execution_tier (router-filtered signals) are excluded; with -tiers, only the listed assembled tiers are loaded."},
			{"one row = one walk", "every row is walked on its own with the live OD-2 fee-net walk (trader.SignalOutcomeTracker): entry at the close of the first 1m bar after the signal minute, exit at the first touch of invalidation_price (stop) or tp1_price (TP1) within the horizon, stop first when one bar touches both. The live tracker's duplicate replacement and capacity limits do not apply."},
			{"deduplicated sample", "the first row of each thesis (symbol, canonical setup, direction) per dedup window; later rows of the same thesis inside the window are left out. Use these tables for the cell rule."},
			{"all rows", "every replayed row. Consecutive rounds re-emit the same thesis, so their walks overlap and n overstates independent samples."},
			{"same rows, different keys", "every table counts each row in its sample at most once, under that table's key. The assembled-tier table and the prompt-final-tier table therefore count the same rows and have the same total Rows; only the grouping differs. Tables that select a subset say so (WATCH x tier_reason: assembled WATCH only; short ignition: ignition_early_short only)."},
		}},
		{Title: "Columns", Entries: []legendEntry{
			{"Key", "the group the row belongs to (see the table list). Composite keys join their parts with \" | \"; the assembled -> prompt-final key is \"<assembled tier> -> <prompt-final tier>\"."},
			{"Rows", "replayed rows in the group, including rows that were not scored."},
			{"n (with R)", "rows that have a fee-net R: outcome tp1, stop or none. Only these enter the mean, the CI and the cell rule."},
			{"TP1 / stop / none", "counts of the scored outcomes: tp1 = TP1 touched first, R = (|TP1-E| - E*cost) / (|E-stop| + E*cost); stop = stop touched first (or both on one bar), R = -1; none = neither inside the horizon, R = 0."},
			{"TP1-first", "tp1 / n."},
			{"Mean fee-net R", "mean of R over the n scored rows."},
			{"90% CI", "bootstrap CI of the mean fee-net R (store.V7BootstrapMeanCI, the dashboard's computation; the level, resamples and seed are in summary.json cell_rule)."},
			{"Cell rule", "store.V7EvaluateCellStatus: " + store.V7CellStatusEarned + " = n >= cell_rule.min_samples and CI low > 0; " + store.V7CellStatusBlocked + " = n >= min_samples and CI high < 0; " + store.V7CellStatusProbation + " = otherwise."},
			{"Not scored", "rows in Rows but not in n: walked without R (degenerate_stop, degenerate_tp1, no_anchor) and skipped rows (skip_*), with their counts."},
		}},
		{Title: "Outcome and status labels", Entries: []legendEntry{
			{"walked", "the live walk ran to an outcome."},
			{store.V7FeeNetTP1, "TP1 touched first (scored)."},
			{store.V7FeeNetStop, "stop touched first, or both on the same bar (scored, R = -1)."},
			{store.V7FeeNetNone, "neither level touched within the horizon (scored, R = 0)."},
			{store.V7FeeNetDegenerateStop, "the entry bar's close is already at or beyond the stop: no R, not scored."},
			{store.V7FeeNetDegenerateTP1, "the entry bar's close is already at or beyond TP1: no R, not scored."},
			{store.V7FeeNetNoAnchor, "no 1m bar after the signal minute inside the horizon: no R, not scored."},
			{skipNoGeometry, "stop or tp1_price is empty: the live walk never starts. Not walked."},
			{skipHorizonOpen, "signal time + horizon is not in the past yet. Not walked."},
			{skipKlinesError, "public 1m klines could not be read (or the tracker refused the row). Not walked."},
			{skipIncompleteKlines, "the walk produced no outcome, or it ended as none / no_anchor without a bar past the horizon (the bars stop early; the live code would have closed it on wall-clock time). Not scored."},
			{store.V7ClassUnassigned, "class key when the row has no signal_class and the default registry has none for its setup."},
			{store.V7BandUnknown, "rr_band key when the row has no stored rr_band (e.g. rows written before U5); market_regime uses the same word when empty."},
		}},
	}
}

// tableLegends documents every table, in groupings() order.
func tableLegends() []tableLegend {
	const all = "every row in the sample"
	return []tableLegend{
		{"tier", "By assembled tier (DB execution_tier)", "assembled", "execution_tier", all},
		{"prompt-final tier", "By prompt-final tier (what the LLM saw; DB prompt_final_tier)", "prompt-final", "prompt_final_tier, or not_in_prompt_or_unrecorded when empty", all + " (the same rows as the assembled-tier table)"},
		{"assembled tier x prompt-final tier", "Assembled tier -> prompt-final tier", "assembled and prompt-final", "\"<execution_tier> -> <prompt-final key>\": how each assembled tier was shown to the LLM", all},
		{"tier x class", "", "assembled", "execution_tier | signal_class", all},
		{"tier x class x rr_band", "By assembled tier x class x rr_band", "assembled", "execution_tier | signal_class | rr_band", all},
		{"class x rr_band (all tiers)", "", "none (pools every tier)", "signal_class | rr_band, the dashboard's cell key", all},
		{"WATCH x tier_reason", "Assembled WATCH by tier_reason", "assembled", "the assembled tier_reason", "rows whose assembled tier is WATCH only (a row demoted to WATCH only at prompt time is not here)"},
		{"setup x direction x tier", "", "assembled", "stack | canonical setup | direction | execution_tier", all},
		{"short ignition", "ignition_early_short, pooled", "none", "\"ignition_early_short | all\"", "rows whose canonical setup is ignition_early_short (with or without the t2_ prefix)"},
		{"short ignition x tier", "ignition_early_short by assembled tier and tier_reason", "assembled", "execution_tier | tier_reason", "ignition_early_short rows only"},
		{"short ignition x rr_band", "ignition_early_short by rr_band", "none", "rr_band", "ignition_early_short rows only"},
		{"market_regime", "", "none", "market_regime", all},
	}
}

// outputLegendJSON is the summary.json "legend" value.
type outputLegendJSON struct {
	Sections []legendSection `json:"sections"`
	Tables   []tableLegend   `json:"tables"`
}

func buildOutputLegend() outputLegendJSON {
	return outputLegendJSON{Sections: outputLegend(), Tables: tableLegends()}
}

// legendMarkdown is the "How to read this report" section of summary.md.
func legendMarkdown() string {
	var b strings.Builder
	b.WriteString("## How to read this report\n\n")
	for _, sec := range outputLegend() {
		fmt.Fprintf(&b, "**%s**\n\n", sec.Title)
		for _, e := range sec.Entries {
			fmt.Fprintf(&b, "- `%s`: %s\n", e.Term, e.Meaning)
		}
		b.WriteString("\n")
	}
	b.WriteString("**Tables** (summary.md shows the titled ones, the long ones cut at 25-30 keys; summary.json has all of them in full under tables_dedup / tables_all_rows)\n\n")
	b.WriteString("| Grouping (summary.json) | Title (summary.md) | Tier layer | Key | Rows counted |\n|---|---|---|---|---|\n")
	for _, t := range tableLegends() {
		title := t.Title
		if title == "" {
			title = "(summary.json only)"
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %s |\n", t.Grouping, mdCell(title), mdCell(t.Layer), mdCell(t.Key), mdCell(t.Rows))
	}
	b.WriteString("\n")
	return b.String()
}

// mdCell escapes the pipes of composite keys so they stay inside one
// markdown table cell.
func mdCell(s string) string { return strings.ReplaceAll(s, "|", "\\|") }
