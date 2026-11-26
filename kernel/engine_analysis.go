package kernel

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Aixxww/AiT/logger"
	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/mcp"
	"github.com/Aixxww/AiT/store"
)

// ============================================================================
// Pre-compiled regular expressions (performance optimization)
// ============================================================================

var (
	// Safe regex: precisely match ```json code blocks
	reJSONFence      = regexp.MustCompile(`(?is)` + "```json\\s*(\\[\\s*\\{.*?\\}\\s*\\])\\s*```")
	reJSONArray      = regexp.MustCompile(`(?is)\[\s*\{.*?\}\s*\]`)
	reArrayHead      = regexp.MustCompile(`^\[\s*\{`)
	reArrayOpenSpace = regexp.MustCompile(`^\[\s+\{`)
	reInvisibleRunes = regexp.MustCompile("[\u200B\u200C\u200D\uFEFF]")

	// XML tag extraction (supports any characters in reasoning chain)
	reReasoningTag = regexp.MustCompile(`(?s)<reasoning>(.*?)</reasoning>`)
	reDecisionTag  = regexp.MustCompile(`(?s)<decision>(.*?)</decision>`)
	// reEmptyJSONArray matches a decision payload that is exactly `[]`.
	reEmptyJSONArray = regexp.MustCompile(`^\s*\[\s*\]\s*$`)
)

// ============================================================================
// Entry Functions - Main API
// ============================================================================

// GetFullDecision gets AI's complete trading decision (batch analysis of all coins and positions)
// Uses default strategy configuration - for production use GetFullDecisionWithStrategy with explicit config
func GetFullDecision(ctx *Context, mcpClient mcp.AIClient) (*FullDecision, error) {
	defaultConfig := store.GetDefaultStrategyConfig("en")
	engine := NewStrategyEngine(&defaultConfig)
	return GetFullDecisionWithStrategy(ctx, mcpClient, engine, "")
}

