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

Ownership (`isOwnedClientID`) accepts:

- `ait-…` (new, no referral);
- `x-<any 1–12 alphanumerics>ait-…` (new, with a referral, including one changed while orders are open);
- `x-KzrpZaP9…` (legacy). Orders placed before the upgrade, such as resting stops and TPs, stay owned, so F26/F27 still cancel them and F28 still counts them. Nothing new is placed with this prefix. It can be dropped once no legacy order is open on any account.

Not owned: manual orders, and `x-ftGmvgAN…`, the go-binance default.

**go-binance auto prefix.** go-binance v2.8.9 fills in its own broker prefix (`x-ftGmvgAN`, from `common.GenerateSwapId`) when an order is sent without a client id. Every order this package creates sets its own id: the 4 regular `NewCreateOrderService` calls use `NewClientOrderID`, and the 2 `NewCreateAlgoOrderService` calls use `ClientAlgoId`. Any new create call must do the same.

Charset: the ids use only `[A-Za-z0-9-]`, inside Binance's documented client-id character set, and stay ≤ 32 chars (the limit this package already used).

## Tests

- `trader/binance/client_ids_test.go`:
  - by default there is no `x-` part and no `KzrpZaP9`;
  - a configured id is sent as the `x-<id>` prefix and the id stays ≤ 32 chars;
  - invalid ids are dropped;
  - intent ids are deterministic;
  - ownership covers the legacy, new and changed-referral forms;
  - a legacy-prefix order is still cancelled by `CancelOwnedOrders`.
- `trader/bybit/referral_test.go`, `trader/bitget/referral_test.go`, `trader/okx/referral_test.go`, `trader/gate/referral_test.go`: by default nothing is sent; a configured id is sent.
- `store/exchange_referral_test.go`: the column exists and defaults to empty.
- The existing F04/F26/F27/F28 tests pass unchanged. They build ids from `ownedOrderPrefix`, which is now `ait-`.

No live or pilot run. `geometry_mode` is untouched.
