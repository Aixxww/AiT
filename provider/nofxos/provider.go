package nofxos

// DataProvider is the interface for market data providers.
// Both nofxos.Client (direct API) and local.Client (Binance-backed)
// implement this interface, so the engine can use either transparently.
type DataProvider interface {
	// AI500 coin scoring
	GetAI500List() ([]CoinData, error)
	GetTopRatedCoins(limit int) ([]string, error)
	GetAvailableCoins() ([]string, error)

	// Open interest rankings
	GetOIRanking(duration string, limit int) (*OIRankingData, error)
	GetOITopPositions() ([]OIPosition, error)
	GetOILowPositions() ([]OIPosition, error)

	// NetFlow (fund flow) rankings
	GetNetFlowRanking(duration string, limit int) (*NetFlowRankingData, error)

	// Price rankings
	GetPriceRanking(durations string, limit int) (*PriceRankingData, error)

	// Per-coin quantitative data
	GetCoinData(symbol string, include string) (*QuantData, error)
	GetCoinDataBatch(symbols []string, include string) map[string]*QuantData
}
