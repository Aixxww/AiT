package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Aixxww/AiT/store"

	_ "modernc.org/sqlite"
)

// moverAuditReport is the output of the daily mover audit.
type moverAuditReport struct {
	GeneratedAt  string         `json:"generated_at"`
	TradeDate    string         `json:"trade_date"`
	TotalSymbols int            `json:"total_symbols"`
	Thresholds   []float64      `json:"thresholds_pct"`
	Summary      []thresholdSum `json:"summary"`
	Movers       []moverEntry   `json:"movers"`
}

type thresholdSum struct {
	ThresholdPct     float64 `json:"threshold_pct"`
	TotalMovers      int     `json:"total_movers"`
	SeenCount        int     `json:"seen_count"`
	WatchCount       int     `json:"watch_count"`
	Reviewable       int     `json:"reviewable_count"`
	Executable       int     `json:"executable_count"`
	RecallRate       float64 `json:"recall_rate_pct"`
	Recall4hCount    int     `json:"recall_4h_before_move_count"`
	Recall4hRate     float64 `json:"recall_4h_before_move_rate_pct"`
	Review4hCount    int     `json:"reviewable_4h_before_move_count"`
	Review4hRate     float64 `json:"reviewable_4h_before_move_rate_pct"`
	MoveStartMissing int     `json:"move_start_missing_count"`
}

type moverEntry struct {
	Symbol          string  `json:"symbol"`
	High24h         float64 `json:"high_24h"`
	Low24h          float64 `json:"low_24h"`
	Amplitude24h    float64 `json:"amplitude_24h_pct"`
	FirstSeenAt     string  `json:"first_seen_at,omitempty"`
	FirstWatchAt    string  `json:"first_watch_at,omitempty"`
	FirstReviewAt   string  `json:"first_reviewable_at,omitempty"`
	FirstExecAt     string  `json:"first_executable_at,omitempty"`
	MoveStartAt     string  `json:"move_start_at,omitempty"`
	BestTier        string  `json:"best_tier"`
	BestTierReason  string  `json:"best_tier_reason"`
	BlockedGate     string  `json:"blocked_gate"`
	MissedStage     string  `json:"missed_stage"`
	LeadTimeMinutes int     `json:"lead_time_minutes"`
	Recall4hBefore  bool    `json:"recall_4h_before_move"`
}

// binanceTicker24h is the raw response from Binance Futures 24h ticker API.
type binanceTicker24h struct {
	Symbol      string `json:"symbol"`
	HighPrice   string `json:"highPrice"`
	LowPrice    string `json:"lowPrice"`
	LastPrice   string `json:"lastPrice"`
	PriceChange string `json:"priceChange"`
	Volume      string `json:"volume"`
	QuoteVolume string `json:"quoteVolume"`
}

type auditKline struct {
	OpenTime time.Time
	Open     float64
	High     float64
	Low      float64
	Close    float64
}

