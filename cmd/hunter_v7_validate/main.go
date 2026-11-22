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
	"path/filepath"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type validationReport struct {
	GeneratedAt       string                       `json:"generated_at"`
	HarnessGitSHA     string                       `json:"harness_git_sha"`
	HarnessDirty      bool                         `json:"harness_dirty"`
	Accepted          bool                         `json:"accepted"`                  // TA-2: passed the contract and was persisted
	RejectedReason    string                       `json:"rejected_reason,omitempty"` // TA-2: why this attempt is in rejected/
	Round             int                          `json:"round"`
	Snapshot          snapshotSummary              `json:"snapshot"`
	Config            local.V7Config               `json:"config"`
	FormatCheck       formatCheck                  `json:"format_check"`
	AIRecognition     aiRecognitionCheck           `json:"ai_recognition"`
	OpportunityCover  opportunityCoverCheck        `json:"opportunity_cover"`
	Signals           []local.V7SignalOutput       `json:"signals"`
	PotentialPool     []local.V7PotentialCandidate `json:"potential_pool"`
	PromptPreviewPath string                       `json:"prompt_preview_path"`
	PromptReprice     repriceSummary               `json:"prompt_reprice"` // fix 04
	TierLayers        tierLayerReport              `json:"tier_layers"`    // fix 12: assembled vs prompt-final tier
	Issues            []issue                      `json:"issues"`
}

type snapshotSummary struct {
	SymbolCount   int   `json:"symbol_count"`
	UniverseCount int   `json:"universe_count"`
	FetchMs       int64 `json:"fetch_ms"` // TA-5: snapshot fetch only (snap.Meta.FetchDuration); was the whole round
	RoundMs       int64 `json:"round_ms"` // TA-5: strategy load + fetch + scoring + assembly + prompt build
	// TA-5 fetch observability (datafetch.SnapshotMeta).
	DetailRequested       int    `json:"detail_requested"`
	DetailCompleted       int    `json:"detail_completed"`
	DetailSkippedDeadline int    `json:"detail_skipped_deadline"`
	FailedRequests        int    `json:"failed_requests"`
	FetchErrorLog         string `json:"fetch_error_log,omitempty"` // TA-6: file with this round's failed requests (path+query)
	// Fix 02: per-endpoint latency (p50/p95) of this round's snapshot fetch.
	EndpointLatency map[string]datafetch.EndpointLatency `json:"endpoint_latency,omitempty"`
	// Fix 05: two-tier fetch (all symbols; slow tier 1/N per round).
	TwoTier            bool     `json:"two_tier"`
	SlowEvery          int      `json:"slow_every,omitempty"`
	FetchIndex         int      `json:"fetch_index,omitempty"`
	T2InputsTopN       int      `json:"t2_inputs_top_n,omitempty"`
	SlowRefreshed      int      `json:"slow_refreshed,omitempty"`
	SlowCarried        int      `json:"slow_carried,omitempty"`
	SlowMissing        int      `json:"slow_missing,omitempty"`
	SlowMaxAgeSec      int64    `json:"slow_max_age_sec,omitempty"`
	RestErrors         int      `json:"rest_errors"`
	RestErrorRate      float64  `json:"rest_error_rate"`
	UniverseCoverage   float64  `json:"universe_coverage"`
	Degraded           bool     `json:"degraded"`
	DegradationReasons []string `json:"degradation_reasons,omitempty"`
	BTC24h             float64  `json:"btc_24h"`
	ETH24h             float64  `json:"eth_24h"`
	Regime             string   `json:"regime"`
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
	topDetail         int
	maxWorkers        int
	maxOutput         int
	watchOutput       int
	minPriority       float64
	aggressive        bool
	strategyID        string
	dbPath            string
	outDir            string
	rounds            int
	roundInterval     time.Duration
	dumpUniverse      string
	persistSignals    bool
	trackOutcomes     bool
	postTrackDuration time.Duration
	postTrackInterval time.Duration
	trackActiveOnly   bool
	store             *store.Store
	watchState        *local.V7SignalStateManager
	t2Store           *local.V7T2EventStore
	binanceURL        string
	outcomeTracker    *aittrader.SignalOutcomeTracker
	// Fix 09: the tracker's 1m backfill (validationBackfillCandles), used to
	// re-register saved ACTIVE rows on --resume; and what that did.
	backfillCandles func(symbol string, from, to time.Time) []aittrader.TrackedCandle
	resumeTracking  *resumeTrackingSummary
	attempt         int // TA-2: attempt number within the round (artifact names in rejected/)

	fetchErrors *fetchErrorLog // TA-6: failed-request log (<out-dir>/fetch-errors.jsonl)

	// Fix 03: detail-fetch deadline (from fetch start) and truncation gate.
	fetchBudget      time.Duration
	maxDetailSkipPct float64

	// Fix 04: pre-prompt repricing.
	promptReprice     bool
	promptPriceMaxAge time.Duration

	// Fix 05: two-tier fetch. The round fetcher is kept across rounds and
	// attempts (shared pointer: attempt copies of opts reuse it) so its
	// slow-tier data carries over; it is not saved in the state file, so a
	// restart warms up again.
	twoTier                 bool
	slowEvery               int
	weightLimitPerMin       float64
	futuresDataLimitPer5Min int
	roundFetcher            *roundFetcherCache

	// TA-3: run state across restarts.
	runID                string
	stateFile            string
	resume               bool
	resumeAllowSHAChange bool
}

type validationOutcomeSummary struct {
	GeneratedAt        string                          `json:"generated_at"`
	HarnessGitSHA      string                          `json:"harness_git_sha"`
	HarnessDirty       bool                            `json:"harness_dirty"`
	PostTrackDuration  string                          `json:"post_track_duration"`
	PostTrackInterval  string                          `json:"post_track_interval"`
	TrackActiveOnly    bool                            `json:"track_active_only"`
	ActiveCount        int                             `json:"active_count"`
	TrackedCount       int                             `json:"tracked_count"`
	Status             []validationStatusOutcome       `json:"status"`
	Setup              []validationSetupOutcome        `json:"setup"`
	CompletedStats     map[string]aittrader.SetupStats `json:"completed_stats_by_setup"`
	Details            []validationOutcomeDetail       `json:"details,omitempty"`
	TP0PartialCount    int                             `json:"tp0_partial_count"`
	RunnerTP1PlusCount int                             `json:"runner_tp1_plus_count"`
	MFE1LossStopCount  int                             `json:"mfe_gt_1_loss_stop_count"`
}

// validationOutcomeDetail preserves the per-signal evidence needed to audit a
// setup change. Aggregates alone cannot distinguish a good entry with a tight
// runner from a bad entry that happened to receive a protected exit.
type validationOutcomeDetail struct {
	Symbol       string  `json:"symbol"`
	Direction    string  `json:"direction"`
	SetupType    string  `json:"setup_type"`
	Tier         string  `json:"tier"` // fix 12: the assembled tier (what the tracker registered)
	Status       string  `json:"status"`
	EntryPrice   float64 `json:"entry_price"`
	ExitPrice    float64 `json:"exit_price,omitempty"`
	StopPrice    float64 `json:"stop_price,omitempty"`
	TP0Price     float64 `json:"tp0_price,omitempty"`
	TP1Price     float64 `json:"tp1_price,omitempty"`
	TP2Price     float64 `json:"tp2_price,omitempty"`
	PnLPct       float64 `json:"pnl_pct"`
	MFE          float64 `json:"mfe_pct"`
	MAE          float64 `json:"mae_pct"`
	TP0Done      bool    `json:"tp0_done,omitempty"`
	TP1Done      bool    `json:"tp1_done,omitempty"`
	TP2Done      bool    `json:"tp2_done,omitempty"`
	RemainingPct float64 `json:"remaining_pct,omitempty"`
	RealizedPnL  float64 `json:"realized_pnl_pct,omitempty"`

	// Fix 12: the prompt-final tier, from the DB row ("" = not shown in the
	// prompt / not recorded); recordID is the row id used for the lookup.
	PromptTier string `json:"prompt_final_tier,omitempty"`
	recordID   int64
}

type validationRunSummary struct {
	GeneratedAt          string                   `json:"generated_at"`
	HarnessGitSHA        string                   `json:"harness_git_sha"`
	HarnessDirty         bool                     `json:"harness_dirty"`
	Rounds               int                      `json:"rounds"`
	ValidRounds          int                      `json:"valid_rounds"`
	SignalCount          int                      `json:"signal_count"`
	OpenReviewCount      int                      `json:"open_review_count"`
	OpenReviewRate       float64                  `json:"open_review_rate"`
	ValidSignalCount     int                      `json:"valid_signal_count"`
	ValidOpenReviewCount int                      `json:"valid_open_review_count"`
	ValidOpenReviewRate  float64                  `json:"valid_open_review_rate"`
	DegradedRounds       []int                    `json:"degraded_rounds,omitempty"`
	Round                []validationRoundSummary `json:"round"`
	RejectedRounds       []rejectedRound          `json:"rejected_rounds,omitempty"` // TA-2: contract-rejected (skipped) rounds
	ResumeTracking       *resumeTrackingSummary   `json:"resume_tracking,omitempty"` // fix 09: saved ACTIVE rows settled on --resume
	ExitCode             int                      `json:"exit_code"`
	ExitReason           string                   `json:"exit_reason,omitempty"`
}

type validationRoundSummary struct {
	Round          int      `json:"round"`
	GeneratedAt    string   `json:"generated_at"`
	Regime         string   `json:"regime"`
	SignalCount    int      `json:"signal_count"`
	OpenReview     int      `json:"open_review"`
	OpenReviewRate float64  `json:"open_review_rate"`
	RestErrors     int      `json:"rest_errors"`
	RestErrorRate  float64  `json:"rest_error_rate"`
	UniverseCount  int      `json:"universe_count"`
	Degraded       bool     `json:"degraded"`
	Reasons        []string `json:"reasons,omitempty"`
}