// GetFullDecisionWithStrategy uses StrategyEngine to get AI decision (unified prompt generation)
func GetFullDecisionWithStrategy(ctx *Context, mcpClient mcp.AIClient, engine *StrategyEngine, variant string) (*FullDecision, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context is nil")
	}
	if engine == nil {
		defaultConfig := store.GetDefaultStrategyConfig("en")
		engine = NewStrategyEngine(&defaultConfig)
	}

	// Clamp strategy limits to prevent token overflow
	engineConfig := engine.GetConfig()
	engineConfig.ClampLimits()

	// Token estimation check — block if exceeding the specific model's context limit
	estimate := engineConfig.EstimateTokens()

	// Determine context limit for the specific model being used
	contextLimit := 131072 // safe default (strictest common limit)
	var providerName string
	if embedder, ok := mcpClient.(mcp.ClientEmbedder); ok {
		base := embedder.BaseClient()
		providerName = base.Provider
		contextLimit = store.GetContextLimitForClient(base.Provider, base.Model)
	}

	if estimate.Total > contextLimit {
		logger.Errorf("🚫 Token estimate %d exceeds %s context limit %d — blocking analysis",
			estimate.Total, providerName, contextLimit)
		return nil, fmt.Errorf("estimated %d tokens exceeds model context limit of %d; reduce coins, timeframes, or K-line count",
			estimate.Total, contextLimit)
	}
	if estimate.Total*100/contextLimit >= 80 {
		logger.Infof("⚠️  Token estimate %d — approaching %s context limit %d",
			estimate.Total, providerName, contextLimit)
	}

	// Propagate market environment classification (ADX-based regime)
	if engine.marketEnv != nil {
		ctx.MarketEnv = engine.marketEnv
	}

	// 1. Fetch market data using strategy config
	if len(ctx.MarketDataMap) == 0 {
		if err := fetchMarketDataWithStrategy(ctx, engine); err != nil {
			return nil, fmt.Errorf("failed to fetch market data: %w", err)
		}
	}

	// Ensure OITopDataMap is initialized
	if ctx.OITopDataMap == nil {
		ctx.OITopDataMap = make(map[string]*OITopData)
		oiPositions, err := engine.aitosClient.GetOITopPositions()
		if err == nil {
			for _, pos := range oiPositions {
				ctx.OITopDataMap[pos.Symbol] = &OITopData{
					Rank:              pos.Rank,
					OIDeltaPercent:    pos.OIDeltaPercent,
					OIDeltaValue:      pos.OIDeltaValue,
					PriceDeltaPercent: pos.PriceDeltaPercent,
				}
			}
		}
	}

	// 2. Build System Prompt using strategy engine
	riskConfig := engine.GetRiskControlConfig()
	systemPrompt := engine.BuildSystemPrompt(ctx.Account.TotalEquity, variant)

	// 3. Build User Prompt using strategy engine
	userPrompt := engine.BuildUserPrompt(ctx)

	// #111 S2: the Hunter v7 prompt contract fails the round closed before
	// the AI call (one wait, backend_guard_risk; not an AI failure).
	if err := engine.hunterV7PromptContractError(systemPrompt, userPrompt); err != nil {
		logger.Errorf("🚫 [%s] %v; round not sent to the AI", hunterV7PromptContractMismatch, err)
		decision := hunterV7PromptContractMismatchDecision(err, systemPrompt, userPrompt)
		decision.Timestamp = time.Now()
		return decision, nil
	}

	// 4. Call AI API
	aiCallStart := time.Now()
	aiResponse, err := callStrategyDecisionAI(mcpClient, engine, systemPrompt, userPrompt)
	aiCallDuration := time.Since(aiCallStart)
	if err != nil {
		return nil, fmt.Errorf("AI API call failed: %w", err)
	}

	// 5. Parse AI response
	// Build Hunter score map for cross-validation
	hunterScoreMap := make(map[string]HunterScoreInfo)
	for _, coin := range ctx.CandidateCoins {
		normalizedSym := market.Normalize(coin.Symbol)
		hunterScoreMap[normalizedSym] = HunterScoreInfo{
			LongScore:  coin.LongScore,
			ShortScore: coin.ShortScore,
			Direction:  coin.Direction,
		}
	}

	// P0-2: build a symbol -> current market price map so the RR validation
	// uses the real entry price instead of a fabricated one.
	priceMap := make(map[string]float64, len(ctx.MarketDataMap))
	for sym, md := range ctx.MarketDataMap {
		if md != nil && md.CurrentPrice > 0 {
			priceMap[market.Normalize(sym)] = md.CurrentPrice
		}
	}

	// #95: the RR floor comes from the resolver; an unset field resolves to
	// store.DefaultMinRiskRewardRatio (1.2), never to the legacy 3.0.
	resolvedMinRR := store.ResolveHunterV7(engine.GetConfig()).MinRR
	rrFloor := FixedDecisionRRFloor(resolvedMinRR)
	if strings.EqualFold(engine.GetConfig().CoinSource.SourceType, "hunter_v7") {
		rrFloor = hunterV7DecisionRRFloor(resolvedMinRR, ctx.CandidateCoins)
	}
	// #104 / AUD-B2: the prompt-final (B7) tier counts gate
	// no_reviewable_candidate in the validator.
	tierCounts := unknownHunterV7TierCounts
	if strings.EqualFold(engine.GetConfig().CoinSource.SourceType, "hunter_v7") {
		tierCounts = hunterV7PromptFinalTierCounts(ctx.HunterV7PromptFinal)
	}
	decision, err := parseFullDecisionResponseWithTiers(
		aiResponse,
		ctx.Account.TotalEquity,
		riskConfig.BTCETHMaxLeverage,
		riskConfig.AltcoinMaxLeverage,
		riskConfig.BTCETHMaxPositionValueRatio,
		riskConfig.AltcoinMaxPositionValueRatio,
		hunterScoreMap,
		strings.EqualFold(engine.GetConfig().CoinSource.SourceType, "hunter_v7"),
		priceMap,
		rrFloor,
		riskConfig.MinConfidence, // #77: the configured min-open confidence
		tierCounts,
		ctx.CandidateCoins, // #106: hydrated before validation
	)

	if decision != nil {
		if strings.EqualFold(engine.GetConfig().CoinSource.SourceType, "hunter_v7") {
			// #94 / AUD-B8: X2 instrumentation (rank of the pick, E+R count).
			decision.HunterV7ERCount, decision.HunterV7ChosenRank = annotateHunterV7ChosenRank(decision.Decisions, ctx.HunterV7PromptFinal)
		}
		decision.Timestamp = time.Now()
		decision.SystemPrompt = systemPrompt
		decision.UserPrompt = userPrompt
		decision.AIRequestDurationMs = aiCallDuration.Milliseconds()
		decision.RawResponse = aiResponse
		// Extract token usage from MCP client
		if embedder, ok := mcpClient.(mcp.ClientEmbedder); ok {
			base := embedder.BaseClient()
			decision.PromptTokens = base.LastUsage.PromptTokens
			decision.CompletionTokens = base.LastUsage.CompletionTokens
			decision.TotalTokens = base.LastUsage.TotalTokens
		}
		// Fallback: estimate tokens if API didn't return usage data
		if decision.TotalTokens == 0 {
			decision.PromptTokens = len(systemPrompt)/4 + len(userPrompt)/4
			decision.CompletionTokens = len(aiResponse) / 4
			decision.TotalTokens = decision.PromptTokens + decision.CompletionTokens
		}
	}

	if err != nil {
		return decision, fmt.Errorf("failed to parse AI response: %w", err)
	}

	return decision, nil
}

