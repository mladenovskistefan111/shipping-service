# API

Package `hipstershop`, service `ShippingService` ([`proto/shipping.proto`](https://github.com/platform-boutique/app-shipping-service/blob/main/proto/shipping.proto)).

| Method | Request | Response | Behaviour |
|---|---|---|---|
| `GetQuote` | `{ Address address, repeated CartItem items }` | `{ Money cost_usd }` | Sums item quantities. **0 items → $0.00; any items → flat $8.99 USD** (address is ignored) |
| `ShipOrder` | `{ Address address, repeated CartItem items }` | `{ string tracking_id }` | Builds a tracking ID like `XK-21345-109876543` from random letters/digits and the length of `"<street>, <city>, <state>"`. Nothing is dispatched or stored. The request **must include an address** — a nil address dereferences a nil pointer, and gRPC-Go does not recover handler panics, so it would crash the process |

`Money` = `{ currency_code, int64 units, int32 nanos }`. `frontend` calls `GetQuote` with a nil address and the cart items for the cart-page estimate; checkout passes the real address.
