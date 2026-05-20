package api

import (
	"net/http"
	"strconv"

	"nofx/provider/local"

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
