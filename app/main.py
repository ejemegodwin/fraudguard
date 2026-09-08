import hashlib
import hmac
import json
import os
import sqlite3
from datetime import timezone
from pathlib import Path

from fastapi import FastAPI, HTTPException, Query, Request
from fastapi.responses import FileResponse

from app.database import (
    check_database,
    get_connection,
    get_stats,
    get_transaction,
    get_user_transactions,
    initialize_database,
    list_transactions,
)
from app.schemas import TransactionCreate, TransactionResponse
from app.services.analytics import summarize_transactions
from app.services.anomaly_model import detect_anomalies
from app.services.risk_engine import calculate_risk_score

app = FastAPI(title="FraudGuard", version="1.1.0")

initialize_database()

BASE_DIR = Path(__file__).resolve().parent.parent
DASHBOARD_PATH = BASE_DIR / "static" / "dashboard.html"
FRAUDGUARD_API_KEY = os.getenv("FRAUDGUARD_API_KEY", "")


def normalized_timestamp(transaction: TransactionCreate) -> str:
    timestamp = transaction.timestamp
    if timestamp.tzinfo is None:
        return timestamp.replace(tzinfo=timezone.utc).isoformat()
    return timestamp.astimezone(timezone.utc).isoformat()


def row_to_response(row, duplicate: bool = False) -> TransactionResponse:
    return TransactionResponse(
        transaction_id=row["transaction_id"],
        user_id=row["user_id"],
        amount=row["amount"],
        timestamp=row["timestamp"],
        location=row["location"],
        device_id=row["device_id"],
        risk_score=row["risk_score"],
        risk_level=row["risk_level"],
        decision=row["decision"],
        reasons=json.loads(row["reasons"]),
        duplicate=duplicate,
    )


def same_transaction_data(row, transaction: TransactionCreate) -> bool:
    return all(
        [
            row["user_id"] == transaction.user_id,
            row["amount"] == transaction.amount,
            row["timestamp"] == normalized_timestamp(transaction),
            row["location"] == transaction.location,
            row["device_id"] == transaction.device_id,
        ]
    )


def transaction_from_history(row) -> dict:
    return {
        "amount": row["amount"],
        "timestamp": row["timestamp"],
        "location": row["location"],
        "device_id": row["device_id"],
    }


def row_to_dict(row) -> dict:
    return dict(row)


def require_service_auth(request: Request) -> None:
    if not FRAUDGUARD_API_KEY:
        return
    supplied = request.headers.get("X-API-Key", "").encode()
    expected = FRAUDGUARD_API_KEY.encode()
    if not hmac.compare_digest(supplied, expected):
        raise HTTPException(status_code=401, detail="invalid API credentials")


@app.get("/")
def root():
    return {"service": "FraudGuard", "status": "running", "version": app.version}


@app.get("/health")
def health():
    return {"status": "healthy"}


@app.get("/ready")
def ready():
    if not check_database():
        raise HTTPException(status_code=503, detail="database is not ready")
    return {"status": "ready", "database": "ok"}


@app.get("/dashboard")
def dashboard():
    return FileResponse(DASHBOARD_PATH)


@app.post("/transactions", response_model=TransactionResponse)
def create_transaction(transaction: TransactionCreate, request: Request):
    require_service_auth(request)
    existing = get_transaction(transaction.transaction_id)

    if existing:
        if same_transaction_data(existing, transaction):
            return row_to_response(existing, duplicate=True)
        raise HTTPException(
            status_code=409,
            detail="transaction_id already exists with different data",
        )

    history_rows = get_user_transactions(transaction.user_id)
    history = [transaction_from_history(row) for row in history_rows]
    score, risk_level, decision, reasons = calculate_risk_score(
        timestamp=transaction.timestamp,
        amount=transaction.amount,
        device_id=transaction.device_id,
        location=transaction.location,
        historical_transactions=history,
    )
    stored_timestamp = normalized_timestamp(transaction)

    try:
        with get_connection() as connection:
            connection.execute(
                """
                INSERT INTO transactions (
                    transaction_id, user_id, amount, timestamp,
                    location, device_id, risk_score, risk_level,
                    decision, reasons
                ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                """,
                (
                    transaction.transaction_id,
                    transaction.user_id,
                    transaction.amount,
                    stored_timestamp,
                    transaction.location,
                    transaction.device_id,
                    score,
                    risk_level,
                    decision,
                    json.dumps(reasons),
                ),
            )
            connection.commit()
    except sqlite3.IntegrityError:
        existing = get_transaction(transaction.transaction_id)
        if existing and same_transaction_data(existing, transaction):
            return row_to_response(existing, duplicate=True)
        raise HTTPException(
            status_code=409,
            detail="transaction_id already exists with different data",
        )

    created = get_transaction(transaction.transaction_id)
    if created is None:
        raise HTTPException(status_code=500, detail="transaction could not be stored")
    return row_to_response(created)


@app.get("/transactions", response_model=list[TransactionResponse])
def get_transactions(
    user_id: str | None = None,
    limit: int = Query(default=50, ge=1, le=100),
    offset: int = Query(default=0, ge=0),
):
    rows = list_transactions(user_id=user_id, limit=limit, offset=offset)
    return [row_to_response(row) for row in rows]


@app.get("/transactions/{transaction_id}", response_model=TransactionResponse)
def get_transaction_by_id(transaction_id: str):
    row = get_transaction(transaction_id)
    if row is None:
        raise HTTPException(status_code=404, detail="transaction not found")
    return row_to_response(row)


@app.get("/stats")
def stats():
    data = get_stats()
    data["high_risk_transactions"] = [
        {
            "transaction_id": row["transaction_id"],
            "user_id": row["user_id"],
            "amount": row["amount"],
            "risk_score": row["risk_score"],
            "risk_level": row["risk_level"],
            "decision": row["decision"],
            "reasons": json.loads(row["reasons"]),
            "timestamp": row["timestamp"],
        }
        for row in data["high_risk_transactions"]
    ]
    return data


@app.get("/analytics")
def analytics(
    user_id: str | None = None,
    limit: int = Query(default=5000, ge=1, le=10000),
):
    rows = list_transactions(user_id=user_id, limit=limit, offset=0)
    summary = summarize_transactions([row_to_dict(row) for row in rows])
    summary["user_id"] = user_id
    return summary


@app.get("/analytics/anomalies")
def anomaly_analysis(
    user_id: str | None = None,
    limit: int = Query(default=5000, ge=1, le=10000),
):
    rows = list_transactions(user_id=user_id, limit=limit, offset=0)
    anomalies = detect_anomalies([row_to_dict(row) for row in rows])
    return {
        "user_id": user_id,
        "minimum_training_rows": 20,
        "anomalies": anomalies,
        "model_used": "IsolationForest" if len(rows) >= 20 else None,
    }