// hydrateHunterV7DecisionExecutionPlans copies price targets only from the
// backend candidate selected by signal ID. The model never supplies these
// fields, preventing a free-form response from changing the protector plan.
// #106 / AUD-B5: it runs inside parseFullDecisionResponseWithTiers BEFORE
// validation, so the RR check sees the backend trigger.
func hydrateHunterV7DecisionExecutionPlans(decisions []Decision, candidates []CandidateCoin) {
	byID := make(map[string]CandidateCoin, len(candidates))
	for _, candidate := range candidates {
		if candidate.V7SignalID != "" {
			byID[candidate.V7SignalID] = candidate
		}
	}
	for i := range decisions {
		decision := &decisions[i]
		resetHunterV7ExecutionPlan(decision)
		if decision.Action != "open_long" && decision.Action != "open_short" {
			continue
		}
		candidate, ok := byID[decision.SelectedHunterV7SignalID]
		if !ok {
			continue
		}
		decision.HunterV7MinRR = candidate.V7RequiredRR
		decision.HunterV7MinStopPct = candidate.V7RequiredMinStop
		decision.HunterV7MaxTPPct = candidate.V7RequiredMaxTP
		decision.HunterV7MaxDriftPct = candidate.V7RequiredMaxDrift
		decision.HunterV7DriftReference = candidate.V7DriftReference
		decision.HunterV7TriggerPrice = candidate.V7TriggerPrice
		decision.HunterV7ConditionalEntry = candidate.V7ConditionalEntry
		decision.HunterV7InvalidationPrice = candidate.V7Invalidation.Price
		decision.HunterV7TP0Price = candidate.V7TP0Price
		decision.HunterV7TP1Price = candidate.V7TP1Price
		decision.HunterV7TP2Price = candidate.V7TP2Price
	}
}

// resetHunterV7ExecutionPlan clears the backend plan fields that
// hydrateHunterV7DecisionExecutionPlans owns, for a decision that is not an
// open on a known signal (or was demoted to wait after hydration).
func resetHunterV7ExecutionPlan(decision *Decision) {
	decision.HunterV7MinRR = 0
	decision.HunterV7MinStopPct = 0
	decision.HunterV7MaxTPPct = 0
	decision.HunterV7MaxDriftPct, decision.HunterV7DriftReference, decision.HunterV7TriggerPrice = 0, "", 0
	decision.HunterV7ConditionalEntry, decision.HunterV7InvalidationPrice, decision.HunterV7IntentKey = false, 0, ""
}

const hunterV7DecisionMaxOutputTokens = 1600

// hunterV7DecisionTemperature: 开仓决策降温到 0.15，提升可复现性——
// 每次 AI 开仓决策是单次采样，全链路随机性的唯一入口。
// 全局默认 MCPClientTemperature=0.5 保持不动（Telegram agent 等其他调用仍用它）。
const hunterV7DecisionTemperature = 0.15

// buildHunterV7DecisionRequest 构造 hunter_v7 开仓决策请求（降温版）。
func buildHunterV7DecisionRequest(systemPrompt, userPrompt string) (*mcp.Request, error) {
	return mcp.NewRequestBuilder().
		WithSystemPrompt(systemPrompt).
		WithUserPrompt(userPrompt).
		WithTemperature(hunterV7DecisionTemperature).
		WithMaxTokens(hunterV7DecisionMaxOutputTokens).
		// Some gateways (WorkBuddy, issue #68) reject non-stream chat requests.
		WithStream(true).
		Build()
}

// callStrategyDecisionAI sends the decision call as a streaming request for every
// provider that speaks OpenAI chat.completion SSE. Providers that cannot (Claude,
// claw402) keep their previous non-stream call; see mcp.SupportsChatCompletionStream.
func callStrategyDecisionAI(mcpClient mcp.AIClient, engine *StrategyEngine, systemPrompt, userPrompt string) (string, error) {
	if engine != nil && strings.EqualFold(engine.GetConfig().CoinSource.SourceType, "hunter_v7") {
		req, err := buildHunterV7DecisionRequest(systemPrompt, userPrompt)
		if err == nil {
			logger.Infof("⚡ Hunter v7 decision output capped at %d tokens to reduce execution drift", hunterV7DecisionMaxOutputTokens)
			return mcp.CallWithRequestPreferStream(mcpClient, req)
		}
		logger.Warnf("⚠️ Hunter v7 request builder failed, falling back to default call: %v", err)
	}
	return mcp.CallWithMessagesPreferStream(mcpClient, systemPrompt, userPrompt)
}

// ============================================================================
// Market Data Fetching
// ============================================================================

