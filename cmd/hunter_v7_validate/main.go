package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/Aixxww/AiT/datafetch"
	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
	aittrader "github.com/Aixxww/AiT/trader"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type validationReport struct {
	GeneratedAt       string                       `json:"generated_at"`
	Snapshot          snapshotSummary              `json:"snapshot"`
	Config            local.V7Config               `json:"config"`
	FormatCheck       formatCheck                  `json:"format_check"`
	AIRecognition     aiRecognitionCheck           `json:"ai_recognition"`
	OpportunityCover  opportunityCoverCheck        `json:"opportunity_cover"`
	Signals           []local.V7SignalOutput       `json:"signals"`
	PotentialPool     []local.V7PotentialCandidate `json:"potential_pool"`
	PromptPreviewPath string                       `json:"prompt_preview_path"`
	Issues            []issue                      `json:"issues"`
}

type snapshotSummary struct {
	SymbolCount   int     `json:"symbol_count"`
	UniverseCount int     `json:"universe_count"`
	FetchMs       int64   `json:"fetch_ms"`
	RestErrors    int     `json:"rest_errors"`
	BTC24h        float64 `json:"btc_24h"`
	ETH24h        float64 `json:"eth_24h"`
	Regime        string  `json:"regime"`
}

type formatCheck struct {
	JSONMarshalOK        bool     `json:"json_marshal_ok"`
	JSONUnmarshalOK      bool     `json:"json_unmarshal_ok"`
	MissingFieldCount    int      `json:"missing_field_count"`
	ExecutableGapCount   int      `json:"executable_gap_count"`
	RequiredSchemaFields []string `json:"required_schema_fields"`
}

type aiRecognitionCheck struct {
	PromptContainsV7JSON      bool           `json:"prompt_contains_v7_json"`
	PromptContainsSetupType   bool           `json:"prompt_contains_setup_type"`
	PromptContainsEntryMode   bool           `json:"prompt_contains_entry_mode"`
	PromptContainsRiskLevel   bool           `json:"prompt_contains_risk_level"`
	PromptContainsConfirms    bool           `json:"prompt_contains_confirmations"`
	PromptContainsInvalid     bool           `json:"prompt_contains_invalidation"`
	PromptContainsTierSummary bool           `json:"prompt_contains_tier_summary"`
	PromptTierCounts          map[string]int `json:"prompt_tier_counts"`
	PromptCandidateCoinCount  int            `json:"prompt_candidate_coin_count"`
	PromptBytes               int            `json:"prompt_bytes"`
}

type opportunityCoverCheck struct {
	SignalCount             int            `json:"signal_count"`
	LongCount               int            `json:"long_count"`
	ShortCount              int            `json:"short_count"`
	BySetupType             map[string]int `json:"by_setup_type"`
	ByStatus                map[string]int `json:"by_status"`
	ByRiskLevel             map[string]int `json:"by_risk_level"`
	ByEntryMode             map[string]int `json:"by_entry_mode"`
	ByExecutionTier         map[string]int `json:"by_execution_tier"`
	ByMarketRegime          map[string]int `json:"by_market_regime"`
	PotentialPoolCount      int            `json:"potential_pool_count"`
	UnmatchedPotentialCount int            `json:"unmatched_potential_count"`
	DistinctSetups          int            `json:"distinct_setups"`
	HasMomentum             bool           `json:"has_momentum"`
	HasReversal             bool           `json:"has_reversal"`
	HasSqueeze              bool           `json:"has_squeeze"`
	HasRange                bool           `json:"has_range"`
	HasFunding              bool           `json:"has_funding"`
	HasAccumulation         bool           `json:"has_accumulation"`
	HasDistribution         bool           `json:"has_distribution"`
}

type issue struct {
	Severity string `json:"severity"`
	Symbol   string `json:"symbol,omitempty"`
	Code     string `json:"code"`
	Detail   string `json:"detail"`
}

type validationOptions struct {
	topDetail      int
	maxWorkers     int
	maxOutput      int
	watchOutput    int
	minPriority    float64
	aggressive     bool
	strategyID     string
	dbPath         string
	outDir         string
	rounds         int
	roundInterval  time.Duration
	dumpUniverse   string
	persistSignals bool
	trackOutcomes  bool
	store          *store.Store
	watchState     *local.V7SignalStateManager
	outcomeTracker *aittrader.SignalOutcomeTracker
}

