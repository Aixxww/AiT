package kucoin

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"testing"
)

// #127 / Audit R2 H1: KuCoin partner attribution is opt-in.
func TestPartnerHeadersOptIn(t *testing.T) {
	req := func(kt *KuCoinTrader) http.Header {
		r, _ := http.NewRequest(http.MethodGet, kucoinBaseURL+kucoinAccountPath, nil)
		kt.setPartnerHeaders(r, "1700000000000")
		return r.Header
	}
	// Not configured: no partner header at all.
	kt := NewKuCoinTrader("api-key", "secret", "pass")
	for _, h := range []string{"KC-API-PARTNER", "KC-API-PARTNER-SIGN", "KC-API-PARTNER-VERIFY"} {
		if v := req(kt).Get(h); v != "" {
			t.Fatalf("unconfigured trader sends %s=%q", h, v)
		}
	}
	// An id without a secret is ignored (never a half-signed request).
	kt.SetPartner("TestPartner", "")
	if req(kt).Get("KC-API-PARTNER") != "" {
		t.Fatal("partner id without a secret must send no partner headers")
	}
	// An invalid id is ignored too.
	kt.SetPartner("bad id!", "k")
	if req(kt).Get("KC-API-PARTNER") != "" {
		t.Fatal("an invalid partner id must send no partner headers")
	}
	// Configured: the same headers and signature formula as before.
	kt.SetPartner("TestPartner", "test-partner-key")
	h := req(kt)
	mac := hmac.New(sha256.New, []byte("test-partner-key"))
	mac.Write([]byte("1700000000000" + "TestPartner" + "api-key"))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if h.Get("KC-API-PARTNER") != "TestPartner" || h.Get("KC-API-PARTNER-SIGN") != want || h.Get("KC-API-PARTNER-VERIFY") != "true" {
		t.Fatalf("configured headers %v, want sign %s", h, want)
	}
	// Re-configuring to empty turns it off again.
	kt.SetPartner("", "")
	if req(kt).Get("KC-API-PARTNER") != "" {
		t.Fatal("SetPartner(\"\", \"\") must disable attribution")
	}
}
