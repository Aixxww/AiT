package coinank_api

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

func requireCoinankLiveTest(t *testing.T) {
	t.Helper()
	if os.Getenv("AIT_LIVE_TESTS") != "1" && os.Getenv("COINANK_LIVE_TESTS") != "1" {
		t.Skip("skipping CoinAnk live API test; set AIT_LIVE_TESTS=1 or COINANK_LIVE_TESTS=1 to run")
	}
}

func TestBaseCoinSymbolsNoArgs(t *testing.T) {
	requireCoinankLiveTest(t)
	resp, err := BaseCoinSymbols(context.TODO(), "", "", "")
	if err != nil {
		t.Error(err)
	}
	res, err := json.Marshal(resp)
	if err != nil {
		t.Error(err)
	}
	t.Logf("%s", res)
}

func TestBaseCoinSymbolsBTC(t *testing.T) {
	requireCoinankLiveTest(t)
	resp, err := BaseCoinSymbols(context.TODO(), "", "", "BTC")
	if err != nil {
		t.Error(err)
	}
	res, err := json.Marshal(resp)
	if err != nil {
		t.Error(err)
	}
	t.Logf("%s", res)
}

func TestBaseCoinSymbolsBTCUSDT(t *testing.T) {
	requireCoinankLiveTest(t)
	resp, err := BaseCoinSymbols(context.TODO(), "", "BTCUSDT", "")
	if err != nil {
		t.Error(err)
	}
	res, err := json.Marshal(resp)
	if err != nil {
		t.Error(err)
	}
	t.Logf("%s", res)
}
