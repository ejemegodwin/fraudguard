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
   +--> Persistent Order + Payment State
   |
   +--> Flutterwave
   |      |
   |      +--> hosted checkout
   |      +--> server-side verification
   |      +--> webhook
   |
   +--> Admin review
          |
          +--> REVIEW → APPROVED → payment
          +--> REVIEW → REJECTED
```

FraudGuard owns fraud intelligence. Go owns the e-commerce request path, order lifecycle, admin review boundary, and payment boundary.

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

The client does not provide a risk score. FraudGuard calculates it from the transaction and stored history.

### Decision response

The response contains the calculated `risk_score`, `risk_level`, `decision`, human-readable `reasons`, and a `duplicate` flag.

## 3. Duplicate behavior

FraudGuard is idempotent for an exact duplicate:

- Same transaction ID + same immutable data → original result with `duplicate: true`.
- Same transaction ID + different data → HTTP `409 Conflict`.

The Go service can safely retry a timed-out FraudGuard request when the same transaction ID is reused.

## 4. Go checkout flow

### `POST /risk-check`

Performs a FraudGuard-only risk check.

### `POST /checkout`

The complete gate is:

```text
Request
  |
  v
FraudGuard
  |
  +--> REJECT → HTTP 403 → order REJECTED → no payment
  |
  +--> REVIEW → HTTP 202 → order REVIEW → no payment
  |
  +--> ALLOW → order PENDING → Flutterwave → PAYMENT_INITIALIZED
```

The order stores the customer contact information, risk decision, score, level, and reasons so a reviewed checkout can be resumed safely.

## 5. Human review workflow

Admin routes are protected by `X-Admin-API-Key` and the `ADMIN_API_KEY` server environment variable.

```http
POST /admin/orders/{order_id}/approve
X-Admin-API-Key: <admin key>
```

Approval moves `REVIEW → APPROVED`, creates/resets the pending payment state, initializes Flutterwave, and returns the hosted payment link. Approval never means the order is already paid.

```http
POST /admin/orders/{order_id}/reject
X-Admin-API-Key: <admin key>
```

Rejection moves `REVIEW → REJECTED` and does not create a payment.

There is intentionally no generic public `PATCH /orders/{id}/status`. Clients cannot mark an order paid without verified payment evidence.

## 6. Order lifecycle

```text
PENDING ───────────────→ PAYMENT_INITIALIZED ─→ PAID
   │                              │               │
   └→ FAILED                      └→ FAILED       terminal

REVIEW ─→ APPROVED ─→ PAYMENT_INITIALIZED
   │
   └────→ REJECTED
```

The Go order store enforces these transitions. Terminal `PAID`, `FAILED`, and `REJECTED` states cannot be arbitrarily changed through the public API.

## 7. Running the two services

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

## 8. Go environment variables

```text
FRAUDGUARD_URL=http://127.0.0.1:8000
PORT=8080
PAYMENT_STORE_PATH=payments.json
ORDER_STORE_PATH=orders.json
ADMIN_API_KEY=<long random admin key>
FLW_SECRET_KEY=<server secret>
FLW_SECRET_HASH=<webhook secret hash>
FLW_REDIRECT_URL=http://localhost:8080/payment/callback
FLW_BASE_URL=https://api.flutterwave.com/v3
```

Never commit real credentials to GitHub.

## 9. Flutterwave payment initialization

Go calls Flutterwave server-side and returns the hosted checkout link. The browser/client never receives the Flutterwave secret key.

## 10. Payment callback verification

The Go service exposes:

```http
GET /payment/callback?tx_ref=<internal_ref>&transaction_id=<flutterwave_id>
```

The callback calls Flutterwave's verification endpoint and checks:

- successful transaction status
- stored transaction reference
- expected currency
- charged amount is at least the stored amount

Only after these checks does Go mark payment and order `PAID`.

## 11. Flutterwave webhook

The Go service exposes:

```http
POST /webhooks/flutterwave
```

The endpoint validates the configured webhook secret/signature, ignores unknown transactions safely, treats an already-paid transaction as idempotently processed, and re-verifies successful payment events server-side before changing state.

## 12. Order queries

```http
GET /orders
GET /orders?id=<order_id>
GET /orders?transaction_id=<transaction_id>
GET /orders?user_id=<user_id>
GET /orders?status=<status>
GET /orders?user_id=<user_id>&status=<status>&limit=20&offset=0
```

Collection results are newest-first and support `limit` from 1–100 and non-negative `offset`.

## 13. Security boundary

- Keep Flutterwave secrets on the Go server.
- Never expose `FLW_SECRET_KEY` to a browser or Flutter client.
- Protect admin review routes with a strong secret and private network access in production.
- Use HTTPS in production.
- Do not trust callback query parameters without server-side verification.
- Never trust a client-provided `risk_score`.
- Use constant-time comparison for webhook/admin secrets.
- Keep request bodies bounded.

## 14. Production requirements

The repository is an MVP implementation. Before real-money production use:

- Replace JSON order/payment persistence with PostgreSQL or another transactional database.
- Store monetary values as integer minor units such as kobo.
- Add authenticated service-to-service communication between Go and FraudGuard.
- Persist webhook event IDs and use transactional idempotency.
- Enforce order/payment transitions atomically in the database.
- Add structured logs, metrics, tracing, rate limiting, retries, and circuit breakers.
- Add an audited admin identity/role system instead of a single API key.
- Tune fraud rules and ML thresholds with governed representative data.
- Add model/data drift monitoring.
- Fulfill inventory only after verified payment and a valid terminal order state.
- Store secrets in a managed secret store.
