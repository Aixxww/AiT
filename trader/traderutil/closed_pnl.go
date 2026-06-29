package traderutil

import (
	"strings"

	"github.com/Aixxww/AiT/trader/types"
)

// ClosedPnLFromTrades converts a slice of TradeRecords into ClosedPnLRecords
// by filtering closing trades (RealizedPnL != 0), determining position side,
// and back-calculating the entry price.
//
// This logic is shared across Binance, Aster, Hyperliquid, and any other
// exchange that derives closed PnL from individual trade fills.
func ClosedPnLFromTrades(trades []types.TradeRecord) []types.ClosedPnLRecord {
	var records []types.ClosedPnLRecord
	for _, trade := range trades {
		if trade.RealizedPnL == 0 {
			continue
		}

		side := inferClosedSide(trade)

		var entryPrice float64
		if trade.Quantity > 0 {
			if side == "long" {
				entryPrice = trade.Price - trade.RealizedPnL/trade.Quantity
			} else {
				entryPrice = trade.Price + trade.RealizedPnL/trade.Quantity
			}
		}

		records = append(records, types.ClosedPnLRecord{
			Symbol:      trade.Symbol,
			Side:        side,
			EntryPrice:  entryPrice,
			ExitPrice:   trade.Price,
			Quantity:    trade.Quantity,
			RealizedPnL: trade.RealizedPnL,
			Fee:         trade.Fee,
			ExitTime:    trade.Time,
			EntryTime:   trade.Time,
			OrderID:     trade.TradeID,
			ExchangeID:  trade.TradeID,
			CloseType:   "unknown",
		})
	}
	return records
}

// inferClosedSide determines the position side that was closed.
func inferClosedSide(trade types.TradeRecord) string {
	ps := strings.ToUpper(trade.PositionSide)
	switch ps {
	case "SHORT":
		return "short"
	case "LONG":
		return "long"
	default:
		// One-way mode (BOTH or empty): selling closes long, buying closes short
		s := strings.ToUpper(trade.Side)
		if s == "SELL" {
			return "long"
		}
		return "short"
	}
}
