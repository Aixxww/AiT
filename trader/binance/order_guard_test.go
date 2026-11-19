package binance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Aixxww/AiT/trader/types"
	"github.com/adshao/go-binance/v2/futures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordedRequest is one request that got past the guard to the server.
type recordedRequest struct {
	method, path string
	clientIDs    []string // newClientOrderId / clientAlgoId (one per batch order)
}

// recordingTransport sits between the guard and the mock server and records
// what was actually sent.
type recordingTransport struct {
	base http.RoundTripper
	mu   sync.Mutex
	reqs []recordedRequest
}

func (r *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	params, err := requestParams(req)
	if err != nil {
		return nil, err
	}
	rec := recordedRequest{method: req.Method, path: req.URL.Path}
	for _, k := range []string{"newClientOrderId", "clientAlgoId"} {
		if v := params.Get(k); v != "" {
			rec.clientIDs = append(rec.clientIDs, v)
		}
	}
	if b := params.Get("batchOrders"); b != "" {
		var orders []map[string]interface{}
		_ = json.Unmarshal([]byte(b), &orders)
		for _, o := range orders {
			id, _ := o["newClientOrderId"].(string)
			rec.clientIDs = append(rec.clientIDs, id)
		}
	}
	r.mu.Lock()
	r.reqs = append(r.reqs, rec)
	r.mu.Unlock()
	return r.base.RoundTrip(req)
}

func (r *recordingTransport) sent(method, path string) []recordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []recordedRequest
	for _, q := range r.reqs {
		if q.method == method && q.path == path {
			out = append(out, q)
		}
	}
	return out
}

// newGuardedMockTrader: the Binance mock suite's server, with a recorder
// under the order guard (guard -> recorder -> server).
func newGuardedMockTrader(t *testing.T) (*FuturesTrader, *recordingTransport) {
	t.Helper()
	suite := NewBinanceFuturesTestSuite(t)
	t.Cleanup(suite.mockServer.Close)
	client := futures.NewClient("test_api_key", "test_secret_key")
	client.BaseURL = suite.mockServer.URL
	client.HTTPClient = suite.mockServer.Client()
	rec := &recordingTransport{base: client.HTTPClient.Transport}
	client.HTTPClient.Transport = rec
	installOrderGuard(client)
	return &FuturesTrader{
		client:               client,
		maxLeverageCache:     make(map[string]int),
		maxLeverageCacheTime: make(map[string]time.Time),
	}, rec
}

// All six current create-order paths pass the guard and send our own id.
func TestOrderGuard_CurrentPathsPass(t *testing.T) {
	for _, ref := range []string{"", "REF1"} {
		t.Run("referral="+ref, func(t *testing.T) {
			ft, rec := newGuardedMockTrader(t)
			ft.SetReferralID(ref)

			_, err := ft.OpenLong("BTCUSDT", 0.01, 10)
			require.NoError(t, err)
			_, err = ft.OpenShort("BTCUSDT", 0.01, 10)
			require.NoError(t, err)
			_, err = ft.OpenLongWithClientID("BTCUSDT", 0.01, 10, ft.ClientOrderIDForIntent("k"))
			require.NoError(t, err)
			_, err = ft.CloseLong("BTCUSDT", 0.01)
			require.NoError(t, err)
			_, err = ft.CloseShort("BTCUSDT", 0.01)
			require.NoError(t, err)
			_, err = ft.PlaceLimitOrder(&types.LimitOrderRequest{Symbol: "BTCUSDT", Side: "BUY", Price: 50000, Quantity: 0.01, Leverage: 10, ClientID: "web_manual"})
			require.NoError(t, err)
			// The mock returns {} for algo orders; only what was sent matters here.
			_ = ft.SetStopLoss("BTCUSDT", "LONG", 0.01, 45000)
			_ = ft.SetTakeProfit("BTCUSDT", "LONG", 0.01, 55000)

			orders := rec.sent(http.MethodPost, "/fapi/v1/order")
			algos := rec.sent(http.MethodPost, "/fapi/v1/algoOrder")
			assert.Len(t, orders, 6, "4 opens/closes + intent open + limit")
			assert.Len(t, algos, 2, "stop loss + take profit")
			for _, q := range append(orders, algos...) {
				require.Len(t, q.clientIDs, 1, q.path)
				id := q.clientIDs[0]
				assert.True(t, isOwnedClientID(id), id)
				assert.True(t, strings.HasPrefix(id, ft.clientIDPrefix()), id)
				assert.NotContains(t, id, "ftGmvgAN")
			}
		})
	}
}

