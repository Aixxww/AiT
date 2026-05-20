package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/your-ait/provider/local"
	"github.com/your-ait/provider/nofxos"
)

type RoundResult struct {
	Round     int              `json:"round"`
	Timestamp string           `json:"timestamp"`
	Source    string           `json:"source"`
	Coins     []nofxos.CoinData `json:"coins"`
}

func main() {
	client := local.NewClient("")
	rounds := 12
	allResults := []RoundResult{}

	fmt.Println("=== AI500 & Hunter 选币实测 ===")
	fmt.Printf("测试时间: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("测试轮数: %d\n\n", rounds)

	// Test AI500
	fmt.Println("--- AI500 智能选币 实测 ---")
	for i := 1; i <= rounds; i++ {
		fmt.Printf("[AI500 Round %d/%d] ", i, rounds)
		coins, err := client.GetAI500List()
		if err != nil {
			fmt.Printf("ERROR: %v\n", err)
			continue
		}
		top10 := coins
		if len(top10) > 10 {
			top10 = top10[:10]
		}
		fmt.Printf("选出 %d 币, Top10: ", len(coins))
		for j, c := range top10 {
			if j > 0 {
				fmt.Print(", ")
			}
			fmt.Printf("%s(%.1f)", c.Pair, c.Score)
		}
		fmt.Println()

		allResults = append(allResults, RoundResult{
			Round:     i,
			Timestamp: time.Now().Format("15:04:05"),
			Source:    "ai500",
			Coins:     top10,
		})
		time.Sleep(130 * time.Second) // wait for cache TTL (120s) + buffer
	}

	// Test Hunter
	fmt.Println("\n--- Hunter 猎手选币 实测 ---")
	for i := 1; i <= rounds; i++ {
		fmt.Printf("[Hunter Round %d/%d] ", i, rounds)
		coins, err := client.GetHunterList()
		if err != nil {
			fmt.Printf("ERROR: %v\n", err)
			continue
		}
		top10 := coins
		if len(top10) > 10 {
			top10 = top10[:10]
		}
		fmt.Printf("选出 %d 币, Top10: ", len(coins))
		for j, c := range top10 {
			if j > 0 {
				fmt.Print(", ")
			}
			fmt.Printf("%s(%.1f)", c.Pair, c.Score)
		}
		fmt.Println()

		allResults = append(allResults, RoundResult{
			Round:     i,
			Timestamp: time.Now().Format("15:04:05"),
			Source:    "hunter",
			Coins:     top10,
		})
		time.Sleep(130 * time.Second)
	}

	// Save results
	data, _ := json.MarshalIndent(allResults, "", "  ")
	os.WriteFile("coinselect_results.json", data, 0644)
	fmt.Println("\n结果已保存到 coinselect_results.json")
}