type validationStatusOutcome struct {
	Status      string  `json:"status"`
	Count       int     `json:"count"`
	AvgPnLPct   float64 `json:"avg_pnl_pct"`
	AvgMFE      float64 `json:"avg_mfe"`
	AvgMAE      float64 `json:"avg_mae"`
	ProfitCount int     `json:"profit_count"`
	LossCount   int     `json:"loss_count"`
	FlatCount   int     `json:"flat_count"`
}

type validationSetupOutcome struct {
	SetupType      string  `json:"setup_type"`
	Count          int     `json:"count"`
	Active         int     `json:"active"`
	Wins           int     `json:"wins"`
	LossStops      int     `json:"loss_stops"`
	ProtectedStops int     `json:"protected_stops"`
	Timeouts       int     `json:"timeouts"`
	ActiveProfit   int     `json:"active_profit"`
	ActiveLoss     int     `json:"active_loss"`
	AvgPnLPct      float64 `json:"avg_pnl_pct"`
	AvgMFE         float64 `json:"avg_mfe"`
	AvgMAE         float64 `json:"avg_mae"`
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
	postTrackDuration := flag.Duration("post-track-duration", 0, "continue tracking ACTIVE validation outcomes after all rounds; 0 disables post tracking")
	postTrackInterval := flag.Duration("post-track-interval", 30*time.Second, "post-round outcome tracking tick interval")
	trackActiveOnly := flag.Bool("track-active-only", true, "stop post tracking early when no ACTIVE validation outcomes remain")
	binanceURL := flag.String("binance-url", "", "Binance futures REST base URL (default https://fapi.binance.com); point at the read-only relay for sandbox validation")
	fetchBudget := flag.Duration("fetch-budget", 3*time.Minute, "fix 03: detail-fetch deadline, measured from the start of the snapshot fetch")
	promptReprice := flag.Bool("prompt-reprice", true, "fix 04: reprice prompt symbols with one bulk ticker call right before the prompt is built")
	promptPriceMaxAge := flag.Duration("prompt-price-max-age", defaultPromptPriceMaxAge, "fix 04: staleness cap for prompt prices; older prices are reported as prompt_price_stale")
	twoTier := flag.Bool("two-tier", true, "fix 05: two-tier fetch of every symbol (fast tier every round, slow tier 1/N of symbols per round); false = the legacy top-detail fetch")
	slowEvery := flag.Int("slow-every", defaultSlowEvery, "fix 05: N, refresh each symbol's slow tier (1h OI history, long/short ratio, 1h/4h/1d klines) once every N rounds; slow fields are up to about N rounds old")
	weightLimitPerMin := flag.Float64("weight-limit-per-min", defaultWeightLimitPerMin, "fix 05 (with --two-tier): Binance request-weight limit per minute for the round fetcher (Binance's IP limit is 2400)")
	futuresDataLimit := flag.Int("futures-data-limit-per-5min", defaultFuturesDataLimitPer5Min, "fix 05 (with --two-tier): request limit per 5 minutes for /futures/data endpoints")
	maxDetailSkipPct := flag.Float64("max-detail-skip-pct", defaultMaxDetailSkipPct, "fix 03: fail the round as transient when more than this percent of detail symbols is skipped by the fetch deadline (>= 100 disables)")
	breakerConsecutive := flag.Int("breaker-consecutive", 3, "TA-2: exit 3 after this many consecutive rounds rejected with the same contract signature")
	runID := flag.String("run-id", "", "TA-3: run identifier; enables the state file (watch state, T2 store, round counter) saved after every round")
	stateFile := flag.String("state-file", "", "TA-3: state file path (default <out-dir>/state-<run-id>.json)")
	resume := flag.Bool("resume", false, "TA-3: continue the run from its state file (requires --run-id)")
	resumeAllowSHAChange := flag.Bool("resume-allow-sha-change", false, "TA-3: allow --resume with a harness built from a different commit")
	flag.Parse()

	opts := validationOptions{
		topDetail:         *topDetail,
		maxWorkers:        *maxWorkers,
		maxOutput:         *maxOutput,
		watchOutput:       *watchOutput,
		minPriority:       *minPriority,
		aggressive:        *aggressive,
		strategyID:        *strategyID,
		dbPath:            *dbPath,
		outDir:            *outDir,
		dumpUniverse:      *dumpUniverse,
		rounds:            *rounds,
		roundInterval:     *roundInterval,
		persistSignals:    *persistSignals,
		trackOutcomes:     *trackOutcomes,
		postTrackDuration: *postTrackDuration,
		postTrackInterval: *postTrackInterval,
		trackActiveOnly:   *trackActiveOnly,
		binanceURL:        *binanceURL,
		// Cross-round Task-2 trigger memory, created once for the run. TA-3:
		// each attempt scores into a clone, committed only on acceptance,
		// and the committed store is saved with --run-id.
		t2Store:              local.NewV7T2EventStore(),
		watchState:           local.NewV7SignalStateManager(),
		runID:                *runID,
		stateFile:            *stateFile,
		resume:               *resume,
		resumeAllowSHAChange: *resumeAllowSHAChange,
		fetchErrors:          newFetchErrorLog(*outDir),
		fetchBudget:          *fetchBudget,
		maxDetailSkipPct:     *maxDetailSkipPct,
		promptReprice:        *promptReprice,
		promptPriceMaxAge:    *promptPriceMaxAge,

		twoTier:                 *twoTier,
		slowEvery:               *slowEvery,
		weightLimitPerMin:       *weightLimitPerMin,
		futuresDataLimitPer5Min: *futuresDataLimit,
		roundFetcher:            &roundFetcherCache{},
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
		opts.store = st // closed by runHarness (os.Exit skips defers in main)
		if opts.trackOutcomes {
			// TA-7: the fee-net walk uses the strategy's round-trip cost,
			// as the live trader does (trader/auto_trader.go).
			strategyCfg, err := loadStrategyConfig(opts.dbPath, opts.strategyID)
			if err != nil {
				log.Fatalf("load strategy config for outcome tracking failed: %v", err)
			}
			tracker := aittrader.NewSignalOutcomeTracker(&aittrader.TrackerConfig{
				PollInterval:          time.Minute,
				TimeoutDuration:       2 * time.Hour,
				MaxTracked:            500,
				SnapshotLimit:         180,
				ActiveOutcomeInterval: time.Second,
				EnableDynamicStop:     true,
				TrackPartialTargets:   true,
				FeeNetCostPct:         validationFeeNetCostPct(strategyCfg),
			}, nil)
			tracker.SetOutcomeCallback(func(outcome aittrader.TrackedOutcome) {
				if err := st.HunterV7Signal().UpdateTrackOutcome(outcome.RecordID, validationTrackUpdate(outcome)); err != nil {
					log.Printf("WARN update validation V7 outcome failed: %v", err)
				}
			})
			backfillFetcher := datafetch.NewDataFetcher(backfillFetcherConfig(opts))
			opts.backfillCandles = func(symbol string, from, to time.Time) []aittrader.TrackedCandle {
				return validationBackfillCandles(backfillFetcher, symbol, from, to)
			}
			tracker.SetCandleBackfillSource(opts.backfillCandles)
			tracker.SetCandleSource(func(symbol string) *aittrader.TrackedCandle {
				return validationLatestCandle(backfillFetcher, symbol)
			})
			opts.outcomeTracker = tracker
		}
	}
	os.Exit(runHarness(opts, *breakerConsecutive))
}

// Injection points for tests (no network, no real waits).
var (
	runValidationFn = runValidation
	sleepFn         = time.Sleep
)

// runHarness runs the round loop under the TA-2 failure policy and returns
// the process exit code: 0 done, 1 fatal, 2 transient (resumable),
// 3 deterministic contract failure.
func runHarness(opts validationOptions, breakerConsecutive int) int {
	if opts.store != nil {
		defer opts.store.Close()
	}
	first, prior, err := prepareRunState(&opts)
	if err != nil {
		log.Printf("run state: %v", err)
		return exitFatal
	}
	if opts.resume {
		log.Printf("resuming run %q at round %d (cycle %d) of %d", opts.runID, first, first, opts.rounds)
		// Fix 09: settle the rows the tracker had ACTIVE before the restart.
		rt, err := restoreTrackedOutcomes(opts, prior, time.Now(), opts.backfillCandles)
		if err != nil {
			log.Printf("resume tracking: %v", err)
			return exitFatal
		}
		opts.resumeTracking = rt
		log.Print(rt.logLine())
		if first > opts.rounds {
			log.Printf("run %q already completed %d of %d rounds; nothing to do", opts.runID, prior.LastRound, opts.rounds)
			return exitOK
		}
	}
	attempt := func(round, n int) (validationReport, error) {
		// Score into isolated state copies. A rejected attempt must not count
		// as another watch cycle, advance the module dry-run breaker, or
		// create / trigger Task-2 events (TA-3).
		attemptOpts := opts
		attemptOpts.attempt = n
		attemptOpts.watchState = opts.watchState.Clone()
		attemptOpts.t2Store = opts.t2Store.Clone()
		report, err := runValidationFn(attemptOpts, round)
		if err == nil {
			opts.watchState.ReplaceFrom(attemptOpts.watchState)
			opts.t2Store.ReplaceFrom(attemptOpts.t2Store)
		}
		return report, err
	}
	accepted, rejected := prior.AcceptedRounds, prior.RejectedRounds
	onRoundDone := func(round int, res roundResult) error {
		if res.status == roundAccepted {
			accepted++
		} else {
			rejected++
		}
		if opts.runID == "" {
			return nil
		}
		st, err := snapshotHarnessState(opts, round, accepted, rejected)
		if err != nil {
			return fmt.Errorf("snapshot run state: %w", err)
		}
		if err := saveHarnessState(opts.stateFile, st); err != nil {
			return fmt.Errorf("save run state: %w", err)
		}
		return nil
	}
	out := runRoundsWithPolicy(first, opts.rounds, opts.roundInterval, defaultRoundPolicy(), newRunBreaker(breakerConsecutive),
		attempt, sleepFn, onRoundDone)
	if err := writeValidationRunSummary(opts, out); err != nil {
		log.Printf("write run summary failed: %v", err)
		if out.exitCode == exitOK {
			return exitFatal
		}
	}
	if out.exitCode != exitOK {
		log.Printf("harness stopping with exit code %d: %s", out.exitCode, out.exitReason)
		return out.exitCode
	}
	if err := runPostTrackOutcomes(opts); err != nil {
		log.Printf("post-track outcomes failed: %v", err)
		return exitFatal
	}
	if err := writeValidationOutcomeSummary(opts); err != nil {
		log.Printf("write outcome summary failed: %v", err)
		return exitFatal
	}
	return exitOK
}

