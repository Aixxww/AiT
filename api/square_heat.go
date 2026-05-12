package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"nofx/logger"
	"nofx/provider/square"

	"github.com/gin-gonic/gin"
)

// handleSquareHeat returns the current Square Heat leaderboard items
// for display in the trading dashboard.
func (s *Server) handleSquareHeat(c *gin.Context) {
	client := square.NewClient("") // defaults to http://localhost:8000

	items, err := client.GetSquareHeatItems(20)
	if err != nil {
		logger.Warnf("⚠️ Square Heat API error: %v", err)
		c.JSON(http.StatusOK, gin.H{
			"items":     []interface{}{},
			"updated_at": nil,
			"error":     fmt.Sprintf("Square Monitor unavailable: %v", err),
		})
		return
	}

	type heatItem struct {
		Token          string   `json:"token"`
		Symbol         string   `json:"symbol"`
		CompositeScore float64  `json:"composite_score"`
		Score          float64  `json:"score"`
		Mentions       int      `json:"mentions"`
		Trend          string   `json:"trend,omitempty"`
		MarkPrice      *float64 `json:"mark_price,omitempty"`
		Change24h      *float64 `json:"change_24h_pct,omitempty"`
		Change1h       *float64 `json:"change_1h_pct,omitempty"`
		FundingRate    *float64 `json:"funding_rate_pct,omitempty"`
		OIChange1h     *float64 `json:"oi_change_1h_pct,omitempty"`
		LSRatio        *float64 `json:"long_short_ratio,omitempty"`
		Direction      string   `json:"direction,omitempty"`
		Verdict        string   `json:"verdict,omitempty"`
		Tags           []string `json:"tags,omitempty"`
	}

	var response []heatItem
	for _, item := range items {
		tok := strings.ToUpper(strings.TrimSpace(item.Token))
		hi := heatItem{
			Token:          tok,
			Symbol:         tok + "USDT",
			CompositeScore: item.CompositeScore,
			Score:          item.Score,
			Mentions:       item.Mentions,
			Trend:          item.Trend,
		}
		if item.Market.Snapshot.MarkPrice != nil {
			hi.MarkPrice = item.Market.Snapshot.MarkPrice
		}
		if item.Market.Snapshot.Change24hPct != nil {
			hi.Change24h = item.Market.Snapshot.Change24hPct
		}
		if item.Market.Snapshot.Change1hPct != nil {
			hi.Change1h = item.Market.Snapshot.Change1hPct
		}
		if item.Market.Snapshot.FundingRatePct != nil {
			hi.FundingRate = item.Market.Snapshot.FundingRatePct
		}
		if item.Market.Snapshot.OIChange1hPct != nil {
			hi.OIChange1h = item.Market.Snapshot.OIChange1hPct
		}
		if item.Market.Snapshot.LongShortRatio != nil {
			hi.LSRatio = item.Market.Snapshot.LongShortRatio
		}
		hi.Direction = item.Market.Analysis.Direction
		hi.Verdict = item.Market.Analysis.Verdict
		hi.Tags = item.Market.Analysis.Tags
		response = append(response, hi)
	}

	c.JSON(http.StatusOK, gin.H{
		"items":      response,
		"updated_at": time.Now().Format(time.RFC3339),
		"count":      len(response),
	})
}
