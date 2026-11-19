package gate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// OD-19: no X-Gate-Channel-Id unless a referral id is configured.
func TestChannelID_DefaultNotSent(t *testing.T) {
	gt := NewGateTrader("k", "s")
	_, present := gt.client.GetConfig().DefaultHeader["X-Gate-Channel-Id"]
	assert.False(t, present)
	gt.SetReferralID("")
	_, present = gt.client.GetConfig().DefaultHeader["X-Gate-Channel-Id"]
	assert.False(t, present)
}

func TestChannelID_ConfiguredIsSent(t *testing.T) {
	gt := NewGateTrader("k", "s")
	gt.SetReferralID("ait")
	assert.Equal(t, "ait", gt.client.GetConfig().DefaultHeader["X-Gate-Channel-Id"])
	gt.SetReferralID("")
	_, present := gt.client.GetConfig().DefaultHeader["X-Gate-Channel-Id"]
	assert.False(t, present, "clearing removes the header")
}
