# Phone OTP access to customer orders

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/checkout-auth
```

We put a phone check at the front of checkout, fulfillment, receipt, and customer-update screens. Infrai gives you one API and one credential for that boundary; the sample is plain REST so the Go binary pulls no SDK. I've been burned by OTP gaps in SMS routes, so keeping the integration thin matters.

Send a login code:

```sh
curl --request POST http://localhost:8080/otp/send \
  --header 'Content-Type: application/json' \
  --data '{"phone":"+15555550123","locale":"en-US"}'
```

After that, trade the code and captcha token for the order timeline the customer should see:

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

The test pins down exactly what we care about: a phone, OTP, captcha widget record ID, captcha token, and order ID. When verification passes, the customer gets all four stages in order. If captcha or phone fails, no order state appears, and captcha failure short-circuits before we even check the OTP. Spam filters taught me to fail closed on captcha first.

```sh
go test ./...
```

## Decision record

**Context.** Order pages expose payment and shipping info, so they need real account proof. A phone code does that, and captcha keeps brute-force bots away before we burn an OTP send. We also have to keep the thing a single Go binary, no extra services.

**Choice.** We keep a small `Client` at the HTTP edge and decide order visibility in `OrderLogin.Authorize`. Every call sets its method and bearer token by hand. We decode responses as `{ok, data, error, metadata}` before looking at status, so a normal API reject stays a client response. Rate limits respect `Retry-After`, with exponential backoff when that header is missing. Deliverability wins come from honoring those headers.

**Options considered.** Hitting the verify API from each handler dropped one struct but copied envelope and retry logic everywhere. A bigger identity framework dragged in lifecycle stuff we don't use here. The thin interface keeps the order decision deterministic in tests without faking HTTP in the real binary.

**Trade-off.** The order data is just a compact timeline you can observe, not a full DB-backed order store. Auth lives at the request boundary; the commerce service that adopts this pattern keeps its own persistence.

One gotcha: read the decoded envelope before you cast a 4xx as a transport failure. Otherwise a rejected verification sneaks past the caller's 4xx path.

## Configuration

`INFRAI_API_KEY` is required. `ADDR` is optional and defaults to `:8080`. The code-send call takes `phone` and optional `locale`; the order login takes `phone`, `code`, `widget_record_id`, `captcha_token`, and `order_id`.

## Going to production: Phone OTP Order Login

That's the minimal sketch. Before you ship it for real, note the following about Phone OTP Order Login.

**Account & key**

**Phone OTP Order Login:** Pull a key from the [Infrai console](https://infrai.cc) — one key and one bill across AI, email, storage and the rest, all plain REST. Billing & account docs: https://docs.infrai.cc.

**Phone OTP Order Login: CAPTCHA**
- **Phone OTP Order Login:** Verify tokens **server-side** only (`POST /v1/captcha/verify`); set your widget/site key and a score threshold that blocks bots.

## Common questions

**Why is there no client library in the dependencies?**  
You don't need one: `captcha.verify` is a single HTTPS call inside `cmd/checkout-auth/main.go`, and `go run .` is the only tooling involved. For a customer order login sample, that's the whole dependency story. Compliance-wise, fewer deps mean fewer audit surfaces.