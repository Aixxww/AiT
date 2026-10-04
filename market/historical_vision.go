package market

// data.binance.vision fallback for K-line downloads.
//
// Rationale: fapi.binance.com is geo-restricted in some regions (e.g. US IPs
// get HTTP 451 "restricted location"). data.binance.vision is Binance's
// official public market-data archive (S3-backed, no API key, no geo block)
// serving the same USDⓈ-M futures klines as daily ZIP archives.
//
// GetKlinesRange tries fapi first and falls back to this automatically, so
// backtests and data collection keep working from restricted networks.
// Downloaded day-files are cached under os.TempDir()/ait-vision-cache to
// avoid re-downloading on repeated runs.

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	visionBaseURL = "https://data.binance.vision/data/futures/um/daily/klines"
	visionTimeout = 60 * time.Second
)

// getKlinesRangeViaVision fetches klines from data.binance.vision daily archives.
func getKlinesRangeViaVision(symbol, normTF string, start, end time.Time) ([]Kline, error) {
	startMs := start.UnixMilli()
	endMs := end.UnixMilli()

	client := &http.Client{Timeout: visionTimeout}

	var all []Kline
	// Iterate UTC calendar days overlapping [start, end).
	day := time.Date(start.UTC().Year(), start.UTC().Month(), start.UTC().Day(), 0, 0, 0, 0, time.UTC)
	lastDay := time.Date(end.UTC().Year(), end.UTC().Month(), end.UTC().Day(), 0, 0, 0, 0, time.UTC)
	for d := day; !d.After(lastDay); d = d.AddDate(0, 0, 1) {
		klines, err := getVisionDayKlines(client, symbol, normTF, d)
		if err != nil {
			// A missing day-file (404) usually means the symbol did not exist
			// yet or the archive has no data for that day — skip, don't fail.
			if isVisionNotFound(err) {
				continue
			}
			return nil, err
		}
		for _, k := range klines {
			if k.OpenTime >= startMs && k.OpenTime < endMs {
				all = append(all, k)
			}
		}
	}

	if len(all) == 0 {
		return nil, fmt.Errorf("data.binance.vision: no klines for %s %s in range", symbol, normTF)
	}

	sort.Slice(all, func(i, j int) bool { return all[i].OpenTime < all[j].OpenTime })
	// Deduplicate on OpenTime (day boundaries can overlap by one kline).
	out := all[:0]
	for i, k := range all {
		if i == 0 || k.OpenTime != all[i-1].OpenTime {
			out = append(out, k)
		}
	}
	return out, nil
}

type visionNotFoundError struct{ msg string }

func (e *visionNotFoundError) Error() string { return e.msg }

func isVisionNotFound(err error) bool {
	_, ok := err.(*visionNotFoundError)
	return ok
}

func getVisionDayKlines(client *http.Client, symbol, normTF string, day time.Time) ([]Kline, error) {
	dateStr := day.Format("2006-01-02")
	fileName := fmt.Sprintf("%s-%s-%s.zip", symbol, normTF, dateStr)
	url := fmt.Sprintf("%s/%s/%s/%s", visionBaseURL, symbol, normTF, fileName)

	body, err := visionCachedDownload(client, symbol, normTF, fileName, url)
	if err != nil {
		return nil, err
	}

	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, fmt.Errorf("data.binance.vision: bad zip %s: %w", fileName, err)
	}
	if len(zr.File) == 0 {
		return nil, &visionNotFoundError{msg: "empty zip " + fileName}
	}

	rc, err := zr.File[0].Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	records, err := csv.NewReader(rc).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("data.binance.vision: bad csv %s: %w", fileName, err)
	}

	klines := make([]Kline, 0, len(records))
	for _, r := range records {
		if len(r) < 11 {
			continue
		}
		openTime, err := strconv.ParseInt(strings.TrimSpace(r[0]), 10, 64)
		if err != nil {
			continue
		}
		k := Kline{OpenTime: openTime}
		k.Open, _ = strconv.ParseFloat(strings.TrimSpace(r[1]), 64)
		k.High, _ = strconv.ParseFloat(strings.TrimSpace(r[2]), 64)
		k.Low, _ = strconv.ParseFloat(strings.TrimSpace(r[3]), 64)
		k.Close, _ = strconv.ParseFloat(strings.TrimSpace(r[4]), 64)
		k.Volume, _ = strconv.ParseFloat(strings.TrimSpace(r[5]), 64)
		k.CloseTime, _ = strconv.ParseInt(strings.TrimSpace(r[6]), 10, 64)
		k.QuoteVolume, _ = strconv.ParseFloat(strings.TrimSpace(r[7]), 64)
		trades, _ := strconv.Atoi(strings.TrimSpace(r[8]))
		k.Trades = trades
		k.TakerBuyBaseVolume, _ = strconv.ParseFloat(strings.TrimSpace(r[9]), 64)
		k.TakerBuyQuoteVolume, _ = strconv.ParseFloat(strings.TrimSpace(r[10]), 64)
		klines = append(klines, k)
	}
	return klines, nil
}

// visionCachedDownload downloads url once and caches the bytes on disk.
func visionCachedDownload(client *http.Client, symbol, normTF, fileName, url string) ([]byte, error) {
	cachePath := filepath.Join(os.TempDir(), "ait-vision-cache", symbol, normTF, fileName)
	if data, err := os.ReadFile(cachePath); err == nil && len(data) > 0 {
		return data, nil
	}

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("data.binance.vision download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, &visionNotFoundError{msg: "not found: " + url}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("data.binance.vision returned status %d for %s", resp.StatusCode, url)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, &visionNotFoundError{msg: "empty body: " + url}
	}
	// Best-effort cache write; a failed write must not fail the fetch.
	_ = os.MkdirAll(filepath.Dir(cachePath), 0o755)
	_ = os.WriteFile(cachePath, data, 0o644)
	return data, nil
}
