// Command v7_fee_net_offline replays the live OD-2 fee-net walk over stored
// hunter_v7_signal_records rows with public historical 1m klines (fix 07
// queue item 8, P1-7). It is read-only: the database is opened with
// mode=ro and query_only, nothing is written to it, and no order or live
// component is touched. Outputs go to -out.
//
// The walk is the production code: each row is registered in its own
// trader.SignalOutcomeTracker (stepFeeNet: entry at the close of the first
// 1m bar after the signal minute, exit at the first touch of the planned
// stop or TP1 within the horizon, stop first on a bar touching both, R from
// store.V7FeeNetRRAtTP1 with the round-trip cost) and walked with one
// TickNow over the cached bars. The cell rule is the dashboard's
// (store.EffectiveV7CellRule, V7BootstrapMeanCI, V7EvaluateCellStatus).
//
// Questions it is built for: (a) is WATCH too strict (WATCH vs REVIEWABLE
// rows, WATCH by tier_reason and by class x rr_band), and (b) should short
// ignition (ignition_early_short, shadow_only) stay shadow-only.
//
// Usage:
//
//	go run ./cmd/v7_fee_net_offline -db data/data.db -out reports/fee-net-offline \
//	    [-from 2026-11-18T00:00:00Z] [-to ...] [-tiers WATCH,REVIEWABLE] \
//	    [-horizon 2h] [-strategy-id <id>] [-kline-cache dir] [-offline]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Aixxww/AiT/store"

	"gorm.io/gorm"
)

func main() {
	dbPath := flag.String("db", "data/data.db", "SQLite database with hunter_v7_signal_records (opened read-only)")
	outDir := flag.String("out", "reports/fee-net-offline", "output directory (rows.jsonl, summary.json, summary.md)")
	fromS := flag.String("from", "", "only rows with timestamp >= this RFC3339 time")
	toS := flag.String("to", "", "only rows with timestamp < this RFC3339 time")
	minID := flag.Int64("min-id", 0, "only rows with id >= this")
	maxID := flag.Int64("max-id", 0, "only rows with id <= this")
	tiersS := flag.String("tiers", "", "comma-separated execution tiers (default: every non-empty tier)")
	setupsS := flag.String("setups", "", "comma-separated canonical setup types (default: all)")
	horizon := flag.Duration("horizon", 2*time.Hour, "fee-net horizon (tracker TimeoutDuration): 2h = validation harness, 8h = live default")
	costPct := flag.Float64("cost-pct", 0, "round-trip fee-net cost as a fraction of entry (default: -strategy-id's v7_outcome_tags, else store default 0.0015)")
	strategyID := flag.String("strategy-id", "", "read the fee-net cost from this strategy's config (read-only)")
	dedupWindow := flag.Duration("dedup-window", 0, "thesis dedup window for the deduplicated tables (default = -horizon; negative disables)")
	tp1Fallback := flag.Bool("tp1-fallback-target1", false, "use target_1 when tp1_price is empty (the live tracker does not; off by default)")
	cacheDir := flag.String("kline-cache", "", "1m kline day-file cache (default <out>/kline-cache)")
	offline := flag.Bool("offline", false, "use only cached klines; never call the network")
	flag.Parse()

	db, err := openReadOnly(*dbPath)
	if err != nil {
		fatal("open db: %v", err)
	}
	f := rowFilter{MinID: *minID, MaxID: *maxID, Tiers: splitList(*tiersS), Setups: splitList(*setupsS)}
	if f.From, err = parseTime(*fromS); err != nil {
		fatal("-from: %v", err)
	}
	if f.To, err = parseTime(*toS); err != nil {
		fatal("-to: %v", err)
	}
	cost, costFrom, err := resolveCost(db, *costPct, *strategyID)
	if err != nil {
		fatal("cost: %v", err)
	}
	recs, err := loadRecords(db, f)
	if err != nil {
		fatal("load rows: %v", err)
	}
	if *cacheDir == "" {
		*cacheDir = filepath.Join(*outDir, "kline-cache")
	}
	window := *dedupWindow
	if window == 0 {
		window = *horizon
	}
	ks := newKlineStore(*cacheDir, *offline, publicRangeFetcher)
	wc := walkConfig{Horizon: *horizon, CostPct: cost}
	results := replay(recs, wc, ks, *tp1Fallback, window, time.Now())

	rule := store.EffectiveV7CellRule(nil)
	sum := buildSummary(results, wc, costFrom, window, rule, ks, *dbPath, f)
	if err := writeOutputs(*outDir, results, sum); err != nil {
		fatal("write: %v", err)
	}
	fmt.Printf("rows %d walked %d with_r %d (dedup: %d with_r); outputs in %s\n",
		sum.Rows, sum.Walked, sum.WithR, sum.DedupWithR, *outDir)
}

