package kernel

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/provider/local"
)

// #134 (F-01): zone_position has ONE definition — percent from the zone's
// lower edge, clamped to [0,100], direction-independent
// (local.V7ZonePositionPctClamped). Every emitted field reports the same
// number for the same fixture.

func zonePos134Coin(direction string, price float64) CandidateCoin {
	coin := hunterV7PromptReadyLeaderCandidate("ZONEUSDT", price)
	coin.Direction = direction
	coin.V7EntryZone = local.V7PriceZone{Lower: 100, Upper: 110}
	coin.V7PriceContext = &local.V7PriceContext{Last: price}
	return coin
}

// zonePos134Fields returns every emitted zone_position encoding for coin.
func zonePos134Fields(t *testing.T, coin CandidateCoin) map[string]float64 {
	t.Helper()
	price := coin.V7PriceContext.Last
	data := hunterV7PromptTestMarketData(coin.Symbol, price)
	out := map[string]float64{}

	readiness := hunterV7PromptExecutionReadiness(coin, data, "EXECUTABLE", "readiness_ready")
	out["execution_readiness.entry_zone_position"] = readiness.EntryZonePos

	geo := buildHunterV7ExecutionGeometry(coin)
	if geo == nil {
		t.Fatal("execution geometry missing")
	}
	out["execution_geometry.entry_zone_position_pct"] = geo.EntryZonePositionPct

	e := &StrategyEngine{}
	compact := e.formatHunterV7ExecutionCompact(data, &coin)
	idx := strings.Index(compact, "entry_zone_pos=")
	if idx < 0 {
		t.Fatalf("compact line has no entry_zone_pos: %s", compact)
	}
	var v float64
	rest := compact[idx+len("entry_zone_pos="):]
	if _, err := fmt.Sscanf(rest, "%f%%", &v); err != nil {
		t.Fatalf("parse entry_zone_pos from %q: %v", rest, err)
	}
	out["compact.entry_zone_pos"] = v

	payload := e.buildHunterV7V2Payload(hunterV7PromptCandidate{Coin: coin, Data: data, Tier: "EXECUTABLE", Reason: "readiness_ready"},
		time.Unix(1_800_000_000, 0), HunterV7ExecutionGeometry{MinRR: 1.2})
	if payload.Signal.ZonePositionPct == nil {
		t.Fatal("hybrid signal.zone_position_pct missing")
	}
	out["hybrid.signal.zone_position_pct"] = *payload.Signal.ZonePositionPct

	if pos, ok := hunterV7EntryZonePositionPct(coin); ok {
		out["kernel.hunterV7EntryZonePositionPct"] = pos
	} else {
		t.Fatal("kernel reader returned !ok")
	}
	return out
}

func TestZonePosition134LegacyAndHybridAgree(t *testing.T) {
	cases := []struct {
		name      string
		direction string
		price     float64
		want      float64
	}{
		{"long_near_top", "LONG", 108.5, 85},
		{"short_near_top", "SHORT", 108.5, 85}, // was 15 in hybrid before #134
		{"long_near_bottom", "LONG", 101.5, 15},
		{"short_near_bottom", "SHORT", 101.5, 15}, // was 85 in hybrid before #134
		{"short_above_zone_clamped", "SHORT", 112, 100},
		{"long_below_zone_clamped", "LONG", 98, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fields := zonePos134Fields(t, zonePos134Coin(tc.direction, tc.price))
			for name, got := range fields {
				if math.Abs(got-tc.want) > 0.06 {
					t.Errorf("%s = %.3f, want %.1f (all encodings must agree)", name, got, tc.want)
				}
			}
		})
	}
}

// The compact line keeps the out-of-zone classification from the raw value
// while the emitted number is clamped.
func TestZonePosition134CompactLocationUsesRaw(t *testing.T) {
	e := &StrategyEngine{}
	coin := zonePos134Coin("SHORT", 112)
	compact := e.formatHunterV7ExecutionCompact(hunterV7PromptTestMarketData(coin.Symbol, 112), &coin)
	if !strings.Contains(compact, "entry_zone_pos=100.0%") || !strings.Contains(compact, "zone_location=above_zone") {
		t.Fatalf("compact = %s, want entry_zone_pos=100.0%% with zone_location=above_zone", compact)
	}
	coin = zonePos134Coin("LONG", 98)
	compact = e.formatHunterV7ExecutionCompact(hunterV7PromptTestMarketData(coin.Symbol, 98), &coin)
	if !strings.Contains(compact, "entry_zone_pos=0.0%") || !strings.Contains(compact, "zone_location=below_zone") {
		t.Fatalf("compact = %s, want entry_zone_pos=0.0%% with zone_location=below_zone", compact)
	}
}

// The doctrine states the unified definition and the range_expansion SHORT
// rule as rebound risk (high % blocks), not as a chase.
func TestZonePosition134DoctrineWording(t *testing.T) {
	for _, lang := range []string{"en", "zh"} {
		_, _, doctrine, _ := u11RenderHybrid(u11HybridConfig(lang))
		if lang == "en" {
			if !strings.Contains(doctrine, "measured from the zone's lower edge for both LONG and SHORT") {
				t.Errorf("hybrid %s doctrine does not state the unified zone_position_pct definition", lang)
			}
		} else if !strings.Contains(doctrine, "LONG 与 SHORT 一律从区间下沿量") {
			t.Errorf("hybrid %s doctrine does not state the unified zone_position_pct definition", lang)
		}
		if strings.Contains(doctrine, "SHORT from its upper edge") || strings.Contains(doctrine, "SHORT 从上沿量") {
			t.Errorf("hybrid %s doctrine still carries the inverted SHORT definition", lang)
		}
	}
	en := u11RenderPrompts(u11FixtureConfig("en"))
	if !strings.Contains(en, "entry_zone_position>60% (measured from the zone lower edge: price has rebounded into the upper zone) or live price rebounding >0.30% is rebound risk") {
		t.Errorf("legacy EN doctrine range_expansion SHORT rule is not phrased as rebound risk")
	}
	if strings.Contains(en, "range_expansion SHORT is a high-volatility chase") {
		t.Errorf("legacy EN doctrine still calls the range_expansion SHORT zone rule a chase")
	}
	zh := u11RenderPrompts(u11FixtureConfig("zh"))
	if !strings.Contains(zh, "价格已反弹回区间上部）或实价反弹超 0.30% 属反弹风险") {
		t.Errorf("legacy ZH doctrine range_expansion SHORT rule is not phrased as rebound risk")
	}
}
