# FraudGuard Integration Guide

## 1. Service contract

FraudGuard exposes HTTP endpoints. The Go e-commerce service should generate the transaction ID and call FraudGuard before finalizing an order.

### Risk request

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

### Risk response

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

The exact score and reasons depend on the user's stored history.

## 2. Go decision flow

```text
Receive order
     |
     v
Create transaction_id
     |
     v
Call FraudGuard
     |
     +---- ALLOW  ----> continue payment/order
     |
     +---- REVIEW ----> hold order for manual/risk review
     |
     +---- REJECT ----> stop order
```

The Go service should treat `decision` as the authoritative action returned by FraudGuard for the MVP.

## 3. Go request example

The following is the shape of the client logic. Keep payment credentials and FraudGuard URLs in environment variables rather than source code.

```go
payload := map[string]any{
    "transaction_id": transactionID,
    "user_id":        userID,
    "amount":         amount,
    "timestamp":      time.Now().Format(time.RFC3339),
    "location":       location,
    "device_id":      deviceID,
}

body, _ := json.Marshal(payload)
resp, err := http.Post(
    os.Getenv("FRAUDGUARD_URL")+"/transactions",
    "application/json",
    bytes.NewBuffer(body),
)
```

Production Go code should check network errors, HTTP status codes, JSON decoding errors, and timeouts. A payment should not be considered successful merely because the FraudGuard HTTP request succeeded.

## 4. Duplicate behavior

FraudGuard is idempotent for an exact duplicate request:

- Same transaction ID + same immutable data → existing result with `duplicate: true`.
- Same transaction ID + different data → HTTP `409 Conflict`.

This is useful when Go retries a request after a timeout.

## 5. Flutterwave boundary

Flutterwave belongs in the Go e-commerce service, not inside FraudGuard.

Recommended flow:

```text
Customer
   |
   v
Go order service
   |
   +--> FraudGuard risk check
   |        |
   |        +--> ALLOW / REVIEW / REJECT
   |
   +--> Flutterwave payment initialization
   |
   +--> Verify payment result
   |
   v
Order completed
```

FraudGuard should receive the transaction information needed for risk analysis and should not store payment secrets, card details, or Flutterwave credentials.

## 6. Operational requirements

Before production deployment:

- Replace SQLite with PostgreSQL or another production database.
- Store money as integer minor units (for example, kobo) rather than floating-point values.
- Add authentication between Go and FraudGuard.
- Add request timeouts and retry policies in Go.
- Add structured logging and monitoring.
- Add rate limiting.
- Protect the service with HTTPS and a private network where possible.
- Keep secrets in environment variables or a secret manager.
- Review fraud thresholds using real, properly governed historical data.

## 7. API documentation

When the FastAPI service is running locally, interactive API documentation is available at:

- `/docs` — Swagger UI
- `/redoc` — ReDoc

These are generated automatically by FastAPI.
