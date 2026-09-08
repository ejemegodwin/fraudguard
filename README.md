# FraudGuard

FraudGuard is a fraud and anomaly detection system for an e-commerce application.

The project uses a polyglot architecture:

- **Go** owns the e-commerce request path, order lifecycle, risk-gating, and payment integration.
- **FraudGuard (Python)** stores transaction history, analyzes behavior, and calculates risk.
- **FastAPI** exposes FraudGuard through HTTP/REST.
- **SQLite** is the initial MVP database.
- **pandas** provides historical transaction analytics.
- **scikit-learn / Isolation Forest** provides optional anomaly analysis after enough history exists.
- **Flutterwave** handles payment checkout after FraudGuard allows the transaction.

## Architecture

```text
Customer
   |
   v
Go E-commerce Service
   |
   +--> Persistent Order
   |       |
   |       +--> PENDING / REVIEW / REJECTED
   |
   +--> FraudGuard risk check
   |       |
   |       +--> ALLOW / REVIEW / REJECT
   |
   +--> Flutterwave checkout
   |       |
   |       +--> PAYMENT_INITIALIZED
   |       +--> PAID / FAILED
   |
   +--> Flutterwave verification/webhook
   |
   v
Order completed

FraudGuard (Python/FastAPI)
   |
   +--> SQLite transaction history
   +--> Rules Engine
   +--> Explainable Risk Score
   +--> pandas analytics
   +--> Isolation Forest anomaly analysis
```

## Project Structure

```text
fraudguard/
├── app/
├── docs/
├── static/
├── tests/
├── go-service/
│   ├── cmd/server/main.go
│   ├── internal/fraudguard/client.go
│   ├── internal/flutterwave/client.go
│   ├── internal/payments/store.go
│   ├── internal/payments/store_test.go
│   ├── internal/orders/store.go
│   ├── internal/orders/store_test.go
│   └── go.mod
├── .github/workflows/test.yml
├── Dockerfile
├── requirements.txt
├── .gitignore
└── README.md
```

# Build Progress

## Step 1 — Project foundation ✅
Created the Python/FastAPI project foundation with Python, FastAPI, Pydantic, and SQLite.

## Step 2 — Transaction schema ✅
Every transaction requires `transaction_id`, `user_id`, `amount`, `timestamp`, `location`, and `device_id`. FraudGuard calculates the risk score itself.

## Step 3 — SQLite database ✅
Created the `transactions` table and a `(user_id, timestamp)` index for behavioral-history queries.

## Step 4 — Duplicate transaction protection ✅
Same ID plus identical immutable data is idempotent. Same ID plus different data returns HTTP `409 Conflict`. Database uniqueness is authoritative during races.

## Step 5 — Initial risk engine ✅
Added the unusual-time rule for transactions between midnight and 5:00 AM.

## Step 6 — Apply risk scoring ✅
`POST /transactions` validates, loads history, calculates risk, stores the result, and returns the decision.

## Step 7 — Amount deviation ✅
With at least 3 historical transactions, an amount above 3× or below 1/3 of the historical average adds `+30` points.

## Step 8 — Velocity detection ✅
Five or more previous transactions in the same user's last 10 minutes adds `+30` points.

## Step 9 — Cold-start detection ✅
Fewer than 3 historical transactions adds `+5` points because behavioral history is limited.

## Step 10 — Behavioral consistency ✅
New device adds `+15`; new location adds `+15`; unusual time adds `+10`.

## Step 11 — Explainable risk scoring engine ✅
The score is capped at 100. Development bands are `0–29 ALLOW`, `30–69 REVIEW`, and `70–100 REJECT`.

## Step 12 — Transaction query endpoints ✅
Implemented transaction listing, user filtering, individual lookup, statistics, and pagination.

## Step 13 — Automated tests and CI ✅
Added pytest coverage and GitHub Actions for Python and Go tests.

## Step 14 — Go FraudGuard client ✅
Added a Go HTTP client with contexts, timeouts, JSON decoding, and non-2xx error handling.