// fetchMarketDataWithStrategy fetches market data using strategy config (multiple timeframes)
func fetchMarketDataWithStrategy(ctx *Context, engine *StrategyEngine) error {
	config := engine.GetConfig()
	ctx.MarketDataMap = make(map[string]*market.Data)

	timeframes := config.Indicators.Klines.SelectedTimeframes
	primaryTimeframe := config.Indicators.Klines.PrimaryTimeframe
	klineCount := config.Indicators.Klines.PrimaryCount

	// Compatible with old configuration
	if len(timeframes) == 0 {
		if primaryTimeframe != "" {
			timeframes = append(timeframes, primaryTimeframe)
		} else {
			timeframes = append(timeframes, "3m")
		}
		if config.Indicators.Klines.LongerTimeframe != "" {
			timeframes = append(timeframes, config.Indicators.Klines.LongerTimeframe)
		}
	}
	if primaryTimeframe == "" {
		primaryTimeframe = timeframes[0]
	}
	if klineCount <= 0 {
		klineCount = 30
	}
	// U11: the hybrid_v2 payload prints BarsPerTimeframe closed bars on
	// each of its timeframes plus the unfinished bar, so it fetches those
	// timeframes and one bar more. Legacy (the default) is unchanged.
	if engine.hunterV7PromptHybridV2() {
		pc := engine.hunterV7PromptConfig()
		timeframes = hunterV7MergeTimeframes(timeframes, pc.Timeframes)
		if klineCount < pc.BarsPerTimeframe+1 {
			klineCount = pc.BarsPerTimeframe + 1
		}
	}

	logger.Infof("📊 Strategy timeframes: %v, Primary: %s, Kline count: %d", timeframes, primaryTimeframe, klineCount)

	// 1. First fetch data for position coins (must fetch)
	for _, pos := range ctx.Positions {
		data, err := market.GetWithTimeframes(pos.Symbol, timeframes, primaryTimeframe, klineCount)
		if err != nil {
			logger.Infof("⚠️  Failed to fetch market data for position %s: %v", pos.Symbol, err)
			continue
		}
		ctx.MarketDataMap[pos.Symbol] = data
	}

	// 2. Fetch data for all candidate coins
	positionSymbols := make(map[string]bool)
	for _, pos := range ctx.Positions {
		positionSymbols[pos.Symbol] = true
	}

	// OI threshold: use HunterConfig value if set, otherwise default 15M.
	// Hunter v7 already scores liquidity from the shared snapshot universe, so
	// keep this secondary prompt-data gate lighter to avoid dropping its top
	// executable candidate after routing.
	minOIThresholdMillions := 15.0
	if config.CoinSource.Hunter != nil {
		if config.CoinSource.Hunter.MinOIValue > 0 {
			minOIThresholdMillions = config.CoinSource.Hunter.MinOIValue / 1_000_000
		} else if strings.EqualFold(config.CoinSource.SourceType, "hunter_v7") {
			minOIThresholdMillions = 3.0
		} else {
			// Hunter exists but MinOIValue not set — use Hunter's own default ($5M)
			minOIThresholdMillions = 5.0
		}
	}

	for _, coin := range ctx.CandidateCoins {
		if _, exists := ctx.MarketDataMap[coin.Symbol]; exists {
			continue
		}

		var data *market.Data
		var err error

		if engine.GetSnapshotEngine() != nil {
			// Prefer the shared SnapshotStore used by SnapshotEngine/Hunter v7.
			// This keeps AI prompt construction from re-fetching candidate market
			// data that was already collected for scoring.
			data, err = buildMarketDataFromSnapshot(engine, coin.Symbol, timeframes, primaryTimeframe, klineCount)
			if err != nil {
				logger.Infof("⚠️  Snapshot data unavailable for %s, trying pre-fetched/fallback path: %v", coin.Symbol, err)
				data = nil
			} else {
				logger.Infof("✅ %s: using SnapshotStore market data", coin.Symbol)
			}
		}

		// Try Hunter pre-fetched klines first (avoids duplicate Binance API calls)
		if data == nil && coin.PreFetchedData != nil && len(coin.PreFetchedData.TimeframeKlines) > 0 {
			data, err = market.BuildDataFromPreFetched(coin.Symbol, coin.PreFetchedData, timeframes, primaryTimeframe, klineCount)
			if err != nil {
				logger.Infof("⚠️  Pre-fetched data failed for %s, falling back to Binance: %v", coin.Symbol, err)
				data = nil
			} else {
				// Supplement OI and FundingRate (not included in pre-fetched data)
				oiData, oiErr := market.GetOIFromBinance(coin.Symbol)
				if oiErr == nil && oiData != nil {
					data.OpenInterest = oiData
				}
				fr, frErr := market.GetFRFromBinance(coin.Symbol)
				if frErr == nil {
					data.FundingRate = fr
				}
				logger.Infof("✅ %s: using Hunter pre-fetched klines (saved %d Binance calls)", coin.Symbol, len(timeframes))
			}
		}

		// Fallback: fetch from Binance/CoinAnk
		if data == nil {
			data, err = market.GetWithTimeframes(coin.Symbol, timeframes, primaryTimeframe, klineCount)
			if err != nil {
				logger.Infof("⚠️  Failed to fetch market data for %s: %v", coin.Symbol, err)
				continue
			}
		}

		// Liquidity filter (skip for xyz dex assets - they don't have OI data from Binance)
		isExistingPosition := positionSymbols[coin.Symbol]
		isXyzAsset := market.IsXyzDexAsset(coin.Symbol)
		if !isExistingPosition && !isXyzAsset && data.OpenInterest != nil && data.CurrentPrice > 0 {
			effectiveMinOIThreshold := minOIThresholdMillions
			if strings.EqualFold(config.CoinSource.SourceType, "hunter_v7") {
				// Adaptive OI threshold: max(accountMaxNotional*1000, symbolMinFloor, quoteVolume24h*0.002)
				riskControl := engine.GetRiskControlConfig()
				maxPosRatio := riskControl.AltcoinMaxPositionValueRatio
				if coin.Symbol == "BTCUSDT" || coin.Symbol == "ETHUSDT" {
					maxPosRatio = riskControl.BTCETHMaxPositionValueRatio
				}
				accountMaxNotional := ctx.Account.TotalEquity * maxPosRatio
				accountBasedOI := accountMaxNotional * 1000 / 1e6       // in millions
				const symbolMinNotionalFloor = 0.5                      // $500K floor
				volumeBasedMinOI := coin.V7QuoteVolume24h * 0.002 / 1e6 // 0.2% of 24h volume

				adaptiveThreshold := accountBasedOI
				if symbolMinNotionalFloor > adaptiveThreshold {
					adaptiveThreshold = symbolMinNotionalFloor
				}
				if volumeBasedMinOI > adaptiveThreshold {
					adaptiveThreshold = volumeBasedMinOI
				}

				if coin.V7ExecutionTier == "EXECUTABLE" || coin.V7ExecutionTier == "REVIEWABLE" {
					// EXECUTABLE: full adaptive threshold (min 1.0M)
					// REVIEWABLE: 60% of adaptive threshold (min 0.5M)
					tierThreshold := adaptiveThreshold
					if coin.V7ExecutionTier == "REVIEWABLE" {
						tierThreshold = adaptiveThreshold * 0.6
					}
					minFloor := 1.0
					if coin.V7ExecutionTier == "REVIEWABLE" {
						minFloor = 0.5
					}
					if tierThreshold < minFloor {
						tierThreshold = minFloor
					}
					effectiveMinOIThreshold = tierThreshold
				} else {
					// Non-executable/reviewable (watch/context): use adaptive as cap
					if effectiveMinOIThreshold > adaptiveThreshold {
						effectiveMinOIThreshold = adaptiveThreshold
					}
				}
			}
			oiValue := data.OpenInterest.Latest * data.CurrentPrice
			oiValueInMillions := oiValue / 1_000_000
			if oiValueInMillions < effectiveMinOIThreshold {
				logger.Infof("⚠️  %s OI value too low (%.2fM USD < %.2fM adaptive), skipping coin",
					coin.Symbol, oiValueInMillions, effectiveMinOIThreshold)
				continue
			}
		}

		// Pre-filter: skip coins with very low Hunter scores to save AI tokens (~2600 tokens per coin)
		if !isExistingPosition {
			selectedScore := coin.LongScore
			if coin.Direction == "SHORT" {
				selectedScore = coin.ShortScore
			}
			const preFilterThreshold = 18.0
			if selectedScore > 0 && selectedScore < preFilterThreshold {
				logger.Infof("🔽 Pre-filter: %s Hunter score %.1f < %.1f, skipping AI analysis (saves ~2600 tokens)",
					coin.Symbol, selectedScore, preFilterThreshold)
				continue
			}
		}

		ctx.MarketDataMap[coin.Symbol] = data
	}

	logger.Infof("📊 Successfully fetched multi-timeframe market data for %d coins", len(ctx.MarketDataMap))

	// #87 L4: fresh price + fast indicators for the top Hunter v7
	// candidates right before the prompt (and its re-tier) is built.
	refreshHunterV7Finalists(ctx, engine, timeframes, primaryTimeframe, klineCount)
	return nil
}