func main() {
	dbPath := flag.String("db", "data/data.db", "Path to SQLite database")
	lookbackHours := flag.Int("lookback", 24, "Lookback window in hours")
	outputFile := flag.String("output", "", "Output JSON file path (default: stdout)")
	thresholds := flag.String("thresholds", "20,30,50", "Comma-separated amplitude thresholds (%)")
	binanceURL := flag.String("binance", "https://fapi.binance.com", "Binance Futures API base URL")
	flag.Parse()

	// Parse thresholds
	var threshPcts []float64
	for _, s := range strings.Split(*thresholds, ",") {
		var v float64
		if _, err := fmt.Sscanf(strings.TrimSpace(s), "%f", &v); err == nil {
			threshPcts = append(threshPcts, v)
		}
	}
	sort.Float64s(threshPcts)

	// Open database
	st, err := store.New(*dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	// Fetch all Binance futures 24h tickers
	tickers, err := fetchBinanceTickers(*binanceURL)
	if err != nil {
		log.Fatalf("Failed to fetch Binance tickers: %v", err)
	}

	now := time.Now().UTC()
	tradeDate := now.Format("2006-01-02")
	lookbackStart := now.Add(-time.Duration(*lookbackHours) * time.Hour)

	// Compute amplitude for all USDT perps
	type symAmplitude struct {
		symbol    string
		high24h   float64
		low24h    float64
		amplitude float64 // percentage
	}
	var allMovers []symAmplitude
	for _, t := range tickers {
		if !strings.HasSuffix(t.Symbol, "USDT") || strings.Contains(t.Symbol, "_") {
			continue
		}
		high := parseFloat(t.HighPrice)
		low := parseFloat(t.LowPrice)
		if low <= 0 || high <= 0 {
			continue
		}
		amp := (high - low) / low * 100
		allMovers = append(allMovers, symAmplitude{
			symbol:    t.Symbol,
			high24h:   high,
			low24h:    low,
			amplitude: amp,
		})
	}
	sort.Slice(allMovers, func(i, j int) bool {
		return allMovers[i].amplitude > allMovers[j].amplitude
	})

	// For each threshold, compute recall stats
	sigStore := st.HunterV7Signal()
	report := moverAuditReport{
		GeneratedAt:  now.Format(time.RFC3339),
		TradeDate:    tradeDate,
		TotalSymbols: len(allMovers),
		Thresholds:   threshPcts,
	}

	moverLabelBySymbol := make(map[string]store.HunterV7MoverLabel)
	klineCache := make(map[string][]auditKline)

	for _, thresh := range threshPcts {
		var filtered []symAmplitude
		for _, m := range allMovers {
			if m.amplitude >= thresh {
				filtered = append(filtered, m)
			}
		}

		sum := thresholdSum{ThresholdPct: thresh, TotalMovers: len(filtered)}
		for _, m := range filtered {
			entry := moverEntry{
				Symbol:       m.symbol,
				High24h:      m.high24h,
				Low24h:       m.low24h,
				Amplitude24h: round2(m.amplitude),
			}

			klines, ok := klineCache[m.symbol]
			if !ok {
				limit := *lookbackHours + 8
				if limit < 24 {
					limit = 24
				}
				if limit > 120 {
					limit = 120
				}
				fetched, err := fetchBinanceKlines(*binanceURL, m.symbol, "1h", limit)
				if err != nil {
					log.Printf("Warning: failed to fetch 1h klines for %s: %v", m.symbol, err)
				}
				klines = fetched
				klineCache[m.symbol] = klines
			}
			moveStart := detectMoveStart(klines, thresh)
			if moveStart != nil {
				entry.MoveStartAt = moveStart.Format(time.RFC3339)
			} else {
				sum.MoveStartMissing++
			}

			// Query signal records for this symbol in lookback window
			firstSeen := sigStore.FirstSeenAt(m.symbol, lookbackStart, now)
			bestTier, bestReason := sigStore.BestTierForSymbol(m.symbol, lookbackStart, now)
			blockedGate := sigStore.BlockedGateForSymbol(m.symbol, lookbackStart, now)

			if firstSeen != nil {
				entry.FirstSeenAt = firstSeen.Format(time.RFC3339)
				if moveStart != nil {
					entry.LeadTimeMinutes = int(moveStart.Sub(*firstSeen).Minutes())
					entry.Recall4hBefore = !firstSeen.After(moveStart.Add(-4 * time.Hour))
				} else {
					entry.LeadTimeMinutes = int(now.Sub(*firstSeen).Minutes())
				}
			}

			// Get all records for tier timestamps
			records, _ := sigStore.QueryBySymbol(m.symbol, lookbackStart, now)
			var firstWatchAt, firstReviewableAt, firstExecutableAt *time.Time
			for _, rec := range records {
				switch rec.ExecutionTier {
				case "WATCH":
					if entry.FirstWatchAt == "" {
						entry.FirstWatchAt = rec.Timestamp.Format(time.RFC3339)
						ts := rec.Timestamp
						firstWatchAt = &ts
					}
				case "REVIEWABLE":
					if entry.FirstReviewAt == "" {
						entry.FirstReviewAt = rec.Timestamp.Format(time.RFC3339)
						ts := rec.Timestamp
						firstReviewableAt = &ts
					}
				case "EXECUTABLE":
					if entry.FirstExecAt == "" {
						entry.FirstExecAt = rec.Timestamp.Format(time.RFC3339)
						ts := rec.Timestamp
						firstExecutableAt = &ts
					}
				}
			}

			entry.BestTier = bestTier
			entry.BestTierReason = bestReason
			entry.BlockedGate = blockedGate
			entry.MissedStage = computeMissedStage(firstSeen, bestTier)

			if firstSeen != nil {
				sum.SeenCount++
				if entry.Recall4hBefore {
					sum.Recall4hCount++
				}
			}
			if entry.FirstWatchAt != "" {
				sum.WatchCount++
			}
			if entry.FirstReviewAt != "" {
				sum.Reviewable++
				if moveStart != nil && firstReviewableAt != nil && !firstReviewableAt.After(moveStart.Add(-4*time.Hour)) {
					sum.Review4hCount++
				}
			}
			if entry.FirstExecAt != "" {
				sum.Executable++
			}

			report.Movers = append(report.Movers, entry)

			// Build mover label for DB persistence
			label := store.HunterV7MoverLabel{
				TradeDate:         tradeDate,
				Symbol:            m.symbol,
				High24h:           m.high24h,
				Low24h:            m.low24h,
				Amplitude24h:      m.amplitude,
				FirstSeenAt:       firstSeen,
				FirstWatchAt:      firstWatchAt,
				FirstReviewableAt: firstReviewableAt,
				FirstExecutableAt: firstExecutableAt,
				MoveStartAt:       moveStart,
				MissedStage:       entry.MissedStage,
				BestTier:          bestTier,
				BestTierReason:    bestReason,
				BlockedGate:       blockedGate,
				LeadTimeMinutes:   entry.LeadTimeMinutes,
			}
			if existing, ok := moverLabelBySymbol[m.symbol]; !ok || label.Amplitude24h > existing.Amplitude24h {
				moverLabelBySymbol[m.symbol] = label
			}
		}

		if sum.TotalMovers > 0 {
			sum.RecallRate = round2(float64(sum.SeenCount) / float64(sum.TotalMovers) * 100)
			sum.Recall4hRate = round2(float64(sum.Recall4hCount) / float64(sum.TotalMovers) * 100)
			sum.Review4hRate = round2(float64(sum.Review4hCount) / float64(sum.TotalMovers) * 100)
		}
		report.Summary = append(report.Summary, sum)
	}

	// Persist mover labels to DB
	moverLabels := make([]store.HunterV7MoverLabel, 0, len(moverLabelBySymbol))
	for _, label := range moverLabelBySymbol {
		moverLabels = append(moverLabels, label)
	}
	if len(moverLabels) > 0 {
		if err := st.HunterV7Mover().UpsertBatch(moverLabels); err != nil {
			log.Printf("Warning: failed to persist mover labels: %v", err)
		}
	}

	// Output report
	output, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal report: %v", err)
	}

	if *outputFile != "" {
		if err := os.WriteFile(*outputFile, output, 0644); err != nil {
			log.Fatalf("Failed to write output file: %v", err)
		}
		fmt.Fprintf(os.Stderr, "Report written to %s\n", *outputFile)
	} else {
		fmt.Println(string(output))
	}

	// Print summary to stderr
	fmt.Fprintf(os.Stderr, "\n=== Hunter v7 Mover Audit Summary (%s) ===\n", tradeDate)
	fmt.Fprintf(os.Stderr, "Total USDT perps scanned: %d\n", len(allMovers))
	for _, sum := range report.Summary {
		fmt.Fprintf(os.Stderr, "Amplitude >= %.0f%%: %d movers, seen=%d (%.1f%%), 4h_recall=%d (%.1f%%), watch=%d, reviewable=%d, reviewable_4h=%d (%.1f%%), executable=%d\n",
			sum.ThresholdPct, sum.TotalMovers, sum.SeenCount, sum.RecallRate,
			sum.Recall4hCount, sum.Recall4hRate, sum.WatchCount, sum.Reviewable,
			sum.Review4hCount, sum.Review4hRate, sum.Executable)
	}
}

