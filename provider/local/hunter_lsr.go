package local

import "fmt"

// lsrEntry matches Binance /futures/data/topLongShortPositionRatio response
type lsrEntry struct {
	Symbol         string `json:"symbol"`
	LongShortRatio string `json:"longShortRatio"`
	LongAccount    string `json:"longAccount"`
	ShortAccount   string `json:"shortAccount"`
	Timestamp      int64  `json:"timestamp"`
}

// fetchLSRHist fetches top trader long/short position ratio history.
// Returns (oldestRatio, newestRatio, error).
// Binance endpoint: GET /futures/data/topLongShortPositionRatio
// Parameters: symbol, period (5m/15m/30m/1h/2h/4h/6h/12h/1d), limit (1-500)
func (c *Client) fetchLSRHist(symbol, period string, limit int) (float64, float64, error) {
	url := fmt.Sprintf("%s/futures/data/topLongShortPositionRatio?symbol=%s&period=%s&limit=%d",
		c.BinanceURL, symbol, period, limit)
	var entries []lsrEntry
	if err := c.fetchJSON(url, &entries); err != nil {
		return 0, 0, err
	}
	if len(entries) < 2 {
		return 0, 0, nil
	}
	oldest := parseFloat(entries[0].LongShortRatio)
	newest := parseFloat(entries[len(entries)-1].LongShortRatio)
	return oldest, newest, nil
}