func buildMarketDataFromSnapshot(engine *StrategyEngine, symbol string, timeframes []string, primaryTimeframe string, klineCount int) (*market.Data, error) {
	if engine == nil || engine.GetSnapshotEngine() == nil {
		return nil, fmt.Errorf("snapshot engine not configured")
	}
	snap := engine.GetSnapshotEngine().GetSnapshot()
	if snap == nil || len(snap.Symbols) == 0 {
		return nil, fmt.Errorf("snapshot is empty")
	}

	normalized := market.Normalize(symbol)
	ss := snap.Symbols[normalized]
	if ss == nil {
		ss = snap.Symbols[symbol]
	}
	if ss == nil {
		return nil, fmt.Errorf("%s not found in snapshot", normalized)
	}

	return market.BuildDataFromSymbolSnapshot(normalized, ss, timeframes, primaryTimeframe, klineCount)
}

// ============================================================================
// AI Response Parsing
// ============================================================================

// hunterV7TierCounts are the prompt-final EXECUTABLE / REVIEWABLE counts of
// a round (#104 / AUD-B2). Known=false means the counts are not available
// (non-hunter_v7 callers, tests); the tier check is then skipped.
type hunterV7TierCounts struct {
	Known      bool
	Executable int
	Reviewable int
}

var unknownHunterV7TierCounts = hunterV7TierCounts{}

// hunterV7PromptFinalTierCounts counts the B7 prompt-final tiers.
func hunterV7PromptFinalTierCounts(final []HunterV7PromptFinalTier) hunterV7TierCounts {
	c := hunterV7TierCounts{Known: true}
	for _, t := range final {
		switch strings.ToUpper(t.Tier) {
		case "EXECUTABLE":
			c.Executable++
		case "REVIEWABLE":
			c.Reviewable++
		}
	}
	return c
}

func parseFullDecisionResponse(aiResponse string, accountEquity float64, btcEthLeverage, altcoinLeverage int, btcEthPosRatio, altcoinPosRatio float64, hunterScoreMap map[string]HunterScoreInfo, hunterV7Mode bool, priceMap map[string]float64, rrFloor DecisionRRFloor, minOpenConfidence int) (*FullDecision, error) {
	return parseFullDecisionResponseWithTiers(aiResponse, accountEquity, btcEthLeverage, altcoinLeverage, btcEthPosRatio, altcoinPosRatio, hunterScoreMap, hunterV7Mode, priceMap, rrFloor, minOpenConfidence, unknownHunterV7TierCounts, nil)
}

