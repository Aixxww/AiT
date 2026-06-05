package datafetch

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WSManager manages Binance WebSocket connections for real-time data.
type WSManager struct {
	baseURL string
	store   *Store
	topN    int // top symbols for kline/aggTrade streams

	mu         sync.Mutex
	conn       *websocket.Conn
	connected  bool
	stopCh     chan struct{}
	backoff    time.Duration
	maxBackoff time.Duration
}

// NewWSManager creates a new WebSocket manager.
func NewWSManager(baseURL string, store *Store, topN int) *WSManager {
	if topN <= 0 {
		topN = 30
	}
	return &WSManager{
		baseURL:    baseURL,
		store:      store,
		topN:       topN,
		stopCh:     make(chan struct{}),
		backoff:    5 * time.Second,
		maxBackoff: 60 * time.Second,
	}
}

// Start connects to Binance WebSocket and patches the snapshot in real-time.
// It blocks until Stop() is called or ctx is done.
func (wm *WSManager) Start(symbols []string) {
	streams := wm.buildStreams(symbols)
	if len(streams) == 0 {
		log.Println("datafetch/ws: no streams to subscribe")
		return
	}

	for {
		select {
		case <-wm.stopCh:
			return
		default:
		}

		err := wm.connectAndRead(streams)
		if err != nil {
			log.Printf("datafetch/ws: connection error: %v, reconnecting in %v", err, wm.backoff)
		}

		select {
		case <-wm.stopCh:
			return
		case <-time.After(wm.backoff):
		}

		// Exponential backoff
		wm.backoff = time.Duration(math.Min(
			float64(wm.maxBackoff),
			float64(wm.backoff)*2,
		))
	}
}

// Stop stops the WebSocket manager.
func (wm *WSManager) Stop() {
	close(wm.stopCh)
	wm.mu.Lock()
	if wm.conn != nil {
		wm.conn.Close()
	}
	wm.mu.Unlock()
}

// IsConnected returns whether the WebSocket is currently connected.
func (wm *WSManager) IsConnected() bool {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	return wm.connected
}

// buildStreams builds the list of Binance combined stream names.
func (wm *WSManager) buildStreams(symbols []string) []string {
	var streams []string

	// All symbols: markPrice@arr@1s
	streams = append(streams, "!markPrice@arr@1s")

	// Top N: kline_1m and aggTrade for real-time taker ratio
	limit := wm.topN
	if limit > len(symbols) {
		limit = len(symbols)
	}
	for _, sym := range symbols[:limit] {
		lower := strings.ToLower(sym)
		streams = append(streams, lower+"@kline_1m")
		streams = append(streams, lower+"@aggTrade")
	}

	return streams
}

// connectAndRead connects to Binance combined stream and processes messages.
func (wm *WSManager) connectAndRead(streams []string) error {
	// Binance combined stream limit: 200 per connection
	// Split into chunks of 200 if needed
	chunks := chunkStreams(streams, 200)
	if len(chunks) == 1 {
		return wm.connectAndReadOne(chunks[0])
	}

	// Multiple connections needed — run them concurrently
	var wg sync.WaitGroup
	errCh := make(chan error, len(chunks))
	for _, chunk := range chunks {
		chunk := chunk
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := wm.connectAndReadOne(chunk); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

// connectAndReadOne connects to a single combined stream and reads messages.
func (wm *WSManager) connectAndReadOne(streams []string) error {
	streamParam := strings.Join(streams, "/")
	u := url.URL{
		Scheme:   "wss",
		Host:     strings.TrimPrefix(wm.baseURL, "wss://"),
		Path:     "/stream",
		RawQuery: "streams=" + streamParam,
	}

	// Handle case where baseURL already has the full path
	if strings.Contains(wm.baseURL, "fstream.binance.com") {
		u.Host = "fstream.binance.com"
	}

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("dial %s: %w", u.String(), err)
	}
	defer conn.Close()

	wm.mu.Lock()
	wm.conn = conn
	wm.connected = true
	wm.backoff = 5 * time.Second // reset on successful connect
	wm.mu.Unlock()

	defer func() {
		wm.mu.Lock()
		wm.connected = false
		wm.mu.Unlock()
	}()

	// Ping handler
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Read pump
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				return
			}
			wm.handleMessage(message)
		}
	}()

	// Ping ticker
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-wm.stopCh:
			return nil
		case <-done:
			return fmt.Errorf("websocket read loop ended")
		case <-ticker.C:
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return fmt.Errorf("ping failed: %w", err)
			}
		}
	}
}

// handleMessage processes a single WebSocket message.
func (wm *WSManager) handleMessage(data []byte) {
	// Combined stream format: {"stream":"...","data":{...}}
	var msg struct {
		Stream string          `json:"stream"`
		Data   json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}

	snap := wm.store.Current()
	if snap == nil {
		return
	}

	switch {
	case strings.Contains(msg.Stream, "markPrice"):
		wm.handleMarkPrice(snap, msg.Data)
	case strings.Contains(msg.Stream, "@kline_1m"):
		wm.handleKline(snap, msg.Data)
	case strings.Contains(msg.Stream, "@aggTrade"):
		wm.handleAggTrade(snap, msg.Data)
	}
}

