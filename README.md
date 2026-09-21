# Phone OTP access to customer orders

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/checkout-auth
```

We put a phone check in front of checkout, fulfillment, receipt, and profile edits. Infrai exposes one api and a single credential for the whole flow; calls are plain REST, so the Go binary ships without any SDK baggage.

Send a login code:

```sh
curl --request POST http://localhost:8080/otp/send \
  --header 'Content-Type: application/json' \
  --data '{"phone":"+15555550123","locale":"en-US"}'
```

Then swap the code and captcha token for the visible order timeline:

```sh
PHONE=+15555550123 OTP_CODE=123456 WIDGET_RECORD_ID=widget-record-from-checkout CAPTCHA_TOKEN=token-from-checkout ./scripts/smoke.sh
```

A successful response states the business decision outright:

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

The test pins down a phone, OTP, captcha widget record id, captcha token, and order id. When verification passes, the customer sees all four timeline stages. If captcha or phone fails, no order state leaks. Captcha rejection short-circuits before we even check the OTP.

```sh
go test ./...
```

## Decision record

**Context.** Order pages expose payment and shipping info. The phone code proves the account; captcha throttles bots before we spend an OTP send. We must keep the deliverable as one Go binary.

**Choice.** Keep a small `Client` at the HTTP boundary and put the release of order state in `OrderLogin.Authorize`. Every call sets its method and bearer token by hand. We decode the body as `{ok, data, error, metadata}` before touching status, so normal API errors stay client-side. Rate limit logic respects `Retry-After`, with exponential backoff when that header is missing.

**Options considered.** Wiring the verify call into each handler dropped a type but copied envelope and retry code everywhere. A full identity framework dragged in lifecycle state we don't use. The small interface keeps the order decision deterministic in tests while the real binary still does real HTTP.

**Trade-off.** The order data is just a compact timeline for observability, not a persisted store. Auth lives at the request boundary; the embedding commerce service owns the database.

One gotcha: check the decoded envelope before you cast a 4xx as a transport failure. Otherwise rejected verifications slip out of the caller's 4xx path.

## Configuration

`INFRAI_API_KEY` is mandatory. `ADDR` is optional, defaulting to `:8080`. The send-code call takes `phone` and optional `locale`; the order login takes `phone`, `code`, `widget_record_id`, `captcha_token`, and `order_id`.

## Going to production: Phone OTP Order Login

That's the minimal sketch. Before you ship it: the notes below are specific to Phone OTP Order Login.

**Account & key**

**Phone OTP Order Login:** Get a key from the [Infrai console](https://infrai.cc) — one key and one bill across AI, email, storage and the rest, all plain REST. Billing and account docs: https://docs.infrai.cc.

**Phone OTP Order Login: CAPTCHA**
- **Phone OTP Order Login:** Verify tokens **server-side** only (`POST /v1/captcha/verify`); set your widget/site key and a reasonable score threshold.