func main() {
	topDetail := flag.Int("top-detail", 220, "number of high-volume symbols with full OI/LSR/kline detail")
	maxWorkers := flag.Int("max-workers", 8, "max concurrent per-symbol Binance REST workers; keep low for live validation")
	maxOutput := flag.Int("max-output", 30, "max Hunter v7 signals to output")
	watchOutput := flag.Int("watch-output", 5, "max Hunter v7 pre-move watch signals to append")
	minPriority := flag.Float64("min-priority", 45, "minimum AI priority")
	aggressive := flag.Bool("aggressive", true, "use aggressive AI priority weighting")
	strategyID := flag.String("strategy-id", "", "strategy ID to load from data/data.db for prompt simulation")
	dbPath := flag.String("db", "data/data.db", "SQLite database path")
	outDir := flag.String("out-dir", "reports", "output directory")
	dumpUniverse := flag.String("dump-universe", "", "write a golden-replay fixture (universe+regime+config JSON) to this path")
	rounds := flag.Int("rounds", 1, "number of validation rounds to run")
	roundInterval := flag.Duration("round-interval", 120*time.Second, "sleep interval between validation rounds")
	persistSignals := flag.Bool("persist-signals", true, "persist classified raw Hunter v7 signals into hunter_v7_signal_records")
	trackOutcomes := flag.Bool("track-outcomes", true, "track persisted EXECUTABLE/REVIEWABLE signals with 1m candles during validation")
	flag.Parse()

	opts := validationOptions{
		topDetail:      *topDetail,
		maxWorkers:     *maxWorkers,
		maxOutput:      *maxOutput,
		watchOutput:    *watchOutput,
		minPriority:    *minPriority,
		aggressive:     *aggressive,
		strategyID:     *strategyID,
		dbPath:         *dbPath,
		outDir:         *outDir,
		dumpUniverse:   *dumpUniverse,
		rounds:         *rounds,
		roundInterval:  *roundInterval,
		persistSignals: *persistSignals,
		trackOutcomes:  *trackOutcomes,
		watchState:     local.NewV7SignalStateManager(),
	}
	if opts.rounds <= 0 {
		opts.rounds = 1
	}
	if opts.maxWorkers <= 0 {
		opts.maxWorkers = 8
	}
	if opts.persistSignals {
		st, err := store.New(opts.dbPath)
		if err != nil {
			log.Fatalf("open store for signal persistence failed: %v", err)
		}
		defer st.Close()
		opts.store = st
		if opts.trackOutcomes {
			tracker := aittrader.NewSignalOutcomeTracker(&aittrader.TrackerConfig{
				PollInterval:          time.Minute,
				TimeoutDuration:       2 * time.Hour,
				MaxTracked:            500,
				SnapshotLimit:         180,
				ActiveOutcomeInterval: time.Second,
				EnableDynamicStop:     true,
			}, nil)
			tracker.SetOutcomeCallback(func(outcome aittrader.TrackedOutcome) {
				if err := st.HunterV7Signal().UpdateTrackOutcome(outcome.RecordID, store.HunterV7SignalTrackUpdate{
					Status:       string(outcome.Status),
					CurrentPrice: outcome.CurrentPrice,
					ExitPrice:    outcome.ExitPrice,
					StopPrice:    outcome.StopUsed,
					PnLPct:       outcome.PnLPct,
					MFE:          outcome.MaxFavorable,
					MAE:          outcome.MaxAdverse,
					ExitTime:     outcome.ExitTime,
					Snapshots:    outcome.SnapshotJSON,
				}); err != nil {
					log.Printf("WARN update validation V7 outcome failed: %v", err)
				}
			})
			backfillFetcher := datafetch.NewDataFetcher(datafetch.FetcherConfig{
				Timeout: 8 * time.Second,
				KlineIntervals: []datafetch.KlineInterval{
					{Interval: "1m", Limit: 120},
				},
			})
			tracker.SetCandleBackfillSource(func(symbol string, from, to time.Time) []aittrader.TrackedCandle {
				return validationBackfillCandles(backfillFetcher, symbol, from, to)
			})
			opts.outcomeTracker = tracker
		}
	}
	for round := 1; round <= opts.rounds; round++ {
		if opts.rounds > 1 {
			fmt.Printf("Hunter v7 validation round %d/%d\n", round, opts.rounds)
		}
		if err := runValidation(opts, round); err != nil {
			log.Fatalf("validation round %d failed: %v", round, err)
		}
		if round < opts.rounds {
			fmt.Printf("Sleeping %s before next Binance REST validation round\n", opts.roundInterval)
			time.Sleep(opts.roundInterval)
		}
	}
}