// validationTrackUpdate maps a tracker outcome to the DB update exactly as
// the live trader's callback does (trader/auto_trader.go), including the U5
// fee-net walk fields (TA-7; they used to be dropped, so fee_net_outcome
// stayed empty on every validation row).
func validationTrackUpdate(outcome aittrader.TrackedOutcome) store.HunterV7SignalTrackUpdate {
	return store.HunterV7SignalTrackUpdate{
		Status:       string(outcome.Status),
		CurrentPrice: outcome.CurrentPrice,
		ExitPrice:    outcome.ExitPrice,
		StopPrice:    outcome.StopUsed,
		PnLPct:       outcome.PnLPct,
		TP0Done:      outcome.TP0Done,
		TP1Done:      outcome.TP1Done,
		TP2Done:      outcome.TP2Done,
		Remaining:    outcome.RemainingRatio,
		RealizedPnL:  outcome.RealizedPnLPct,
		MFE:          outcome.MaxFavorable,
		MAE:          outcome.MaxAdverse,
		ExitTime:     outcome.ExitTime,
		Snapshots:    outcome.SnapshotJSON,

		FeeNetOutcome:   outcome.FeeNetOutcome,
		FeeNetR:         outcome.FeeNetR,
		FeeNetEntry:     outcome.FeeNetEntry,
		DriftAtEntryPct: outcome.DriftAtEntryPct,
	}
}

// validationFeeNetCostPct is the strategy's effective fee-net round-trip
// cost (v7_outcome_tags.fee_net_cost_pct, default when unset), the value
// the live trader gives its tracker.
func validationFeeNetCostPct(cfg *store.StrategyConfig) float64 {
	var tags *store.V7OutcomeTagConfig
	if cfg != nil && cfg.CoinSource.Hunter != nil {
		tags = cfg.CoinSource.Hunter.V7OutcomeTags
	}
	return store.EffectiveV7OutcomeTags(tags).FeeNetCostPct
}

// roundFetcherConfig is the per-round snapshot fetcher's config. TA-0:
// --binance-url must reach this fetcher and the tracker backfill fetcher.
func roundFetcherConfig(opts validationOptions, t2Enabled, includeNonCryptoFutures bool) datafetch.FetcherConfig {
	cfg := datafetch.FetcherConfig{
		TopNForDetail:           opts.topDetail,
		MaxWorkers:              opts.maxWorkers,
		Timeout:                 15 * time.Second,
		FetchBudget:             roundFetchBudget(opts),
		BinanceURL:              opts.binanceURL,
		IncludeT2Inputs:         t2Enabled,
		IncludeNonCryptoFutures: includeNonCryptoFutures,
	}
	if opts.twoTier {
		cfg.Tiered = &datafetch.TieredFetchConfig{SlowEvery: opts.slowEvery, T2InputsTopN: opts.topDetail}
		cfg.WeightLimitPerMin = opts.weightLimitPerMin
		cfg.FuturesDataLimitPer5Min = opts.futuresDataLimitPer5Min
	}
	return cfg
}

// Fix 05 harness defaults (owner-approved plan). N=4 keeps the
// /futures/data load at about 478 requests per round (5m OI for the 220
// T2-input symbols + 1h OI history and long/short ratio for 1/4 of 515), so
// even two rounds' bursts inside one 5-minute window stay under 1,000.
// 1,800 weight/min (+10% burst) stays under Binance's 2,400 IP limit.
const (
	defaultSlowEvery               = 4
	defaultWeightLimitPerMin       = 1800.0
	defaultFuturesDataLimitPer5Min = 1000
)

// roundFetcherCache keeps the round fetcher across rounds (fix 05).
type roundFetcherCache struct {
	mu      sync.Mutex
	key     string
	fetcher *datafetch.DataFetcher
}

// roundFetcher returns the fetcher for this round. In two-tier mode it is
// reused while the config is unchanged (a changed config, e.g. the strategy
// toggling T2, starts a new fetcher and a new warm-up); the error sink is
// set per round and attempt. The legacy mode builds one per round, as before.
func roundFetcher(opts validationOptions, cfg datafetch.FetcherConfig) *datafetch.DataFetcher {
	if cfg.Tiered == nil || opts.roundFetcher == nil {
		return datafetch.NewDataFetcher(cfg)
	}
	c := opts.roundFetcher
	c.mu.Lock()
	defer c.mu.Unlock()
	sink := cfg.ErrorSink
	keyCfg := cfg
	keyCfg.ErrorSink, keyCfg.Tiered = nil, nil // no func or pointer values in the key
	key := fmt.Sprintf("%+v|%+v", keyCfg, *cfg.Tiered)
	if c.fetcher == nil || c.key != key {
		c.fetcher, c.key = datafetch.NewDataFetcher(cfg), key
	}
	c.fetcher.SetErrorSink(sink)
	return c.fetcher
}

// defaultMaxDetailSkipPct is the --max-detail-skip-pct default: owner
// decision after fix 03 (10 -> 25), so the gate rejects heavily truncated
// rounds but accepts today's typical ~18% truncation until the two-tier
// fetch (fix queue item 5) removes it.
const defaultMaxDetailSkipPct = 25.0

// roundFetchBudget is the detail-fetch deadline (fix 03); 3 minutes when
// unset, the previous round-context value.
func roundFetchBudget(opts validationOptions) time.Duration {
	if opts.fetchBudget > 0 {
		return opts.fetchBudget
	}
	return 3 * time.Minute
}

// detailTruncationError rejects a snapshot whose detail fetch lost more than
// maxPct percent of its symbols to the deadline (fix 03). The harness
// reports it as transient, so the round is retried rather than accepted
// with silently partial data. maxPct >= 100 disables the gate.
func detailTruncationError(m datafetch.SnapshotMeta, maxPct float64) error {
	if maxPct >= 100 || m.DetailRequested <= 0 || m.DetailSkippedDeadline <= 0 {
		return nil
	}
	pct := float64(m.DetailSkippedDeadline) / float64(m.DetailRequested) * 100
	if pct <= maxPct {
		return nil
	}
	return fmt.Errorf("snapshot quality rejected: detail fetch truncated by deadline: %d of %d detail symbols skipped (%.1f%% > %.1f%%)",
		m.DetailSkippedDeadline, m.DetailRequested, pct, maxPct)
}

// backfillFetcherConfig is the outcome tracker's 1m backfill/latest-candle
// fetcher config (same --binance-url as the round fetcher).
func backfillFetcherConfig(opts validationOptions) datafetch.FetcherConfig {
	return datafetch.FetcherConfig{
		BinanceURL: opts.binanceURL,
		ErrorSink:  opts.fetchErrors.sink(0, 0, fetchSourceBackfill), // TA-6
		Timeout:    8 * time.Second,
		KlineIntervals: []datafetch.KlineInterval{
			{Interval: "1m", Limit: 120},
		},
	}
}

// harnessBuildInfo returns the VCS revision and dirty flag embedded by
// "go build" (TA-0). Binaries built without VCS stamping (go test,
// -buildvcs=false, no .git) report "unknown" and false.
func harnessBuildInfo() (sha string, dirty bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown", false
	}
	return buildInfoVCS(info.Settings)
}

