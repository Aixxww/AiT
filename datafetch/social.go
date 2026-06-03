package datafetch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

// LunarCrushClient fetches social data from the LunarCrush API.
type LunarCrushClient struct {
	apiKey  string
	baseURL string
	client  *http.Client
	breaker *CircuitBreaker

	// Cache: symbol → data + expiry
	cacheMu sync.RWMutex
	cache   map[string]*socialCacheEntry
	ttl     time.Duration
}

type socialCacheEntry struct {
	data    *SocialData
	expires time.Time
}

// LunarCrushAPIResponse is the raw response from LunarCrush coins/v1.
type LunarCrushAPIResponse struct {
	Data struct {
		ID                 int     `json:"id"`
		Symbol             string  `json:"symbol"`
		Name               string  `json:"name"`
		Price              float64 `json:"price"`
		PriceBTC           float64 `json:"price_btc"`
		Volume24h          float64 `json:"volume_24h"`
		Volatility         float64 `json:"volatility"`
		CirculatingSupply  float64 `json:"circulating_supply"`
		MaxSupply          float64 `json:"max_supply"`
		MarketCap          float64 `json:"market_cap"`
		MarketDominance    float64 `json:"market_dominance"`
		PercentChange24h   float64 `json:"percent_change_24h"`
		PercentChange7d    float64 `json:"percent_change_7d"`
		PercentChange30d   float64 `json:"percent_change_30d"`
		SocialVolume24h    int     `json:"social_volume_24h"`
		SocialVolumeChange float64 `json:"social_volume_24h_change_percent"`
		Sentiment          float64 `json:"sentiment"`
		SentimentUp        float64 `json:"sentiment_up"`
		SentimentDown      float64 `json:"sentiment_down"`
		GalaxyScore        float64 `json:"galaxy_score"`
		AltRank            int     `json:"alt_rank"`
		CorrelationRank    float64 `json:"correlation_rank"`
		KOLCount           int     `json:"kol_count"`
	} `json:"data"`
}

// NewLunarCrushClient creates a new LunarCrush client.
func NewLunarCrushClient(apiKey, baseURL string) *LunarCrushClient {
	if baseURL == "" {
		baseURL = "https://lunarcrush.com/api4"
	}
	return &LunarCrushClient{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		breaker: NewCircuitBreaker(3, 60*time.Second),
		cache:   make(map[string]*socialCacheEntry),
		ttl:     15 * time.Minute,
	}
}

// FetchCoinMetrics fetches social data for a single coin. Returns cached data if fresh.
func (lc *LunarCrushClient) FetchCoinMetrics(ctx context.Context, symbol string) (*SocialData, error) {
	// Normalize: BTCUSDT → BTC
	base := strings.TrimSuffix(strings.ToUpper(symbol), "USDT")

	// Check cache
	lc.cacheMu.RLock()
	if entry, ok := lc.cache[base]; ok && time.Now().Before(entry.expires) {
		lc.cacheMu.RUnlock()
		return entry.data, nil
	}
	lc.cacheMu.RUnlock()

	if !lc.breaker.Allow() {
		return nil, fmt.Errorf("lunarcrush circuit breaker open for %s", base)
	}

	if lc.apiKey == "" {
		return nil, fmt.Errorf("lunarcrush API key not configured")
	}

	url := fmt.Sprintf("%s/public/coins/%s/v1", lc.baseURL, base)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+lc.apiKey)

	resp, err := lc.client.Do(req)
	if err != nil {
		lc.breaker.Failure()
		return nil, fmt.Errorf("lunarcrush GET %s: %w", base, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		lc.breaker.Failure()
		return nil, fmt.Errorf("lunarcrush read body %s: %w", base, err)
	}

	if resp.StatusCode == 429 {
		lc.breaker.Failure()
		return nil, fmt.Errorf("lunarcrush rate limited for %s", base)
	}
	if resp.StatusCode != http.StatusOK {
		lc.breaker.Failure()
		return nil, fmt.Errorf("lunarcrush %s: HTTP %d: %s", base, resp.StatusCode, truncateBody(body, 200))
	}

	var apiResp LunarCrushAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		lc.breaker.Failure()
		return nil, fmt.Errorf("lunarcrush unmarshal %s: %w", base, err)
	}

	lc.breaker.Success()

	d := apiResp.Data
	sd := &SocialData{
		HeatScore:      calcHeatScore(d.GalaxyScore, d.SocialVolume24h, d.Sentiment, d.AltRank),
		Sentiment:      d.Sentiment,
		SentimentDelta: 0, // computed from cache history if available
		SocialVolume:   d.SocialVolume24h,
		VolumeChange:   d.SocialVolumeChange,
		KOLCount:       d.KOLCount,
		GalaxyScore:    d.GalaxyScore,
		AltRank:        d.AltRank,
		UpdatedAt:      time.Now(),
	}

	// Update cache
	lc.cacheMu.Lock()
	// Compute sentiment delta from previous cache
	if prev, ok := lc.cache[base]; ok && prev.data != nil {
		sd.SentimentDelta = sd.Sentiment - prev.data.Sentiment
	}
	lc.cache[base] = &socialCacheEntry{
		data:    sd,
		expires: time.Now().Add(lc.ttl),
	}
	lc.cacheMu.Unlock()

	return sd, nil
}

// FetchTopCoins fetches top coins by GalaxyScore (not cached, uses bulk endpoint).
func (lc *LunarCrushClient) FetchTopCoins(ctx context.Context, limit int) ([]*SocialData, error) {
	// This would use a different endpoint; for now, return nil as the primary
	// use case is per-coin metrics. Can be extended later.
	return nil, fmt.Errorf("FetchTopCoins not implemented — use FetchCoinMetrics per symbol")
}

// calcHeatScore computes a composite 0-100 social heat score.
// Formula: GalaxyScore×0.4 + log10(SocialVolume)×0.3 + Sentiment×0.2 + AltRank×0.1
func calcHeatScore(galaxyScore float64, socialVolume int, sentiment float64, altRank int) float64 {
	sv := 0.0
	if socialVolume > 0 {
		sv = math.Log10(float64(socialVolume))
	}
	// Normalize sv: log10(10000)=4, log10(1)=0 → scale to 0-100
	svNorm := sv * 25 // 4*25=100

	// AltRank: lower is better, 1-1000 range. Invert: 1→100, 1000→0
	arNorm := 0.0
	if altRank > 0 {
		arNorm = math.Max(0, 100-float64(altRank)*0.1)
	}

	score := galaxyScore*0.4 + svNorm*0.3 + sentiment*0.2 + arNorm*0.1
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}

func init() {
	// Suppress unused import warnings
	_ = log.Println
}