func fetchBinanceTickers(baseURL string) ([]binanceTicker24h, error) {
	url := baseURL + "/fapi/v1/ticker/24hr"
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP GET %s: %w", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	var tickers []binanceTicker24h
	if err := json.Unmarshal(body, &tickers); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}
	return tickers, nil
}

func fetchBinanceKlines(baseURL, symbol, interval string, limit int) ([]auditKline, error) {
	url := fmt.Sprintf("%s/fapi/v1/klines?symbol=%s&interval=%s&limit=%d", baseURL, symbol, interval, limit)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP GET %s: %w", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	var raw [][]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse klines JSON: %w", err)
	}
	klines := make([]auditKline, 0, len(raw))
	for _, row := range raw {
		if len(row) < 5 {
			continue
		}
		openTimeMs := int64(toFloat(row[0]))
		k := auditKline{
			OpenTime: time.UnixMilli(openTimeMs).UTC(),
			Open:     toFloat(row[1]),
			High:     toFloat(row[2]),
			Low:      toFloat(row[3]),
			Close:    toFloat(row[4]),
		}
		if k.High > 0 && k.Low > 0 {
			klines = append(klines, k)
		}
	}
	sort.Slice(klines, func(i, j int) bool {
		return klines[i].OpenTime.Before(klines[j].OpenTime)
	})
	return klines, nil
}

