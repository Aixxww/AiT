package kernel

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Entropy Recoil E0.3 — lock the composite-gate surface in engine.go so
// live-patch sessions cannot silently re-grow setup-specific predicates.
// Caps start as a baseline lock and tighten toward the targets in
// docs/hunter-v7-entropy-recoil-20260813.md §3.5 after E5.1.

var hunterV7CompositeGateName = regexp.MustCompile(`^hunterV7(AltLadder|Whale|MMS).*$`)

func TestHunterV7EntropyBudget(t *testing.T) {
	enginePath := filepath.Join("engine.go")
	src, err := os.ReadFile(enginePath)
	if err != nil {
		t.Fatalf("read engine.go: %v", err)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, enginePath, src, 0)
	if err != nil {
		t.Fatalf("parse engine.go: %v", err)
	}

	type gateFn struct {
		name  string
		lines int
	}
	var gates []gateFn
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Body == nil {
			continue
		}
		name := fn.Name.Name
		if !hunterV7CompositeGateName.MatchString(name) {
			continue
		}
		start := fset.Position(fn.Pos()).Line
		end := fset.Position(fn.End()).Line
		gates = append(gates, gateFn{name: name, lines: end - start + 1})
	}

	const (
		maxGateFns      = 16 // baseline lock; target ≤3 after E5.1
		maxGateLineSum  = 280 // baseline lock; target ≤80 after E5.1
		maxSingleGateFn = 80
	)

	totalLines := 0
	names := make([]string, 0, len(gates))
	for _, g := range gates {
		totalLines += g.lines
		names = append(names, g.name)
		if g.lines > maxSingleGateFn {
			t.Errorf("composite gate %s is %d lines (max %d); move thresholds into tier_rules.go", g.name, g.lines, maxSingleGateFn)
		}
	}
	if len(gates) > maxGateFns {
		t.Errorf("engine.go has %d AltLadder/Whale/MMS composite gates (max %d): %v", len(gates), maxGateFns, names)
	}
	if totalLines > maxGateLineSum {
		t.Errorf("AltLadder/Whale/MMS composite gates total %d lines (max %d)", totalLines, maxGateLineSum)
	}

	// Soft-release numeric literals must not reappear in engine.go helper
	// bodies now that they live as named constants in tier_rules.go.
	forbidden := []string{
		"entryZonePos > 45",
		"ConfirmedAtMost(coin, 0.38)",
		"ConfirmedAtMost(coin, 0.34)",
		"stopDistancePct <= 2.25",
		"stopDistancePct <= 2.45",
		"OIChange1h >= 0.8",
	}
	engineText := string(src)
	for _, lit := range forbidden {
		if strings.Contains(engineText, lit) {
			t.Errorf("engine.go still embeds soft-release literal pattern %q; use tier_rules constants/rows", lit)
		}
	}
}
