package binance

import (
	"strings"
	"testing"

	"github.com/adshao/go-binance/v2/futures"
	"github.com/stretchr/testify/assert"
)

// OD-19: client ids carry no referral part unless one is configured.
func TestClientIDs_DefaultHasNoReferral(t *testing.T) {
	ft := &FuturesTrader{}
	ft.SetReferralID("")
	for _, id := range []string{ft.newClientOrderID(), ft.ClientOrderIDForIntent("trader|1|BTCUSDT|long|sig")} {
		assert.True(t, strings.HasPrefix(id, ownedOrderPrefix), id)
		assert.NotContains(t, id, "x-", "no broker prefix by default")
		assert.NotContains(t, id, "KzrpZaP9", "old hard-coded broker id is gone")
		assert.Len(t, id, len(ownedOrderPrefix)+clientIDBodyLen)
		assert.Regexp(t, `^[\.A-Z\:/a-z0-9_-]{1,32}$`, id)
		assert.True(t, isOwnedClientID(id))
	}
}

func TestClientIDs_ConfiguredReferralIsSent(t *testing.T) {
	ft := &FuturesTrader{}
	ft.SetReferralID("  ABCD1234 ")
	for _, id := range []string{ft.newClientOrderID(), ft.ClientOrderIDForIntent("k")} {
		assert.True(t, strings.HasPrefix(id, "x-ABCD1234"+ownedOrderPrefix), id)
		assert.LessOrEqual(t, len(id), binanceClientIDMaxLen)
		assert.Regexp(t, `^[\.A-Z\:/a-z0-9_-]{1,32}$`, id)
		assert.True(t, isOwnedClientID(id))
	}

	// Longest allowed referral still fits 32 characters and stays unique.
	ft.SetReferralID("ABCDEFGHIJKL")
	a, b := ft.newClientOrderID(), ft.newClientOrderID()
	assert.Len(t, a, binanceClientIDMaxLen)
	assert.NotEqual(t, a, b)
	assert.True(t, isOwnedClientID(a))
}

func TestClientIDs_InvalidReferralIsDropped(t *testing.T) {
	for _, bad := range []string{"ABCDEFGHIJKLM", "ab-cd", "a b", "x/y", "ünï"} {
		ft := &FuturesTrader{}
		ft.SetReferralID(bad)
		assert.Equal(t, "", ft.referralID, bad)
		assert.True(t, strings.HasPrefix(ft.newClientOrderID(), ownedOrderPrefix), bad)
	}
}

// F04: the intent id is deterministic for a given config and key.
func TestClientOrderIDForIntent_Deterministic(t *testing.T) {
	ft := &FuturesTrader{}
	assert.Equal(t, ft.ClientOrderIDForIntent("k1"), ft.ClientOrderIDForIntent("k1"))
	assert.NotEqual(t, ft.ClientOrderIDForIntent("k1"), ft.ClientOrderIDForIntent("k2"))
	ft.SetReferralID("REF1")
	assert.Equal(t, ft.ClientOrderIDForIntent("k1"), ft.ClientOrderIDForIntent("k1"))
}

// Orders placed under any referral id stay owned, so F26/F27 cancels and
// the F28 stop check see them. The pre-OD-19 prefix is no longer owned.
func TestIsOwnedClientID(t *testing.T) {
	cases := map[string]bool{
		"x-KzrpZaP91234567890123abcdef12": false, // removed pre-OD-19 prefix
		"ait-1234567890123abcdef12":       true,  // new, no referral
		"x-ABCD1234ait-890123abcdef12":    true,  // new, with referral
		"x-OTHERait-890123abcdef12":       true,  // referral changed while open
		"x-ftGmvgAN1234567890":            false, // go-binance default prefix
		"web_12345":                       false, // manual order
		"ios_ait-123":                     false, // ownership prefix not at the start
		"":                                false,
	}
	for id, want := range cases {
		assert.Equal(t, want, isOwnedClientID(id), id)
	}
	assert.False(t, ownedSideMatch("x-KzrpZaP9x", futures.PositionSideTypeLong, "LONG"))
}
