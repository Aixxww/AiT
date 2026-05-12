// Package telemetry - disabled, all tracking removed
package telemetry

// Struct definitions preserved for API compatibility
type TradeEvent struct {
	Exchange  string
	TradeType string
	Symbol    string
	AmountUSD float64
	Leverage  int
	UserID    string
	TraderID  string
}

type AIUsageEvent struct {
	UserID        string
	TraderID      string
	ModelProvider string
	ModelName     string
	Channel       string
	InputTokens   int
	OutputTokens  int
}

func Init(_ bool, _ string)                   {}
func SetInstallationID(_ string)              {}
func GetInstallationID() string               { return "" }
func SetEnabled(_ bool)                       {}
func IsEnabled() bool                         { return false }
func TrackTrade(_ TradeEvent)                 {}
func TrackStartup(_ string)                   {}
func TrackAIUsage(_ AIUsageEvent)             {}
