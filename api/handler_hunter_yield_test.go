package api

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Aixxww/AiT/store"

	"github.com/gin-gonic/gin"
)

type yieldResponse struct {
	Yield hunterV7YieldTable `json:"yield"`
}

func u13WithKlines(t *testing.T, f func(tf string, start, end time.Time) ([]store.V7PriceBar, error)) {
	t.Helper()
	prev := hunterV7BTCKlines
	hunterV7BTCKlines = f
	btcKlineCacheMu.Lock()
	btcKlineCache = map[string]btcKlineCacheEntry{}
	btcKlineCacheMu.Unlock()
	t.Cleanup(func() {
		hunterV7BTCKlines = prev
		btcKlineCacheMu.Lock()
		btcKlineCache = map[string]btcKlineCacheEntry{}
		btcKlineCacheMu.Unlock()
	})
}

// u13Bars is a synthetic BTC series: quiet (LOW) before split, volatile
// (HIGH) after it.
func u13Bars(split time.Time) func(tf string, start, end time.Time) ([]store.V7PriceBar, error) {
	return func(tf string, start, end time.Time) ([]store.V7PriceBar, error) {
		step := map[string]time.Duration{"1h": time.Hour, "5m": 5 * time.Minute}[tf]
		var out []store.V7PriceBar
		price := 100.0
		i := 0
		for t := start.Truncate(step); !t.After(end); t = t.Add(step) {
			amp := 0.0005 // 0.05% swings: RV24 ~0.05 -> LOW
			if !t.Before(split) {
				amp = 0.01 // 1% swings -> HIGH
			}
			if tf == "1h" {
				price *= math.Exp([]float64{amp, -amp}[i%2])
			}
			out = append(out, store.V7PriceBar{OpenMs: t.UnixMilli(), Close: price})
			i++
		}
		return out, nil
	}
}

func u13Get(t *testing.T, s *Server) hunterV7YieldTable {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/hunter/v7/cells?days=7", nil)
	s.handleHunterV7Cells(c)
	if w.Code != http.StatusOK {
		t.Fatalf("code %d", w.Code)
	}
	var r yieldResponse
	if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return r.Yield
}

func TestU13YieldTableOnCellsEndpoint(t *testing.T) {
	now := time.Now().UTC()
	split := now.Add(-3 * 24 * time.Hour).Truncate(time.Hour)
	u13WithKlines(t, u13Bars(split))
	trig := 1.0
	var recs []store.HunterV7SignalRecord
	add := func(at time.Time, sym string, state string) {
		tp := trig
		recs = append(recs, store.HunterV7SignalRecord{CycleNumber: len(recs), Timestamp: at, Symbol: sym, Direction: "LONG",
			SetupType: "ignition_early_long", Status: "candidate", Stack: store.V7StackT2, LifecycleStateAtDecision: state, TriggerPrice: &tp})
	}
	// LOW period (before split, inside the window): 3 events, one repeated.
	for i, h := range []int{100, 110, 120} {
		at := now.Add(-time.Duration(h) * time.Hour)
		add(at, string(rune('A'+i)), store.V7T2StateTriggered)
		add(at.Add(5*time.Minute), string(rune('A'+i)), store.V7T2StateTriggered) // same event, next cycle
	}
	// HIGH period: 1 event, plus a WATCH record that does not count.
	add(now.Add(-24*time.Hour), "H", store.V7T2StateTriggered)
	add(now.Add(-23*time.Hour), "W", store.V7T2StateWatch)
	s := u12Server(t, recs)

	y := u13Get(t, s)
	if !y.Available || y.Stage != store.V7YieldStageT2Triggered || y.FormingBarTF != "5m" || y.Events != 4 || y.T2Records != len(recs) {
		t.Fatalf("yield = %+v", y)
	}
	got := map[string]store.V7YieldRow{}
	for _, r := range y.Rows {
		got[r.Tier] = r
	}
	if got[store.V7RV24TierLow].Events != 3 || got[store.V7RV24TierHigh].Events != 1 || got[store.V7RV24TierMid].Events != 0 {
		t.Fatalf("rows = %+v", y.Rows)
	}
	total := 0
	for _, n := range y.RoundsPerTier {
		total += n
	}
	if total != 7*288 && total != 7*288+1 {
		t.Fatalf("rounds = %v (total %d)", y.RoundsPerTier, total)
	}
	high := got[store.V7RV24TierHigh]
	if high.Target == nil || !high.BelowTarget || high.Rounds == 0 {
		t.Fatalf("HIGH row = %+v", high)
	}
	cfg := store.EffectiveV7YieldTarget(nil)
	lo, hi := store.V7PoissonInterval(1, cfg.CILevel)
	if math.Abs(high.CILo-lo/float64(high.Rounds)) > 1e-15 || math.Abs(high.CIHi-hi/float64(high.Rounds)) > 1e-15 {
		t.Fatalf("HIGH CI = [%v, %v]", high.CILo, high.CIHi)
	}
}

func TestU13YieldTableUnavailable(t *testing.T) {
	calls := 0
	u13WithKlines(t, func(string, time.Time, time.Time) ([]store.V7PriceBar, error) {
		calls++
		return nil, errFake
	})
	// No T2 records: no fetch, reason given.
	y := u13Get(t, u12Server(t, nil))
	if y.Available || y.Reason == "" || calls != 0 {
		t.Fatalf("empty: %+v calls=%d", y, calls)
	}
	tp := 1.0
	s := u12Server(t, []store.HunterV7SignalRecord{{CycleNumber: 1, Timestamp: time.Now().UTC().Add(-time.Hour), Symbol: "A", Direction: "LONG",
		SetupType: "x", Status: "candidate", Stack: store.V7StackT2, LifecycleStateAtDecision: store.V7T2StateTriggered, TriggerPrice: &tp}})
	if y := u13Get(t, s); y.Available || y.Reason == "" || calls != 1 {
		t.Fatalf("fetch failure: %+v calls=%d", y, calls)
	}
}

type fakeErr struct{}

func (fakeErr) Error() string { return "network down" }

var errFake error = fakeErr{}