## Step 15 — Go risk-gating service ✅
Added `/risk-check`. Go creates the transaction ID and enforces `REJECT → 403`, `REVIEW → 202`, and `ALLOW → payment flow`.

## Step 16 — Flutterwave checkout integration ✅
Added `/checkout` and the Flutterwave Standard hosted-payment flow.

## Step 17 — Flutterwave verification and callback ✅
Added `/payment/callback`. The browser redirect is not trusted by itself; Flutterwave is queried server-side and the stored reference, amount, currency, and successful status are checked.

## Step 18 — Flutterwave webhook boundary ✅
Added `/webhooks/flutterwave`, signature validation, server-side verification, and duplicate-paid protection.

## Step 19 — Data analysis with pandas ✅
Added `/analytics` with historical transaction statistics.

## Step 20 — Machine-learning anomaly detection ✅
Added `/analytics/anomalies` using Isolation Forest after enough historical records exist. ML remains separate from the authoritative rules decision.

## Step 21 — FraudGuard dashboard ✅
Added `/dashboard` with transaction counts, decisions, average risk, recent transactions, and reasons.

## Step 22 — Container and deployment foundation ✅
Added a Dockerfile for the Python FraudGuard service.

## Step 23 — Persistent payment state and webhook idempotency ✅
Added `go-service/internal/payments/store.go` with persistent JSON-backed payment state.

Payment lifecycle:

```text
PENDING
   ↓
PAYMENT_INITIALIZED
   ├──→ PAID
   └──→ FAILED
```

The payment record stores the internal transaction reference, expected amount, currency, status, optional Flutterwave transaction ID, and update timestamp. Successful callbacks and webhooks re-verify Flutterwave before marking a payment `PAID`. A webhook for an already-paid transaction is acknowledged without processing it again.

## Step 24 — Persistent order lifecycle ✅
Added `go-service/internal/orders/store.go` so the Go service now owns a persistent order record as well as payment state.

Each checkout creates an order ID and links it to the FraudGuard transaction:

```text
Order Created
     ↓
   PENDING
     ↓
PAYMENT_INITIALIZED
     ↓
    PAID
```

Risk outcomes are also represented:

```text
REVIEW   → order held, no payment
REJECTED → order blocked, no payment
```

The order stores:

- `order_id`
- `transaction_id`
- `user_id`
- `amount`
- `currency`
- `status`
- `payment_status`
- creation/update timestamps

New endpoint:

```text
GET /orders?id=<order_id>
GET /orders?transaction_id=<transaction_id>
```

The callback and webhook now update both payment and order state after successful server-side verification. This establishes the important ownership boundary: **Go owns orders/payments; Python owns fraud intelligence and transaction history.**

The current JSON stores are an MVP persistence layer. Production needs a transactional database and atomic order/payment state transitions.

## Step 25 — Order management queries and pagination ✅
Expanded the Go order API so orders can now be queried as a collection, not only by a single ID.

Supported queries:

```text
GET /orders
GET /orders?user_id=<user_id>
GET /orders?status=<status>
GET /orders?user_id=<user_id>&status=<status>
GET /orders?limit=<1-100>&offset=<0+>
GET /orders?id=<order_id>
GET /orders?transaction_id=<transaction_id>
```

Collection responses return an `orders` array together with the applied `limit` and `offset`.

The order store sorts results newest-first and supports user/status filtering plus offset pagination. Tests now cover filtering, ordering, persistence, and pagination.

The API deliberately does **not** expose a generic public status-mutation endpoint yet. Order status changes currently happen through the controlled checkout/payment workflow. This prevents an arbitrary caller from marking an order `PAID` without verified payment. A dedicated authenticated review/admin workflow will be added after the order model stores the customer information needed to safely resume a reviewed checkout.

# API Quick Reference

