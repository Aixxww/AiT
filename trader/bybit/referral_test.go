package bybit

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

type captureRT struct{ got *http.Request }

func (c *captureRT) RoundTrip(r *http.Request) (*http.Response, error) {
	c.got = r
	return &http.Response{StatusCode: 200, Body: http.NoBody, Request: r}, nil
}

// OD-19: no Referer header unless a referral id is configured.
func TestReferer_DefaultNotSent(t *testing.T) {
	bt := NewBybitTrader("k", "s")
	rt := &captureRT{}
	bt.header.base = rt
	req, _ := http.NewRequest("GET", "https://api.bybit.com/v5/order/create", nil)
	_, _ = bt.header.RoundTrip(req)
	assert.Empty(t, rt.got.Header.Get("Referer"))
}

func TestReferer_ConfiguredIsSent(t *testing.T) {
	bt := NewBybitTrader("k", "s")
	bt.SetReferralID("Ref123")
	rt := &captureRT{}
	bt.header.base = rt
	req, _ := http.NewRequest("GET", "https://api.bybit.com/v5/order/create", nil)
	_, _ = bt.header.RoundTrip(req)
	assert.Equal(t, "Ref123", rt.got.Header.Get("Referer"))
	assert.Empty(t, req.Header.Get("Referer"), "caller's request is not mutated")

	bt.SetReferralID("bad id!")
	_, _ = bt.header.RoundTrip(req)
	assert.Empty(t, rt.got.Header.Get("Referer"), "invalid id is dropped")
}

// N2: a trader without a transport does not panic and sends nothing.
func TestSetReferralID_NilHeaderIsSafe(t *testing.T) {
	bt := &BybitTrader{}
	assert.NotPanics(t, func() { bt.SetReferralID("Ref123") })
	assert.Nil(t, bt.header)
}
