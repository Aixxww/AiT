package binance

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"time"

	"github.com/Aixxww/AiT/trader/traderutil"
)

// OD-19: Binance client order ids.
//
// A client id has two parts:
//   - referral part: "x-<referral_id>", only when the exchange account has a
//     referral_id configured (Binance attributes broker orders by this
//     "x-<brokerId>" prefix). Default empty, so nothing is sent.
//   - ownership part: ownedOrderPrefix, always present. F26/F27/F28 use it
//     to recognise this bot's orders.
//
// Before OD-19 both roles shared one hard-coded broker prefix. It was
// accepted as owned during the transition and has since been removed
// (no order with it was open); such an order now counts as not owned.
//
// Every order this package places sets its own client id. go-binance
// v2.8.9 fills in its own broker prefix ("x-ftGmvgAN") when the id is left
// empty; order_guard.go refuses any create-order request without our id.

const (
	// ownedOrderPrefix marks orders placed by this bot (neutral, no referral).
	ownedOrderPrefix = "ait-"
	// binanceClientIDMaxLen is the futures client-id limit used by this package.
	binanceClientIDMaxLen = 32
	// binanceReferralIDMaxLen keeps at least 14 characters of id body after
	// "x-" + referral + ownedOrderPrefix.
	binanceReferralIDMaxLen = 12
	// clientIDBodyLen is the body length when there is room (13-digit
	// timestamp + 8 hex random, as before OD-19).
	clientIDBodyLen = 21
)

// ownedClientIDRe matches the ownership prefix, alone or after any
// "x-<referral>" part, so orders stay owned if the referral id is changed
// while they are open.
var ownedClientIDRe = regexp.MustCompile(`^(x-[A-Za-z0-9]{1,` + fmt.Sprint(binanceReferralIDMaxLen) + `})?` + regexp.QuoteMeta(ownedOrderPrefix))

// isOwnedClientID reports whether a client id was placed by this bot.
func isOwnedClientID(clientID string) bool {
	return ownedClientIDRe.MatchString(clientID)
}

// SetReferralID sets the Binance broker id from the exchange config. Call it
// right after construction, before any order. Empty or invalid = none.
func (t *FuturesTrader) SetReferralID(id string) {
	t.referralID = traderutil.NormalizeReferralID("Binance", id, binanceReferralIDMaxLen)
}

// clientIDPrefix is the referral part (if configured) + the ownership part.
func (t *FuturesTrader) clientIDPrefix() string {
	if t.referralID == "" {
		return ownedOrderPrefix
	}
	return "x-" + t.referralID + ownedOrderPrefix
}

func (t *FuturesTrader) clientIDBodyLen() int {
	n := binanceClientIDMaxLen - len(t.clientIDPrefix())
	if n > clientIDBodyLen {
		n = clientIDBodyLen
	}
	return n
}

// newClientOrderID generates a unique client order id:
// prefix + {13-digit nanosecond timestamp}{8 hex random}, with the body
// shortened from the left (keeping the random part) when a referral prefix
// leaves less room. Never longer than 32 characters.
func (t *FuturesTrader) newClientOrderID() string {
	timestamp := time.Now().UnixNano() % 10000000000000 // 13 digits
	randomBytes := make([]byte, 4)
	rand.Read(randomBytes)
	body := fmt.Sprintf("%013d%s", timestamp, hex.EncodeToString(randomBytes))
	return t.clientIDPrefix() + body[len(body)-t.clientIDBodyLen():]
}

// ClientOrderIDForIntent derives a deterministic client order id from an
// intent key (F04 read-back): prefix + leading hex chars of sha256(key),
// filling up to 21 characters of body within the 32-character limit.
func (t *FuturesTrader) ClientOrderIDForIntent(intentKey string) string {
	sum := sha256.Sum256([]byte(intentKey))
	return t.clientIDPrefix() + hex.EncodeToString(sum[:])[:t.clientIDBodyLen()]
}
