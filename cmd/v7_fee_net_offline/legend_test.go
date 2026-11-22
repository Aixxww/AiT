package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Aixxww/AiT/store"
)

// Fix 13: the legend covers the output as the code produces it, and its
// claims hold on real tables.

func legendTerms() map[string]bool {
	terms := map[string]bool{}
	for _, sec := range outputLegend() {
		for _, e := range sec.Entries {
			terms[e.Term] = true
		}
	}
	return terms
}

func TestLegendCoversOutput(t *testing.T) {
	// One table entry per grouping, in the same order.
	gs, tl := groupings(), tableLegends()
	if len(gs) != len(tl) {
		t.Fatalf("%d groupings, %d table legends", len(gs), len(tl))
	}
	for i := range gs {
		if gs[i].Name != tl[i].Grouping || tl[i].Layer == "" || tl[i].Key == "" || tl[i].Rows == "" {
			t.Fatalf("table legend %d %+v for grouping %q", i, tl[i], gs[i].Name)
		}
	}
	// Every summary.md table heading is documented, and every documented
	// title is a heading.
	md := markdownSummary(summary{Tables: map[string][]groupStats{}, TablesDedup: map[string][]groupStats{}})
	headings := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^#### (.+)$`).FindAllStringSubmatch(md, -1) {
		headings[m[1]] = true
	}
	titles := map[string]bool{}
	for _, l := range tl {
		if l.Title != "" {
			titles[l.Title] = true
			if !headings[l.Title] {
				t.Errorf("legend title %q is not a summary.md heading", l.Title)
			}
		}
	}
	for h := range headings {
		if !titles[h] {
			t.Errorf("summary.md heading %q has no legend entry", h)
		}
	}
	// Every walk status and outcome label, and the named empty value.
	terms := legendTerms()
	for _, term := range []string{"walked", skipNoGeometry, skipHorizonOpen, skipKlinesError, skipIncompleteKlines,
		store.V7FeeNetTP1, store.V7FeeNetStop, store.V7FeeNetNone, store.V7FeeNetDegenerateStop, store.V7FeeNetDegenerateTP1, store.V7FeeNetNoAnchor,
		promptTierKey(""), store.V7ClassUnassigned, store.V7BandUnknown,
		"assembled tier", "prompt-final tier", "module status"} {
		if !terms[term] {
			t.Errorf("legend has no entry for %q", term)
		}
	}
	// Every column of the markdown table.
	for _, col := range []string{"Key", "Rows", "n (with R)", "TP1 / stop / none", "TP1-first", "Mean fee-net R", "90% CI", "Cell rule", "Not scored"} {
		if !terms[col] {
			t.Errorf("legend has no entry for column %q", col)
		}
	}
	// Every legend table row has exactly 5 cells (composite keys escaped).
	for _, line := range strings.Split(legendMarkdown(), "\n") {
		if strings.HasPrefix(line, "| `") {
			if cells := strings.Count(strings.ReplaceAll(line, "\\|", ""), "|"); cells != 6 {
				t.Fatalf("legend table row splits into %d cells: %s", cells-1, line)
			}
		}
	}
	// The legend comes before the first table.
	if i, j := strings.Index(md, "## How to read this report"), strings.Index(md, "#### "); i < 0 || i > j {
		t.Fatalf("legend position %d, first table %d", i, j)
	}
}

// The legend's claims, checked on tables built by the code: the assembled
// and prompt-final tables count the same rows; Rows = n + not scored; the
// WATCH reason table holds assembled-WATCH rows only.
func TestLegendClaimsHold(t *testing.T) {
	rule := store.EffectiveV7CellRule(nil)
	win, loss := 1.2, -1.0
	mk := func(tier, prompt string, w walkResult) result {
		in := longRow()
		in.Tier, in.PromptTier = tier, prompt
		return result{inputRow: in, Walk: w, Dedup: true}
	}
	rs := []result{
		mk("WATCH", "REVIEWABLE", walkResult{Status: "walked", Outcome: store.V7FeeNetTP1, R: &win}),
		mk("WATCH", "", walkResult{Status: "walked", Outcome: store.V7FeeNetStop, R: &loss}),
		mk("REVIEWABLE", "WATCH", walkResult{Status: "walked", Outcome: store.V7FeeNetDegenerateTP1}),
		mk("REVIEWABLE", "REVIEWABLE", walkResult{Status: skipNoGeometry}),
	}
	tables := buildTables(rs, false, rule)
	sum := func(name string) (rows, n, unscored int) {
		for _, g := range tables[name] {
			rows += g.Rows
			n += g.N
			for _, c := range g.NoR {
				unscored += c
			}
			for _, c := range g.Skipped {
				unscored += c
			}
		}
		return
	}
	ar, an, au := sum("tier")
	pr, pn, pu := sum("prompt-final tier")
	xr, _, _ := sum("assembled tier x prompt-final tier")
	if ar != 4 || pr != ar || xr != ar || an != pn || au != pu {
		t.Fatalf("assembled %d/%d/%d, prompt-final %d/%d/%d, cross %d", ar, an, au, pr, pn, pu, xr)
	}
	if an+au != ar {
		t.Fatalf("Rows %d != n %d + not scored %d", ar, an, au)
	}
	wr, _, _ := sum("WATCH x tier_reason")
	if wr != 2 {
		t.Fatalf("WATCH x tier_reason counted %d rows, want the 2 assembled-WATCH rows (not the row demoted at prompt time)", wr)
	}
}

// summary.json carries the same legend.
func TestSummaryJSONCarriesLegend(t *testing.T) {
	s := buildSummary(nil, wc2h, "store default", 0, store.EffectiveV7CellRule(nil), newKlineStore("", true, nil), "x.db", rowFilter{})
	dir := t.TempDir()
	if err := writeOutputs(dir, nil, s); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Legend outputLegendJSON `json:"legend"`
	}
	if err := json.Unmarshal(b, &got); err != nil || len(got.Legend.Tables) != len(groupings()) || len(got.Legend.Sections) != len(outputLegend()) {
		t.Fatalf("summary.json legend %+v (%v)", got.Legend, err)
	}
	md, _ := os.ReadFile(filepath.Join(dir, "summary.md"))
	if !strings.Contains(string(md), "`not_in_prompt_or_unrecorded`: the key for a row whose prompt_final_tier is empty") {
		t.Fatal("summary.md lacks the not_in_prompt_or_unrecorded definition")
	}
}
