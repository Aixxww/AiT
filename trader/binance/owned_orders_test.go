package binance

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"

	"github.com/adshao/go-binance/v2/futures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOwnedSideMatch(t *testing.T) {
	own := ownedOrderPrefix + "abc"
	cases := []struct {
		name  string
		id    string
		side  futures.PositionSideType
		want  string
		match bool
	}{
		{"owned same side", own, futures.PositionSideTypeLong, "LONG", true},
		{"owned same side lower-case want", own, futures.PositionSideTypeShort, "short", true},
		{"owned opposite side", own, futures.PositionSideTypeShort, "LONG", false},
		{"owned one-way BOTH", own, futures.PositionSideTypeBoth, "SHORT", true},
		{"owned empty side", own, "", "LONG", true},
		{"manual same side", "web_12345", futures.PositionSideTypeLong, "LONG", false},
		{"empty client id", "", futures.PositionSideTypeBoth, "LONG", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.match, ownedSideMatch(c.id, c.side, c.want))
		})
	}
}

// ownedMockExchange is a minimal stateful stand-in for the four endpoints
// CancelOwnedOrders uses.
type ownedMockExchange struct {
	mu            sync.Mutex
	orders        map[int64][2]string // orderId -> {clientOrderId, positionSide}
	algos         map[int64][2]string // algoId  -> {clientAlgoId, positionSide}
	ignoreCancel  map[int64]bool      // cancel returns 200 but the order stays
	failListAlgos bool
	canceled      []int64
}

func (m *ownedMockExchange) handler(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	q := r.URL.Query()
	if body, err := io.ReadAll(r.Body); err == nil && len(body) > 0 {
		if form, err := url.ParseQuery(string(body)); err == nil {
			for k, v := range form {
				q[k] = v
			}
		}
	}
	switch {
	case r.URL.Path == "/fapi/v1/openOrders" && r.Method == http.MethodGet:
		out := []map[string]interface{}{}
		for id, v := range m.orders {
			out = append(out, map[string]interface{}{
				"symbol": "BTCUSDT", "orderId": id, "clientOrderId": v[0], "positionSide": v[1], "status": "NEW",
			})
		}
		json.NewEncoder(w).Encode(out)
	case r.URL.Path == "/fapi/v1/openAlgoOrders" && r.Method == http.MethodGet:
		if m.failListAlgos {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"code":-1000,"msg":"internal"}`))
			return
		}
		out := []map[string]interface{}{}
		for id, v := range m.algos {
			out = append(out, map[string]interface{}{
				"symbol": "BTCUSDT", "algoId": id, "clientAlgoId": v[0], "positionSide": v[1], "algoStatus": "NEW",
			})
		}
		json.NewEncoder(w).Encode(out)
	case r.URL.Path == "/fapi/v1/order" && r.Method == http.MethodDelete:
		id, _ := strconv.ParseInt(q.Get("orderId"), 10, 64)
		m.canceled = append(m.canceled, id)
		if !m.ignoreCancel[id] {
			delete(m.orders, id)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"symbol": "BTCUSDT", "orderId": id, "status": "CANCELED"})
	case r.URL.Path == "/fapi/v1/algoOrder" && r.Method == http.MethodDelete:
		id, _ := strconv.ParseInt(q.Get("algoId"), 10, 64)
		m.canceled = append(m.canceled, id)
		if !m.ignoreCancel[id] {
			delete(m.algos, id)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"algoId": id, "code": "200", "msg": "success"})
	default:
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"code":-1,"msg":"unexpected ` + r.Method + " " + r.URL.Path + `"}`))
	}
}

func newOwnedMockTrader(t *testing.T, m *ownedMockExchange) *FuturesTrader {
	t.Helper()
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Skipf("local TCP listener unavailable in this environment: %v", err)
	}
	listener.Close()
	srv := httptest.NewServer(http.HandlerFunc(m.handler))
	t.Cleanup(srv.Close)
	client := futures.NewClient("test_api_key", "test_secret_key")
	client.BaseURL = srv.URL
	client.HTTPClient = srv.Client()
	return &FuturesTrader{client: client}
}

func hedgeBook() *ownedMockExchange {
	own := ownedOrderPrefix
	return &ownedMockExchange{
		orders: map[int64][2]string{
			1: {own + "long-tp", "LONG"},   // owned, LONG -> cancel
			2: {own + "short-tp", "SHORT"}, // owned, SHORT -> keep
			3: {"web_manual_long", "LONG"}, // manual, LONG -> keep
		},
		algos: map[int64][2]string{
			11: {own + "long-sl", "LONG"},    // owned, LONG -> cancel
			12: {own + "short-sl", "SHORT"},  // owned, SHORT -> keep (hedge protection)
			13: {"manual_algo_long", "LONG"}, // manual algo -> keep
		},
		ignoreCancel: map[int64]bool{},
	}
}

func TestCancelOwnedOrders_CancelsOnlyOwnedSameSide(t *testing.T) {
	m := hedgeBook()
	ft := newOwnedMockTrader(t, m)

	require.NoError(t, ft.CancelOwnedOrders("BTCUSDT", "LONG"))

	assert.ElementsMatch(t, []int64{1, 11}, m.canceled)
	assert.Contains(t, m.orders, int64(2), "opposite-side owned order kept")
	assert.Contains(t, m.orders, int64(3), "manual order kept")
	assert.Contains(t, m.algos, int64(12), "opposite-side stop (hedge protection) kept")
	assert.Contains(t, m.algos, int64(13), "manual algo order kept")
	assert.NotContains(t, m.orders, int64(1))
	assert.NotContains(t, m.algos, int64(11))
}

func TestCancelOwnedOrders_OneWayBothMatchesEitherSide(t *testing.T) {
	m := &ownedMockExchange{
		orders:       map[int64][2]string{1: {ownedOrderPrefix + "a", "BOTH"}, 2: {"manual", "BOTH"}},
		algos:        map[int64][2]string{11: {ownedOrderPrefix + "b", "BOTH"}},
		ignoreCancel: map[int64]bool{},
	}
	ft := newOwnedMockTrader(t, m)

	require.NoError(t, ft.CancelOwnedOrders("BTCUSDT", "SHORT"))
	assert.ElementsMatch(t, []int64{1, 11}, m.canceled)
	assert.Contains(t, m.orders, int64(2), "manual order kept in one-way mode")
}

func TestCancelOwnedOrders_NothingOwnedIsNoop(t *testing.T) {
	m := &ownedMockExchange{
		orders:       map[int64][2]string{3: {"web_manual", "LONG"}},
		algos:        map[int64][2]string{},
		ignoreCancel: map[int64]bool{},
	}
	ft := newOwnedMockTrader(t, m)

	require.NoError(t, ft.CancelOwnedOrders("BTCUSDT", "LONG"))
	assert.Empty(t, m.canceled)
}

func TestCancelOwnedOrders_ListFailureIsError(t *testing.T) {
	m := hedgeBook()
	m.failListAlgos = true
	ft := newOwnedMockTrader(t, m)

	err := ft.CancelOwnedOrders("BTCUSDT", "LONG")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list open algo orders")
	assert.Empty(t, m.canceled, "no cancels when the book cannot be read")
}

func TestCancelOwnedOrders_SurvivorIsError(t *testing.T) {
	m := hedgeBook()
	m.ignoreCancel[11] = true // exchange acks but the stop stays open
	ft := newOwnedMockTrader(t, m)

	err := ft.CancelOwnedOrders("BTCUSDT", "LONG")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "1 owned order(s) still open")
}
