package store

// U13 measured targets: the 90% exact Poisson lower bound of each stage's
// rate per RV24 tier (events per 5-minute round) over the 48-day study
// window, from `go run ./cmd/t2_u13_yield` (record and inputs in
// docs/t2-u13-yield-target.md). Rounds: LOW 3,814, MID 4,678, HIGH 5,332.
const (
	// T2 triggered events: LOW 177, MID 243, HIGH 274.
	defaultV7YieldTriggeredLow  = 0.0408232115318206
	defaultV7YieldTriggeredMid  = 0.046588337082117036
	defaultV7YieldTriggeredHigh = 0.046390300202643377
	// t2_m2_eval structure-admitted at RR 1.2: LOW 8, MID 7, HIGH 4.
	defaultV7YieldRR12Low  = 0.0010437395873594451
	defaultV7YieldRR12Mid  = 0.00070229065666834942
	defaultV7YieldRR12High = 0.00025624876158097557
	// t2_m2_eval structure-admitted at RR 1.5: LOW 4, MID 3, HIGH 2.
	defaultV7YieldRR15Low  = 0.00035823764990817032
	defaultV7YieldRR15Mid  = 0.00017479509345098859
	defaultV7YieldRR15High = 6.6646944992187609e-05
)