func buildInfoVCS(settings []debug.BuildSetting) (sha string, dirty bool) {
	sha = "unknown"
	for _, s := range settings {
		switch s.Key {
		case "vcs.revision":
			if s.Value != "" {
				sha = s.Value
			}
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	return sha, dirty
}

// t2LifecycleParams mirrors kernel hunterV7T2Lifecycle (unexported): a
// straight field copy; nil keeps every scanner constant.
func t2LifecycleParams(c *store.V7T2LifecycleConfig) local.V7T2LifecycleParams {
	if c == nil {
		return local.V7T2LifecycleParams{}
	}
	return local.V7T2LifecycleParams{
		MinAlertScore:           c.MinAlertScore,
		MinWatchScore:           c.MinWatchScore,
		ArmedExpiryHours:        c.ArmedExpiryHours,
		WatchExpiryHours:        c.WatchExpiryHours,
		CooldownMinutes:         c.CooldownMinutes,
		ObserveOnlyOverride:     c.ObserveOnlyOverride,
		ObserveOnlySetups:       append([]string(nil), c.ObserveOnlySetups...),
		DisableTrigger:          c.DisableTrigger,
		TriggeredSurfaceMinutes: c.TriggeredSurfaceMinutes,
	}
}

func runValidation(opts validationOptions, round int) (validationReport, error) {
	var report validationReport
	start := time.Now()

	strategyCfg, err := loadStrategyConfig(opts.dbPath, opts.strategyID)
	if err != nil {
		return report, fatalError(fmt.Errorf("load strategy config failed: %w", err))
	}
	hunterCfg := (*store.HunterConfig)(nil)
	if strategyCfg != nil {
		hunterCfg = strategyCfg.CoinSource.Hunter
	}
	// Task-2 module (M3): the harness enables it exactly when the
	// strategy does. Its depth + 5m OI inputs must be fetched or the
	// detector reads them as unavailable (fail-closed) and emits nothing.
	t2Enabled := hunterCfg != nil && hunterCfg.V7T2Enabled
	includeNonCryptoFutures := false
	if strategyCfg != nil && strategyCfg.CoinSource.Hunter != nil {
		includeNonCryptoFutures = strategyCfg.CoinSource.Hunter.IncludeNonCryptoFutures
	}

	fcfg := roundFetcherConfig(opts, t2Enabled, includeNonCryptoFutures)
	fcfg.ErrorSink = opts.fetchErrors.sink(round, opts.attempt, fetchSourceRound) // TA-6
	fetcher := roundFetcher(opts, fcfg)
	// Fix 03: the deadline is the fetcher's FetchBudget, which starts with
	// the fetch itself (it used to start at round entry, before strategy load).
	snap, err := fetcher.Fetch(context.Background())
	if err != nil {
		return report, transientError(fmt.Errorf("fetch snapshot failed: %w", err))
	}
	if err := detailTruncationError(snap.Meta, opts.maxDetailSkipPct); err != nil {
		return report, transientError(err)
	}
	// Gate transport quality before scoring. In particular, do not turn a
	// failed retry into a synthetic watch/breaker cycle or persisted outcome.
	preflight := annotateSnapshotQuality(snapshotSummary{
		SymbolCount: snap.Meta.SymbolCount,
		RestErrors:  snap.Meta.RestErrors,
	})
	if err := snapshotAcceptanceError(preflight); err != nil {
		return report, transientError(err)
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

	if t2Enabled {
		// Mirror kernel hunterV7T2ModuleConfig (unexported): zero
		// lifecycle keeps the scanner constants (ARMED 82 / WATCH 72,
		// 4h / 2h expiry, 60 min cooldown, trigger on, 15 min surface).
		cfg.T2 = &local.V7T2Config{
			Enabled:            true,
			EnabledSetups:      hunterCfg.V7T2Setups,
			SymbolValidityGate: hunterCfg.V7T2SymbolValidityGate,
			SetupPrefix:        hunterCfg.V7T2SetupPrefix,
			Lifecycle:          t2LifecycleParams(hunterCfg.V7T2Lifecycle),
		}
		// Cross-round trigger memory, shared by pointer like the
		// production engine's store (no per-attempt isolation: the
		// production engine has none either).
		cfg.T2Store = opts.t2Store
		// U3 one-direction-per-symbol final pass, via the exported
		// production accessor (built for offline tools).
		cfg.FinalConflict = kernel.HunterV7FinalConflictConfigFor(hunterCfg)
	}

	v7Result := local.ScoreHunterV7Detailed(snap, cfg)
	universe := v7Result.Universe
	regime := v7Result.Regime
	signals := v7Result.Signals
	if opts.dumpUniverse != "" {
		// Golden-replay capture: freeze this cycle's routing inputs so the
		// refactor safety net can replay RouteDetailed deterministically.
		if err := local.DumpV7GoldenFixture(opts.dumpUniverse, universe, regime, cfg); err != nil {
			return report, fatalError(fmt.Errorf("dump golden fixture failed: %w", err))
		}
		fmt.Printf("golden fixture: %s (universe=%d regime=%s)\n", opts.dumpUniverse, len(universe), regime)
	}
	geometry := executionGeometryFromStrategy(strategyCfg)
	candidates := kernel.AssembleHunterV7CandidateCoins(signals, "BOTH", geometry)
	// Fix 04: one bulk ticker call right before the prompt; the prompt sees
	// prices seconds old. Scoring and the assembled tiers above are final.
	promptSnap := snap
	promptCandidates := candidates // persisted candidates stay as assembled
	var reprice repriceSummary
	if opts.promptReprice {
		maxAge := opts.promptPriceMaxAge
		if maxAge <= 0 {
			maxAge = defaultPromptPriceMaxAge
		}
		prices, fetchedAt, err := fetcher.FetchTickerPrices(context.Background())
		if err != nil {
			return report, transientError(fmt.Errorf("pre-prompt reprice failed: %w", err))
		}
		now := time.Now()
		promptSnap, reprice = repriceForPrompt(snap, promptSymbols(signals, candidates), prices, fetchedAt, now, maxAge)
		promptCandidates = append([]kernel.CandidateCoin(nil), candidates...)
		applyPromptFreshness(promptCandidates, promptSnap, snap, now, maxAge)
	}
	prompt, promptFinal := buildPrompt(promptCandidates, signals, promptSnap, strategyCfg)

	now := time.Now()
	stamp := now.Format("20060102-150405")
	if opts.rounds > 1 {
		stamp = fmt.Sprintf("%s-r%02d", stamp, round)
	}

	sha, dirty := harnessBuildInfo()
	report = validationReport{
		GeneratedAt:   now.Format("2006-01-02 15:04:05 MST"),
		HarnessGitSHA: sha,
		HarnessDirty:  dirty,
		Round:         round,
		Snapshot:      buildSnapshotSummary(snap, len(universe), string(regime), time.Since(start)),
		Config:        cfg,
		Signals:       signals,
		PotentialPool: v7Result.PotentialPool,
	}
	report.Snapshot = annotateSnapshotQuality(report.Snapshot)
	if report.Snapshot.FailedRequests > 0 && opts.fetchErrors != nil {
		report.Snapshot.FetchErrorLog = opts.fetchErrors.path // TA-6
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
	report.PromptReprice = reprice
	report.Issues = append(report.Issues, repriceIssues(reprice)...)
	report.TierLayers = buildTierLayerReport(candidates, promptFinal) // fix 12

	// TA-2: the contract decides where this attempt's artifacts go. A
	// contract failure always fails the round; its artifacts are kept for
	// diagnosis under <out-dir>/rejected/, never next to accepted rounds.
	if acceptErr := reportAcceptanceError(report); acceptErr != nil {
		report.Accepted = false
		report.RejectedReason = acceptErr.Error()
		if pp, _, _, werr := writeRoundArtifacts(opts, report, prompt, stamp, true); werr != nil {
			log.Printf("WARN write rejected artifacts failed: %v", werr)
		} else {
			report.PromptPreviewPath = pp
		}
		return report, contractError(contractSignature(report), acceptErr)
	}
	report.Accepted = true
	promptPath, rawPath, mdPath, err := writeRoundArtifacts(opts, report, prompt, stamp, false)
	if err != nil {
		return report, err
	}
	report.PromptPreviewPath = promptPath
	if err := persistValidationSignals(opts, round, v7Result.RawSignals, candidates, promptFinal, now.UTC(), snap); err != nil {
		// CreateInBatches runs in one transaction, so nothing was stored and
		// the round can be retried. Move this attempt's artifacts aside.
		for _, path := range []string{promptPath, rawPath, mdPath} {
			_ = os.Remove(path)
		}
		report.Accepted = false
		report.RejectedReason = err.Error()
		if pp, _, _, werr := writeRoundArtifacts(opts, report, prompt, stamp, true); werr != nil {
			log.Printf("WARN write rejected artifacts failed: %v", werr)
		} else {
			report.PromptPreviewPath = pp
		}
		return report, transientError(err)
	}

	printConsoleSummary(report, rawPath, mdPath)
	return report, nil
}

// writeRoundArtifacts writes the prompt, raw JSON and markdown report of one
// attempt. Accepted attempts go to <out-dir>; rejected attempts go to
// <out-dir>/rejected/ with the attempt number in the file name (TA-2).
// buildSnapshotSummary maps the snapshot metadata into the round report.
// TA-5: fetch_ms is the snapshot fetch alone; round_ms is the whole round
// so far (strategy load, fetch, scoring, assembly, prompt build).
func buildSnapshotSummary(snap *datafetch.Snapshot, universeCount int, regime string, roundElapsed time.Duration) snapshotSummary {
	return snapshotSummary{
		SymbolCount:           snap.Meta.SymbolCount,
		UniverseCount:         universeCount,
		FetchMs:               snap.Meta.FetchDuration.Milliseconds(),
		RoundMs:               roundElapsed.Milliseconds(),
		DetailRequested:       snap.Meta.DetailRequested,
		DetailCompleted:       snap.Meta.DetailCompleted,
		DetailSkippedDeadline: snap.Meta.DetailSkippedDeadline,
		FailedRequests:        snap.Meta.FailedRequests,
		EndpointLatency:       snap.Meta.EndpointLatency,
		TwoTier:               snap.Meta.Tiered,
		SlowEvery:             snap.Meta.SlowEvery,
		FetchIndex:            snap.Meta.FetchIndex,
		T2InputsTopN:          snap.Meta.T2InputsTopN,
		SlowRefreshed:         snap.Meta.SlowRefreshed,
		SlowCarried:           snap.Meta.SlowCarried,
		SlowMissing:           snap.Meta.SlowMissing,
		SlowMaxAgeSec:         int64(snap.Meta.SlowMaxAge / time.Second),
		RestErrors:            snap.Meta.RestErrors,
		BTC24h:                priceChange24h(snap, "BTCUSDT"),
		ETH24h:                priceChange24h(snap, "ETHUSDT"),
		Regime:                regime,
	}
}

func writeRoundArtifacts(opts validationOptions, report validationReport, prompt, stamp string, rejected bool) (promptPath, rawPath, mdPath string, err error) {
	dir := opts.outDir
	if rejected {
		dir = filepath.Join(opts.outDir, "rejected")
		stamp = fmt.Sprintf("%s-a%d", stamp, opts.attempt)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", "", "", transientError(fmt.Errorf("create out dir failed: %w", err))
	}
	promptPath = fmt.Sprintf("%s/hunter-v7-live-prompt-%s.txt", dir, stamp)
	rawPath = fmt.Sprintf("%s/hunter-v7-live-validation-raw-%s.json", dir, stamp)
	mdPath = fmt.Sprintf("%s/hunter-v7-live-validation-report-%s.md", dir, stamp)
	report.PromptPreviewPath = promptPath
	if err := os.WriteFile(promptPath, []byte(prompt), 0644); err != nil {
		return "", "", "", transientError(fmt.Errorf("write prompt failed: %w", err))
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", "", "", fatalError(fmt.Errorf("marshal report failed: %w", err))
	}
	if err := os.WriteFile(rawPath, raw, 0644); err != nil {
		return "", "", "", transientError(fmt.Errorf("write raw report failed: %w", err))
	}
	if err := os.WriteFile(mdPath, []byte(formatMarkdown(report, rawPath)), 0644); err != nil {
		return "", "", "", transientError(fmt.Errorf("write markdown report failed: %w", err))
	}
	return promptPath, rawPath, mdPath, nil
}

func persistValidationSignals(opts validationOptions, cycleNum int, rawSignals []local.V7SignalOutput, candidates []kernel.CandidateCoin, promptFinal []kernel.HunterV7PromptFinalTier, ts time.Time, snap *datafetch.Snapshot) error {
	if !opts.persistSignals || opts.store == nil {
		return nil
	}
	records := kernel.BuildHunterV7SignalRecords(rawSignals, candidates)
	dbRecords := kernel.BuildHunterV7SignalDBRecords(cycleNum, records, ts, string(aittrader.TrackedActive))
	if len(dbRecords) == 0 {
		return nil
	}
	// Fix 12: execution_tier stays the assembled tier (it also decides
	// tracking below); prompt_final_tier records what the LLM saw.
	kernel.ApplyHunterV7PromptFinalTiers(dbRecords, promptFinal)
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
		// TA-7: trigger price (drift_at_entry_pct) and outcome source, as live.
		if _, replacedID := opts.outcomeTracker.RegisterWithMeta(
			aittrader.TrackedSignalMeta{OutcomeSource: rec.Tags.OutcomeSource, TriggerPrice: rec.Tags.TriggerPrice},
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
		); replacedID > 0 {
			now := time.Now().UTC()
			if err := opts.store.HunterV7Signal().UpdateTrackOutcome(replacedID, store.HunterV7SignalTrackUpdate{
				Status:   "DUPLICATE_CONTEXT",
				ExitTime: &now,
			}); err != nil {
				log.Printf("WARN mark validation V7 duplicate context failed: %v", err)
			}
		}
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
	if minutes > validationBackfillMaxMinutes {
		minutes = validationBackfillMaxMinutes
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

func validationLatestCandle(fetcher *datafetch.DataFetcher, symbol string) *aittrader.TrackedCandle {
	if fetcher == nil || symbol == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	klines, err := fetcher.FetchKlines(ctx, symbol, "1m", 2)
	if err != nil {
		log.Printf("WARN validation Hunter v7 latest 1m candle failed for %s: %v", symbol, err)
		return nil
	}
	for i := len(klines) - 1; i >= 0; i-- {
		k := klines[i]
		if k.Close <= 0 {
			continue
		}
		return &aittrader.TrackedCandle{
			T:      time.UnixMilli(k.CloseTime).UTC(),
			Open:   k.Open,
			High:   k.High,
			Low:    k.Low,
			Close:  k.Close,
			Volume: k.Volume,
		}
	}
	return nil
}

func runPostTrackOutcomes(opts validationOptions) error {
	if opts.outcomeTracker == nil || opts.postTrackDuration <= 0 {
		return nil
	}
	interval := opts.postTrackInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if err := os.MkdirAll(opts.outDir, 0755); err != nil {
		return fmt.Errorf("create out dir failed: %w", err)
	}
	deadline := time.Now().Add(opts.postTrackDuration)
	fmt.Printf("Post-tracking Hunter v7 outcomes for up to %s (interval=%s, active_only=%v)\n",
		opts.postTrackDuration, interval, opts.trackActiveOnly)
	opts.outcomeTracker.TickNow()
	for time.Now().Before(deadline) {
		active := len(opts.outcomeTracker.GetActive())
		if opts.trackActiveOnly && active == 0 {
			fmt.Println("Post-tracking complete: no ACTIVE validation outcomes remain")
			return nil
		}
		remaining := time.Until(deadline)
		sleepFor := interval
		if remaining < sleepFor {
			sleepFor = remaining
		}
		if sleepFor <= 0 {
			break
		}
		fmt.Printf("Post-tracking ACTIVE outcomes: %d; next tick in %s\n", active, sleepFor.Round(time.Second))
		time.Sleep(sleepFor)
		opts.outcomeTracker.TickNow()
	}
	fmt.Printf("Post-tracking finished at duration limit; ACTIVE outcomes remaining=%d\n", len(opts.outcomeTracker.GetActive()))
	return nil
}

func writeValidationOutcomeSummary(opts validationOptions) error {
	if opts.outcomeTracker == nil {
		return nil
	}
	if err := os.MkdirAll(opts.outDir, 0755); err != nil {
		return fmt.Errorf("create out dir failed: %w", err)
	}
	now := time.Now()
	summary := buildValidationOutcomeSummary(opts, now)
	stamp := now.Format("20060102-150405")
	rawPath := fmt.Sprintf("%s/hunter-v7-validation-outcomes-%s.json", opts.outDir, stamp)
	mdPath := fmt.Sprintf("%s/hunter-v7-validation-outcomes-%s.md", opts.outDir, stamp)
	raw, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal outcome summary failed: %w", err)
	}
	if err := os.WriteFile(rawPath, raw, 0644); err != nil {
		return fmt.Errorf("write outcome summary json failed: %w", err)
	}
	if err := os.WriteFile(mdPath, []byte(formatOutcomeSummaryMarkdown(summary, rawPath)), 0644); err != nil {
		return fmt.Errorf("write outcome summary markdown failed: %w", err)
	}
	fmt.Printf("outcome_summary: %s\noutcome_report: %s\n", rawPath, mdPath)
	return nil
}

func writeValidationRunSummary(opts validationOptions, out runOutcome) error {
	reports := out.reports
	if len(reports) == 0 && len(out.rejected) == 0 && out.exitCode == exitOK {
		return nil
	}
	if err := os.MkdirAll(opts.outDir, 0755); err != nil {
		return fmt.Errorf("create out dir failed: %w", err)
	}
	now := time.Now()
	summary := buildValidationRunSummary(reports, now)
	summary.RejectedRounds = append([]rejectedRound(nil), out.rejected...)
	summary.ExitCode, summary.ExitReason = out.exitCode, out.exitReason
	summary.ResumeTracking = opts.resumeTracking
	stamp := now.Format("20060102-150405")
	rawPath := fmt.Sprintf("%s/hunter-v7-validation-run-summary-%s.json", opts.outDir, stamp)
	mdPath := fmt.Sprintf("%s/hunter-v7-validation-run-summary-%s.md", opts.outDir, stamp)
	raw, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal run summary failed: %w", err)
	}
	if err := os.WriteFile(rawPath, raw, 0644); err != nil {
		return fmt.Errorf("write run summary json failed: %w", err)
	}
	if err := os.WriteFile(mdPath, []byte(formatRunSummaryMarkdown(summary, rawPath)), 0644); err != nil {
		return fmt.Errorf("write run summary markdown failed: %w", err)
	}
	fmt.Printf("run_summary: %s\nrun_report: %s\n", rawPath, mdPath)
	return nil
}

func buildValidationRunSummary(reports []validationReport, now time.Time) validationRunSummary {
	sha, dirty := harnessBuildInfo()
	summary := validationRunSummary{
		GeneratedAt:   now.Format("2006-01-02 15:04:05 MST"),
		HarnessGitSHA: sha,
		HarnessDirty:  dirty,
		Rounds:        len(reports),
		Round:         make([]validationRoundSummary, 0, len(reports)),
	}
	for _, report := range reports {
		signalCount := report.OpportunityCover.SignalCount
		openReview := reportOpenReviewCount(report)
		round := validationRoundSummary{
			Round:          report.Round,
			GeneratedAt:    report.GeneratedAt,
			Regime:         report.Snapshot.Regime,
			SignalCount:    signalCount,
			OpenReview:     openReview,
			OpenReviewRate: pct(openReview, signalCount),
			RestErrors:     report.Snapshot.RestErrors,
			RestErrorRate:  report.Snapshot.RestErrorRate,
			UniverseCount:  report.Snapshot.UniverseCount,
			Degraded:       report.Snapshot.Degraded,
			Reasons:        append([]string(nil), report.Snapshot.DegradationReasons...),
		}
		summary.Round = append(summary.Round, round)
		summary.SignalCount += signalCount
		summary.OpenReviewCount += openReview
		if report.Snapshot.Degraded {
			summary.DegradedRounds = append(summary.DegradedRounds, report.Round)
			continue
		}
		summary.ValidRounds++
		summary.ValidSignalCount += signalCount
		summary.ValidOpenReviewCount += openReview
	}
	summary.OpenReviewRate = pct(summary.OpenReviewCount, summary.SignalCount)
	summary.ValidOpenReviewRate = pct(summary.ValidOpenReviewCount, summary.ValidSignalCount)
	return summary
}

func reportOpenReviewCount(report validationReport) int {
	return report.AIRecognition.PromptTierCounts["EXECUTABLE"] + report.AIRecognition.PromptTierCounts["REVIEWABLE"]
}

func pct(n, d int) float64 {
	if d <= 0 {
		return 0
	}
	return float64(n) / float64(d) * 100
}

func formatRunSummaryMarkdown(summary validationRunSummary, rawPath string) string {
	var sb strings.Builder
	sb.WriteString("# Hunter v7 验证 Run 汇总\n\n")
	sb.WriteString(fmt.Sprintf("> 生成时间：%s\n", summary.GeneratedAt))
	sb.WriteString(fmt.Sprintf("> 原始 JSON：`%s`\n\n", rawPath))
	sb.WriteString("## 1. 总览\n\n")
	sb.WriteString("| 项目 | 数值 |\n|---|---:|\n")
	sb.WriteString(fmt.Sprintf("| rounds | %d |\n", summary.Rounds))
	sb.WriteString(fmt.Sprintf("| valid_rounds | %d |\n", summary.ValidRounds))
	sb.WriteString(fmt.Sprintf("| all_open_review_rate | %.1f%% |\n", summary.OpenReviewRate))
	sb.WriteString(fmt.Sprintf("| valid_open_review_rate | %.1f%% |\n", summary.ValidOpenReviewRate))
	sb.WriteString(fmt.Sprintf("| all_signals | %d |\n", summary.SignalCount))
	sb.WriteString(fmt.Sprintf("| all_open_review | %d |\n", summary.OpenReviewCount))
	sb.WriteString(fmt.Sprintf("| valid_signals | %d |\n", summary.ValidSignalCount))
	sb.WriteString(fmt.Sprintf("| valid_open_review | %d |\n\n", summary.ValidOpenReviewCount))
	if summary.Rounds > 0 && summary.ValidRounds == 0 {
		sb.WriteString("> INVALID_SAMPLE_DO_NOT_USE_FOR_WINRATE: 本次所有轮次均为 degraded，all_open_review_rate 仅可用于 smoke 回归，不可作为胜率/开仓率验收。\n\n")
	}
	sb.WriteString("## 2. Round 明细\n\n")
	sb.WriteString("> Open-review = EXECUTABLE + REVIEWABLE at the **prompt-final tier** (what the LLM saw), over all module signals.\n\n")
	sb.WriteString("| Round | Regime | Signals | Open-review (prompt-final) | Open-rate | REST errors | REST error rate | Universe | Degraded | Reasons |\n")
	sb.WriteString("|---:|---|---:|---:|---:|---:|---:|---:|---|---|\n")
	for _, round := range summary.Round {
		sb.WriteString(fmt.Sprintf("| %d | %s | %d | %d | %.1f%% | %d | %.1f%% | %d | %v | %s |\n",
			round.Round, round.Regime, round.SignalCount, round.OpenReview, round.OpenReviewRate,
			round.RestErrors, round.RestErrorRate*100, round.UniverseCount, round.Degraded, strings.Join(round.Reasons, ", ")))
	}
	if len(summary.RejectedRounds) > 0 {
		sb.WriteString("\n## 3. Rejected rounds (contract, skipped; artifacts in rejected/)\n\n")
		sb.WriteString("| Round | Attempts | Signature |\n|---:|---:|---|\n")
		for _, rr := range summary.RejectedRounds {
			sb.WriteString(fmt.Sprintf("| %d | %d | %s |\n", rr.Round, rr.Attempts, rr.Signature))
		}
	}
	if rt := summary.ResumeTracking; rt != nil {
		sb.WriteString("\n## Resume: outcome tracking (fix 09)\n\n")
		sb.WriteString(fmt.Sprintf("> %s\n", rt.logLine()))
		if len(rt.Orphaned) > 0 {
			sb.WriteString(fmt.Sprintf("> %s ids %v: compute their fee-net with cmd/v7_fee_net_offline\n", trackRestartOrphaned, rt.Orphaned))
		}
	}
	if summary.ExitCode != exitOK {
		sb.WriteString(fmt.Sprintf("\n> RUN STOPPED: exit code %d: %s\n", summary.ExitCode, summary.ExitReason))
	}
	return sb.String()
}

func buildValidationOutcomeSummary(opts validationOptions, now time.Time) validationOutcomeSummary {
	sha, dirty := harnessBuildInfo()
	summary := validationOutcomeSummary{
		GeneratedAt:       now.Format("2006-01-02 15:04:05 MST"),
		HarnessGitSHA:     sha,
		HarnessDirty:      dirty,
		PostTrackDuration: opts.postTrackDuration.String(),
		PostTrackInterval: opts.postTrackInterval.String(),
		TrackActiveOnly:   opts.trackActiveOnly,
		CompletedStats:    opts.outcomeTracker.GetStatsBySetupType(),
	}
	statuses := []aittrader.TrackedStatus{
		aittrader.TrackedActive,
		aittrader.TrackedWinTP0,
		aittrader.TrackedWinTP1,
		aittrader.TrackedWinTP2,
		aittrader.TrackedProtectedStop,
		aittrader.TrackedAmbiguousIntrabar,
		aittrader.TrackedStop,
		aittrader.TrackedTimeout,
	}
	setupBuckets := make(map[string]*validationSetupOutcome)
	for _, status := range statuses {
		signals := opts.outcomeTracker.GetByStatus(status)
		if len(signals) == 0 {
			continue
		}
		statusSummary := summarizeTrackedStatus(status, signals)
		summary.Status = append(summary.Status, statusSummary)
		summary.TrackedCount += statusSummary.Count
		if status == aittrader.TrackedActive {
			summary.ActiveCount = statusSummary.Count
		}
		for _, sig := range signals {
			setup := sig.SetupType
			if setup == "" {
				setup = "unknown"
			}
			bucket := setupBuckets[setup]
			if bucket == nil {
				bucket = &validationSetupOutcome{SetupType: setup}
				setupBuckets[setup] = bucket
			}
			addSignalToSetupOutcome(bucket, sig)
			summary.Details = append(summary.Details, validationOutcomeDetail{
				Symbol: sig.Symbol, Direction: sig.Direction, SetupType: sig.SetupType,
				Tier: sig.Tier, recordID: sig.RecordID, Status: string(sig.Status), EntryPrice: sig.SignalPrice,
				ExitPrice: sig.ExitPrice, StopPrice: sig.StopPrice, TP0Price: sig.TP0Price,
				TP1Price: sig.TP1Price, TP2Price: sig.TP2Price,
				PnLPct: opts.outcomeTracker.FinalPnLPctForReport(sig), MFE: sig.MaxFavorable, MAE: sig.MaxAdverse,
				TP0Done: sig.TP0Done, TP1Done: sig.TP1Done, TP2Done: sig.TP2Done,
				RemainingPct: sig.RemainingRatio * 100, RealizedPnL: sig.RealizedPnLPct,
			})
			if sig.TP0Done {
				summary.TP0PartialCount++
			}
			if sig.TP0Done && (sig.Status == aittrader.TrackedWinTP1 || sig.Status == aittrader.TrackedWinTP2) {
				summary.RunnerTP1PlusCount++
			}
			if sig.Status == aittrader.TrackedStop && sig.MaxFavorable > 1.0 {
				summary.MFE1LossStopCount++
			}
		}
	}
	// Fix 12: the prompt-final tier of each tracked row, from the DB.
	ids := map[int64]bool{}
	for _, d := range summary.Details {
		if d.recordID > 0 {
			ids[d.recordID] = true
		}
	}
	if pt := promptFinalTiersByID(opts.store, sortedIDs(ids)); pt != nil {
		for i := range summary.Details {
			summary.Details[i].PromptTier = pt[summary.Details[i].recordID]
		}
	}
	sort.Slice(summary.Status, func(i, j int) bool {
		return summary.Status[i].Status < summary.Status[j].Status
	})
	for _, bucket := range setupBuckets {
		if bucket.Count > 0 {
			n := float64(bucket.Count)
			bucket.AvgPnLPct /= n
			bucket.AvgMFE /= n
			bucket.AvgMAE /= n
		}
		summary.Setup = append(summary.Setup, *bucket)
	}
	sort.Slice(summary.Setup, func(i, j int) bool {
		return summary.Setup[i].SetupType < summary.Setup[j].SetupType
	})
	sort.Slice(summary.Details, func(i, j int) bool {
		if summary.Details[i].SetupType == summary.Details[j].SetupType {
			return summary.Details[i].Symbol < summary.Details[j].Symbol
		}
		return summary.Details[i].SetupType < summary.Details[j].SetupType
	})
	return summary
}

func summarizeTrackedStatus(status aittrader.TrackedStatus, signals []*aittrader.TrackedSignal) validationStatusOutcome {
	out := validationStatusOutcome{Status: string(status), Count: len(signals)}
	for _, sig := range signals {
		pnl := trackedSignalPnLPct(sig)
		out.AvgPnLPct += pnl
		out.AvgMFE += sig.MaxFavorable
		out.AvgMAE += sig.MaxAdverse
		switch {
		case pnl > 0:
			out.ProfitCount++
		case pnl < 0:
			out.LossCount++
		default:
			out.FlatCount++
		}
	}
	if out.Count > 0 {
		n := float64(out.Count)
		out.AvgPnLPct /= n
		out.AvgMFE /= n
		out.AvgMAE /= n
	}
	return out
}

func addSignalToSetupOutcome(out *validationSetupOutcome, sig *aittrader.TrackedSignal) {
	if out == nil || sig == nil {
		return
	}
	out.Count++
	pnl := trackedSignalPnLPct(sig)
	out.AvgPnLPct += pnl
	out.AvgMFE += sig.MaxFavorable
	out.AvgMAE += sig.MaxAdverse
	switch sig.Status {
	case aittrader.TrackedActive:
		out.Active++
		if pnl > 0 {
			out.ActiveProfit++
		} else if pnl < 0 {
			out.ActiveLoss++
		}
	case aittrader.TrackedWinTP0, aittrader.TrackedWinTP1, aittrader.TrackedWinTP2:
		out.Wins++
	case aittrader.TrackedProtectedStop:
		out.ProtectedStops++
	case aittrader.TrackedStop:
		if pnl >= 0 {
			out.ProtectedStops++
		} else {
			out.LossStops++
		}
	case aittrader.TrackedTimeout:
		out.Timeouts++
	}
}

func trackedSignalPnLPct(sig *aittrader.TrackedSignal) float64 {
	if sig == nil || sig.SignalPrice <= 0 {
		return 0
	}
	price := sig.CurrentPrice
	if sig.ExitPrice > 0 {
		price = sig.ExitPrice
	}
	if price <= 0 {
		return 0
	}
	var unrealized float64
	switch strings.ToUpper(sig.Direction) {
	case string(local.V7DirLong):
		unrealized = (price - sig.SignalPrice) / sig.SignalPrice * 100
	case string(local.V7DirShort):
		unrealized = (sig.SignalPrice - price) / sig.SignalPrice * 100
	default:
		return 0
	}
	if sig.RemainingRatio > 0 && sig.RemainingRatio <= 1 {
		return sig.RealizedPnLPct + sig.RemainingRatio*unrealized
	}
	return unrealized
}

func formatOutcomeSummaryMarkdown(summary validationOutcomeSummary, rawPath string) string {
	var sb strings.Builder
	sb.WriteString("# Hunter v7 验证 Outcome 汇总\n\n")
	sb.WriteString(fmt.Sprintf("> 生成时间：%s\n", summary.GeneratedAt))
	sb.WriteString(fmt.Sprintf("> 原始 JSON：`%s`\n\n", rawPath))
	sb.WriteString("## 1. 总览\n\n")
	sb.WriteString("| 项目 | 数值 |\n|---|---:|\n")
	sb.WriteString(fmt.Sprintf("| tracked | %d |\n", summary.TrackedCount))
	sb.WriteString(fmt.Sprintf("| active | %d |\n", summary.ActiveCount))
	sb.WriteString(fmt.Sprintf("| post_track_duration | %s |\n", summary.PostTrackDuration))
	sb.WriteString(fmt.Sprintf("| post_track_interval | %s |\n", summary.PostTrackInterval))
	sb.WriteString("| TP0 partial exits | " + strconv.Itoa(summary.TP0PartialCount) + " |\n")
	sb.WriteString("| TP0→TP1/TP2 runners | " + strconv.Itoa(summary.RunnerTP1PlusCount) + " |\n")
	sb.WriteString("| MFE>1% then STOP alerts | " + strconv.Itoa(summary.MFE1LossStopCount) + " |\n\n")
	sb.WriteString("## 2. 状态分布\n\n")
	if len(summary.Status) == 0 {
		sb.WriteString("- 无 tracked outcome。\n\n")
	} else {
		sb.WriteString("| Status | Count | Profit | Loss | Flat | Avg PnL% | Avg MFE% | Avg MAE% |\n")
		sb.WriteString("|---|---:|---:|---:|---:|---:|---:|---:|\n")
		for _, st := range summary.Status {
			sb.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %d | %.3f | %.3f | %.3f |\n",
				st.Status, st.Count, st.ProfitCount, st.LossCount, st.FlatCount, st.AvgPnLPct, st.AvgMFE, st.AvgMAE))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("## 3. Setup 汇总\n\n")
	if len(summary.Setup) == 0 {
		sb.WriteString("- 无 setup outcome。\n")
		return sb.String()
	}
	sb.WriteString("| Setup | Count | Active | Wins | Loss Stops | Protected Stops | Timeouts | Active Profit | Active Loss | Avg PnL% | Avg MFE% | Avg MAE% |\n")
	sb.WriteString("|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, setup := range summary.Setup {
		sb.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %d | %d | %d | %d | %d | %.3f | %.3f | %.3f |\n",
			setup.SetupType, setup.Count, setup.Active, setup.Wins, setup.LossStops, setup.ProtectedStops,
			setup.Timeouts, setup.ActiveProfit, setup.ActiveLoss, setup.AvgPnLPct, setup.AvgMFE, setup.AvgMAE))
	}
	sb.WriteString("\n## 4. 逐笔 Outcome 明细\n\n")
	if len(summary.Details) == 0 {
		sb.WriteString("- 无逐笔 outcome。\n")
		return sb.String()
	}
	sb.WriteString("| Symbol | Dir | Setup | Assembled tier | Prompt-final tier | Status | Entry | Exit | TP0/TP1/TP2 | PnL% | Realized% | MFE% | MAE% | TP0/TP1/TP2 | Remaining |\n")
	sb.WriteString("|---|---|---|---|---|---|---:|---:|---|---:|---:|---:|---:|---|---:|\n")
	for _, item := range summary.Details {
		pt := item.PromptTier
		if pt == "" {
			pt = "-"
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %.6g | %.6g | %.6g / %.6g / %.6g | %.3f | %.3f | %.3f | %.3f | %t/%t/%t | %.1f%% |\n",
			item.Symbol, item.Direction, item.SetupType, item.Tier, pt, item.Status, item.EntryPrice, item.ExitPrice,
			item.TP0Price, item.TP1Price, item.TP2Price, item.PnLPct, item.RealizedPnL, item.MFE, item.MAE,
			item.TP0Done, item.TP1Done, item.TP2Done, item.RemainingPct))
	}
	return sb.String()
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

// buildPrompt returns the prompt and, from the same build, the prompt-final
// tier of every candidate it showed (fix 12).
func buildPrompt(candidates []kernel.CandidateCoin, signals []local.V7SignalOutput, snap *datafetch.Snapshot, strategyCfg *store.StrategyConfig) (string, []kernel.HunterV7PromptFinalTier) {
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
	md := promptMarketData(signals, snap)
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
	return systemPrompt + "\n\n--- USER PROMPT ---\n\n" + userPrompt, ctx.HunterV7PromptFinal
}

// promptMarketData builds the prompt's per-symbol market data. The full
// production converter is preferred; a signal's own derivatives context
// overrides funding only when present (TA-1: a T2 signal without one used
// to show funding 0 instead of the snapshot's rate).
func promptMarketData(signals []local.V7SignalOutput, snap *datafetch.Snapshot) map[string]*market.Data {
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
		hasDerivCtx := sig.DerivativesCtx != nil
		if hasDerivCtx {
			funding = sig.DerivativesCtx.FundingRate
			oi = sig.DerivativesCtx.OIValue
		} else if snap != nil {
			// TA-1: without a derivatives context, fall back to the
			// snapshot's own funding rate instead of reporting 0.
			if ss, ok := snap.Symbols[sig.Symbol]; ok && ss != nil {
				funding = ss.FundingRate
			}
		}
		// Prefer the production converter so the prompt cycle sees the same
		// TimeframeData (klines + EMA20 series) the live trader would; the
		// pre-prompt live-confirmation pass depends on it. Fall back to the
		// sparse context-only Data when the snapshot lacks klines.
		if snap != nil {
			if ss, ok := snap.Symbols[sig.Symbol]; ok && ss != nil && len(ss.Klines) > 0 {
				if full, err := market.BuildDataFromSymbolSnapshot(sig.Symbol, ss, []string{"5m", "15m", "1h"}, "15m", 60); err == nil {
					// BuildDataFromSymbolSnapshot already carries ss.FundingRate;
					// only the signal's own derivatives context may override it.
					if hasDerivCtx {
						full.FundingRate = funding
					}
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
	return md
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
	restRate := s.RestErrorRate
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
	if s.DetailSkippedDeadline > 0 {
		issues = append(issues, issue{
			Severity: "medium",
			Code:     "detail_truncated_by_deadline",
			Detail: fmt.Sprintf("round deadline expired during the detail fetch: %d of %d detail symbols skipped (no OI/LSR/kline detail)",
				s.DetailSkippedDeadline, s.DetailRequested),
		})
	}
	if s.TwoTier && s.SlowMissing > 0 {
		issues = append(issues, issue{
			Severity: "medium",
			Code:     "slow_tier_missing",
			Detail: fmt.Sprintf("two-tier fetch: %d of %d fetched symbols have no slow-tier data yet (1h OI history, long/short ratio, 1h/4h/1d klines); expected for up to %d rounds after start or resume (warm-up), otherwise their slow requests failed",
				s.SlowMissing, s.DetailCompleted, s.SlowEvery),
		})
	}
	coverage := s.UniverseCoverage
	if s.UniverseCount > 0 && coverage < 0.30 {
		issues = append(issues, issue{
			Severity: "medium",
			Code:     "universe_coverage_low",
			Detail: fmt.Sprintf("Hunter v7 routeable universe=%d/%d (%.1f%%); this is a routing observation, not a REST success-rate gate",
				s.UniverseCount, s.SymbolCount, coverage*100),
		})
	}
	if s.Degraded {
		issues = append(issues, issue{
			Severity: "high",
			Code:     "degraded_round_excluded_from_main_stats",
			Detail:   "this validation round is marked degraded and should be excluded from valid-round open-rate/outcome conclusions: " + strings.Join(s.DegradationReasons, ", "),
		})
	}
	return issues
}

func annotateSnapshotQuality(s snapshotSummary) snapshotSummary {
	if s.SymbolCount > 0 {
		s.RestErrorRate = float64(s.RestErrors) / float64(s.SymbolCount)
		s.UniverseCoverage = float64(s.UniverseCount) / float64(s.SymbolCount)
	}
	if s.RestErrorRate > 0.20 {
		s.Degraded = true
		s.DegradationReasons = append(s.DegradationReasons, fmt.Sprintf("rest_error_rate_gt_20pct(%.1f%%)", s.RestErrorRate*100))
	}
	return s
}

func snapshotAcceptanceError(s snapshotSummary) error {
	if s.SymbolCount <= 0 {
		return fmt.Errorf("snapshot quality rejected: no Binance symbols returned")
	}
	if s.Degraded {
		return fmt.Errorf("snapshot quality rejected: %s", strings.Join(s.DegradationReasons, ", "))
	}
	return nil
}

// reportAcceptanceError blocks persistence only for broken contract data. A
// legitimate no-signal round remains valid evidence; it is not retried or
// silently converted into an artificial open-rate observation.
func reportAcceptanceError(report validationReport) error {
	check := report.FormatCheck
	if !check.JSONMarshalOK || !check.JSONUnmarshalOK || check.MissingFieldCount > 0 {
		return fmt.Errorf("validation contract rejected: json marshal=%v unmarshal=%v missing_fields=%d",
			check.JSONMarshalOK, check.JSONUnmarshalOK, check.MissingFieldCount)
	}
	if check.ExecutableGapCount > 0 {
		return fmt.Errorf("validation contract rejected: executable_field_gaps=%d", check.ExecutableGapCount)
	}
	openReview := reportOpenReviewCount(report)
	if openReview > 0 && (!report.AIRecognition.PromptContainsV7JSON ||
		!report.AIRecognition.PromptContainsSetupType ||
		!report.AIRecognition.PromptContainsEntryMode ||
		!report.AIRecognition.PromptContainsRiskLevel ||
		!report.AIRecognition.PromptContainsConfirms ||
		!report.AIRecognition.PromptContainsInvalid) {
		return fmt.Errorf("validation contract rejected: prompt missing required v7 fields for %d open-review candidates", openReview)
	}
	return nil
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
	sb.WriteString(fmt.Sprintf("> Prompt 预览：`%s`\n", r.PromptPreviewPath))
	sb.WriteString(fmt.Sprintf("> Harness：`%s` dirty=%v\n\n", r.HarnessGitSHA, r.HarnessDirty))

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
	sb.WriteString(fmt.Sprintf("| Degraded round | %v |\n", r.Snapshot.Degraded))
	if len(r.Snapshot.DegradationReasons) > 0 {
		sb.WriteString(fmt.Sprintf("| Degradation reasons | %s |\n", strings.Join(r.Snapshot.DegradationReasons, ", ")))
	}
	sb.WriteString(fmt.Sprintf("| Fetch ms | %d |\n", r.Snapshot.FetchMs))
	sb.WriteString(fmt.Sprintf("| Round ms | %d |\n", r.Snapshot.RoundMs))
	sb.WriteString(fmt.Sprintf("| Detail requested / completed / skipped (deadline) | %d / %d / %d |\n",
		r.Snapshot.DetailRequested, r.Snapshot.DetailCompleted, r.Snapshot.DetailSkippedDeadline))
	sb.WriteString(fmt.Sprintf("| Failed requests | %d |\n", r.Snapshot.FailedRequests))
	if r.Snapshot.TwoTier {
		sb.WriteString(fmt.Sprintf("| Two-tier fetch: slow every N / fetch index / T2-input top N | %d / %d / %d |\n",
			r.Snapshot.SlowEvery, r.Snapshot.FetchIndex, r.Snapshot.T2InputsTopN))
		sb.WriteString(fmt.Sprintf("| Slow tier refreshed / carried / missing | %d / %d / %d |\n",
			r.Snapshot.SlowRefreshed, r.Snapshot.SlowCarried, r.Snapshot.SlowMissing))
		sb.WriteString(fmt.Sprintf("| Slow tier max age s | %d |\n", r.Snapshot.SlowMaxAgeSec))
	}
	if r.Snapshot.FetchErrorLog != "" {
		sb.WriteString(fmt.Sprintf("| Failed request log (path+query) | `%s` |\n", r.Snapshot.FetchErrorLog))
	}
	sb.WriteString("\n")
	if len(r.Snapshot.EndpointLatency) > 0 {
		sb.WriteString("| Endpoint | Calls | Attempts | Failed | Net p50/p95/max ms | Limiter wait p50/p95 ms | Call p50/p95 ms |\n")
		sb.WriteString("|---|---:|---:|---:|---:|---:|---:|\n")
		keys := make([]string, 0, len(r.Snapshot.EndpointLatency))
		for k := range r.Snapshot.EndpointLatency {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			e := r.Snapshot.EndpointLatency[k]
			sb.WriteString(fmt.Sprintf("| `%s` | %d | %d | %d | %d / %d / %d | %d / %d | %d / %d |\n",
				k, e.Calls, e.Attempts, e.Failed, e.NetP50Ms, e.NetP95Ms, e.NetMaxMs, e.LimiterWaitP50Ms, e.LimiterWaitP95Ms, e.CallP50Ms, e.CallP95Ms))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## 4. 实时信号\n\n")
	sb.WriteString("| # | Symbol | Dir | Setup | Status (module) | Assembled tier | Prompt-final tier | Priority | Timing | Risk | Entry | Reasons |\n")
	sb.WriteString("|---:|---|---|---|---|---|---|---:|---:|---:|---|---|\n")
	assembledTiers, finalTiers := r.TierLayers.tierMaps()
	for i, sig := range r.Signals {
		if i >= 30 {
			break
		}
		at, ft := signalTierColumns(sig.Symbol, string(sig.SetupType), assembledTiers, finalTiers)
		sb.WriteString(fmt.Sprintf("| %d | %s | %s | `%s` | `%s` | %s | %s | %.1f | %.1f | %.1f | `%s` | %s |\n",
			i+1, sig.Symbol, sig.Direction, sig.SetupType, sig.Status, at, ft, sig.AIPriority,
			sig.TimingScore, sig.RiskScore, sig.EntryMode, strings.Join(sig.ReasonCodes, ", ")))
	}
	sb.WriteString("\n")

	sb.WriteString("## 5. 机会覆盖\n\n")
	sb.WriteString(fmt.Sprintf("- setup 分布：%s\n", sortedMap(r.OpportunityCover.BySetupType)))
	sb.WriteString(fmt.Sprintf("- status 分布 [module status]：%s\n", sortedMap(r.OpportunityCover.ByStatus)))
	sb.WriteString(fmt.Sprintf("- entry_mode 分布：%s\n", sortedMap(r.OpportunityCover.ByEntryMode)))
	sb.WriteString(fmt.Sprintf("- runtime tier 分布（后端初筛）[assembled tier, re-assembled per signal]：%s\n", sortedMap(r.OpportunityCover.ByExecutionTier)))
	sb.WriteString(fmt.Sprintf("- prompt-final tier 分布（AIT 最终可执行口径）[prompt-final tier, parsed from the prompt]：%s\n", sortedMap(r.AIRecognition.PromptTierCounts)))
	sb.WriteString(fmt.Sprintf("- 潜力池：top=%d，未命中模块=%d\n", r.OpportunityCover.PotentialPoolCount, r.OpportunityCover.UnmatchedPotentialCount))
	sb.WriteString(fmt.Sprintf("- 覆盖家族：momentum=%v, reversal=%v, squeeze=%v, range=%v, funding=%v, accumulation=%v, distribution=%v\n\n",
		r.OpportunityCover.HasMomentum, r.OpportunityCover.HasReversal, r.OpportunityCover.HasSqueeze,
		r.OpportunityCover.HasRange, r.OpportunityCover.HasFunding, r.OpportunityCover.HasAccumulation, r.OpportunityCover.HasDistribution))

	sb.WriteString(tierLayerMarkdown(r.TierLayers))

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
	if r.Snapshot.Degraded {
		fmt.Printf("snapshot_degraded: true reasons=%s\n", strings.Join(r.Snapshot.DegradationReasons, ","))
	}
	fmt.Printf("signals: total=%d long=%d short=%d setups=%s\n",
		r.OpportunityCover.SignalCount, r.OpportunityCover.LongCount, r.OpportunityCover.ShortCount, sortedMap(r.OpportunityCover.BySetupType))
	fmt.Printf("potential_pool: top=%d unmatched=%d\n", r.OpportunityCover.PotentialPoolCount, r.OpportunityCover.UnmatchedPotentialCount)
	fmt.Printf("assembled tiers (DB execution_tier): %s\n", sortedMap(r.TierLayers.AssembledCounts))
	fmt.Printf("prompt-final tiers (LLM; DB prompt_final_tier): %s (promoted %d, demoted %d)\n",
		sortedMap(r.TierLayers.PromptFinalCounts), len(r.TierLayers.Promoted), len(r.TierLayers.Demoted))
	fmt.Printf("format: json=%v/%v missing=%d executable_gaps=%d prompt_v7_json=%v\n",
		r.FormatCheck.JSONMarshalOK, r.FormatCheck.JSONUnmarshalOK, r.FormatCheck.MissingFieldCount,
		r.FormatCheck.ExecutableGapCount, r.AIRecognition.PromptContainsV7JSON)
	fmt.Printf("issues: %d\n", len(r.Issues))
	fmt.Printf("raw: %s\nreport: %s\nprompt: %s\n", rawPath, mdPath, r.PromptPreviewPath)
}