func detectMoveStart(klines []auditKline, thresholdPct float64) *time.Time {
	if len(klines) < 2 || thresholdPct <= 0 {
		return nil
	}
	first := klines[0]
	last := klines[len(klines)-1]
	if last.Close >= first.Open {
		return detectLongMoveStart(klines, thresholdPct)
	}
	return detectShortMoveStart(klines, thresholdPct)
}

func detectLongMoveStart(klines []auditKline, thresholdPct float64) *time.Time {
	var low float64
	var lowAt time.Time
	for _, k := range klines {
		if k.Low <= 0 {
			continue
		}
		if low == 0 || k.Low < low {
			low = k.Low
			lowAt = k.OpenTime
		}
		if low > 0 && k.High > 0 && (k.High-low)/low*100 >= thresholdPct {
			ts := lowAt
			return &ts
		}
	}
	return nil
}

func detectShortMoveStart(klines []auditKline, thresholdPct float64) *time.Time {
	var high float64
	var highAt time.Time
	for _, k := range klines {
		if k.High <= 0 {
			continue
		}
		if high == 0 || k.High > high {
			high = k.High
			highAt = k.OpenTime
		}
		if high > 0 && k.Low > 0 && (high-k.Low)/k.Low*100 >= thresholdPct {
			ts := highAt
			return &ts
		}
	}
	return nil
}

func toFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case string:
		return parseFloat(n)
	default:
		return 0
	}
}

func parseFloat(s string) float64 {
	var v float64
	fmt.Sscanf(s, "%f", &v)
	return v
}

func computeMissedStage(firstSeen *time.Time, bestTier string) string {
	if firstSeen == nil {
		return "not_in_universe"
	}
	switch bestTier {
	case "EXECUTABLE":
		return "" // Not missed
	case "REVIEWABLE":
		return "llm_wait_or_backend_reject"
	case "WATCH":
		return "tier_watch_not_upgraded"
	case "REJECTED":
		return "kernel_tier_rejected"
	default:
		return "module_no_match_or_router_filtered"
	}
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}
