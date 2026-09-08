# FraudGuard Integration Guide

## 1. Architecture

```text
Customer
   |
   v
Go E-commerce Service
   |
   +--> FraudGuard
   |      |
   |      +--> ALLOW / REVIEW / REJECT
   |
   +--> Flutterwave
   |
   +--> payment verification / webhook
   |
   v
Order completed
```

FraudGuard owns fraud intelligence. Go owns the e-commerce request path and payment boundary.

## 2. FraudGuard contract

### Request

```http
POST /transactions
Content-Type: application/json
```

```json
{
  "transaction_id": "txn_8f31c2",
  "user_id": "user_123",
  "amount": 450000,
  "timestamp": "2026-09-08T10:25:31+01:00",
  "location": "Lagos",
  "device_id": "device_456"
}
```

### Decision response

```json
{
  "transaction_id": "txn_8f31c2",
  "user_id": "user_123",
  "amount": 450000,
  "timestamp": "2026-09-08T10:25:31+01:00",
  "location": "Lagos",
  "device_id": "device_456",
  "risk_score": 87,
  "risk_level": "HIGH",
  "decision": "REJECT",
  "reasons": [
    "Unusually high transaction amount",
    "New device detected",
    "New location detected"
  ],
  "duplicate": false
}
```

The exact score and reasons depend on stored history.

## 3. Duplicate behavior

FraudGuard is idempotent for an exact duplicate:

- Same transaction ID + same immutable data → original result with `duplicate: true`.
- Same transaction ID + different data → HTTP `409 Conflict`.

The Go service can safely retry a timed-out request using the same transaction ID.

## 4. Go service endpoints

The repository now contains a runnable Go service in `go-service/`.

### `POST /risk-check`

Performs a FraudGuard-only risk check.

### `POST /checkout`

Performs the complete gate:

```text
Request
  |
  v
FraudGuard
  |
  +--> REJECT → HTTP 403 → no payment
  |
  +--> REVIEW → HTTP 202 → no payment
  |
  +--> ALLOW
       |
       v
Flutterwave payment initialization
       |
       v
Return hosted payment link
```

Example request:

```json
{
  "user_id": "user_123",
  "amount": 450000,
  "location": "Lagos",
  "device_id": "device_456",
  "email": "customer@example.com",
  "name": "Customer Name",
  "phone_number": "+2348012345678",
  "currency": "NGN"
}
```

## 5. Running the two services

Start FraudGuard first:

```bash
uvicorn app.main:app --reload
```

Then start Go:

```bash
cd go-service
go run ./cmd/server
```

Default addresses:

```text
FraudGuard: http://127.0.0.1:8000
Go service: http://127.0.0.1:8080
```

## 6. Go environment variables

```text
FRAUDGUARD_URL=http://127.0.0.1:8000
PORT=8080
FLW_SECRET_KEY=<server secret>
FLW_SECRET_HASH=<webhook secret hash>
FLW_REDIRECT_URL=http://localhost:8080/payment/callback
FLW_BASE_URL=https://api.flutterwave.com/v3
```

Never commit real credentials to GitHub.

## 7. Flutterwave payment initialization

The Go service calls Flutterwave's server-side payment endpoint and returns the hosted checkout link to the caller.

Flutterwave's Standard flow uses a server-side request to create the payment, then redirects the customer to the returned hosted payment page. citeturn0search6

## 8. Payment callback verification

The Go service exposes:

```http
GET /payment/callback
```

The callback requires `transaction_id` and `tx_ref`, checks that they match, and then calls Flutterwave's verification endpoint instead of trusting the redirect alone.

Before giving value, the application should verify:

- transaction status
- transaction reference
- expected amount
- expected currency

Flutterwave explicitly recommends server-side verification before giving value. citeturn0search0turn0search5

## 9. Flutterwave webhook

The Go service exposes:

```http
POST /webhooks/flutterwave
```

The endpoint validates `verif-hash` against `FLW_SECRET_HASH` for the v3 webhook configuration and acknowledges valid requests quickly.

The webhook handler currently logs/acknowledges the event; production fulfillment should enqueue it, make processing idempotent, and re-query Flutterwave before changing the order to paid.

Flutterwave recommends signature validation, quick responses, idempotent processing, and server-side verification of critical transaction data. citeturn1search0turn1search1

## 10. Security

- Keep Flutterwave secrets on the Go server.
- Never expose `FLW_SECRET_KEY` to a browser or Flutter client.
- Keep FraudGuard private behind authentication/private networking in production.
- Use HTTPS in production.
- Validate amount, currency, transaction reference, and payment status before fulfillment.
- Never trust a client-provided `risk_score`.

Flutterwave's current security guidance also recommends server-side secret management and not hardcoding API keys. citeturn1search4turn1search6

## 11. Production requirements

Before handling real money at scale:

- Replace SQLite with PostgreSQL.
- Store monetary values as integer minor units such as kobo.
- Persist Go orders and payment references.
- Persist webhook event IDs for idempotency.
- Add authentication between Go and FraudGuard.
- Add structured logs, metrics, tracing, and alerting.
- Add retries/circuit breakers for service-to-service calls.
- Tune fraud rules using governed historical data.
- Add a human review workflow for `REVIEW` decisions.
- Store secrets in a secrets manager.