// handleMarkPrice updates mark/index prices and funding rates.
func (wm *WSManager) handleMarkPrice(snap *Snapshot, data json.RawMessage) {
	// !markPrice@arr@1s sends an array
	var arr []struct {
		Symbol          string `json:"s"`
		MarkPrice       string `json:"p"`
		IndexPrice      string `json:"i"`
		FundingRate     string `json:"r"`
		NextFundingTime int64  `json:"T"`
	}
	if err := json.Unmarshal(data, &arr); err != nil {
		// Try single object
		var single struct {
			Symbol          string `json:"s"`
			MarkPrice       string `json:"p"`
			IndexPrice      string `json:"i"`
			FundingRate     string `json:"r"`
			NextFundingTime int64  `json:"T"`
		}
		if err2 := json.Unmarshal(data, &single); err2 != nil {
			return
		}
		arr = append(arr, single)
	}

	for _, item := range arr {
		ss, ok := snap.Symbols[item.Symbol]
		if !ok {
			continue
		}
		ss.MarkPrice = parseFloat(item.MarkPrice)
		ss.IndexPrice = parseFloat(item.IndexPrice)
		ss.FundingRate = parseFloat(item.FundingRate)
		ss.NextFundingTime = item.NextFundingTime
		if ss.IndexPrice > 0 {
			ss.Spread = (ss.MarkPrice - ss.IndexPrice) / ss.IndexPrice * 100
		}
		ss.Price = ss.MarkPrice
	}
}

// handleKline updates the 1m kline data for a symbol.
func (wm *WSManager) handleKline(snap *Snapshot, data json.RawMessage) {
	var msg struct {
		Symbol string `json:"s"`
		Kline  struct {
			OpenTime  int64  `json:"t"`
			CloseTime int64  `json:"T"`
			Open      string `json:"o"`
			High      string `json:"h"`
			Low       string `json:"l"`
			Close     string `json:"c"`
			Volume    string `json:"v"`
			TakerBuy  string `json:"V"`
			IsFinal   bool   `json:"x"`
		} `json:"k"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}

	ss, ok := snap.Symbols[msg.Symbol]
	if !ok {
		return
	}

	k := Kline{
		OpenTime:  msg.Kline.OpenTime,
		CloseTime: msg.Kline.CloseTime,
		Open:      parseFloat(msg.Kline.Open),
		High:      parseFloat(msg.Kline.High),
		Low:       parseFloat(msg.Kline.Low),
		Close:     parseFloat(msg.Kline.Close),
		Volume:    parseFloat(msg.Kline.Volume),
		TakerBuy:  parseFloat(msg.Kline.TakerBuy),
	}

	klines := ss.Klines["1m"]
	if len(klines) > 0 && klines[len(klines)-1].OpenTime == k.OpenTime {
		// Update last bar
		klines[len(klines)-1] = k
	} else if msg.Kline.IsFinal {
		// Append new completed bar, trim to 60
		klines = append(klines, k)
		if len(klines) > 60 {
			klines = klines[len(klines)-60:]
		}
		ss.Klines["1m"] = klines
	}

	ss.Price = k.Close
	ss.Timestamp = time.Now()
}

// handleAggTrade updates real-time taker buy ratio via TakerAccumulator.
func (wm *WSManager) handleAggTrade(snap *Snapshot, data json.RawMessage) {
	var msg struct {
		Symbol string `json:"s"`
		Price  string `json:"p"`
		Qty    string `json:"q"`
		IsBuy  bool   `json:"m"` // true = buyer is maker (seller is taker)
		Time   int64  `json:"T"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}

	ss, ok := snap.Symbols[msg.Symbol]
	if !ok {
		return
	}

	qty := parseFloat(msg.Qty)
	// If buyer is maker (msg.m=true), then seller is taker — NOT a taker buy
	// If buyer is taker (msg.m=false), it IS a taker buy
	if !msg.IsBuy {
		// Update running taker buy ratio (simple EMA-like approach)
		// This is approximate — the real ratio comes from klines
		if ss.TakerBuyRatio == 0 {
			ss.TakerBuyRatio = 0.5 // seed
		}
		ss.TakerBuyRatio = ss.TakerBuyRatio*0.99 + 1.0*0.01*(qty)
	}
}

// chunkStreams splits streams into chunks of size n.
func chunkStreams(streams []string, n int) [][]string {
	var chunks [][]string
	for i := 0; i < len(streams); i += n {
		end := i + n
		if end > len(streams) {
			end = len(streams)
		}
		chunks = append(chunks, streams[i:end])
	}
	return chunks
}

// parseFloatWS is an alias kept for readability in WS context.
func parseFloatWS(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}