func runValidation(opts validationOptions, round int) error {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	strategyCfg, err := loadStrategyConfig(opts.dbPath, opts.strategyID)
	if err != nil {
		return fmt.Errorf("load strategy config failed: %w", err)
	}
	includeNonCryptoFutures := false
	if strategyCfg != nil && strategyCfg.CoinSource.Hunter != nil {
		includeNonCryptoFutures = strategyCfg.CoinSource.Hunter.IncludeNonCryptoFutures
	}

	fetcher := datafetch.NewDataFetcher(datafetch.FetcherConfig{
		TopNForDetail:           opts.topDetail,
		MaxWorkers:              opts.maxWorkers,
		Timeout:                 15 * time.Second,
		IncludeNonCryptoFutures: includeNonCryptoFutures,
	})
	snap, err := fetcher.Fetch(ctx)
	if err != nil {
		return fmt.Errorf("fetch snapshot failed: %w", err)
	}

	cfg := local.DefaultV7Config()
	cfg.MaxOutput = opts.maxOutput
	cfg.WatchOutput = opts.watchOutput
	cfg.MinAIPriority = opts.minPriority
	cfg.Aggressive = opts.aggressive
	cfg.CycleNumber = round
	cfg.WatchStateManager = opts.watchState
	if strategyCfg != nil {
		if strategyCfg.CoinSource.Hunter != nil {
			if strategyCfg.CoinSource.Hunter.V7MaxOutput > 0 {
				cfg.MaxOutput = strategyCfg.CoinSource.Hunter.V7MaxOutput
			}
			if strategyCfg.CoinSource.Hunter.V7WatchOutput > 0 {
				cfg.WatchOutput = strategyCfg.CoinSource.Hunter.V7WatchOutput
			}
			if strategyCfg.CoinSource.Hunter.V7MinAIPriority > 0 {
				cfg.MinAIPriority = strategyCfg.CoinSource.Hunter.V7MinAIPriority
			}
			cfg.Aggressive = strategyCfg.CoinSource.Hunter.V7Aggressive
		}
		if strategyCfg.CoinSource.HunterLimit > 0 && (cfg.MaxOutput <= 0 || strategyCfg.CoinSource.HunterLimit < cfg.MaxOutput) {
			cfg.MaxOutput = strategyCfg.CoinSource.HunterLimit
		}
	}

	v7Result := local.ScoreHunterV7Detailed(snap, cfg)
	universe := v7Result.Universe
	regime := v7Result.Regime
	signals := v7Result.Signals
	if opts.dumpUniverse != "" {
		// Golden-replay capture: freeze this cycle's routing inputs so the
		// refactor safety net can replay RouteDetailed deterministically.
		if err := local.DumpV7GoldenFixture(opts.dumpUniverse, universe, regime, cfg); err != nil {
			return fmt.Errorf("dump golden fixture failed: %w", err)
		}
		fmt.Printf("golden fixture: %s (universe=%d regime=%s)\n", opts.dumpUniverse, len(universe), regime)
	}
	geometry := executionGeometryFromStrategy(strategyCfg)
	candidates := kernel.AssembleHunterV7CandidateCoins(signals, "BOTH", geometry)
	prompt := buildPrompt(candidates, signals, snap, strategyCfg)

	now := time.Now()
	stamp := now.Format("20060102-150405")
	if opts.rounds > 1 {
		stamp = fmt.Sprintf("%s-r%02d", stamp, round)
	}
	if err := os.MkdirAll(opts.outDir, 0755); err != nil {
		return fmt.Errorf("create out dir failed: %w", err)
	}
	promptPath := fmt.Sprintf("%s/hunter-v7-live-prompt-%s.txt", opts.outDir, stamp)
	rawPath := fmt.Sprintf("%s/hunter-v7-live-validation-raw-%s.json", opts.outDir, stamp)
	mdPath := fmt.Sprintf("%s/hunter-v7-live-validation-report-%s.md", opts.outDir, stamp)

	if err := os.WriteFile(promptPath, []byte(prompt), 0644); err != nil {
		return fmt.Errorf("write prompt failed: %w", err)
	}

	report := validationReport{
		GeneratedAt: now.Format("2006-01-02 15:04:05 MST"),
		Snapshot: snapshotSummary{
			SymbolCount:   snap.Meta.SymbolCount,
			UniverseCount: len(universe),
			FetchMs:       time.Since(start).Milliseconds(),
			RestErrors:    snap.Meta.RestErrors,
			BTC24h:        priceChange24h(snap, "BTCUSDT"),
			ETH24h:        priceChange24h(snap, "ETHUSDT"),
			Regime:        string(regime),
		},
		Config:            cfg,
		Signals:           signals,
		PotentialPool:     v7Result.PotentialPool,
		PromptPreviewPath: promptPath,
	}
	report.FormatCheck, report.Issues = validateFormat(signals)
	report.AIRecognition = validatePrompt(prompt, len(candidates))
	report.OpportunityCover = validateCoverage(signals, geometry)
	report.OpportunityCover.PotentialPoolCount = len(v7Result.PotentialPool)
	for _, candidate := range v7Result.PotentialPool {
		if !candidate.MatchedModule {
			report.OpportunityCover.UnmatchedPotentialCount++
		}
	}
	report.Issues = append(report.Issues, promptIssues(report.AIRecognition)...)
	report.Issues = append(report.Issues, coverageIssues(report.OpportunityCover)...)
	report.Issues = append(report.Issues, snapshotIssues(report.Snapshot)...)

	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report failed: %w", err)
	}
	if err := os.WriteFile(rawPath, raw, 0644); err != nil {
		return fmt.Errorf("write raw report failed: %w", err)
	}
	if err := os.WriteFile(mdPath, []byte(formatMarkdown(report, rawPath)), 0644); err != nil {
		return fmt.Errorf("write markdown report failed: %w", err)
	}
	if err := persistValidationSignals(opts, round, v7Result.RawSignals, candidates, now.UTC(), snap); err != nil {
		return err
	}

	printConsoleSummary(report, rawPath, mdPath)
	return nil
}

func persistValidationSignals(opts validationOptions, cycleNum int, rawSignals []local.V7SignalOutput, candidates []kernel.CandidateCoin, ts time.Time, snap *datafetch.Snapshot) error {
	if !opts.persistSignals || opts.store == nil {
		return nil
	}
	records := kernel.BuildHunterV7SignalRecords(rawSignals, candidates)
	dbRecords := kernel.BuildHunterV7SignalDBRecords(cycleNum, records, ts, string(aittrader.TrackedActive))
	if len(dbRecords) == 0 {
		return nil
	}
	if err := opts.store.HunterV7Signal().CreateBatch(dbRecords); err != nil {
		return fmt.Errorf("persist hunter v7 validation signals failed: %w", err)
	}
	if opts.outcomeTracker == nil {
		return nil
	}
	opts.outcomeTracker.SetCandleHistorySource(func(symbol string, since time.Time) []aittrader.TrackedCandle {
		return validationTrackedCandlesFromSnapshot(snap, symbol, since)
	})
	for i, dbRec := range dbRecords {
		if i >= len(records) || dbRec.ID <= 0 {
			continue
		}
		rec := records[i]
		if !kernel.HunterV7ShouldTrackSignal(rec) {
			continue
		}
		entry := kernel.HunterV7SignalEntryPrice(rec.Signal)
		if entry <= 0 || rec.Signal.Invalidation.Price <= 0 {
			continue
		}
		opts.outcomeTracker.Register(
			dbRec.ID,
			rec.Signal.Symbol,
			string(rec.Signal.Direction),
			string(rec.Signal.SetupType),
			rec.Tier,
			entry,
			rec.Signal.Invalidation.Price,
			rec.Signal.TP0Price,
			rec.Signal.TP1Price,
			rec.Signal.TP2Price,
			ts,
		)
	}
	opts.outcomeTracker.TickNow()
	fmt.Printf("persisted Hunter v7 signals: %d (tracking open-review outcomes)\n", len(dbRecords))
	return nil
}