// replay walks every row (sorted by time) and marks the dedup sample.
func replay(recs []store.HunterV7SignalRecord, wc walkConfig, ks *klineStore, tp1Fallback bool, window time.Duration, now time.Time) []result {
	classes := store.DefaultV7SignalClassRegistry()
	results := make([]result, 0, len(recs))
	for _, r := range recs {
		in := buildInputRow(r, tp1Fallback, classes)
		results = append(results, result{inputRow: in, Walk: walkRow(in, wc, ks, now)})
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].Time.Before(results[j].Time) })
	dedupRows(results, window)
	return results
}

// resolveCost picks the fee-net cost: the flag, else the strategy's
// effective v7_outcome_tags cost (as the harness and live trader do), else
// the store default.
func resolveCost(db *gorm.DB, flagCost float64, strategyID string) (float64, string, error) {
	if flagCost > 0 {
		return flagCost, "flag -cost-pct", nil
	}
	if strings.TrimSpace(strategyID) != "" {
		var row struct{ Config string }
		if err := db.Raw("SELECT config FROM strategies WHERE id = ?", strategyID).Scan(&row).Error; err != nil {
			return 0, "", err
		}
		if row.Config == "" {
			return 0, "", fmt.Errorf("strategy %q not found", strategyID)
		}
		var cfg store.StrategyConfig
		if err := json.Unmarshal([]byte(row.Config), &cfg); err != nil {
			return 0, "", err
		}
		var tags *store.V7OutcomeTagConfig
		if cfg.CoinSource.Hunter != nil {
			tags = cfg.CoinSource.Hunter.V7OutcomeTags
		}
		return store.EffectiveV7OutcomeTags(tags).FeeNetCostPct, "strategy " + strategyID, nil
	}
	return store.DefaultV7FeeNetCostPct, "store default", nil
}

type summary struct {
	GeneratedAt   string                  `json:"generated_at"`
	DB            string                  `json:"db"`
	Filter        rowFilter               `json:"filter"`
	Horizon       string                  `json:"horizon"`
	CostPct       float64                 `json:"cost_pct"`
	CostFrom      string                  `json:"cost_from"`
	DedupWindow   string                  `json:"dedup_window"`
	CellRule      store.V7CellRuleConfig  `json:"cell_rule"`
	Rows          int                     `json:"rows"`
	Walked        int                     `json:"walked"`
	WithR         int                     `json:"with_r"`
	DedupRows     int                     `json:"dedup_rows"`
	DedupWithR    int                     `json:"dedup_with_r"`
	ByStatus      map[string]int          `json:"by_status"`
	ByOutcome     map[string]int          `json:"by_outcome"`
	SpanFrom      string                  `json:"span_from,omitempty"`
	SpanTo        string                  `json:"span_to,omitempty"`
	Regimes       map[string]int          `json:"market_regimes"`
	Agreement     agreement               `json:"agreement_with_stored_fee_net"`
	Klines        map[string]any          `json:"klines"`
	Tables        map[string][]groupStats `json:"tables_all_rows"`
	TablesDedup   map[string][]groupStats `json:"tables_dedup"`
	ClassFallback int                     `json:"rows_class_from_registry"`
}

func buildSummary(results []result, wc walkConfig, costFrom string, window time.Duration, rule store.V7CellRuleConfig, ks *klineStore, db string, f rowFilter) summary {
	s := summary{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339), DB: db, Filter: f,
		Horizon: wc.Horizon.String(), CostPct: wc.CostPct, CostFrom: costFrom, CellRule: rule,
		Rows: len(results), ByStatus: map[string]int{}, ByOutcome: map[string]int{}, Regimes: map[string]int{},
	}
	if window > 0 {
		s.DedupWindow = window.String()
	} else {
		s.DedupWindow = "off"
	}
	for _, r := range results {
		s.ByStatus[r.Walk.Status]++
		s.Regimes[r.Regime]++
		if r.ClassSource == "registry" {
			s.ClassFallback++
		}
		if r.Walk.Status == "walked" {
			s.Walked++
			s.ByOutcome[r.Walk.Outcome]++
			if r.Walk.R != nil {
				s.WithR++
				if r.Dedup {
					s.DedupWithR++
				}
			}
		}
		if r.Dedup {
			s.DedupRows++
		}
	}
	if len(results) > 0 {
		s.SpanFrom = results[0].Time.UTC().Format(time.RFC3339)
		s.SpanTo = results[len(results)-1].Time.UTC().Format(time.RFC3339)
	}
	s.Agreement = compareStored(results)
	s.Klines = map[string]any{"cache_dir": ks.dir, "offline": ks.offline, "day_files_fetched": ks.Fetched, "day_files_from_cache": ks.Cached, "errors": ks.Errors}
	s.Tables = buildTables(results, false, rule)
	s.TablesDedup = buildTables(results, true, rule)
	return s
}

