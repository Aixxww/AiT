package local

import "testing"

// A code that qualifies a candidate for REVIEWABLE promotion must also be
// settleable by the trader's refresh — otherwise the funnel promotes
// candidates whose gap can never be closed at decision time.
func TestV7ConfirmLiveReviewableImpliesRefreshSatisfiable(t *testing.T) {
	for code, spec := range v7ConfirmRegistry {
		if spec.LiveReviewable && !spec.RefreshSatisfiable {
			t.Errorf("%s: LiveReviewable without RefreshSatisfiable — promoted candidates could never settle", code)
		}
	}
}

// Every registered confirmation code must carry catalog semantics; the LLM
// reads confirmations from the same vocabulary the machine settles.
func TestV7ConfirmRegistryCodesAreCatalogued(t *testing.T) {
	for code := range v7ConfirmRegistry {
		if _, ok := hunterV7TagCatalog[code]; !ok {
			t.Errorf("%s: registered confirmation code missing from tag catalog", code)
		}
	}
}