| Service | Method | Endpoint | Purpose |
|---|---|---|---|
| FraudGuard | `GET` | `/` | Service information |
| FraudGuard | `GET` | `/health` | Health check |
| FraudGuard | `GET` | `/dashboard` | Monitoring dashboard |
| FraudGuard | `POST` | `/transactions` | Score and store a transaction |
| FraudGuard | `GET` | `/transactions` | List transactions |
| FraudGuard | `GET` | `/transactions/{transaction_id}` | Get one transaction |
| FraudGuard | `GET` | `/stats` | Operational statistics |
| FraudGuard | `GET` | `/analytics` | pandas statistics |
| FraudGuard | `GET` | `/analytics/anomalies` | Isolation Forest analysis |
| Go | `GET` | `/health` | Go service health |
| Go | `POST` | `/risk-check` | Risk-only decision |
| Go | `POST` | `/checkout` | Create order + risk gate + payment initialization |
| Go | `GET` | `/orders` | List orders with filters and pagination |
| Go | `GET` | `/orders?id=...` | Get order state |
| Go | `GET` | `/orders?transaction_id=...` | Find order by transaction |
| Go | `GET` | `/orders?user_id=...` | List a user's orders |
| Go | `GET` | `/orders?status=...` | List orders by status |
| Go | `GET` | `/payment/callback` | Server-side payment verification |
| Go | `POST` | `/webhooks/flutterwave` | Flutterwave webhook receiver |

FastAPI also provides generated API documentation at `/docs` and `/redoc`.

# Running FraudGuard

```bash
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
uvicorn app.main:app --reload
```

Then open `http://127.0.0.1:8000/docs` or `http://127.0.0.1:8000/dashboard`.

Run tests:

```bash
pytest
```

# Running the Go Service

From `go-service/`:

```bash
go run ./cmd/server
```

Environment variables:

```text
FRAUDGUARD_URL=http://127.0.0.1:8000
PORT=8080
PAYMENT_STORE_PATH=payments.json
ORDER_STORE_PATH=orders.json
FLW_SECRET_KEY=<your Flutterwave server secret>
FLW_SECRET_HASH=<your Flutterwave webhook secret hash>
FLW_REDIRECT_URL=http://localhost:8080/payment/callback
FLW_BASE_URL=https://api.flutterwave.com/v3
```

Never commit real credentials or secret hashes.

# Example Go Checkout Request

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

If FraudGuard returns `ALLOW`, Go creates an order, creates pending payment state, initializes Flutterwave, and returns the hosted `payment_link`.

If FraudGuard returns `REVIEW`, Go creates an order in `REVIEW` and returns HTTP `202` without initializing payment.

If FraudGuard returns `REJECT`, Go creates an order in `REJECTED` and returns HTTP `403` without initializing payment.

# MVP Status

The repository now contains:

- Python fraud microservice
- Explainable fraud rules
- SQLite transaction persistence
- Idempotency/conflict handling
- Transaction history
- pandas analytics
- Isolation Forest anomaly analysis
- Automated tests and GitHub Actions CI
- Browser dashboard
- Go FraudGuard client
- Go risk-gating service
- Persistent Go payment state
- Persistent Go order state
- Order listing, filtering, and pagination
- Flutterwave payment initialization
- Flutterwave server-side verification
- Idempotent webhook boundary
- Docker deployment foundation

The system is **development/MVP ready**, not yet a production financial system.

# Production Hardening Checklist

Before real-money production use:

- Replace JSON order/payment persistence with PostgreSQL or another transactional production database.
- Store money as integer minor units such as kobo, not floating-point values.
- Add authentication/authorization between Go and FraudGuard.
- Use HTTPS/private networking.
- Add retries, circuit-breaking, and appropriate timeouts.
- Make webhook event processing fully persistent and idempotent.
- Enforce valid order/payment state transitions transactionally.
- Verify Flutterwave amount, currency, and transaction reference against the stored order before fulfillment.
- Add structured logging and monitoring.
- Add rate limiting.
- Add model/data drift monitoring.
- Tune fraud thresholds using representative governed data.
- Establish a human-review workflow for `REVIEW` decisions.
- Store secrets in a secret manager.
- Add audit logging for risk decisions and payment state changes.

# Development Philosophy

FraudGuard starts with explainable rules and then adds statistical and machine-learning analysis. The Go service owns the fast e-commerce path, order lifecycle, and payment boundary, while Python owns fraud intelligence and historical analysis.