func writeOutputs(dir string, results []result, s summary) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var rows strings.Builder
	for _, r := range results {
		b, err := json.Marshal(r)
		if err != nil {
			return err
		}
		rows.Write(b)
		rows.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(dir, "rows.jsonl"), []byte(rows.String()), 0o644); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), b, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "summary.md"), []byte(markdownSummary(s)), 0o644)
}

func markdownSummary(s summary) string {
	var b strings.Builder
	b.WriteString("# Offline fee-net replay (OD-2 walk over stored rows)\n\n")
	fmt.Fprintf(&b, "- Rows %d (span %s to %s); walked %d; with fee-net R %d; deduplicated sample %d rows, %d with R (thesis window %s).\n",
		s.Rows, s.SpanFrom, s.SpanTo, s.Walked, s.WithR, s.DedupRows, s.DedupWithR, s.DedupWindow)
	fmt.Fprintf(&b, "- Horizon %s; round-trip cost %.4f (%s). Cell rule: n >= %d and the %.0f%% bootstrap CI of mean fee-net R above 0 (earned) or below 0 (blocked), else probation.\n",
		s.Horizon, s.CostPct, s.CostFrom, s.CellRule.MinSamples, s.CellRule.CILevel*100)
	fmt.Fprintf(&b, "- Status: %s. Outcomes: %s.\n", kv(s.ByStatus), kv(s.ByOutcome))
	fmt.Fprintf(&b, "- Market regime of the rows: %s. **Conclusions hold only for these regimes; a sample from one market phase (e.g. a falling market) says nothing about the others.**\n", kv(s.Regimes))
	fmt.Fprintf(&b, "- Agreement with fee-net already stored by the tracker: %d compared, %d same outcome, %d same R; %d stored rows not walked here.\n\n",
		s.Agreement.Compared, s.Agreement.SameOutcome, s.Agreement.SameR, s.Agreement.StoredNoWalk)
	for _, part := range []struct {
		title  string
		tables map[string][]groupStats
	}{{"Deduplicated sample (use this for the cell rule)", s.TablesDedup}, {"All rows (overlapping walks; n overstates independent samples)", s.Tables}} {
		fmt.Fprintf(&b, "## %s\n\n", part.title)
		b.WriteString("### (a) Is WATCH too strict?\n\n")
		b.WriteString(markdownTable("By assembled tier (DB execution_tier)", part.tables["tier"], 0))
		b.WriteString(markdownTable("By prompt-final tier (what the LLM saw; DB prompt_final_tier)", part.tables["prompt-final tier"], 0))
		b.WriteString(markdownTable("Assembled tier -> prompt-final tier", part.tables["assembled tier x prompt-final tier"], 0))
		b.WriteString(markdownTable("Assembled WATCH by tier_reason", part.tables["WATCH x tier_reason"], 25))
		b.WriteString(markdownTable("By assembled tier x class x rr_band", part.tables["tier x class x rr_band"], 30))
		b.WriteString("### (b) Should short ignition stay shadow-only?\n\n")
		b.WriteString(markdownTable("ignition_early_short, pooled", part.tables["short ignition"], 0))
		b.WriteString(markdownTable("ignition_early_short by assembled tier and tier_reason", part.tables["short ignition x tier"], 0))
		b.WriteString(markdownTable("ignition_early_short by rr_band", part.tables["short ignition x rr_band"], 0))
	}
	return b.String()
}

func kv(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s %d", k, m[k]))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseTime(s string) (time.Time, error) {
	if strings.TrimSpace(s) == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "v7_fee_net_offline: "+format+"\n", args...)
	os.Exit(1)
}
