package store

import (
	"encoding/json"
	"testing"
)

// #157: social_enabled is tri-state; a stored config without the key (every
// pre-#157 config, the field was omitempty) reads as unset.
func TestSocialEnabledTriState157(t *testing.T) {
	var c IndicatorHubCfg
	if err := json.Unmarshal([]byte(`{"tech_weight":40}`), &c); err != nil || c.SocialEnabled != nil {
		t.Fatalf("missing key must be nil: %v %v", err, c.SocialEnabled)
	}
	if err := json.Unmarshal([]byte(`{"social_enabled":false}`), &c); err != nil || c.SocialEnabled == nil || *c.SocialEnabled {
		t.Fatalf("explicit false must be kept: %v %v", err, c.SocialEnabled)
	}
}
