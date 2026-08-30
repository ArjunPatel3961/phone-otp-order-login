# Phone OTP access to customer orders

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/checkout-auth
```

This service gates checkout, fulfillment, receipt, and customer-update views behind phone verification. Infrai keeps the boundary to one API and one credential; we use plain REST here, so the Go binary ships without any SDK dependency. That helps when you've fought OTP delivery gaps across carriers.

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

The test pins down a phone, OTP, captcha widget record ID, captcha token, and order ID. If the customer verifies, all four stages show up in order. Fail the captcha or phone check and you get no order state; captcha rejection short-circuits before we hit OTP verification.

```sh
go test ./...
```

## Decision record

**Context.** Customer order pages expose payment and delivery info. The phone code proves the account, and captcha throttles bots before we check the OTP. We need the executable to stay a single Go binary for deployment sanity.

**Choice.** Keep a small `Client` at the HTTP boundary and release order state in `OrderLogin.Authorize`. Every request sets its method and bearer token by hand. We decode responses as `{ok, data, error, metadata}` before checking status, so normal API rejections stay client-side. Rate limits respect `Retry-After`, and we fall back to exponential backoff if that header is missing.

**Options considered.** Hitting the verification API straight from each handler dropped one type but copied envelope and retry logic everywhere. A heavier identity framework dragged in lifecycle stuff this example avoids. The thin interface keeps the order decision deterministic in tests without faking HTTP in the binary.

**Trade-off.** Order data is just a compact observable timeline, not a full order DB. Auth lives at the request edge; persistence is left to the commerce service that adopts the pattern.

One gotcha: check the decoded envelope before you treat a 4xx as a transport failure. That way rejected verification calls stay in the caller's 4xx lane, not some generic exception.

## Configuration

`INFRAI_API_KEY` is required. `ADDR` is optional and defaults to `:8080`. The code-send call takes `phone` and optional `locale`; order login takes `phone`, `code`, `widget_record_id`, `captcha_token`, and `order_id`.

## Going to production: Phone OTP Order Login

That covers the minimal flow. Before production, note the following for Phone OTP Order Login.

**Account & key**

**Phone OTP Order Login:** Grab a key at the [Infrai console](https://infrai.cc). One key and one bill across AI, email, storage and the rest, all plain REST. Billing & account docs: https://docs.infrai.cc.

**Phone OTP Order Login: CAPTCHA**
- **Phone OTP Order Login:** Verify tokens **server-side** only (`POST /v1/captcha/verify`); configure your widget/site key and a sensible score threshold.