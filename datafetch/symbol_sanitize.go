package datafetch

import "regexp"

// #94 / AUD-B6: exchange symbols are restricted to [A-Z0-9] at ingestion.
// Every symbol reaches the LLM prompt, and plain-text prompt lines are
// line-oriented; a symbol with any other character (newline, space,
// punctuation, non-ASCII) is dropped here instead of being interpolated.
var validSymbolRe = regexp.MustCompile(`^[A-Z0-9]+$`)

// IsValidSymbol reports whether an exchange symbol uses only [A-Z0-9].
func IsValidSymbol(symbol string) bool {
	return validSymbolRe.MatchString(symbol)
}
