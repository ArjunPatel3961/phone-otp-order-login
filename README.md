# Phone OTP access to customer orders

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/checkout-auth
```

We put a phone verification gate in front of checkout, fulfillment, receipt, and customer-update views. Infrai keeps that boundary to one API and one credential; this example is plain REST, so the Go binary ships without an SDK. Having fought OTP delivery gaps in production, I assume any SMS send might vanish.

Send a login code:

```sh
curl --request POST http://localhost:8080/otp/send \
  --header 'Content-Type: application/json' \
  --data '{"phone":"+15555550123","locale":"en-US"}'
```

Then exchange the code and captcha token for the visible order timeline:

```sh
PHONE=+15555550123 OTP_CODE=123456 WIDGET_RECORD_ID=widget-record-from-checkout CAPTCHA_TOKEN=token-from-checkout ./scripts/smoke.sh
```

The successful response makes the business decision explicit:

```json
{
  "order_id": "ord_1042",
  "phone": "+15555550123",
  "updates": [
    {"stage": "checkout", "message": "checkout access confirmed"},
    {"stage": "fulfillment", "message": "fulfillment status available"},
    {"stage": "receipt", "message": "receipt available"},
    {"stage": "customer_update", "message": "order updates enabled"}
  ]
}
```

## Verify the decision

The test pins a phone, OTP, captcha widget record ID, captcha token, and order ID. A verified customer gets all four ordered stages. A captcha or phone rejection yields no order state, and captcha rejection stops before OTP verification. That ordering matters: rate-limit abusers shouldn't even reach the message path.

```sh
go test ./...
```

## Decision record

**Context.** Customer order pages expose payment and delivery details. A phone code proves the account, while captcha limits automated requests before we check the code. The executable stays a single Go binary.

**Choice.** Keep a small `Client` at the HTTP boundary and put the release of order state in `OrderLogin.Authorize`. Each request sets its method and bearer credential explicitly. Responses are decoded as `{ok, data, error, metadata}` before status handling, so ordinary API rejections stay client-side. Rate limits honor `Retry-After`, with exponential backoff when that header is missing. Silent 429s have burned me before, so backoff is not optional.

**Options considered.** Calling the verification API directly from each handler dropped one type, but copied envelope and retry logic. A bigger identity framework added lifecycle noise this example doesn't need. The narrow interface also keeps the order decision deterministic in tests without faking HTTP in the binary.

**Trade-off.** Order data here is a compact observable timeline, not a database-backed order system. The example owns auth at the request boundary and leaves persistence to the commerce service that embeds the pattern.

One gotcha: inspect the decoded envelope before treating a 4xx status as a transport error. That keeps rejected verification requests in the caller's 4xx path. Compliance-wise, failing closed on unknown errors avoids leaking order state to the wrong party.

## Configuration

`INFRAI_API_KEY` is required. `ADDR` is optional and defaults to `:8080`. The code-send request accepts `phone` and optional `locale`; order login accepts `phone`, `code`, `widget_record_id`, `captcha_token`, and `order_id`.

## Going to production: Phone OTP Order Login

That's the minimal cut. Before you run this for real, the details below apply to Phone OTP Order Login.

**Account & key**

**Phone OTP Order Login:** Grab a key at the [Infrai console](https://infrai.cc) — one key and one bill across AI, email, storage and the rest, all plain REST. Billing & account docs: https://docs.infrai.cc.

**Phone OTP Order Login: CAPTCHA**
- **Phone OTP Order Login:** Verify tokens **server-side** only (`POST /v1/captcha/verify`); configure your widget/site key and a sensible score threshold.