func validationTrackedCandlesFromSnapshot(snap *datafetch.Snapshot, symbol string, since time.Time) []aittrader.TrackedCandle {
	if snap == nil || snap.Symbols == nil || symbol == "" {
		return nil
	}
	ss := snap.Symbols[symbol]
	if ss == nil {
		return nil
	}
	out := make([]aittrader.TrackedCandle, 0, len(ss.Klines["1m"]))
	for _, k := range ss.Klines["1m"] {
		candleTime := time.UnixMilli(k.CloseTime).UTC()
		if !since.IsZero() && !candleTime.After(since) {
			continue
		}
		closePrice := k.Close
		if closePrice <= 0 {
			closePrice = ss.Price
		}
		if closePrice <= 0 {
			continue
		}
		out = append(out, aittrader.TrackedCandle{
			T:      candleTime,
			Open:   k.Open,
			High:   k.High,
			Low:    k.Low,
			Close:  closePrice,
			Volume: k.Volume,
		})
	}
	return out
}

func validationBackfillCandles(fetcher *datafetch.DataFetcher, symbol string, from, to time.Time) []aittrader.TrackedCandle {
	if fetcher == nil || symbol == "" || from.IsZero() {
		return nil
	}
	if to.IsZero() || to.Before(from) {
		to = time.Now().UTC()
	}
	minutes := int(to.Sub(from).Minutes()) + 3
	if minutes < 10 {
		minutes = 10
	}
	if minutes > 120 {
		minutes = 120
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	klines, err := fetcher.FetchKlines(ctx, symbol, "1m", minutes)
	if err != nil {
		log.Printf("WARN validation Hunter v7 1m backfill failed for %s: %v", symbol, err)
		return nil
	}
	out := make([]aittrader.TrackedCandle, 0, len(klines))
	for _, k := range klines {
		candleTime := time.UnixMilli(k.CloseTime).UTC()
		if candleTime.Before(from) || candleTime.After(to.Add(time.Minute)) || k.Close <= 0 {
			continue
		}
		out = append(out, aittrader.TrackedCandle{
			T:      candleTime,
			Open:   k.Open,
			High:   k.High,
			Low:    k.Low,
			Close:  k.Close,
			Volume: k.Volume,
		})
	}
	return out
}

// executionGeometryFromStrategy mirrors the production engine's geometry
// derivation so validate-side tiering matches what the live kernel would emit.
func executionGeometryFromStrategy(strategyCfg *store.StrategyConfig) kernel.HunterV7ExecutionGeometry {
	if strategyCfg == nil {
		return kernel.HunterV7EffectiveExecutionGeometry(0, 0, 0, 0, true)
	}
	riskControl := strategyCfg.RiskControl
	return kernel.HunterV7EffectiveExecutionGeometry(
		riskControl.MaxTakeProfitPriceMovePct,
		riskControl.MinStopLossPriceMovePct,
		riskControl.MaxEntryPriceDeviationPct,
		riskControl.MinRiskRewardRatio,
		true,
	)
}

func loadStrategyConfig(dbPath, strategyID string) (*store.StrategyConfig, error) {
	if strings.TrimSpace(strategyID) == "" {
		return nil, nil
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var configJSON string
	if err := db.QueryRow("SELECT config FROM strategies WHERE id = ?", strategyID).Scan(&configJSON); err != nil {
		return nil, err
	}
	var cfg store.StrategyConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func buildPrompt(candidates []kernel.CandidateCoin, signals []local.V7SignalOutput, snap *datafetch.Snapshot, strategyCfg *store.StrategyConfig) string {
	cfg := store.GetDefaultStrategyConfig("zh")
	if strategyCfg != nil {
		cfg = *strategyCfg
	} else {
		cfg.CoinSource.SourceType = "hunter_v7"
		cfg.CoinSource.HunterLimit = len(candidates)
		cfg.CoinSource.Hunter = &store.HunterConfig{V7MaxOutput: len(candidates), V7MinAIPriority: 45, V7Aggressive: true}
	}
	if cfg.CoinSource.HunterLimit <= 0 || cfg.CoinSource.HunterLimit > len(candidates) {
		cfg.CoinSource.HunterLimit = len(candidates)
	}
	engine := kernel.NewStrategyEngine(&cfg)
	md := make(map[string]*market.Data, len(signals)+1)
	for _, sig := range signals {
		price := 0.0
		change1h := 0.0
		change4h := 0.0
		funding := 0.0
		oi := 0.0
		if sig.PriceCtx != nil {
			price = sig.PriceCtx.Last
			change1h = sig.PriceCtx.Change1h
			change4h = sig.PriceCtx.Change4h
		}
		if sig.DerivativesCtx != nil {
			funding = sig.DerivativesCtx.FundingRate
			oi = sig.DerivativesCtx.OIValue
		}
		// Prefer the production converter so the prompt cycle sees the same
		// TimeframeData (klines + EMA20 series) the live trader would; the
		// pre-prompt live-confirmation pass depends on it. Fall back to the
		// sparse context-only Data when the snapshot lacks klines.
		if snap != nil {
			if ss, ok := snap.Symbols[sig.Symbol]; ok && ss != nil && len(ss.Klines) > 0 {
				if full, err := market.BuildDataFromSymbolSnapshot(sig.Symbol, ss, []string{"5m", "15m", "1h"}, "15m", 60); err == nil {
					full.FundingRate = funding
					md[sig.Symbol] = full
					continue
				}
			}
		}
		md[sig.Symbol] = &market.Data{
			Symbol:        sig.Symbol,
			CurrentPrice:  price,
			PriceChange1h: change1h,
			PriceChange4h: change4h,
			FundingRate:   funding,
			OpenInterest:  &market.OIData{Latest: oi},
		}
	}
	ctx := &kernel.Context{
		CurrentTime:    time.Now().Format("2006-01-02 15:04:05"),
		RuntimeMinutes: 0,
		CallCount:      1,
		Account: kernel.AccountInfo{
			TotalEquity:      1000,
			AvailableBalance: 1000,
		},
		CandidateCoins: candidates,
		MarketDataMap:  md,
	}
	systemPrompt := engine.BuildSystemPrompt(1000, "balanced")
	userPrompt := engine.BuildUserPrompt(ctx)
	return systemPrompt + "\n\n--- USER PROMPT ---\n\n" + userPrompt
}

func validateFormat(signals []local.V7SignalOutput) (formatCheck, []issue) {
	required := []string{
		"symbol", "direction", "setup_type", "status", "setup_score", "risk_score",
		"liquidity_score", "timing_score", "regime_fit_score", "ai_priority",
		"reason_codes", "entry_mode", "entry_zone", "invalidation", "targets",
		"required_confirmations", "confidence", "risk_level", "market_regime",
		"price_context", "derivatives_context",
	}
	check := formatCheck{RequiredSchemaFields: required}
	var issues []issue

	data, err := json.Marshal(signals)
	check.JSONMarshalOK = err == nil
	if err != nil {
		issues = append(issues, issue{Severity: "critical", Code: "json_marshal_failed", Detail: err.Error()})
		return check, issues
	}
	var decoded []map[string]any
	err = json.Unmarshal(data, &decoded)
	check.JSONUnmarshalOK = err == nil
	if err != nil {
		issues = append(issues, issue{Severity: "critical", Code: "json_unmarshal_failed", Detail: err.Error()})
		return check, issues
	}

	for i, sig := range signals {
		symbol := sig.Symbol
		if i < len(decoded) {
			for _, field := range required {
				if _, ok := decoded[i][field]; !ok {
					check.MissingFieldCount++
					issues = append(issues, issue{Severity: "critical", Symbol: symbol, Code: "missing_json_field", Detail: field})
				}
			}
		}
		if sig.Symbol == "" || sig.Direction == "" || sig.SetupType == "" || sig.Status == "" {
			check.MissingFieldCount++
			issues = append(issues, issue{Severity: "critical", Symbol: symbol, Code: "empty_identity_field", Detail: "symbol/direction/setup_type/status must be non-empty"})
		}
		if len(sig.ReasonCodes) == 0 {
			check.ExecutableGapCount++
			issues = append(issues, issue{Severity: "high", Symbol: symbol, Code: "empty_reason_codes", Detail: "AI cannot understand why this setup was selected"})
		}
		if len(sig.RequiredConfirms) == 0 && needsConfirmations(sig) {
			check.ExecutableGapCount++
			issues = append(issues, issue{Severity: "high", Symbol: symbol, Code: "missing_required_confirmations", Detail: "wait/conflict signal must tell AI what confirmation to wait for"})
		}
		if sig.EntryMode == "" {
			check.ExecutableGapCount++
			issues = append(issues, issue{Severity: "high", Symbol: symbol, Code: "missing_entry_mode", Detail: "AI cannot distinguish immediate entry vs wait-confirm"})
		}
		if sig.Status == local.V7StatusConflictWatch {
			continue
		}
		if sig.Invalidation.Price <= 0 {
			check.ExecutableGapCount++
			issues = append(issues, issue{Severity: "high", Symbol: symbol, Code: "missing_invalidation", Detail: "signal lacks a concrete invalidation price"})
		}
		if len(sig.Targets) == 0 {
			check.ExecutableGapCount++
			issues = append(issues, issue{Severity: "medium", Symbol: symbol, Code: "missing_targets", Detail: "signal lacks take-profit target context"})
		}
		if sig.PriceCtx == nil {
			check.ExecutableGapCount++
			issues = append(issues, issue{Severity: "medium", Symbol: symbol, Code: "missing_price_context", Detail: "AI loses price/ATR/change context"})
		}
		if sig.DerivativesCtx == nil {
			check.ExecutableGapCount++
			issues = append(issues, issue{Severity: "medium", Symbol: symbol, Code: "missing_derivatives_context", Detail: "AI loses OI/funding/LSR/taker context"})
		}
	}

	return check, issues
}

func needsConfirmations(sig local.V7SignalOutput) bool {
	if sig.Status == local.V7StatusWaitConfirm || sig.Status == local.V7StatusConflictWatch {
		return true
	}
	switch sig.EntryMode {
	case local.V7EntryImmediate:
		return false
	default:
		return sig.EntryMode != ""
	}
}

func validatePrompt(prompt string, candidateCount int) aiRecognitionCheck {
	promptTierCounts := parsePromptTierCounts(prompt)
	return aiRecognitionCheck{
		PromptContainsV7JSON:      strings.Contains(prompt, "hunter_v7_signal_json: {"),
		PromptContainsSetupType:   strings.Contains(prompt, "\"setup_type\""),
		PromptContainsEntryMode:   strings.Contains(prompt, "\"entry_mode\""),
		PromptContainsRiskLevel:   strings.Contains(prompt, "\"risk_level\""),
		PromptContainsConfirms:    strings.Contains(prompt, "\"required_confirmations\""),
		PromptContainsInvalid:     strings.Contains(prompt, "\"invalidation\""),
		PromptContainsTierSummary: len(promptTierCounts) > 0,
		PromptTierCounts:          promptTierCounts,
		PromptCandidateCoinCount:  candidateCount,
		PromptBytes:               len([]byte(prompt)),
	}
}

func parsePromptTierCounts(prompt string) map[string]int {
	const prefix = "Tier Summary:"
	for _, line := range strings.Split(prompt, "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, prefix) {
			continue
		}
		_, summary, ok := strings.Cut(line, prefix)
		if !ok {
			continue
		}
		counts := map[string]int{}
		for _, part := range strings.Split(summary, "|") {
			key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
			if !ok {
				continue
			}
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				continue
			}
			key = strings.TrimSpace(key)
			if key != "" {
				counts[key] = n
			}
		}
		return counts
	}
	return nil
}

func validateCoverage(signals []local.V7SignalOutput, geometry kernel.HunterV7ExecutionGeometry) opportunityCoverCheck {
	c := opportunityCoverCheck{
		SignalCount:     len(signals),
		BySetupType:     map[string]int{},
		ByStatus:        map[string]int{},
		ByRiskLevel:     map[string]int{},
		ByEntryMode:     map[string]int{},
		ByExecutionTier: map[string]int{},
		ByMarketRegime:  map[string]int{},
	}
	for _, sig := range signals {
		if sig.Direction == local.V7DirLong {
			c.LongCount++
		} else if sig.Direction == local.V7DirShort {
			c.ShortCount++
		}
		setup := string(sig.SetupType)
		c.BySetupType[setup]++
		c.ByStatus[string(sig.Status)]++
		c.ByRiskLevel[string(sig.RiskLevel)]++
		c.ByEntryMode[string(sig.EntryMode)]++
		cc := kernel.AssembleHunterV7CandidateCoins([]local.V7SignalOutput{sig}, "BOTH", geometry)
		if len(cc) > 0 {
			tier := cc[0].V7ExecutionTier
			if tier == "" {
				tier = "UNKNOWN"
			}
			c.ByExecutionTier[tier]++
		}
		c.ByMarketRegime[string(sig.MarketRegime)]++
		switch sig.SetupType {
		case local.V7SetupLeaderMomentumLong, local.V7SetupTrendBreakoutLong, local.V7SetupDisplacementLong, local.V7SetupIntradayScalp:
			c.HasMomentum = true
		case local.V7SetupPullbackLong, local.V7SetupPanicReversalLong:
			c.HasReversal = true
		case local.V7SetupShortSqueezeLong, local.V7SetupLongSqueezeShort:
			c.HasSqueeze = true
		case local.V7SetupRangeReversion:
			c.HasRange = true
			c.HasReversal = true
		case local.V7SetupRangeExpansion:
			c.HasRange = true
			c.HasMomentum = true
		case local.V7SetupFundingReversal:
			c.HasFunding = true
		case local.V7SetupAccumulationLong:
			c.HasAccumulation = true
		case local.V7SetupDistributionShort:
			c.HasDistribution = true
		}
	}
	c.DistinctSetups = len(c.BySetupType)
	return c
}

func promptIssues(c aiRecognitionCheck) []issue {
	var issues []issue
	openReviewCount := c.PromptTierCounts["EXECUTABLE"] + c.PromptTierCounts["REVIEWABLE"]
	if openReviewCount > 0 {
		if !c.PromptContainsV7JSON {
			issues = append(issues, issue{Severity: "critical", Code: "prompt_missing_v7_json", Detail: "AIT prompt does not contain hunter_v7_signal_json for open-review candidates"})
		}
		if !c.PromptContainsSetupType || !c.PromptContainsEntryMode || !c.PromptContainsRiskLevel {
			issues = append(issues, issue{Severity: "high", Code: "prompt_missing_core_v7_tags", Detail: "AIT prompt lacks one or more core v7 fields for open-review candidates"})
		}
		if !c.PromptContainsConfirms || !c.PromptContainsInvalid {
			issues = append(issues, issue{Severity: "high", Code: "prompt_missing_execution_fields", Detail: "AIT prompt lacks confirmations or invalidation fields for open-review candidates"})
		}
	}
	if !c.PromptContainsTierSummary {
		issues = append(issues, issue{Severity: "medium", Code: "prompt_missing_tier_summary", Detail: "AIT prompt lacks final execution tier summary; validator cannot compare prompt-final tier distribution"})
	}
	return issues
}

func coverageIssues(c opportunityCoverCheck) []issue {
	var issues []issue
	if c.SignalCount == 0 {
		return []issue{{Severity: "critical", Code: "no_v7_signals", Detail: "v7 produced no real-time signals under current config"}}
	}
	if c.LongCount == 0 || c.ShortCount == 0 {
		issues = append(issues, issue{Severity: "medium", Code: "single_direction_output", Detail: fmt.Sprintf("LONG=%d SHORT=%d; may be normal in one-sided markets but limits opportunity diversity", c.LongCount, c.ShortCount)})
	}
	if c.DistinctSetups < 2 && c.SignalCount >= 3 {
		issues = append(issues, issue{Severity: "medium", Code: "single_setup_concentration", Detail: "signals concentrate in one setup type; verify router is not over-filtering other patterns"})
	}
	if !c.HasReversal && !c.HasMomentum && !c.HasSqueeze && !c.HasFunding && !c.HasAccumulation && !c.HasDistribution {
		issues = append(issues, issue{Severity: "high", Code: "no_known_opportunity_family", Detail: "signals do not map to expected v7 opportunity families"})
	}
	return issues
}

func snapshotIssues(s snapshotSummary) []issue {
	var issues []issue
	if s.SymbolCount <= 0 {
		return []issue{{Severity: "critical", Code: "snapshot_empty", Detail: "Binance snapshot returned no symbols"}}
	}
	restRate := float64(s.RestErrors) / float64(s.SymbolCount)
	if s.RestErrors > 0 {
		severity := "medium"
		if s.RestErrors >= 50 || restRate >= 0.20 {
			severity = "high"
		}
		issues = append(issues, issue{
			Severity: severity,
			Code:     "binance_rest_partial_coverage",
			Detail: fmt.Sprintf("REST errors=%d across %d symbols (%.1f%%); some OI/LSR/kline detail may be missing and universe coverage can shrink",
				s.RestErrors, s.SymbolCount, restRate*100),
		})
	}
	coverage := float64(s.UniverseCount) / float64(s.SymbolCount)
	if s.UniverseCount > 0 && coverage < 0.30 {
		issues = append(issues, issue{
			Severity: "medium",
			Code:     "universe_coverage_low",
			Detail: fmt.Sprintf("Hunter v7 universe=%d/%d (%.1f%%); validate REST stability and top-detail breadth before judging no-opportunity cycles",
				s.UniverseCount, s.SymbolCount, coverage*100),
		})
	}
	return issues
}

func priceChange24h(snap *datafetch.Snapshot, symbol string) float64 {
	if snap == nil || snap.Symbols == nil || snap.Symbols[symbol] == nil {
		return 0
	}
	return snap.Symbols[symbol].PriceChange24h
}

func formatMarkdown(r validationReport, rawPath string) string {
	var sb strings.Builder
	sb.WriteString("# Hunter v7 实时信号 JSON 与 AIT 识别验证报告\n\n")
	sb.WriteString(fmt.Sprintf("> 生成时间：%s\n", r.GeneratedAt))
	sb.WriteString(fmt.Sprintf("> 原始 JSON：`%s`\n", rawPath))
	sb.WriteString(fmt.Sprintf("> Prompt 预览：`%s`\n\n", r.PromptPreviewPath))

	sb.WriteString("## 1. 结论\n\n")
	if r.FormatCheck.JSONMarshalOK && r.FormatCheck.JSONUnmarshalOK && r.FormatCheck.MissingFieldCount == 0 && !hasCriticalOrHighIssues(r.Issues) {
		if r.AIRecognition.PromptTierCounts["EXECUTABLE"]+r.AIRecognition.PromptTierCounts["REVIEWABLE"] > 0 {
			sb.WriteString("- JSON 结构可序列化/反序列化，核心字段完整，AIT prompt 已包含 open-review 候选的 `hunter_v7_signal_json`。\n")
		} else {
			sb.WriteString("- JSON 结构可序列化/反序列化，核心字段完整；本轮无 EXECUTABLE/REVIEWABLE，因此 prompt 不展开 `hunter_v7_signal_json` 属于正常 wait 链路。\n")
		}
	} else {
		sb.WriteString("- JSON 或 prompt 识别链路存在问题，需优先修复 critical/high 问题。\n")
	}
	sb.WriteString(fmt.Sprintf("- 本轮实时输出 %d 个信号：LONG=%d，SHORT=%d，setup=%d 类。\n",
		r.OpportunityCover.SignalCount, r.OpportunityCover.LongCount, r.OpportunityCover.ShortCount, r.OpportunityCover.DistinctSetups))
	sb.WriteString(fmt.Sprintf("- 市场 regime=%s，BTC 24h=%+.2f%%，ETH 24h=%+.2f%%。\n\n",
		r.Snapshot.Regime, r.Snapshot.BTC24h, r.Snapshot.ETH24h))

	sb.WriteString("## 2. JSON / Prompt 校验\n\n")
	sb.WriteString(fmt.Sprintf("| 项目 | 结果 |\n|---|---|\n| JSON marshal | %v |\n| JSON unmarshal | %v |\n| 缺字段数 | %d |\n| 执行性缺口 | %d |\n| Prompt 含 v7 JSON | %v |\n| Prompt bytes | %d |\n\n",
		r.FormatCheck.JSONMarshalOK, r.FormatCheck.JSONUnmarshalOK, r.FormatCheck.MissingFieldCount,
		r.FormatCheck.ExecutableGapCount, r.AIRecognition.PromptContainsV7JSON, r.AIRecognition.PromptBytes))

	restRate := 0.0
	universeCoverage := 0.0
	if r.Snapshot.SymbolCount > 0 {
		restRate = float64(r.Snapshot.RestErrors) / float64(r.Snapshot.SymbolCount) * 100
		universeCoverage = float64(r.Snapshot.UniverseCount) / float64(r.Snapshot.SymbolCount) * 100
	}
	sb.WriteString("## 3. 实时数据覆盖质量\n\n")
	sb.WriteString("| 项目 | 结果 |\n|---|---:|\n")
	sb.WriteString(fmt.Sprintf("| Binance symbols | %d |\n", r.Snapshot.SymbolCount))
	sb.WriteString(fmt.Sprintf("| Hunter v7 universe | %d |\n", r.Snapshot.UniverseCount))
	sb.WriteString(fmt.Sprintf("| Universe coverage | %.1f%% |\n", universeCoverage))
	sb.WriteString(fmt.Sprintf("| REST errors | %d |\n", r.Snapshot.RestErrors))
	sb.WriteString(fmt.Sprintf("| REST error rate | %.1f%% |\n", restRate))
	sb.WriteString(fmt.Sprintf("| Fetch ms | %d |\n\n", r.Snapshot.FetchMs))

	sb.WriteString("## 4. 实时信号\n\n")
	sb.WriteString("| # | Symbol | Dir | Setup | Status | Priority | Timing | Risk | Entry | Reasons |\n")
	sb.WriteString("|---:|---|---|---|---|---:|---:|---:|---|---|\n")
	for i, sig := range r.Signals {
		if i >= 30 {
			break
		}
		sb.WriteString(fmt.Sprintf("| %d | %s | %s | `%s` | `%s` | %.1f | %.1f | %.1f | `%s` | %s |\n",
			i+1, sig.Symbol, sig.Direction, sig.SetupType, sig.Status, sig.AIPriority,
			sig.TimingScore, sig.RiskScore, sig.EntryMode, strings.Join(sig.ReasonCodes, ", ")))
	}
	sb.WriteString("\n")

	sb.WriteString("## 5. 机会覆盖\n\n")
	sb.WriteString(fmt.Sprintf("- setup 分布：%s\n", sortedMap(r.OpportunityCover.BySetupType)))
	sb.WriteString(fmt.Sprintf("- status 分布：%s\n", sortedMap(r.OpportunityCover.ByStatus)))
	sb.WriteString(fmt.Sprintf("- entry_mode 分布：%s\n", sortedMap(r.OpportunityCover.ByEntryMode)))
	sb.WriteString(fmt.Sprintf("- runtime tier 分布（后端初筛）：%s\n", sortedMap(r.OpportunityCover.ByExecutionTier)))
	sb.WriteString(fmt.Sprintf("- prompt-final tier 分布（AIT 最终可执行口径）：%s\n", sortedMap(r.AIRecognition.PromptTierCounts)))
	sb.WriteString(fmt.Sprintf("- 潜力池：top=%d，未命中模块=%d\n", r.OpportunityCover.PotentialPoolCount, r.OpportunityCover.UnmatchedPotentialCount))
	sb.WriteString(fmt.Sprintf("- 覆盖家族：momentum=%v, reversal=%v, squeeze=%v, range=%v, funding=%v, accumulation=%v, distribution=%v\n\n",
		r.OpportunityCover.HasMomentum, r.OpportunityCover.HasReversal, r.OpportunityCover.HasSqueeze,
		r.OpportunityCover.HasRange, r.OpportunityCover.HasFunding, r.OpportunityCover.HasAccumulation, r.OpportunityCover.HasDistribution))

	sb.WriteString("## 6. 潜力池强制跟踪\n\n")
	if len(r.PotentialPool) == 0 {
		sb.WriteString("- 本轮未生成潜力池候选。\n\n")
	} else {
		sb.WriteString("| # | Symbol | Dir | Potential | Amp24h | Vel5m | Vel15m | VolBurst | OI1h | RS4h | ModuleHit | Setups |\n")
		sb.WriteString("|---:|---|---|---:|---:|---:|---:|---:|---:|---:|---|---|\n")
		for i, p := range r.PotentialPool {
			if i >= 20 {
				break
			}
			burst := p.VolumeBurst5m
			if p.VolumeBurst15m > burst {
				burst = p.VolumeBurst15m
			}
			sb.WriteString(fmt.Sprintf("| %d | %s | %s | %.1f | %.1f%% | %+.2f%% | %+.2f%% | %.2fx | %+.2f%% | %+.2f%% | %v | %s |\n",
				i+1, p.Symbol, p.Direction, p.OpportunityPotentialScore, p.Amplitude24h,
				p.Velocity5m, p.Velocity15m, burst, p.OIDelta1h, p.RelativeStrength4h,
				p.MatchedModule, formatSetupSlice(p.MatchedSetups)))
		}
		sb.WriteString("\n")
		sb.WriteString("- 跟踪口径：潜力池用于 30m/60m MFE、MAE、后续模块命中审计；未命中模块的高分标的不计入真实开仓胜率。\n\n")
	}

	sb.WriteString("## 7. 问题清单\n\n")
	if len(r.Issues) == 0 {
		sb.WriteString("- 未发现格式或识别阻断问题。\n")
	} else {
		sb.WriteString("| Severity | Symbol | Code | Detail |\n|---|---|---|---|\n")
		for _, is := range r.Issues {
			sb.WriteString(fmt.Sprintf("| %s | %s | `%s` | %s |\n", is.Severity, is.Symbol, is.Code, is.Detail))
		}
	}
	return sb.String()
}

func hasCriticalOrHighIssues(issues []issue) bool {
	for _, is := range issues {
		if is.Severity == "critical" || is.Severity == "high" {
			return true
		}
	}
	return false
}

func sortedMap(m map[string]int) string {
	if len(m) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, m[k]))
	}
	return strings.Join(parts, ", ")
}

