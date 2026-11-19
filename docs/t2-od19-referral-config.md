# OD-19: referral/broker ids in exchange config (F22)

Owner decision: option (b). Move the hard-coded referral/broker ids into exchange config and split the Binance client-id prefix. Default empty. The owner has no referral id of their own, so nothing is filled in.

Rejected: (a) keep the hard-coded ids; (c) remove the mechanism entirely. Keeping today's values as the default was not chosen.

## Config

`store.Exchange.ReferralID` (column `referral_id`, JSON `referral_id`), one per exchange account, `NOT NULL DEFAULT ''`.

- SQLite: added by `AutoMigrate`.
- PostgreSQL with an existing `exchanges` table (where `AutoMigrate` is skipped): `ALTER TABLE exchanges ADD COLUMN IF NOT EXISTS referral_id TEXT NOT NULL DEFAULT ''` in `initTables`.
- Existing accounts get `''`, so after the upgrade no referral data is sent by any adapter.
- Threaded through `manager/trader_manager.go` → `AutoTraderConfig.ReferralID` → each adapter's `SetReferralID`, and through `trader/factory.go` (API probes and one-off operations).
- Not exposed in the exchange API or web UI in this unit. The owner has no id to enter; it can be set in the DB if one is ever needed.
- If the PostgreSQL `ALTER TABLE` fails, the error is logged (`exchanges: adding column referral_id failed`) and startup continues. Without the column, reads still work and `referral_id` reads as empty. Creating a new exchange account fails until the column exists.
- Each adapter reads `referral_id` once, when the trader is built. A changed value takes effect only after the trader is rebuilt, for example on a restart.
- Bybit: if the trader has no HTTP transport, `SetReferralID` logs a warning and sends nothing.

### Rule: do not change `referral_id` while orders or open intents exist

Change an account's `referral_id` only when that account has no open orders and no open order still being placed (submitted, not yet confirmed).

Validation (`traderutil.NormalizeReferralID`): trimmed, 1..N ASCII letters or digits. An invalid value is logged and dropped, so a malformed id is never sent. N is 12 for Binance (room for the client-id body), 16 for OKX (`tag` limit), 32 for Bybit, Bitget and Gate.

## Per exchange

| Exchange | Before OD-19 (hard-coded) | Now, default (empty) | Now, configured |
|---|---|---|---|
| Binance | client-id prefix `x-KzrpZaP9` on every order | client id `ait-` + 21-char body; no `x-` broker prefix | `x-<id>ait-` + body, ≤ 32 chars |
| Bybit | `Referer: Up000938` on every request | no `Referer` header | `Referer: <id>` |
| Bitget | `X-CHANNEL-API-CODE: 7fygt` on `/order/` paths | no header | `X-CHANNEL-API-CODE: <id>` on `/order/` paths |
| OKX | order `tag` `4c363c81edc5BCDE`, also used as the `clOrdId` prefix | no `tag` field; `clOrdId` = `ait` + 13-digit timestamp + 8 hex (24 chars) | `tag: <id>`; `clOrdId` unchanged |
| Gate | `X-Gate-Channel-Id: ait` default header | no header | `X-Gate-Channel-Id: <id>` |
| Hyperliquid | builder already `nil` | unchanged | unchanged |

Gate's `ait` may have been the app's own name, not a third-party referral. Per the owner it defaults to empty too; set `referral_id = 'ait'` on the Gate account to restore it.

## Binance client ids

Code: `trader/binance/client_ids.go`.

A client id now has two parts:

- **Referral part** `x-<referral_id>`, only when configured. Binance attributes broker orders by this prefix.
- **Ownership part** `ait-`, always present. F26/F27 (owned, side-scoped cancels) and F28 (`ProtectiveStopOpen`) use it to recognise this bot's orders.

Body: 13-digit nanosecond timestamp + 8 hex random (21 chars). With a referral prefix the body is cut from the left to fit 32 chars, keeping the random part (at least 14 chars with the 12-char maximum referral). F04 intent ids (`ClientOrderIDForIntent`) use the same prefix plus the leading sha256 hex of the intent key, so they stay deterministic for a given config.

