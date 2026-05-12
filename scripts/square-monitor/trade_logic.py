"""
Stub trade_logic module - provides no-op implementations
of the functions that web.py imports.

The real trading logic lives in the original binance-square-monitor project.
This stub allows web.py to start and serve the /api/leaderboard endpoint
which is the only endpoint the AiT Go backend consumes.
"""

from typing import Any, Dict, List, Optional


def manual_open_on_watch(conn: Any, token: str, settings: Any) -> Optional[Dict]:
    """Stub: no-op trading on watchlist add."""
    return None


def manual_close_on_unwatch(conn: Any, token: str) -> Optional[Dict]:
    """Stub: no-op trading on watchlist remove."""
    return None


def account_summary(conn: Any) -> Dict[str, Any]:
    """Stub: empty account summary."""
    return {
        "balance": 0.0,
        "equity": 0.0,
        "unrealized_pnl": 0.0,
        "positions_count": 0,
        "mode": "stub",
    }


def build_trade_candidates_from_leaderboard(
    conn: Any,
    leaderboard_items: List[Dict],
    passed_only: bool = True,
) -> List[Dict]:
    """Stub: no trade candidates since we don't have trading logic."""
    return []
