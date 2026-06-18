package local

import (
	"sort"
	"strings"
)

// ============================================================================
// Hunter v8 - Sector Rotation Analyzer (v8-SPEC P2-3)
// ============================================================================
// Detects local theme leadership inside the current futures universe. This is a
// conservative output enhancer for rotation regimes: it adds small scoring and
// diagnostic context to signals from themes where multiple symbols are
// outperforming together.

type V7SectorTheme string

const (
	V7SectorAI       V7SectorTheme = "ai"
	V7SectorL1L2     V7SectorTheme = "l1_l2"
	V7SectorDeFi     V7SectorTheme = "defi"
	V7SectorMeme     V7SectorTheme = "meme"
	V7SectorGaming   V7SectorTheme = "gaming"
	V7SectorRWA      V7SectorTheme = "rwa"
	V7SectorStorage  V7SectorTheme = "storage"
	V7SectorExchange V7SectorTheme = "exchange"
	V7SectorOther    V7SectorTheme = "other"
)

type V7SectorStat struct {
	Theme        V7SectorTheme `json:"theme"`
	Count        int           `json:"count"`
	AvgChange4h  float64       `json:"avg_change_4h"`
	AvgChange24h float64       `json:"avg_change_24h"`
	Leader       bool          `json:"leader"`
}

type SectorRotationAnalyzer struct {
	stats map[V7SectorTheme]V7SectorStat
}

func NewSectorRotationAnalyzer(universe []V7SymbolContext) *SectorRotationAnalyzer {
	type acc struct {
		count       int
		sumChange4h float64
		sumChange24 float64
	}
	buckets := make(map[V7SectorTheme]*acc)
	totalCount := 0
	totalChange4h := 0.0
	totalChange24h := 0.0

	for i := range universe {
		ctx := &universe[i]
		if ctx.Symbol == "" {
			continue
		}
		theme := ClassifyV7SectorTheme(ctx.Symbol)
		if theme == V7SectorOther {
			continue
		}
		b := buckets[theme]
		if b == nil {
			b = &acc{}
			buckets[theme] = b
		}
		b.count++
		b.sumChange4h += ctx.Change4h
		b.sumChange24 += ctx.Change24h
		totalCount++
		totalChange4h += ctx.Change4h
		totalChange24h += ctx.Change24h
	}

	avg4h := 0.0
	avg24h := 0.0
	if totalCount > 0 {
		avg4h = totalChange4h / float64(totalCount)
		avg24h = totalChange24h / float64(totalCount)
	}

	stats := make(map[V7SectorTheme]V7SectorStat, len(buckets))
	for theme, b := range buckets {
		if b.count == 0 {
			continue
		}
		stat := V7SectorStat{
			Theme:        theme,
			Count:        b.count,
			AvgChange4h:  b.sumChange4h / float64(b.count),
			AvgChange24h: b.sumChange24 / float64(b.count),
		}
		stat.Leader = b.count >= 2 &&
			stat.AvgChange4h >= avg4h+1.5 &&
			stat.AvgChange24h >= avg24h+4
		stats[theme] = stat
	}

	return &SectorRotationAnalyzer{stats: stats}
}

func (a *SectorRotationAnalyzer) EnhanceSignal(sig *V7SignalOutput, ctx *V7SymbolContext, regime V7MarketRegime) {
	if a == nil || sig == nil || ctx == nil || regime != V7RegimeRotation {
		return
	}
	theme := ClassifyV7SectorTheme(ctx.Symbol)
	if theme == V7SectorOther {
		return
	}
	stat := a.stats[theme]
	if !stat.Leader {
		return
	}

	sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "sector_rotation_leader")
	sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "sector_theme_"+string(theme))
	if sig.Direction == V7DirLong {
		sig.SetupScore = clampFloat(sig.SetupScore+4, 0, 100)
		sig.TimingScore = clampFloat(sig.TimingScore+2, 0, 100)
	}
}

func (a *SectorRotationAnalyzer) Stats() []V7SectorStat {
	if a == nil {
		return nil
	}
	out := make([]V7SectorStat, 0, len(a.stats))
	for _, stat := range a.stats {
		out = append(out, stat)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Leader == out[j].Leader {
			return out[i].AvgChange24h > out[j].AvgChange24h
		}
		return out[i].Leader && !out[j].Leader
	})
	return out
}

func ClassifyV7SectorTheme(symbol string) V7SectorTheme {
	base := strings.ToUpper(symbol)
	for _, suffix := range []string{"USDT", "USDC", "BUSD", "FDUSD", "USD"} {
		base = strings.TrimSuffix(base, suffix)
	}

	switch base {
	case "AI", "FET", "RNDR", "RENDER", "TAO", "WLD", "ARKM", "GRT", "NMR", "PHA", "VIRTUAL":
		return V7SectorAI
	case "SOL", "SUI", "APT", "SEI", "ARB", "OP", "STRK", "TIA", "AVAX", "NEAR", "INJ", "ATOM":
		return V7SectorL1L2
	case "UNI", "AAVE", "CRV", "MKR", "ENA", "PENDLE", "LDO", "JTO", "JUP", "DYDX", "GMX":
		return V7SectorDeFi
	case "DOGE", "SHIB", "PEPE", "BONK", "WIF", "FLOKI", "MEME", "TURBO", "BOME", "PNUT":
		return V7SectorMeme
	case "AXS", "SAND", "MANA", "GALA", "IMX", "PIXEL", "YGG", "RONIN", "MAGIC", "PORTAL":
		return V7SectorGaming
	case "ONDO", "OM", "POLYX", "MPLX", "CFG", "CPOOL":
		return V7SectorRWA
	case "FIL", "AR", "STORJ", "SC":
		return V7SectorStorage
	case "BNB", "OKB", "GT", "CRO", "CAKE":
		return V7SectorExchange
	default:
		return V7SectorOther
	}
}
