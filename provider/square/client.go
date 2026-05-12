package square

import (
	"encoding/json"
	"fmt"
	"net/http"
	"nofx/logger"
	"strings"
	"time"
)

const (
 DefaultBaseURL = "http://localhost:8000"
 DefaultTimeout = 3 * time.Second
 DefaultMinScore = 25
)

// SquareItem represents a single token from the Square monitor leaderboard
type SquareItem struct {
 Token          string    `json:"token"`
 CompositeScore float64   `json:"composite_score"`
 Score          float64   `json:"score"`
 Mentions       int       `json:"mentions"`
 Trend          string    `json:"trend,omitempty"` // "↑↑","↑","—","↓","↓↓","🆕"
 Market         MarketInfo `json:"market,omitempty"`
}

// MarketInfo holds contract data for a token
type MarketInfo struct {
 Snapshot SnapshotData  `json:"snapshot,omitempty"`
 Analysis AnalysisData  `json:"analysis,omitempty"`
}

// SnapshotData holds real-time or snapshot market data
type SnapshotData struct {
 MarkPrice      *float64 `json:"mark_price"`
 OIChange1hPct  *float64 `json:"oi_change_1h_pct"`
 Change1hPct    *float64 `json:"change_1h_pct"`
 Change24hPct   *float64 `json:"change_24h_pct"`
 FundingRatePct *float64 `json:"funding_rate_pct"`
 Volume24hUSD   *float64 `json:"volume_24h_usd"`
 LongShortRatio *float64 `json:"long_short_ratio"`
}

// AnalysisData holds signals analysis for a token
type AnalysisData struct {
 Verdict   string   `json:"verdict,omitempty"`
 Direction string   `json:"direction,omitempty"`
 Score     *float64 `json:"score,omitempty"`
 Tags      []string `json:"tags,omitempty"`
 Notes     []string `json:"notes,omitempty"`
}

// leaderboardResponse wraps the /api/leaderboard JSON response
type leaderboardResponse struct {
 Items              []SquareItem `json:"items"`
 SkippedNoContract  int          `json:"skipped_no_contract"`
}

// Client queries the Python Square Monitor service
type Client struct {
 BaseURL  string
 HTTP     *http.Client
 MinScore float64
}

// NewClient creates a new Square heat client
func NewClient(baseURL string) *Client {
 if strings.TrimSpace(baseURL) == "" {
  baseURL = DefaultBaseURL
 }
 minScore := float64(DefaultMinScore)
 return &Client{
  BaseURL:  strings.TrimRight(baseURL, "/"),
  HTTP:     &http.Client{Timeout: DefaultTimeout},
  MinScore: minScore,
 }
}

// GetSquareHeatItems fetches the leaderboard and filters by MinScore.
// Only tokens with a valid contract (mark_price != nil) are included.
// Returns items sorted by CompositeScore descending (as returned by the service).
// limit=0 means "return all" (service max is 20).
func (c *Client) GetSquareHeatItems(limit int) ([]SquareItem, error) {
 raw, err := c.fetchLeaderboard()
 if err != nil {
  return nil, err
 }

 var items []SquareItem
 for _, item := range raw {
  // Must have a valid composite score above threshold
  if item.CompositeScore < c.MinScore {
   continue
  }
  // Must have a contract: mark_price must be present
  if item.Market.Snapshot.MarkPrice == nil {
   continue
  }
  // Token must look valid (uppercase letters, 2+ chars)
  tok := strings.ToUpper(strings.TrimSpace(item.Token))
  if len(tok) < 2 {
   continue
  }
  item.Token = tok
  items = append(items, item)

  if limit > 0 && len(items) >= limit {
   break
  }
 }

 logger.Infof("📊 Square Heat: API返回 %d 币, 过滤后 %d 币 (minScore=%.1f)", len(raw), len(items), c.MinScore)
 return items, nil
}

// GetSquareHeatSymbols returns just the normalized USDT symbols (for engine wrapping).
// "PEPE" → "PEPEUSDT", "BTC" → "BTCUSDT". Same filtering as GetSquareHeatItems.
func (c *Client) GetSquareHeatSymbols(limit int) ([]string, error) {
 items, err := c.GetSquareHeatItems(limit)
 if err != nil {
  return nil, err
 }
 symbols := make([]string, 0, len(items))
 for _, item := range items {
  symbols = append(symbols, toSymbol(item.Token))
 }
 return symbols, nil
}

// GetSquareHeatMap returns token → SquareItem (useful for enriching AI prompt with heat scores)
func (c *Client) GetSquareHeatMap() (map[string]SquareItem, error) {
 items, err := c.GetSquareHeatItems(0)
 if err != nil {
  return nil, err
 }
 m := make(map[string]SquareItem, len(items))
 for _, item := range items {
  m[item.Token] = item
 }
 return m, nil
}

// Ping checks if the Square Monitor service is reachable
func (c *Client) Ping() error {
 resp, err := c.HTTP.Get(c.BaseURL + "/api/leaderboard")
 if err != nil {
  return fmt.Errorf("square service unreachable at %s: %w", c.BaseURL, err)
 }
 defer resp.Body.Close()
 if resp.StatusCode != http.StatusOK {
  return fmt.Errorf("square service returned status %d", resp.StatusCode)
 }
 return nil
}

// fetchLeaderboard hits GET /api/leaderboard
func (c *Client) fetchLeaderboard() ([]SquareItem, error) {
 url := c.BaseURL + "/api/leaderboard"
 resp, err := c.HTTP.Get(url)
 if err != nil {
  return nil, fmt.Errorf("square request failed: %w", err)
 }
 defer resp.Body.Close()

 if resp.StatusCode != http.StatusOK {
  return nil, fmt.Errorf("square API returned status %d", resp.StatusCode)
 }

 var lr leaderboardResponse
 if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
  return nil, fmt.Errorf("square response decode failed: %w", err)
 }

 return lr.Items, nil
}

// toSymbol normalizes a bare token into a USDT perp symbol
// "PEPE" → "PEPEUSDT"
func toSymbol(token string) string {
 t := strings.ToUpper(strings.TrimSpace(token))
 if strings.HasSuffix(t, "USDT") {
  return t
 }
 return t + "USDT"
}
