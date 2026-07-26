package kernel

import (
	"testing"

	"github.com/Aixxww/AiT/provider/local"
)

// Every code the kernel claims to verify pre-prompt must be a registered
// confirmation code. This is the guard that killed the taker_buy_15m_gt_0_50
// dead branch (a verifier for a code no module ever produced) and prevents
// its recurrence.
func TestHunterV7LiveConfirmVerifiersAreRegistered(t *testing.T) {
	for code := range hunterV7LiveConfirmVerifiers {
		if !local.V7ConfirmKnown(code) {
			t.Errorf("%s: kernel verifier exists for an unregistered confirmation code", code)
		}
	}
}
