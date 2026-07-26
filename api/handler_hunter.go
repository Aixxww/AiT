package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Aixxww/AiT/provider/local"

	"github.com/gin-gonic/gin"
)

// handleHunterCoins returns the Hunter scored coin list with 4-pillar scores and signal tags.
// Public endpoint — no authentication required (Binance market data only).
func (s *Server) handleHunterCoins(c *gin.Context) {
	limit := 30
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 30 {
		limit = 30
	}

	client := local.NewClient("")
	coins, err := client.GetHunterList()
	if err != nil {
		SafeInternalError(c, "Get hunter coins", err)
		return
	}

	if limit < len(coins) {
		coins = coins[:limit]
	}

	c.JSON(http.StatusOK, gin.H{
		"coins": coins,
		"count": len(coins),
	})
}

// handleHunterV7Outcomes returns a dry-run report of tracked outcome stats and
// regime-adaptive weight suggestions.
func (s *Server) handleHunterV7Outcomes(c *gin.Context) {
	if s.store == nil {
		SafeInternalError(c, "Hunter v7 outcome report", fmt.Errorf("store not initialized"))
		return
	}

	days := 7
	if v := c.Query("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			days = n
		}
	}
	minSamples := 5
	if v := c.Query("min_samples"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			minSamples = n
		}
	}
	if days > 180 {
		days = 180
	}
	if minSamples > 200 {
		minSamples = 200
	}

	to := time.Now().UTC()
	from := to.AddDate(0, 0, -days)
	signalStore := s.store.HunterV7Signal()
	if signalStore == nil {
		SafeInternalError(c, "Hunter v7 outcome report", fmt.Errorf("signal store not initialized"))
		return
	}

	tp0, err := signalStore.OutcomeWindowStats(from, to, 30*time.Minute, []string{"WIN_TP0", "WIN_TP1", "WIN_TP2"}, "tp0_30m")
	if err != nil {
		SafeInternalError(c, "Hunter v7 30m outcome stats", err)
		return
	}
	tp1, err := signalStore.OutcomeWindowStats(from, to, 2*time.Hour, []string{"WIN_TP1", "WIN_TP2"}, "tp1_2h")
	if err != nil {
		SafeInternalError(c, "Hunter v7 2h outcome stats", err)
		return
	}
	grouped, err := signalStore.SetupRegimeOutcomeStats(from, to, minSamples)
	if err != nil {
		SafeInternalError(c, "Hunter v7 setup/regime stats", err)
		return
	}

	// The dry-run adaptive-weight preview was removed with the
	// RegimeAdaptiveEngine (U5.4): its output never fed back into routing, so
	// the endpoint now reports the raw setup/regime outcome stats only.
	c.JSON(http.StatusOK, gin.H{
		"from":             from.Format(time.RFC3339),
		"to":               to.Format(time.RFC3339),
		"days":             days,
		"min_samples":      minSamples,
		"tp0_30m":          tp0,
		"tp1_2h":           tp1,
		"setup_regime":     grouped,
		"window_source":    "hunter_v7_signal_records",
		"report_generated": true,
	})
}

// handleHunterV7Matrix returns a regime×setup diagnostic matrix from persisted
// raw signal records. It is intentionally independent of terminal PnL outcomes
// so P2 tuning can inspect noisy, blocked, and executable cells together.
func (s *Server) handleHunterV7Matrix(c *gin.Context) {
	if s.store == nil {
		SafeInternalError(c, "Hunter v7 matrix report", fmt.Errorf("store not initialized"))
		return
	}

	days := 7
	if v := c.Query("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			days = n
		}
	}
	if days > 180 {
		days = 180
	}
	regime := c.Query("regime")

	to := time.Now().UTC()
	from := to.AddDate(0, 0, -days)
	signalStore := s.store.HunterV7Signal()
	if signalStore == nil {
		SafeInternalError(c, "Hunter v7 matrix report", fmt.Errorf("signal store not initialized"))
		return
	}

	rows, err := signalStore.QueryMatrixSource(from, to, regime)
	if err != nil {
		SafeInternalError(c, "Hunter v7 matrix source", err)
		return
	}

	records := make([]local.V7SignalRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, local.V7SignalRecord{
			Signal: local.V7SignalOutput{
				Symbol:           row.Symbol,
				Direction:        local.V7Direction(row.Direction),
				SetupType:        local.V7SetupType(row.SetupType),
				Status:           local.V7SignalStatus(row.Status),
				ExecutionQuality: local.V7ExecutionQuality(row.ExecutionQuality),
				AIPriority:       row.AIPriority,
				SetupScore:       row.SetupScore,
				TimingScore:      row.TimingScore,
				RiskScore:        row.RiskScore,
				LiquidityScore:   row.LiquidityScore,
				RegimeFitScore:   row.RegimeFitScore,
				MarketRegime:     local.V7MarketRegime(row.MarketRegime),
				ResonanceBonus:   row.ResonanceBonus,
			},
			Tier:        row.ExecutionTier,
			TierReason:  row.TierReason,
			BlockedGate: row.BlockedGate,
		})
	}

	reportRegime := local.V7MarketRegime(regime)
	report := GenerateMatrixReport(records, reportRegime)
	c.JSON(http.StatusOK, gin.H{
		"from":             from.Format(time.RFC3339),
		"to":               to.Format(time.RFC3339),
		"days":             days,
		"regime":           regime,
		"source_rows":      len(rows),
		"matrix":           report,
		"window_source":    "hunter_v7_signal_records",
		"report_generated": true,
	})
}
