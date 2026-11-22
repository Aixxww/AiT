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

// unescapedPipes counts the "|" cell separators of a markdown table row
// (a pipe preceded by a backslash is cell content, as GitHub renders it).
var unescapedPipe = regexp.MustCompile(`(^|[^\\])\|`)

func cellCount(row string) int {
	return len(unescapedPipe.FindAllString(row, -1)) - 1
}

// Composite keys ("WATCH | TRIGGER | 1.2_1.5") render as one Key cell in
// summary.md; the JSON keeps the raw key.
func TestDataTableCompositeKeyIsOneCell(t *testing.T) {
	rule := store.EffectiveV7CellRule(nil)
	win := 1.2
	in := longRow()
	in.Tier, in.Class, in.RRBand = "WATCH", "TRIGGER", "1.2_1.5"
	rs := []result{{inputRow: in, Walk: walkResult{Status: "walked", Outcome: store.V7FeeNetTP1, R: &win}, Dedup: true}}
	tables := buildTables(rs, false, rule)
	rows := tables["tier x class x rr_band"]
	if len(rows) != 1 || rows[0].Key != "WATCH | TRIGGER | 1.2_1.5" {
		t.Fatalf("raw key %+v", rows)
	}
	md := markdownTable("By assembled tier x class x rr_band", rows, 0)
	var header, data string
	for _, l := range strings.Split(md, "\n") {
		switch {
		case strings.HasPrefix(l, "| Key |"):
			header = l
		case strings.HasPrefix(l, "| `"):
			data = l
		}
	}
	if header == "" || data == "" {
		t.Fatalf("table lines missing:\n%s", md)
	}
	if cellCount(data) != cellCount(header) {
		t.Fatalf("data row has %d cells, header %d:\n%s", cellCount(data), cellCount(header), data)
	}
	if !strings.Contains(data, "| `WATCH \\| TRIGGER \\| 1.2_1.5` |") {
		t.Fatalf("key cell not escaped: %s", data)
	}
	// summary.json keeps the raw key.
	s := buildSummary(rs, wc2h, "store default", 0, rule, newKlineStore("", true, nil), "x.db", rowFilter{})
	dir := t.TempDir()
	if err := writeOutputs(dir, rs, s); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "summary.json"))
	var got struct {
		Tables map[string][]groupStats `json:"tables_all_rows"`
	}
	if err := json.Unmarshal(b, &got); err != nil || got.Tables["tier x class x rr_band"][0].Key != "WATCH | TRIGGER | 1.2_1.5" {
		t.Fatalf("JSON key changed: %+v (%v)", got.Tables["tier x class x rr_band"], err)
	}
	// Every table row of the written summary.md has its header's cell count
	// (data tables and the legend table alike).
	mdFile, _ := os.ReadFile(filepath.Join(dir, "summary.md"))
	want, checked := 0, 0
	for _, l := range strings.Split(string(mdFile), "\n") {
		switch {
		case strings.HasPrefix(l, "| Key |"), strings.HasPrefix(l, "| Grouping"):
			want = cellCount(l)
		case !strings.HasPrefix(l, "|"):
			want = 0
		case want > 0 && strings.HasPrefix(l, "| `"):
			checked++
			if cellCount(l) != want {
				t.Fatalf("summary.md row has %d cells, want %d: %s", cellCount(l), want, l)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no table rows checked")
	}
}