**Why the rule above.** F04 intent ids include the referral part, so the same intent key gives a different client id under a different `referral_id`. The F04 read-back looks the order up by the id it just submitted, within the same process, so a change cannot take effect during one read-back. The rule makes this hold by procedure rather than by timing, and keeps every order of an account under one prefix while it is open. Open orders stay owned after a change (see below); the rule is about traceability, not ownership. The intent id is not derived from the ownership part alone, because that would drop the referral prefix from opens, the main orders a referral is credited for.

Ownership (`isOwnedClientID`) accepts:

- `ait-…` (new, no referral);
- `x-<any 1–12 alphanumerics>ait-…` (new, with a referral, including one changed while orders are open);
Not owned: manual orders, `x-ftGmvgAN…` (the go-binance default), and `x-KzrpZaP9…` (the pre-OD-19 prefix, see below).

**Legacy prefix removed (referral-leftovers cleanup, after #35).** During the transition, `x-KzrpZaP9…` was also accepted as owned, so orders placed before the upgrade stayed under F26/F27/F28. That recognition is now removed (`legacyOwnedOrderPrefix` is gone). It was removed after the owner verified that the live Binance account had 0 open orders, 0 open algo orders and 0 open positions, so no resting order with that prefix could be left unowned. Any such order found later is treated like a manual order: the bot neither cancels nor counts it.

**go-binance auto prefix.** go-binance v2.8.9 fills in its own broker prefix (`x-ftGmvgAN`, from `common.GenerateSwapId`) when an order is sent without a client id. Every order this package creates sets its own id: the 4 regular `NewCreateOrderService` calls use `NewClientOrderID`, and the 2 `NewCreateAlgoOrderService` calls use `ClientAlgoId`.

**Order guard** (`trader/binance/order_guard.go`). There is one choke point for any new create call: `NewFuturesTrader` wraps the client's HTTP transport, after the hook that may replace the client. The wrapper checks every POST to these endpoints:

- `/fapi/v1/order` and `/fapi/v1/order/test`: `newClientOrderId`;
- `/fapi/v1/batchOrders`: `newClientOrderId` of each order;
- `/fapi/v1/algoOrder`: `clientAlgoId`.

It reads the query string and the form body. If an id is missing or not owned (`isOwnedClientID`), the request is not sent. The caller gets an error wrapping `ErrUnownedClientID`, and the guard logs it at error level. `IsAmbiguousOrderError` returns false for this error, because the order was definitely not placed, so F04 does not run a read-back. Requests that only query, cancel or amend existing orders are not checked. `trader/binance/futures.go` is the only place that builds a go-binance futures client, so a new order path cannot bypass the guard. The one exception is go-binance's websocket order service, which does not use the HTTP transport; a test keeps it out of the package.

The guard changes nothing in live trading today, because all 6 current paths already send their own id.

Charset: the ids use only `[A-Za-z0-9-]`, inside Binance's documented client-id character set, and stay ≤ 32 chars (the limit this package already used).

## Tests

- `trader/binance/client_ids_test.go`:
  - by default there is no `x-` part and no `KzrpZaP9`;
  - a configured id is sent as the `x-<id>` prefix and the id stays ≤ 32 chars;
  - invalid ids are dropped;
  - intent ids are deterministic;
  - ownership covers the new and changed-referral forms, and the removed `x-KzrpZaP9` prefix is not owned.
- `trader/binance/order_guard_test.go`:
  - all 6 current paths pass the guard and send our id, with and without a referral id;
  - a create call with no id (go-binance's `x-ftGmvgAN`), with a foreign id, an algo order without `ClientAlgoId`, and a batch with one id missing are each refused, and nothing reaches the mock exchange;
  - query, cancel and amend requests pass unchecked;
  - installing the guard twice has no extra effect;
  - no websocket order service is used in the package.
  - Mutation check (run once, not committed): removing `NewClientOrderID` from `CloseLong` makes the path test fail, with the guard refusing `x-ftGmvgAN…`.
- The Binance mock-exchange test helpers install the guard too, so the existing order tests run through it.
- `trader/bybit/referral_test.go`, `trader/bitget/referral_test.go`, `trader/okx/referral_test.go`, `trader/gate/referral_test.go`: by default nothing is sent; a configured id is sent.
- `store/exchange_referral_test.go`: the column exists and defaults to empty.
- The existing F04/F26/F27/F28 tests pass unchanged. They build ids from `ownedOrderPrefix`, which is now `ait-`.

No live or pilot run. `geometry_mode` is untouched.
