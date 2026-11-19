package traderutil

import (
	"regexp"
	"strings"

	"github.com/Aixxww/AiT/logger"
)

// OD-19: exchange referral / broker ids come from the exchange account
// config (store.Exchange.ReferralID) and default to empty. An empty id means
// no referral data is sent: no header, no order tag, no client-id broker
// prefix. Adapters call NormalizeReferralID once at construction.

var referralIDRe = regexp.MustCompile(`^[A-Za-z0-9]+$`)

// NormalizeReferralID trims id and checks it is 1..maxLen ASCII letters or
// digits (the common subset every supported exchange accepts in its broker
// field and, for Binance, inside a client order id). An invalid id is
// logged and dropped, so a malformed value is never sent.
func NormalizeReferralID(exchange, id string, maxLen int) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	if len(id) > maxLen || !referralIDRe.MatchString(id) {
		logger.Warnf("⚠ [%s] referral_id ignored: must be 1-%d letters or digits", exchange, maxLen)
		return ""
	}
	return id
}
