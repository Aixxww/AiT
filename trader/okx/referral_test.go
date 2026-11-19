package okx

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

// OD-19: no order tag unless a referral id is configured.
func TestOrderTag_DefaultNotSent(t *testing.T) {
	ot := &OKXTrader{}
	ot.SetReferralID("")
	body := ot.withTag(map[string]interface{}{"instId": "BTC-USDT-SWAP"})
	_, present := body["tag"]
	assert.False(t, present)
}

func TestOrderTag_ConfiguredIsSent(t *testing.T) {
	ot := &OKXTrader{}
	ot.SetReferralID("Tag123")
	assert.Equal(t, "Tag123", ot.withTag(map[string]interface{}{})["tag"])

	ot.SetReferralID("TOOLONGTAG1234567") // 17 > 16
	_, present := ot.withTag(map[string]interface{}{})["tag"]
	assert.False(t, present, "invalid id is dropped")
}

func TestClOrdID_NeutralFormat(t *testing.T) {
	a, b := genOkxClOrdID(), genOkxClOrdID()
	assert.Regexp(t, regexp.MustCompile(`^ait[0-9]{13}[0-9a-f]{8}$`), a)
	assert.LessOrEqual(t, len(a), 32)
	assert.NotEqual(t, a, b)
}