// parseFullDecisionResponseWithTiers is parseFullDecisionResponse with the
// round's prompt-final tier counts for the wait-code check (#104) and, in
// hunter_v7 mode, the round's candidates: their backend execution plan
// (trigger price, conditional entry, ...) is hydrated onto the decisions
// BEFORE validation, so the RR check measures from the backend trigger
// (#106 / AUD-B5).
func parseFullDecisionResponseWithTiers(aiResponse string, accountEquity float64, btcEthLeverage, altcoinLeverage int, btcEthPosRatio, altcoinPosRatio float64, hunterScoreMap map[string]HunterScoreInfo, hunterV7Mode bool, priceMap map[string]float64, rrFloor DecisionRRFloor, minOpenConfidence int, tiers hunterV7TierCounts, candidates []CandidateCoin) (*FullDecision, error) {
	cotTrace := extractCoTTrace(aiResponse)

	decisions, err := extractDecisions(aiResponse)
	if err != nil {
		return &FullDecision{
			CoTTrace:  cotTrace,
			Decisions: []Decision{},
		}, fmt.Errorf("failed to extract decisions: %w", err)
	}

	normalizeWhyThisCouldFail(decisions) // #111 S3: opens only, bounded
	if hunterV7Mode {
		normalizeHunterV7WaitReasons(decisions)
		hydrateHunterV7DecisionExecutionPlans(decisions, candidates)
	}
	// F06: validate each decision on its own. A decision that fails is
	// demoted to wait (backend_guard_risk) and the rest of the batch still
	// executes; validation failures are no longer returned as an error, so
	// they no longer count as AI failures or feed safe mode. Only an
	// unparseable response (above) is an AI failure.
	demoteInvalidDecisions(decisions, accountEquity, btcEthLeverage, altcoinLeverage, btcEthPosRatio, altcoinPosRatio, hunterScoreMap, priceMap, rrFloor, hunterV7Mode, minOpenConfidence, tiers)
	fillMissingDecisionReasoning(decisions, cotTrace)

	return &FullDecision{
		CoTTrace:  cotTrace,
		Decisions: decisions,
	}, nil
}

var hunterV7BlockedReasonCodes = map[string]bool{
	"entry_not_in_zone":       true,
	"rr_insufficient":         true,
	"confirmation_missing":    true,
	"oi_too_low":              true,
	"funding_crowded":         true,
	"account_risk":            true,
	"backend_guard_risk":      true,
	"no_reviewable_candidate": true,
}

func normalizeHunterV7WaitReasons(decisions []Decision) {
	for i := range decisions {
		if decisions[i].Action != "wait" {
			continue
		}
		decisions[i].BlockedReasonCode = strings.TrimSpace(decisions[i].BlockedReasonCode)
		if decisions[i].BlockedReasonCode == "" && decisions[i].Symbol == "ALL" && strings.Contains(decisions[i].Reasoning, "Model didn't output structured JSON decision") {
			decisions[i].BlockedReasonCode = "backend_guard_risk"
		}
	}
}

// demoteInvalidDecisions replaces every decision that fails validation with
// a wait carrying blocked_reason_code=backend_guard_risk, recording the
// original action and the validator message. It returns the number demoted.
func demoteInvalidDecisions(decisions []Decision, accountEquity float64, btcEthLeverage, altcoinLeverage int, btcEthPosRatio, altcoinPosRatio float64, hunterScoreMap map[string]HunterScoreInfo, priceMap map[string]float64, rrFloor DecisionRRFloor, hunterV7Mode bool, minOpenConfidence int, tiers hunterV7TierCounts) int {
	demoted := 0
	for i := range decisions {
		err := validateDecision(&decisions[i], accountEquity, btcEthLeverage, altcoinLeverage, btcEthPosRatio, altcoinPosRatio, hunterScoreMap, priceMap, rrFloor, minOpenConfidence)
		if err == nil && hunterV7Mode {
			err = validateHunterV7WaitReason(decisions[i], tiers)
		}
		if err == nil {
			continue
		}
		d := &decisions[i]
		logger.Warnf("⚠️ decision #%d %s %s rejected by validator, demoted to wait: %v", i+1, d.Symbol, d.Action, err)
		d.RejectedAction = d.Action
		d.ValidatorRejection = err.Error()
		d.Action = "wait"
		d.BlockedReasonCode = "backend_guard_risk"
		// #106: a demoted decision carries no backend execution plan, as when
		// hydration ran after validation. The TP ladder is cleared too: it is
		// the plan of an open that is no longer executed.
		resetHunterV7ExecutionPlan(d)
		d.HunterV7TP0Price, d.HunterV7TP1Price, d.HunterV7TP2Price = 0, 0, 0
		if d.BlockedSignalSymbol == "" && d.Symbol != "" && d.Symbol != "ALL" {
			d.BlockedSignalSymbol = d.Symbol
		}
		demoted++
	}
	return demoted
}

