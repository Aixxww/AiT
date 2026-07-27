package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Aixxww/AiT/store"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestHandleHunterV7Outcomes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&store.HunterV7SignalRecord{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	st, err := store.NewFromGorm(db)
	if err != nil {
		t.Fatalf("store from gorm: %v", err)
	}
	s := &Server{store: st}

	base := time.Now().UTC().Add(-time.Hour)
	exit10m := base.Add(10 * time.Minute)
	exit40m := base.Add(40 * time.Minute)
	records := []store.HunterV7SignalRecord{
		{CycleNumber: 1, Timestamp: base, Symbol: "A", Direction: "LONG", SetupType: "volatility_squeeze", MarketRegime: "compression", Status: "candidate", TrackStatus: "WIN_TP0", TrackExitTime: &exit10m, TrackPnLPct: 2.5, TrackMFE: 3, TrackMAE: -0.2},
		{CycleNumber: 1, Timestamp: base, Symbol: "B", Direction: "LONG", SetupType: "volatility_squeeze", MarketRegime: "compression", Status: "candidate", TrackStatus: "WIN_TP1", TrackExitTime: &exit40m, TrackPnLPct: 3, TrackMFE: 3.5, TrackMAE: -0.3},
	}
	if err := st.HunterV7Signal().CreateBatch(records); err != nil {
		t.Fatalf("create records: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/api/hunter/v7/outcomes?days=1&min_samples=1", nil)
	c.Request = req

	s.handleHunterV7Outcomes(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := body["tp0_30m"].(map[string]interface{}); !ok {
		t.Fatalf("missing tp0_30m: %+v", body)
	}
	// The adaptive dry-run preview died with the RegimeAdaptiveEngine (U5.4);
	// the endpoint must expose the raw grouped stats instead.
	if _, present := body["adaptive_report"]; present {
		t.Fatalf("adaptive_report should be gone after U5.4: %+v", body["adaptive_report"])
	}
	if rows, ok := body["setup_regime"].([]interface{}); !ok || len(rows) == 0 {
		t.Fatalf("missing setup_regime rows: %+v", body["setup_regime"])
	}
}

func TestHandleHunterV7Signals(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&store.HunterV7SignalRecord{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	st, err := store.NewFromGorm(db)
	if err != nil {
		t.Fatalf("store from gorm: %v", err)
	}
	s := &Server{store: st}

	base := time.Now().UTC().Add(-time.Hour)
	rawJSON := `{"signal_id":"sig-a","symbol":"BTCUSDT","direction":"LONG","setup_type":"volatility_squeeze","status":"candidate","ai_priority":88,"reason_codes":["flow_taker_buy_strong"],"entry_zone":{"lower":100,"upper":110},"invalidation":{"price":95,"reason":"below zone"},"targets":[{"price":130,"reason":"tp1"}],"confirmation_summary":{"passed_hard":true,"passed_review":true,"rr":2.4},"execution_readiness":{"tier":"EXECUTABLE","reason":"all clear","ready_score":90,"window_health":1,"entry_zone_position":0.4,"price_deviation_pct":0.1,"data_quality":"good"}}`
	records := []store.HunterV7SignalRecord{
		// Legacy row without a raw snapshot — must be rebuilt from columns.
		{CycleNumber: 1, Timestamp: base, Symbol: "ETHUSDT", Direction: "SHORT", SetupType: "funding_reversal", MarketRegime: "trend_down", Status: "candidate", ExecutionTier: "WATCH", TierReason: "timing weak", EntryZoneLower: 2000, EntryZoneUpper: 2050, InvalidationPrice: 2100},
		{CycleNumber: 2, Timestamp: base.Add(time.Minute), Symbol: "BTCUSDT", Direction: "LONG", SetupType: "volatility_squeeze", MarketRegime: "compression", Status: "candidate", ExecutionTier: "EXECUTABLE", TierReason: "all clear", RawJSON: rawJSON},
	}
	if err := st.HunterV7Signal().CreateBatch(records); err != nil {
		t.Fatalf("create records: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/hunter/v7/signals?limit=10", nil)

	s.handleHunterV7Signals(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Count   int                      `json:"count"`
		Signals []map[string]interface{} `json:"signals"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Count != 2 || len(body.Signals) != 2 {
		t.Fatalf("count = %d, signals = %d, want 2", body.Count, len(body.Signals))
	}
	// Newest first: the EXECUTABLE BTC row was inserted last.
	first := body.Signals[0]
	if first["execution_tier"] != "EXECUTABLE" {
		t.Fatalf("first execution_tier = %v, want EXECUTABLE", first["execution_tier"])
	}
	sig, ok := first["signal"].(map[string]interface{})
	if !ok {
		t.Fatalf("first signal not an object: %+v", first["signal"])
	}
	if sig["symbol"] != "BTCUSDT" || sig["signal_id"] != "sig-a" {
		t.Fatalf("raw snapshot not passed through: %+v", sig)
	}
	if _, ok := sig["execution_readiness"].(map[string]interface{}); !ok {
		t.Fatalf("execution_readiness missing from raw snapshot: %+v", sig)
	}
	// Legacy row must still expose the contract shape from flat columns.
	second := body.Signals[1]
	legacySig, ok := second["signal"].(map[string]interface{})
	if !ok {
		t.Fatalf("legacy signal not an object: %+v", second["signal"])
	}
	if legacySig["symbol"] != "ETHUSDT" {
		t.Fatalf("legacy symbol = %v", legacySig["symbol"])
	}
	zone, ok := legacySig["entry_zone"].(map[string]interface{})
	if !ok || zone["lower"].(float64) != 2000 {
		t.Fatalf("legacy entry_zone not rebuilt: %+v", legacySig["entry_zone"])
	}
}

func TestHandleHunterV7Matrix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&store.HunterV7SignalRecord{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	st, err := store.NewFromGorm(db)
	if err != nil {
		t.Fatalf("store from gorm: %v", err)
	}
	s := &Server{store: st}

	base := time.Now().UTC().Add(-time.Hour)
	records := []store.HunterV7SignalRecord{
		{CycleNumber: 1, Timestamp: base, Symbol: "A", Direction: "LONG", SetupType: "volatility_squeeze_breakout", MarketRegime: "compression", Status: "candidate", ExecutionTier: "EXECUTABLE", AIPriority: 80, SetupScore: 75, TimingScore: 70},
		{CycleNumber: 1, Timestamp: base, Symbol: "B", Direction: "LONG", SetupType: "volatility_squeeze_breakout", MarketRegime: "compression", Status: "wait_confirm", ExecutionTier: "WATCH", AIPriority: 60, SetupScore: 65, TimingScore: 45},
		{CycleNumber: 1, Timestamp: base, Symbol: "C", Direction: "SHORT", SetupType: "funding_reversal", MarketRegime: "trend_down", Status: "candidate", ExecutionTier: "REVIEWABLE", AIPriority: 72, SetupScore: 70, TimingScore: 68},
	}
	if err := st.HunterV7Signal().CreateBatch(records); err != nil {
		t.Fatalf("create records: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/api/hunter/v7/matrix?days=1&regime=compression", nil)
	c.Request = req

	s.handleHunterV7Matrix(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["source_rows"].(float64) != 2 {
		t.Fatalf("source_rows = %v, want 2", body["source_rows"])
	}
	matrix, ok := body["matrix"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing matrix: %+v", body)
	}
	cells, ok := matrix["cells"].([]interface{})
	if !ok || len(cells) != 1 {
		t.Fatalf("cells = %+v, want one compression cell", matrix["cells"])
	}
	cell := cells[0].(map[string]interface{})
	if cell["signal_count"].(float64) != 2 || cell["exec_count"].(float64) != 1 {
		t.Fatalf("unexpected matrix cell: %+v", cell)
	}
	if cell["setup_type"] != "volatility_squeeze_breakout" {
		t.Fatalf("setup_type = %v", cell["setup_type"])
	}
}
