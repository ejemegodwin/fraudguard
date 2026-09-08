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
   +--> FraudGuard risk check (X-API-Key)
   |       |
   |       +--> ALLOW / REVIEW / REJECT
   |
   +--> Persistent Order
   |       |
   |       +--> PENDING / REVIEW / APPROVED / PAYMENT_INITIALIZED / PAID / FAILED / REJECTED
   |
   +--> Flutterwave
   |       |
   |       +--> hosted checkout
   |       +--> server-side verification
   |       +--> webhook
   |
   +--> Admin review
           |
           +--> REVIEW → APPROVED → payment
           +--> REVIEW → REJECTED

FraudGuard (Python/FastAPI)
   |
   +--> SQLite transaction history (WAL)
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
Added a Go HTTP client with contexts, timeouts, JSON decoding, non-2xx error handling, and optional `X-API-Key` authentication.

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
Added persistent JSON-backed payment state with controlled payment transitions and hosted payment-link persistence.

Payment lifecycle:

```text
PENDING
   ↓
PAYMENT_INITIALIZED
   ├──→ PAID
   └──→ FAILED
```

## Step 24 — Persistent order lifecycle ✅
Added persistent Go order state linked to each FraudGuard transaction. Orders retain customer checkout information and the risk decision that produced the order state.

## Step 25 — Order management queries and pagination ✅
Added `GET /orders` filtering by user/status plus `limit`/`offset` pagination, while retaining direct lookup by order ID or transaction ID.

## Step 26 — Controlled human fraud-review workflow and final MVP hardening ✅
Completed the planned MVP in one integrated implementation.

Implemented:

- Refactored the Go service around an injectable `Server` and dedicated `http.ServeMux`.
- Added controlled order state transitions.
- Added persisted customer email/name/phone data to reviewed orders so approved checkouts can resume safely.
- Persisted FraudGuard score, level, decision, and reasons on orders.
- Added authenticated admin review endpoints:
  - `POST /admin/orders/{order_id}/approve`
  - `POST /admin/orders/{order_id}/reject`
- Added constant-time admin API-key verification with `ADMIN_API_KEY`.
- Approval transitions `REVIEW → APPROVED → PAYMENT_INITIALIZED` and returns the hosted payment link.
- Rejection transitions `REVIEW → REJECTED` without creating payment.
- Prevented arbitrary public order-status mutation, especially fake `PAID` transitions.
- Hardened payment transitions and persisted hosted payment links.
- Added bounded JSON request bodies and stricter single-object JSON decoding.
- Kept callback/webhook payment verification server-side and idempotent.
- Updated integration documentation to describe the completed workflow.

## Step 27 — Production security and reliability hardening ✅
Added the first production-hardening layer:

- Added optional authenticated Go → FraudGuard service calls using `FRAUDGUARD_API_KEY` and `X-API-Key`.
- Added constant-time comparison for FraudGuard service authentication.
- Added FraudGuard `/ready` readiness endpoint with a real SQLite connectivity check.
- Added SQLite WAL mode, busy timeout, and normal synchronous mode for better concurrent reliability.
- Normalized naive transaction timestamps as UTC rather than leaving them ambiguous.
- Hardened the FraudGuard Docker image with a non-root runtime user.
- Added a Docker healthcheck against `/ready`.
- Updated integration/security documentation and environment-variable reference.

This hardening layer improves the MVP deployment boundary but does **not** make the system production-ready for real-money financial use by itself.

# API Quick Reference

| Service | Method | Endpoint | Purpose |
|---|---|---|---|
| FraudGuard | `GET` | `/` | Service information |
| FraudGuard | `GET` | `/health` | Liveness check |
| FraudGuard | `GET` | `/ready` | Database readiness check |
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
| Go | `POST` | `/admin/orders/{id}/approve` | Approve a reviewed order and initialize payment |
| Go | `POST` | `/admin/orders/{id}/reject` | Reject a reviewed order |
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

Optional service authentication:

```bash
export FRAUDGUARD_API_KEY="replace-with-a-long-random-secret"
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
FRAUDGUARD_API_KEY=<same service key used by FraudGuard>
PORT=8080
PAYMENT_STORE_PATH=payments.json
ORDER_STORE_PATH=orders.json
ADMIN_API_KEY=<long random admin key>
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

If FraudGuard returns `REVIEW`, Go creates an order in `REVIEW` and returns HTTP `202` without initializing payment. An authorized admin can then approve or reject it.

If FraudGuard returns `REJECT`, Go creates an order in `REJECTED` and returns HTTP `403` without initializing payment.

# MVP Status

The integrated MVP is **feature-complete for the planned development scope**:

- Python fraud microservice
- Explainable fraud rules
- SQLite transaction persistence
- Idempotency/conflict handling
- Transaction history
- pandas analytics
- Isolation Forest anomaly analysis
- Automated Python/Go tests and GitHub Actions CI
- Browser dashboard
- Go FraudGuard client
- Go risk-gating service
- Persistent Go payment state
- Persistent Go order state
- Customer data retained for reviewed checkouts
- Order listing, filtering, and pagination
- Controlled order/payment state transitions
- Human fraud-review approval/rejection workflow
- Flutterwave payment initialization
- Flutterwave server-side verification
- Idempotent webhook boundary
- Docker deployment foundation
- Service-to-service authentication boundary
- Readiness/health checks
- SQLite concurrency hardening
- Integration and architecture documentation

The system is **development/MVP ready**, not a production financial platform yet.

# Production Hardening Checklist

Before real-money production use:

- Replace JSON order/payment persistence with PostgreSQL or another transactional database.
- Store money as integer minor units such as kobo, not floating-point values.
- Persist webhook event IDs and make webhook processing transactional and idempotent.
- Enforce order/payment transitions atomically in the database.
- Add structured logging, metrics, tracing, rate limiting, retries, and circuit breakers.
- Replace the single admin API key with an audited identity/role system.
- Add deployment-level HTTPS and private service networking.
- Tune fraud thresholds using representative governed data.
- Add model/data drift monitoring.
- Fulfill inventory only after verified payment and valid terminal order state.
- Store secrets in a managed secret store.
- Add audit logging for risk decisions, admin actions, and payment state changes.
- Add database backups, migrations, disaster recovery, and multi-instance deployment strategy.
- Add end-to-end integration tests using fake FraudGuard and Flutterwave servers.
- Add security testing and an independent production review before handling real customer funds.

# Development Philosophy

FraudGuard starts with explainable rules and then adds statistical and machine-learning analysis. The Go service owns the fast e-commerce path, order lifecycle, review workflow, and payment boundary, while Python owns fraud intelligence and historical analysis.

The planned MVP scope is complete. Future work should focus on production infrastructure, database transactions, authentication/identity, observability, security review, and model/rule governance—not on extending the core proof-of-concept architecture.
