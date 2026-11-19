package bitget

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

type captureRT struct{ got []*http.Request }

func (c *captureRT) RoundTrip(r *http.Request) (*http.Response, error) {
	c.got = append(c.got, r)
	body := `{"code":"00000","msg":"success","data":{}}`
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
}

func newCaptureTrader(rt *captureRT) *BitgetTrader {
	return &BitgetTrader{apiKey: "k", secretKey: "s", passphrase: "p", httpClient: &http.Client{Transport: rt}}
}

// OD-19: no channel code unless a referral id is configured.
func TestChannelCode_DefaultNotSent(t *testing.T) {
	rt := &captureRT{}
	bt := newCaptureTrader(rt)
	bt.SetReferralID("")
	_, _ = bt.doRequest("POST", "/api/v2/mix/order/place-order", map[string]string{"a": "b"})
	if assert.Len(t, rt.got, 1) {
		_, present := rt.got[0].Header["X-Channel-Api-Code"]
		assert.False(t, present)
	}
}

func TestChannelCode_ConfiguredIsSentOnOrderPathsOnly(t *testing.T) {
	rt := &captureRT{}
	bt := newCaptureTrader(rt)
	bt.SetReferralID("abc12")
	_, _ = bt.doRequest("POST", "/api/v2/mix/order/place-order", map[string]string{"a": "b"})
	_, _ = bt.doRequest("GET", "/api/v2/mix/account/accounts", nil)
	if assert.Len(t, rt.got, 2) {
		assert.Equal(t, "abc12", rt.got[0].Header.Get("X-CHANNEL-API-CODE"))
		assert.Empty(t, rt.got[1].Header.Get("X-CHANNEL-API-CODE"))
	}
}
