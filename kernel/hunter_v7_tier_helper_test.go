package kernel

// classifyHunterV7CandidateTier is a test-only shim that classifies with the
// default Hunter v7 execution geometry. Production code must always pass the
// engine-configured geometry via classifyHunterV7CandidateTierWithGeometry.
func classifyHunterV7CandidateTier(coin CandidateCoin) (string, string) {
	return classifyHunterV7CandidateTierWithGeometry(coin, HunterV7EffectiveExecutionGeometry(0, 0, 0, 0, true))
}
