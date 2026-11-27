package trader

import (
	"testing"

	"github.com/Aixxww/AiT/store"
)

// #157: indicator_hub.social_enabled is copied into the hub config; unset
// keeps the default (on).
func TestHubConfigSocialEnabled157(t *testing.T) {
	if !hubConfigFromIndicatorHub(nil).SocialEnabled || !hubConfigFromIndicatorHub(&store.IndicatorHubCfg{}).SocialEnabled {
		t.Fatal("unset social_enabled must keep social on")
	}
	off := false
	if hubConfigFromIndicatorHub(&store.IndicatorHubCfg{SocialEnabled: &off}).SocialEnabled {
		t.Fatal("social_enabled=false must reach the hub config")
	}
	// The other overrides still apply (extraction is behavior-preserving).
	if got := hubConfigFromIndicatorHub(&store.IndicatorHubCfg{TechWeight: 55, MinScore: 61}); got.TechWeight != 55 || got.MinScore != 61 {
		t.Fatalf("overrides lost: %+v", got)
	}
}