// A create call that forgets the client id is refused and never sent.
func TestOrderGuard_RefusesMissingClientID(t *testing.T) {
	ft, rec := newGuardedMockTrader(t)
	ctx := context.Background()

	// go-binance fills in "x-ftGmvgAN…" here.
	_, err := ft.client.NewCreateOrderService().Symbol("BTCUSDT").Side(futures.SideTypeBuy).
		PositionSide(futures.PositionSideTypeLong).Type(futures.OrderTypeMarket).Quantity("0.01").Do(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUnownedClientID), err)
	assert.Contains(t, err.Error(), "x-ftGmvgAN")
	assert.False(t, ft.IsAmbiguousOrderError(err), "refused = definitely not placed, no F04 read-back")

	// An explicit foreign id is refused too.
	_, err = ft.client.NewCreateOrderService().Symbol("BTCUSDT").Side(futures.SideTypeBuy).
		Type(futures.OrderTypeMarket).Quantity("0.01").NewClientOrderID("web_123").Do(ctx)
	assert.True(t, errors.Is(err, ErrUnownedClientID), err)

	// Algo order without ClientAlgoId (go-binance sends none).
	_, err = ft.client.NewCreateAlgoOrderService().Symbol("BTCUSDT").Side(futures.SideTypeSell).
		PositionSide(futures.PositionSideTypeLong).Type(futures.AlgoOrderTypeStopMarket).
		TriggerPrice("45000").Quantity("0.01").Do(ctx)
	assert.True(t, errors.Is(err, ErrUnownedClientID), err)

	// Batch: one order without an id refuses the whole batch.
	good := ft.client.NewCreateOrderService().Symbol("BTCUSDT").Side(futures.SideTypeBuy).
		Type(futures.OrderTypeMarket).Quantity("0.01").NewClientOrderID(ft.newClientOrderID())
	bad := ft.client.NewCreateOrderService().Symbol("BTCUSDT").Side(futures.SideTypeBuy).
		Type(futures.OrderTypeMarket).Quantity("0.01")
	_, err = ft.client.NewCreateBatchOrdersService().OrderList([]*futures.CreateOrderService{good, bad}).Do(ctx)
	assert.True(t, errors.Is(err, ErrUnownedClientID), err)

	assert.Empty(t, rec.sent(http.MethodPost, "/fapi/v1/order"), "nothing reached the exchange")
	assert.Empty(t, rec.sent(http.MethodPost, "/fapi/v1/algoOrder"))
	assert.Empty(t, rec.sent(http.MethodPost, "/fapi/v1/batchOrders"))
}

func TestOrderGuard_BatchWithOwnIDsPasses(t *testing.T) {
	ft, rec := newGuardedMockTrader(t)
	mk := func() *futures.CreateOrderService {
		return ft.client.NewCreateOrderService().Symbol("BTCUSDT").Side(futures.SideTypeBuy).
			Type(futures.OrderTypeMarket).Quantity("0.01").NewClientOrderID(ft.newClientOrderID())
	}
	_, err := ft.client.NewCreateBatchOrdersService().OrderList([]*futures.CreateOrderService{mk(), mk()}).Do(context.Background())
	assert.False(t, errors.Is(err, ErrUnownedClientID), err)
	sent := rec.sent(http.MethodPost, "/fapi/v1/batchOrders")
	require.Len(t, sent, 1)
	assert.Len(t, sent[0].clientIDs, 2)
}

// Non-create requests (query, cancel, account) are not checked.
func TestCheckOrderRequest_OnlyCreateCalls(t *testing.T) {
	for _, c := range []struct{ method, url string }{
		{http.MethodGet, "https://fapi.binance.com/fapi/v1/order?symbol=BTCUSDT&origClientOrderId=web_1"},
		{http.MethodDelete, "https://fapi.binance.com/fapi/v1/order?symbol=BTCUSDT&orderId=1"},
		{http.MethodPut, "https://fapi.binance.com/fapi/v1/order?symbol=BTCUSDT&orderId=1"},
		{http.MethodPost, "https://fapi.binance.com/fapi/v1/leverage?symbol=BTCUSDT&leverage=10"},
		{http.MethodGet, "https://fapi.binance.com/fapi/v1/openAlgoOrders"},
	} {
		req, _ := http.NewRequest(c.method, c.url, nil)
		assert.NoError(t, checkOrderRequest(req), c.method+" "+c.url)
	}
	req, _ := http.NewRequest(http.MethodPost, "https://fapi.binance.com/fapi/v1/order?newClientOrderId=x-ftGmvgANabc", nil)
	assert.Error(t, checkOrderRequest(req), "query-string id is checked too")
}

func TestInstallOrderGuard_Idempotent(t *testing.T) {
	client := futures.NewClient("k", "s")
	installOrderGuard(client)
	first := client.HTTPClient.Transport
	installOrderGuard(client)
	assert.Same(t, first, client.HTTPClient.Transport)
	_, ok := first.(*orderGuardTransport)
	assert.True(t, ok)
	installOrderGuard(nil) // no panic
}

// The websocket order service bypasses the HTTP transport (and so the
// guard), so it must not be used in this package.
func TestNoWebsocketOrderService(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		require.NoError(t, err)
		for _, banned := range []string{"OrderPlaceWsService", "NewOrderPlaceWsService"} {
			assert.NotContains(t, string(b), banned, "%s uses go-binance's websocket order service, which bypasses the order guard", f)
		}
	}
}
