package engine

import (
	"testing"

	"github.com/Aixxww/AiT/datafetch"
)

// #157: social_enabled=false excludes the social component from scoring.
func TestSocialEnabled157(t *testing.T) {
	snap := &datafetch.SymbolSnapshot{Symbol: "SOCUSDT", Price: 100,
		Social: datafetch.SocialData{HeatScore: 90, Sentiment: 80, VolumeChange: 60}}
	on := DefaultHubConfig()
	if !on.SocialEnabled {
		t.Fatal("social must be on by default")
	}
	a := scoreSymbol(snap, on)
	off := on
	off.SocialEnabled = false
	b := scoreSymbol(snap, off)
	if a.SocialBullScore <= 0 || a.SocialHeatScore != 90 {
		t.Fatalf("social on: bull=%v heat=%v", a.SocialBullScore, a.SocialHeatScore)
	}
	if b.SocialBullScore != 0 || b.SocialBearScore != 0 || b.SocialHeatScore != 0 || b.SocialSentiment != 0 || b.SocialVolumePct != 0 {
		t.Fatalf("social off still scores: %+v", b)
	}
}
