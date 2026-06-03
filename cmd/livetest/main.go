package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"nofx/provider/local"
	"nofx/provider/nofxos"
)

type CoinSnapshot struct {
	Symbol      string   `json:"symbol"`
	Score       float64  `json:"score"`
	Direction   string   `json:"direction,omitempty"`
	LongScore   float64  `json:"long_score,omitempty"`
	ShortScore  float64  `json:"short_score,omitempty"`
	SignalTags  []string `json:"signal_tags,omitempty"`
	LongTags    []string `json:"long_tags,omitempty"`
	ShortTags   []string `json:"short_tags,omitempty"`
	PriceChange float64  `json:"price_change_24h"`
}

type TestRound struct {
	Mode      string         `json:"mode"`
	Round     int            `json:"round"`
	Timestamp string         `json:"timestamp"`
	CoinCount int            `json:"coin_count"`
	Coins     []CoinSnapshot `json:"coins"`
	LongCnt   int            `json:"long_count"`
	ShortCnt  int            `json:"short_count"`
}

func main() {
	client := local.NewClient("")
	rounds := 3
	all := []TestRound{}

	fmt.Println("=== AiT 数据源选币实时测试 ===")
	fmt.Printf("时间: %s | 轮数: %d/模式\n\n", time.Now().Format("2006-01-02 15:04:05"), rounds)

	// === AI500 ===
	fmt.Println("━━━ AI500 ━━━")
	for i := 1; i <= rounds; i++ {
		fmt.Printf("[Round %d] ", i)
		coins, err := client.GetAI500List()
		if err != nil {
			fmt.Printf("ERR: %v\n", err)
			continue
		}
		top := limit(coins, 15)
		lc, sc := 0, 0
		snaps := toSnapshots(top, &lc, &sc)
		fmt.Printf("%d coins | L:%d S:%d\n", len(coins), lc, sc)
		printTop(top, 5)
		all = append(all, TestRound{"ai500", i, now(), len(coins), snaps, lc, sc})
		sleep(5)
	}

	// === Hunter ===
	fmt.Println("\n━━━ Hunter ━━━")
	for i := 1; i <= rounds; i++ {
		fmt.Printf("[Round %d] ", i)
		coins, err := client.GetHunterList()
		if err != nil {
			fmt.Printf("ERR: %v\n", err)
			continue
		}
		if coins == nil {
			fmt.Println("宁缺勿滥: 0")
			all = append(all, TestRound{"hunter", i, now(), 0, nil, 0, 0})
			continue
		}
		top := limit(coins, 15)
		lc, sc := 0, 0
		snaps := toSnapshots(top, &lc, &sc)
		fmt.Printf("%d coins | L:%d S:%d\n", len(coins), lc, sc)
		printTop(top, 5)
		all = append(all, TestRound{"hunter", i, now(), len(coins), snaps, lc, sc})
		sleep(5)
	}

	// === Hunter Sniff ===
	fmt.Println("\n━━━ Hunter Sniff ━━━")
	for i := 1; i <= rounds; i++ {
		fmt.Printf("[Round %d] ", i)
		coins, err := client.GetHunterList()
		if err != nil || coins == nil {
			fmt.Println("无Hunter候选")
			all = append(all, TestRound{"sniff", i, now(), 0, nil, 0, 0})
			continue
		}
		_, _, meta, _ := client.GetHunterCoinsWithData(len(coins), nil)
		res := client.FilterAmbushCandidates(coins, meta)
		lc := len(res.LongAmbush)
		sc := len(res.ShortDist)
		fmt.Printf("input=%d → LONG_AMBUSH:%d SHORT_DIST:%d\n", len(coins), lc, sc)
		fmt.Printf("  blocked: dir=%d score=%d squeeze=%d signal=%d wall=%d wash=%d\n",
			res.Stats.BlockedByDirection, res.Stats.BlockedByScore,
			res.Stats.BlockedBySqueeze, res.Stats.BlockedBySignal,
			res.Stats.BlockedByWall, res.Stats.BlockedByWash)

		snaps := []CoinSnapshot{}
		for _, c := range res.LongAmbush {
			snaps = append(snaps, CoinSnapshot{c.Symbol, c.Coin.Score, "LONG_AMBUSH", c.Meta.LongScore, 0, c.Reasons, c.Meta.LongTags, nil, c.Coin.IncreasePercent})
		}
		for _, c := range res.ShortDist {
			snaps = append(snaps, CoinSnapshot{c.Symbol, c.Coin.Score, "SHORT_DIST", 0, c.Meta.ShortScore, c.Reasons, nil, c.Meta.ShortTags, c.Coin.IncreasePercent})
		}
		for j, s := range snaps {
			if j >= 5 { break }
			fmt.Printf("  #%d %s score=%.1f %s %v\n", j+1, s.Symbol, s.Score, s.Direction, s.SignalTags)
		}
		if len(snaps) == 0 {
			fmt.Println("  无标的通过嗅探")
		}
		all = append(all, TestRound{"sniff", i, now(), lc + sc, snaps, lc, sc})
		sleep(5)
	}

	data, _ := json.MarshalIndent(all, "", "  ")
	os.WriteFile("coinselect_results.json", data, 0644)
	fmt.Println("\n✅ → coinselect_results.json")
	summary(all)
}

func limit(coins []nofxos.CoinData, n int) []nofxos.CoinData {
	if len(coins) > n { return coins[:n] }
	return coins
}
func now() string { return time.Now().Format("15:04:05") }
func sleep(s int) { time.Sleep(time.Duration(s) * time.Second) }

func toSnapshots(coins []nofxos.CoinData, lc, sc *int) []CoinSnapshot {
	snaps := make([]CoinSnapshot, 0, len(coins))
	for _, c := range coins {
		snaps = append(snaps, CoinSnapshot{c.Pair, c.Score, c.Direction, c.LongScore, c.ShortScore, c.SignalTags, c.LongTags, c.ShortTags, c.IncreasePercent})
		if c.Direction == "LONG" { *lc++ } else if c.Direction == "SHORT" { *sc++ }
	}
	return snaps
}

func printTop(coins []nofxos.CoinData, n int) {
	for i, c := range coins {
		if i >= n { break }
		d := c.Direction
		if d == "" { d = "-" }
		fmt.Printf("  #%d %s %.1f %s %v\n", i+1, c.Pair, c.Score, d, c.SignalTags)
	}
}

func summary(rounds []TestRound) {
	fmt.Println("\n════════════════════════════════════")
	type S struct { Total, Long, Short, Unique int; MaxScore float64 }
	m := map[string]*S{}
	uniq := map[string]map[string]bool{}
	for _, r := range rounds {
		if m[r.Mode] == nil { m[r.Mode] = &S{}; uniq[r.Mode] = map[string]bool{} }
		s := m[r.Mode]
		s.Total += r.CoinCount
		s.Long += r.LongCnt
		s.Short += r.ShortCnt
		for _, c := range r.Coins {
			uniq[r.Mode][c.Symbol] = true
			if c.Score > s.MaxScore { s.MaxScore = c.Score }
		}
		s.Unique = len(uniq[r.Mode])
	}
	for _, mode := range []string{"ai500","hunter","sniff"} {
		s := m[mode]
		if s == nil { continue }
		fmt.Printf("[%s] scored=%d L/S=%d/%d unique=%d max=%.1f\n",
			mode, s.Total, s.Long, s.Short, s.Unique, s.MaxScore)
	}
}
