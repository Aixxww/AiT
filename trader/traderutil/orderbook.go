package traderutil

import "strconv"

// ParseOrderBookEntries converts order book entries from string pairs
// (the format most exchange REST APIs return) to float64 pairs.
// Each input entry is expected to have at least 2 elements: [price, quantity].
// Entries with fewer elements are skipped.
func ParseOrderBookEntries(entries [][]string) [][]float64 {
	result := make([][]float64, 0, len(entries))
	for _, e := range entries {
		if len(e) < 2 {
			continue
		}
		price, _ := strconv.ParseFloat(e[0], 64)
		qty, _ := strconv.ParseFloat(e[1], 64)
		result = append(result, []float64{price, qty})
	}
	return result
}