func validateHunterV7WaitReason(d Decision, tiers hunterV7TierCounts) error {
	if d.Action != "wait" {
		return nil
	}
	if d.BlockedReasonCode == "" {
		return fmt.Errorf("wait missing blocked_reason_code")
	}
	if !hunterV7BlockedReasonCodes[d.BlockedReasonCode] {
		return fmt.Errorf("invalid blocked_reason_code: %s", d.BlockedReasonCode)
	}
	// #104 / AUD-B2: no_reviewable_candidate is true only when the prompt
	// showed no EXECUTABLE and no REVIEWABLE candidate.
	if d.BlockedReasonCode == "no_reviewable_candidate" && tiers.Known && tiers.Executable+tiers.Reviewable > 0 {
		return fmt.Errorf("blocked_reason_code no_reviewable_candidate contradicts the prompt tiers (EXECUTABLE=%d REVIEWABLE=%d)", tiers.Executable, tiers.Reviewable)
	}
	return nil
}

func fillMissingDecisionReasoning(decisions []Decision, cotTrace string) {
	reason := compactDecisionReasoning(cotTrace, 240)
	if reason == "" {
		return
	}
	for i := range decisions {
		if strings.TrimSpace(decisions[i].Reasoning) == "" {
			decisions[i].Reasoning = reason
		}
	}
}

func compactDecisionReasoning(reason string, maxLen int) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"\r\n", " ",
		"\n", " ",
		"\t", " ",
		"**", "",
		"`", "",
	)
	reason = replacer.Replace(reason)
	reason = strings.Join(strings.Fields(reason), " ")
	if maxLen > 0 && len([]rune(reason)) > maxLen {
		runes := []rune(reason)
		reason = string(runes[:maxLen]) + "..."
	}
	return reason
}

func extractCoTTrace(response string) string {
	if match := reReasoningTag.FindStringSubmatch(response); match != nil && len(match) > 1 {
		logger.Infof("✓ Extracted reasoning chain using <reasoning> tag")
		return strings.TrimSpace(match[1])
	}

	if decisionIdx := strings.Index(response, "<decision>"); decisionIdx > 0 {
		logger.Infof("✓ Extracted content before <decision> tag as reasoning chain")
		return strings.TrimSpace(response[:decisionIdx])
	}

	jsonStart := strings.Index(response, "[")
	if jsonStart > 0 {
		logger.Infof("⚠️  Extracted reasoning chain using old format ([ character separator)")
		return strings.TrimSpace(response[:jsonStart])
	}

	return strings.TrimSpace(response)
}

// AI failure reasons (#94). They are AI failures: they count toward safe
// mode like a failed call, and the trader records the reason code.
const (
	AIFailureNoStructuredDecision   = "no_structured_decision"
	AIFailureMultipleDecisionBlocks = "multiple_decision_blocks"
)

var (
	// ErrNoStructuredDecision: the reply contains no decision JSON (AUD-B1).
	ErrNoStructuredDecision = errors.New(AIFailureNoStructuredDecision)
	// ErrMultipleDecisionBlocks: the reply contains more than one
	// <decision> block (AUD-B3).
	ErrMultipleDecisionBlocks = errors.New(AIFailureMultipleDecisionBlocks)
)

// AIFailureReason returns the #94 reason code carried by an AI decision
// error, or "" for other errors.
func AIFailureReason(err error) string {
	switch {
	case errors.Is(err, ErrNoStructuredDecision):
		return AIFailureNoStructuredDecision
	case errors.Is(err, ErrMultipleDecisionBlocks):
		return AIFailureMultipleDecisionBlocks
	}
	return ""
}

func extractDecisions(response string) ([]Decision, error) {
	s := removeInvisibleRunes(response)
	s = sanitizeWhyThisCouldFail(s) // #111 S3: fail-soft free-text field
	s = strings.TrimSpace(s)
	s = fixMissingQuotes(s)

	var jsonPart string
	if blocks := reDecisionTag.FindAllStringSubmatch(s, -1); len(blocks) > 1 {
		// #94 / AUD-B3: an example block in the reasoning must never be
		// executed in place of the real one; reject the reply, log every block.
		for i, b := range blocks {
			logger.Warnf("⚠  [%s] <decision> block %d of %d: %s", AIFailureMultipleDecisionBlocks, i+1, len(blocks), strings.TrimSpace(b[1]))
		}
		return nil, fmt.Errorf("%w: %d blocks", ErrMultipleDecisionBlocks, len(blocks))
	} else if len(blocks) == 1 && len(blocks[0]) > 1 {
		jsonPart = strings.TrimSpace(blocks[0][1])
		logger.Infof("✓ Extracted JSON using <decision> tag")
	} else {
		jsonPart = s
		logger.Infof("⚠️  <decision> tag not found, searching JSON in full text")
	}

	jsonPart = fixMissingQuotes(jsonPart)

	if m := reJSONFence.FindStringSubmatch(jsonPart); m != nil && len(m) > 1 {
		jsonContent := strings.TrimSpace(m[1])
		jsonContent = compactArrayOpen(jsonContent)
		jsonContent = fixMissingQuotes(jsonContent)
		if err := validateJSONFormat(jsonContent); err != nil {
			return nil, fmt.Errorf("JSON format validation failed: %w\nJSON content: %s\nFull response:\n%s", err, jsonContent, response)
		}
		var decisions []Decision
		if err := json.Unmarshal([]byte(jsonContent), &decisions); err != nil {
			return nil, fmt.Errorf("JSON parsing failed: %w\nJSON content: %s", err, jsonContent)
		}
		return decisions, nil
	}

	jsonContent := strings.TrimSpace(reJSONArray.FindString(jsonPart))
	if jsonContent == "" && reEmptyJSONArray.MatchString(jsonPart) {
		return []Decision{}, nil // an explicit empty decision list is structured
	}
	if jsonContent == "" {
		// #94 / AUD-B1: a reply with no structured decision is an AI
		// failure (counts toward safe mode), not a successful wait.
		cotSummary := jsonPart
		if len(cotSummary) > 240 {
			cotSummary = cotSummary[:240] + "..."
		}
		logger.Warnf("⚠  [%s] AI reply has no structured decision; counted as an AI failure", AIFailureNoStructuredDecision)
		return nil, fmt.Errorf("%w; summary: %s", ErrNoStructuredDecision, cotSummary)
	}

	jsonContent = compactArrayOpen(jsonContent)
	jsonContent = fixMissingQuotes(jsonContent)

	if err := validateJSONFormat(jsonContent); err != nil {
		return nil, fmt.Errorf("JSON format validation failed: %w\nJSON content: %s\nFull response:\n%s", err, jsonContent, response)
	}

	var decisions []Decision
	if err := json.Unmarshal([]byte(jsonContent), &decisions); err != nil {
		return nil, fmt.Errorf("JSON parsing failed: %w\nJSON content: %s", err, jsonContent)
	}

	return decisions, nil
}

