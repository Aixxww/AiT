// Package btcregime loads BTCUSDT futures klines from data.binance.vision
// archive zips (the files are never committed) and builds the U13 RV24
// series over them. Shared by cmd/t2_u13_yield and cmd/t2_m2_eval.
package btcregime

import (
	"archive/zip"
	"encoding/csv"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Aixxww/AiT/store"
)

// Interval lengths of the archive timeframes the RV24 series reads.
var intervalMs = map[string]int64{"1m": 60_000, "5m": 300_000, "1h": 3_600_000}

// IntervalMs returns a timeframe's bar length.
func IntervalMs(tf string) (int64, error) {
	ms, ok := intervalMs[tf]
	if !ok {
		return 0, fmt.Errorf("unsupported timeframe %q", tf)
	}
	return ms, nil
}

// LoadArchive reads every <symbol>-<tf>-*.zip in dir (monthly and daily
// archives may overlap; duplicates by open time are dropped) and returns
// the bars sorted by open time.
func LoadArchive(dir, symbol, tf string) ([]store.V7PriceBar, error) {
	files, err := filepath.Glob(filepath.Join(dir, fmt.Sprintf("%s-%s-*.zip", symbol, tf)))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no %s %s archives in %s", symbol, tf, dir)
	}
	seen := map[int64]bool{}
	var out []store.V7PriceBar
	for _, f := range files {
		zr, err := zip.OpenReader(f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		for _, zf := range zr.File {
			rc, err := zf.Open()
			if err != nil {
				zr.Close()
				return nil, err
			}
			rows, err := csv.NewReader(rc).ReadAll()
			rc.Close()
			if err != nil && err != io.EOF {
				zr.Close()
				return nil, fmt.Errorf("%s: %w", f, err)
			}
			for _, r := range rows {
				if len(r) < 5 || strings.HasPrefix(r[0], "open") {
					continue // header
				}
				open, err1 := strconv.ParseInt(r[0], 10, 64)
				cl, err2 := strconv.ParseFloat(r[4], 64)
				if err1 != nil || err2 != nil {
					zr.Close()
					return nil, fmt.Errorf("%s: bad row %v", f, r)
				}
				if open > 1e14 { // microsecond timestamps in newer archives
					open /= 1000
				}
				if !seen[open] {
					seen[open] = true
					out = append(out, store.V7PriceBar{OpenMs: open, Close: cl})
				}
			}
		}
		zr.Close()
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OpenMs < out[j].OpenMs })
	return out, nil
}

// Series loads BTCUSDT 1h and the forming-bar timeframe fine from dir.
func Series(dir, fine string, cfg store.V7YieldTargetConfig) (*store.V7RV24Series, error) {
	fineMs, err := IntervalMs(fine)
	if err != nil {
		return nil, err
	}
	h1, err := LoadArchive(dir, "BTCUSDT", "1h")
	if err != nil {
		return nil, err
	}
	fb, err := LoadArchive(dir, "BTCUSDT", fine)
	if err != nil {
		return nil, err
	}
	return store.NewV7RV24Series(cfg, h1, fb, fineMs), nil
}
