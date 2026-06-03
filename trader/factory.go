package trader

import (
	"fmt"
	"nofx/store"
	"nofx/trader/aster"
	"nofx/trader/binance"
	"nofx/trader/bitget"
	"nofx/trader/bybit"
	"nofx/trader/gate"
	"nofx/trader/hyperliquid"
	"nofx/trader/indodax"
	"nofx/trader/kucoin"
	"nofx/trader/lighter"
	"nofx/trader/okx"
)

// NewTraderFromExchange creates a trader adapter from a persisted exchange account.
// It is the shared factory for API probes and one-off manual operations.
func NewTraderFromExchange(exchangeCfg *store.Exchange, userID string) (Trader, error) {
	if exchangeCfg == nil {
		return nil, fmt.Errorf("exchange config is nil")
	}

	switch exchangeCfg.ExchangeType {
	case "binance":
		return binance.NewFuturesTrader(string(exchangeCfg.APIKey), string(exchangeCfg.SecretKey), userID, exchangeCfg.ProxyURL), nil
	case "bybit":
		return bybit.NewBybitTrader(string(exchangeCfg.APIKey), string(exchangeCfg.SecretKey)), nil
	case "okx":
		return okx.NewOKXTrader(string(exchangeCfg.APIKey), string(exchangeCfg.SecretKey), string(exchangeCfg.Passphrase)), nil
	case "bitget":
		return bitget.NewBitgetTrader(string(exchangeCfg.APIKey), string(exchangeCfg.SecretKey), string(exchangeCfg.Passphrase)), nil
	case "gate":
		return gate.NewGateTrader(string(exchangeCfg.APIKey), string(exchangeCfg.SecretKey)), nil
	case "kucoin":
		return kucoin.NewKuCoinTrader(string(exchangeCfg.APIKey), string(exchangeCfg.SecretKey), string(exchangeCfg.Passphrase)), nil
	case "indodax":
		return indodax.NewIndodaxTrader(string(exchangeCfg.APIKey), string(exchangeCfg.SecretKey)), nil
	case "hyperliquid":
		return hyperliquid.NewHyperliquidTrader(
			string(exchangeCfg.APIKey),
			exchangeCfg.HyperliquidWalletAddr,
			exchangeCfg.Testnet,
			exchangeCfg.HyperliquidUnifiedAcct,
		)
	case "aster":
		return aster.NewAsterTrader(
			exchangeCfg.AsterUser,
			exchangeCfg.AsterSigner,
			string(exchangeCfg.AsterPrivateKey),
		)
	case "lighter":
		if exchangeCfg.LighterWalletAddr == "" || string(exchangeCfg.LighterAPIKeyPrivateKey) == "" {
			return nil, fmt.Errorf("Lighter requires wallet address and API Key private key")
		}
		// Lighter only supports mainnet in the current adapter.
		return lighter.NewLighterTraderV2(
			exchangeCfg.LighterWalletAddr,
			string(exchangeCfg.LighterAPIKeyPrivateKey),
			exchangeCfg.LighterAPIKeyIndex,
			false,
		)
	default:
		return nil, fmt.Errorf("unsupported exchange type: %s", exchangeCfg.ExchangeType)
	}
}