func fixMissingQuotes(jsonStr string) string {
	jsonStr = strings.ReplaceAll(jsonStr, "\u201c", "\"")
	jsonStr = strings.ReplaceAll(jsonStr, "\u201d", "\"")
	jsonStr = strings.ReplaceAll(jsonStr, "\u2018", "'")
	jsonStr = strings.ReplaceAll(jsonStr, "\u2019", "'")

	jsonStr = strings.ReplaceAll(jsonStr, "［", "[")
	jsonStr = strings.ReplaceAll(jsonStr, "］", "]")
	jsonStr = strings.ReplaceAll(jsonStr, "｛", "{")
	jsonStr = strings.ReplaceAll(jsonStr, "｝", "}")
	jsonStr = strings.ReplaceAll(jsonStr, "：", ":")
	jsonStr = strings.ReplaceAll(jsonStr, "，", ",")

	jsonStr = strings.ReplaceAll(jsonStr, "【", "[")
	jsonStr = strings.ReplaceAll(jsonStr, "】", "]")
	jsonStr = strings.ReplaceAll(jsonStr, "〔", "[")
	jsonStr = strings.ReplaceAll(jsonStr, "〕", "]")
	jsonStr = strings.ReplaceAll(jsonStr, "、", ",")

	jsonStr = strings.ReplaceAll(jsonStr, "　", " ")

	return jsonStr
}

func validateJSONFormat(jsonStr string) error {
	trimmed := strings.TrimSpace(jsonStr)

	if !reArrayHead.MatchString(trimmed) {
		if strings.HasPrefix(trimmed, "[") && !strings.Contains(trimmed[:min(20, len(trimmed))], "{") {
			return fmt.Errorf("not a valid decision array (must contain objects {}), actual content: %s", trimmed[:min(50, len(trimmed))])
		}
		return fmt.Errorf("JSON must start with [{ (whitespace allowed), actual: %s", trimmed[:min(20, len(trimmed))])
	}

	if containsUnquotedTilde(jsonStr) {
		return fmt.Errorf("JSON cannot contain range symbol ~ outside of string values, all numbers must be precise single values")
	}

	for i := 0; i < len(jsonStr)-4; i++ {
		if jsonStr[i] >= '0' && jsonStr[i] <= '9' &&
			jsonStr[i+1] == ',' &&
			jsonStr[i+2] >= '0' && jsonStr[i+2] <= '9' &&
			jsonStr[i+3] >= '0' && jsonStr[i+3] <= '9' &&
			jsonStr[i+4] >= '0' && jsonStr[i+4] <= '9' {
			return fmt.Errorf("JSON numbers cannot contain thousand separator comma, found: %s", jsonStr[i:min(i+10, len(jsonStr))])
		}
	}

	return nil
}

// containsUnquotedTilde checks if ~ appears outside of JSON string values.
// This prevents false positives where ~ in reason text (e.g., "range ~0.5")
// incorrectly triggers the range-symbol validation.
func containsUnquotedTilde(s string) bool {
	inString := false
	escaped := false
	for i := 0; i < len(s); i++ {
		if escaped {
			escaped = false
			continue
		}
		if s[i] == '\\' {
			escaped = true
			continue
		}
		if s[i] == '"' {
			inString = !inString
			continue
		}
		if !inString && s[i] == '~' {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func removeInvisibleRunes(s string) string {
	return reInvisibleRunes.ReplaceAllString(s, "")
}

func compactArrayOpen(s string) string {
	return reArrayOpenSpace.ReplaceAllString(strings.TrimSpace(s), "[{")
}
