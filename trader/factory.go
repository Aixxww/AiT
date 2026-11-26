package trader

import (
	"fmt"
	"github.com/Aixxww/AiT/store"
	"github.com/Aixxww/AiT/trader/aster"
	"github.com/Aixxww/AiT/trader/binance"
	"github.com/Aixxww/AiT/trader/bitget"
	"github.com/Aixxww/AiT/trader/bybit"
	"github.com/Aixxww/AiT/trader/gate"
	"github.com/Aixxww/AiT/trader/hyperliquid"
	"github.com/Aixxww/AiT/trader/indodax"
	"github.com/Aixxww/AiT/trader/kucoin"
	"github.com/Aixxww/AiT/trader/lighter"
	"github.com/Aixxww/AiT/trader/okx"
	"os"
)

// NewTraderFromExchange creates a trader adapter from a persisted exchange account.
// It is the shared factory for API probes and one-off manual operations.
func NewTraderFromExchange(exchangeCfg *store.Exchange, userID string) (Trader, error) {
	if exchangeCfg == nil {
		return nil, fmt.Errorf("exchange config is nil")
	}

	switch exchangeCfg.ExchangeType {
	case "binance":
		t := binance.NewFuturesTrader(string(exchangeCfg.APIKey), string(exchangeCfg.SecretKey), userID, exchangeCfg.ProxyURL)
		t.SetReferralID(exchangeCfg.ReferralID)
		return t, nil
	case "bybit":
		t := bybit.NewBybitTrader(string(exchangeCfg.APIKey), string(exchangeCfg.SecretKey))
		t.SetReferralID(exchangeCfg.ReferralID)
		return t, nil
	case "okx":
		t := okx.NewOKXTrader(string(exchangeCfg.APIKey), string(exchangeCfg.SecretKey), string(exchangeCfg.Passphrase))
		t.SetReferralID(exchangeCfg.ReferralID)
		return t, nil
	case "bitget":
		t := bitget.NewBitgetTrader(string(exchangeCfg.APIKey), string(exchangeCfg.SecretKey), string(exchangeCfg.Passphrase))
		t.SetReferralID(exchangeCfg.ReferralID)
		return t, nil
	case "gate":
		t := gate.NewGateTrader(string(exchangeCfg.APIKey), string(exchangeCfg.SecretKey))
		t.SetReferralID(exchangeCfg.ReferralID)
		return t, nil
	case "kucoin":
		t := kucoin.NewKuCoinTrader(string(exchangeCfg.APIKey), string(exchangeCfg.SecretKey), string(exchangeCfg.Passphrase))
		t.SetPartner(exchangeCfg.ReferralID, os.Getenv(kucoin.PartnerKeyEnv)) // #127: opt-in
		return t, nil
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