func formatSetupSlice(values []local.V7SetupType) string {
	if len(values) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, string(v))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

func printConsoleSummary(r validationReport, rawPath, mdPath string) {
	fmt.Printf("Hunter v7 validation complete\n")
	fmt.Printf("snapshot: symbols=%d universe=%d regime=%s BTC24h=%+.2f%% ETH24h=%+.2f%% rest_errors=%d\n",
		r.Snapshot.SymbolCount, r.Snapshot.UniverseCount, r.Snapshot.Regime, r.Snapshot.BTC24h, r.Snapshot.ETH24h, r.Snapshot.RestErrors)
	fmt.Printf("signals: total=%d long=%d short=%d setups=%s\n",
		r.OpportunityCover.SignalCount, r.OpportunityCover.LongCount, r.OpportunityCover.ShortCount, sortedMap(r.OpportunityCover.BySetupType))
	fmt.Printf("potential_pool: top=%d unmatched=%d\n", r.OpportunityCover.PotentialPoolCount, r.OpportunityCover.UnmatchedPotentialCount)
	fmt.Printf("runtime tiers: %s\n", sortedMap(r.OpportunityCover.ByExecutionTier))
	fmt.Printf("prompt-final tiers: %s\n", sortedMap(r.AIRecognition.PromptTierCounts))
	fmt.Printf("format: json=%v/%v missing=%d executable_gaps=%d prompt_v7_json=%v\n",
		r.FormatCheck.JSONMarshalOK, r.FormatCheck.JSONUnmarshalOK, r.FormatCheck.MissingFieldCount,
		r.FormatCheck.ExecutableGapCount, r.AIRecognition.PromptContainsV7JSON)
	fmt.Printf("issues: %d\n", len(r.Issues))
	fmt.Printf("raw: %s\nreport: %s\nprompt: %s\n", rawPath, mdPath, r.PromptPreviewPath)
}
