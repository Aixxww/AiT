package binance

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Aixxww/AiT/logger"
	"github.com/adshao/go-binance/v2/futures"
)

// Order guard: the single choke point every Binance futures request passes
// through (the client's HTTP transport). It refuses any create-order request
// whose client id is not one of ours, so go-binance's default broker prefix
// ("x-ftGmvgAN", filled in when NewClientOrderID is not set) can never be
// sent, whichever service or code path builds the order.
//
// Checked (POST only; amend/cancel/query reuse an existing id):
//   - /fapi/v1/order, /fapi/v1/order/test  -> newClientOrderId
//   - /fapi/v1/batchOrders                 -> newClientOrderId of every order
//   - /fapi/v1/algoOrder                   -> clientAlgoId (go-binance sends
//     none when unset; such an order would be unowned)
//
// A refused request is never sent. The caller gets an error wrapping
// ErrUnownedClientID, which IsAmbiguousOrderError treats as definitely not
// placed, and the refusal is logged at error level.
//
// go-binance's websocket order service does not use this transport; a test
// (TestNoWebsocketOrderService) keeps it out of this package.

// ErrUnownedClientID marks an order refused by the order guard.
var ErrUnownedClientID = errors.New("binance order guard: create-order request without this bot's client id")

type orderGuardTransport struct {
	base http.RoundTripper
}

// installOrderGuard wraps client's HTTP transport with the order guard.
// Idempotent. Call it after anything that may replace the client.
func installOrderGuard(client *futures.Client) {
	if client == nil {
		return
	}
	if client.HTTPClient == nil {
		client.HTTPClient = &http.Client{}
	}
	if _, ok := client.HTTPClient.Transport.(*orderGuardTransport); ok {
		return
	}
	base := client.HTTPClient.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	client.HTTPClient.Transport = &orderGuardTransport{base: base}
}

func (g *orderGuardTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := checkOrderRequest(req); err != nil {
		logger.Errorf("❌ %v", err)
		return nil, err
	}
	return g.base.RoundTrip(req)
}

// checkOrderRequest returns a non-nil error for a create-order request that
// does not carry this bot's client id. Other requests pass.
func checkOrderRequest(req *http.Request) error {
	if req.Method != http.MethodPost || req.URL == nil {
		return nil
	}
	path := strings.TrimSuffix(req.URL.Path, "/")
	var idKey string
	batch := false
	switch path {
	case "/fapi/v1/order", "/fapi/v1/order/test":
		idKey = "newClientOrderId"
	case "/fapi/v1/batchOrders":
		idKey, batch = "newClientOrderId", true
	case "/fapi/v1/algoOrder":
		idKey = "clientAlgoId"
	default:
		return nil
	}

	params, err := requestParams(req)
	if err != nil {
		return fmt.Errorf("%w: %s: unreadable parameters: %v", ErrUnownedClientID, path, err)
	}

	if !batch {
		if id := params.Get(idKey); !isOwnedClientID(id) {
			return fmt.Errorf("%w: %s %s=%q", ErrUnownedClientID, path, idKey, id)
		}
		return nil
	}
	var orders []map[string]interface{}
	if err := json.Unmarshal([]byte(params.Get("batchOrders")), &orders); err != nil || len(orders) == 0 {
		return fmt.Errorf("%w: %s: unreadable batchOrders", ErrUnownedClientID, path)
	}
	for i, o := range orders {
		id, _ := o[idKey].(string)
		if !isOwnedClientID(id) {
			return fmt.Errorf("%w: %s order %d %s=%q", ErrUnownedClientID, path, i, idKey, id)
		}
	}
	return nil
}

// requestParams merges the query string and a form-encoded body. The body
// is read and restored so the request is sent unchanged.
func requestParams(req *http.Request) (url.Values, error) {
	params := req.URL.Query()
	if req.Body == nil || req.Body == http.NoBody {
		return params, nil
	}
	body, err := io.ReadAll(req.Body)
	req.Body.Close()
	if err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	form, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, err
	}
	for k, vs := range form {
		for _, v := range vs {
			params.Add(k, v)
		}
	}
	return params, nil
}
