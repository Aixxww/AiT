package datafetch

import (
	"net/url"
	"strconv"
	"strings"
)

// binanceRequestWeight is the IP request weight of one public USDS-M futures
// request (fix 05). Values were measured read-only from the
// X-MBX-USED-WEIGHT-1M deltas (2026-11-21): klines limit 60/50/100 -> 1,
// limit 120 -> 2; depth limit 100 -> 5; openInterest 1; premiumIndex (all
// symbols) 10; ticker/24hr (all symbols) 40; exchangeInfo 1; /futures/data/*
// carry no X-MBX-USED-WEIGHT header (separate limit, weight 0 here). Other
// bands follow the documented tables.
func binanceRequestWeight(path string) float64 {
	p, rawQuery, _ := strings.Cut(path, "?")
	q, _ := url.ParseQuery(rawQuery)
	limit, _ := strconv.Atoi(q.Get("limit"))
	hasSymbol := q.Get("symbol") != ""
	switch {
	case strings.HasPrefix(p, "/futures/data/"):
		return 0
	case p == "/fapi/v1/klines" || p == "/fapi/v1/continuousKlines" || p == "/fapi/v1/markPriceKlines":
		switch {
		case limit <= 0:
			return 2 // default limit 500
		case limit <= 100:
			return 1
		case limit <= 500:
			return 2
		case limit <= 1000:
			return 5
		default:
			return 10
		}
	case p == "/fapi/v1/depth":
		switch {
		case limit <= 0:
			return 10 // default limit 500
		case limit <= 50:
			return 2
		case limit <= 100:
			return 5
		case limit <= 500:
			return 10
		default:
			return 20
		}
	case p == "/fapi/v1/ticker/24hr":
		if hasSymbol {
			return 1
		}
		return 40
	case p == "/fapi/v1/premiumIndex":
		if hasSymbol {
			return 1
		}
		return 10
	default:
		return 1
	}
}
