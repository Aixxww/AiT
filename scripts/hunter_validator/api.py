"""
Hunter Validator - Binance Futures API 客户端
带限速、磁盘缓存、自动重试。
"""

import json
import os
import ssl
import time
import hashlib
from typing import Any, Optional
from urllib.request import urlopen, Request
from urllib.error import HTTPError, URLError
from urllib.parse import urlencode

# 跳过 SSL 验证 (代理网络常见自签名证书)
_SSL_CTX = ssl.create_default_context()
_SSL_CTX.check_hostname = False
_SSL_CTX.verify_mode = ssl.CERT_NONE

BINANCE_FAPI = "https://fapi.binance.com"
CACHE_DIR = os.path.expanduser("~/.gstack/hunter_validator_cache")
RATE_LIMIT_INTERVAL = 0.1  # 100ms between requests


class BinanceAPI:
    """Binance Futures REST API 客户端。"""

    def __init__(self, cache_ttl: int = 300):
        os.makedirs(CACHE_DIR, exist_ok=True)
        self._cache_ttl = cache_ttl
        self._last_request_time = 0.0

    def _rate_limit(self):
        elapsed = time.time() - self._last_request_time
        if elapsed < RATE_LIMIT_INTERVAL:
            time.sleep(RATE_LIMIT_INTERVAL - elapsed)
        self._last_request_time = time.time()

    def _cache_key(self, url: str) -> str:
        return hashlib.md5(url.encode()).hexdigest()

    def _cache_path(self, url: str) -> str:
        return os.path.join(CACHE_DIR, f"{self._cache_key(url)}.json")

    def _get_cached(self, url: str) -> Optional[Any]:
        path = self._cache_path(url)
        if not os.path.exists(path):
            return None
        try:
            with open(path) as f:
                cached = json.load(f)
            if time.time() - cached.get("ts", 0) > self._cache_ttl:
                return None
            return cached["data"]
        except (json.JSONDecodeError, KeyError):
            return None

    def _set_cache(self, url: str, data: Any):
        path = self._cache_path(url)
        with open(path, 'w') as f:
            json.dump({"ts": time.time(), "data": data}, f)

    def fetch_json(self, url: str, use_cache: bool = True) -> Any:
        """GET 请求，返回 JSON。带限速和磁盘缓存。"""
        if use_cache:
            cached = self._get_cached(url)
            if cached is not None:
                return cached

        self._rate_limit()
        req = Request(url, headers={"User-Agent": "HunterValidator/1.0"})

        for attempt in range(3):
            try:
                with urlopen(req, timeout=15, context=_SSL_CTX) as resp:
                    data = json.loads(resp.read().decode())
                    if use_cache:
                        self._set_cache(url, data)
                    return data
            except HTTPError as e:
                if e.code == 429:
                    time.sleep(2 ** attempt)
                    continue
                raise
            except URLError:
                if attempt < 2:
                    time.sleep(1)
                    continue
                raise

        return None

    # ── 高层 API 封装 ──

    def get_tickers_24hr(self) -> list[dict]:
        """获取所有 USDT 永续合约 24h ticker。"""
        url = f"{BINANCE_FAPI}/fapi/v1/ticker/24hr"
        return self.fetch_json(url)

    def get_klines(self, symbol: str, interval: str, limit: int = 20,
                   end_time: Optional[int] = None) -> list[dict]:
        """获取 K 线数据。end_time 为毫秒时间戳。"""
        params = {"symbol": symbol, "interval": interval, "limit": limit}
        if end_time:
            params["endTime"] = end_time
        url = f"{BINANCE_FAPI}/fapi/v1/klines?{urlencode(params)}"
        raw = self.fetch_json(url, use_cache=(end_time is None))
        if raw is None:
            return []
        return [{
            "open_time": k[0],
            "open": float(k[1]),
            "high": float(k[2]),
            "low": float(k[3]),
            "close": float(k[4]),
            "volume": float(k[5]),
            "close_time": k[6],
            "quote_volume": float(k[7]),
            "trades": int(k[8]),
            "taker_buy_volume": float(k[9]),
            "taker_buy_quote_volume": float(k[10]),
        } for k in raw]

    def get_open_interest(self, symbol: str) -> float:
        """获取当前 OI (合约张数)。"""
        url = f"{BINANCE_FAPI}/fapi/v1/openInterest?symbol={symbol}"
        data = self.fetch_json(url, use_cache=False)
        return float(data.get("openInterest", 0))

    def get_oi_history(self, symbol: str, period: str = "4h",
                       limit: int = 2) -> list[dict]:
        """获取 OI 历史数据。"""
        url = (f"{BINANCE_FAPI}/futures/data/openInterestHist"
               f"?symbol={symbol}&period={period}&limit={limit}")
        raw = self.fetch_json(url, use_cache=False)
        if not raw:
            return []
        return [{
            "symbol": e["symbol"],
            "sumOpenInterest": float(e["sumOpenInterest"]),
            "sumOpenInterestValue": float(e["sumOpenInterestValue"]),
            "timestamp": e["timestamp"],
        } for e in raw]

    def get_lsr_history(self, symbol: str, period: str = "1h",
                        limit: int = 4) -> list[dict]:
        """获取大户多空比历史。"""
        url = (f"{BINANCE_FAPI}/futures/data/topLongShortPositionRatio"
               f"?symbol={symbol}&period={period}&limit={limit}")
        raw = self.fetch_json(url, use_cache=False)
        if not raw:
            return []
        return [{
            "symbol": e["symbol"],
            "longShortRatio": float(e["longShortRatio"]),
            "longAccount": float(e["longAccount"]),
            "shortAccount": float(e["shortAccount"]),
            "timestamp": e["timestamp"],
        } for e in raw]

    def get_funding_rate_history(self, symbol: str, limit: int = 8) -> list[dict]:
        """获取资金费率历史。每8h一次，limit=8 = 最近64小时。"""
        url = (f"{BINANCE_FAPI}/fapi/v1/fundingRate"
               f"?symbol={symbol}&limit={limit}")
        raw = self.fetch_json(url, use_cache=False)
        if not raw:
            return []
        return [{
            "symbol": e["symbol"],
            "fundingRate": float(e["fundingRate"]),
            "fundingTime": e["fundingTime"],
        } for e in raw]

    # v6: 剔除代币化商品/股票 — 它们的行为模式与加密资产不同
    EXCLUDED_TOKENIZED = {
        "CL",      # 原油
        "XAU",     # 黄金
        "XAG",     # 白银
        "EWY",     # 韩国ETF
        "NVDA",    # 英伟达
        "MU",      # 美光
        "INTC",    # 英特尔
        "PAXG",    # Pax Gold
        "SPCX",    # S&P 500
        "BABA",    # 阿里巴巴
        "TSLA",    # 特斯拉
        "NATGAS",  # 天然气
    }

    def is_usdt_perp(self, symbol: str) -> bool:
        """判断是否为 USDT 永续合约 (排除杠杆币和代币化商品/股票)。"""
        if not symbol.endswith("USDT"):
            return False
        if any(symbol.endswith(s) for s in ["BULL", "BEAR", "UP", "DOWN"]):
            return False
        base = symbol.removesuffix("USDT")
        if base in self.EXCLUDED_TOKENIZED:
            return False
        return True

    def get_usdt_perp_tickers(self) -> list[dict]:
        """获取所有 USDT 永续合约 ticker，按成交额排序取 Top-50。"""
        tickers = self.get_tickers_24hr()
        perps = [t for t in tickers if self.is_usdt_perp(t["symbol"])]
        perps.sort(key=lambda t: float(t.get("quoteVolume", 0)), reverse=True)
        return perps
