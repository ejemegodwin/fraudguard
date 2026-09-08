import json

from fastapi import FastAPI, HTTPException

from app.database import get_connection, initialize_database
from app.schemas import TransactionCreate, TransactionResponse
from app.services.risk_engine import calculate_risk_score

app = FastAPI(title="FraudGuard", version="0.1.0")

initialize_database()


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


@app.get("/")
def root():
    return {"service": "FraudGuard", "status": "running"}


@app.get("/health")
def health():
    return {"status": "healthy"}


@app.post("/transactions", response_model=TransactionResponse)
def create_transaction(transaction: TransactionCreate):
    with get_connection() as connection:
        existing = connection.execute(
            "SELECT * FROM transactions WHERE transaction_id = ?",
            (transaction.transaction_id,),
        ).fetchone()

        if existing:
            same_data = all(
                [
                    existing["user_id"] == transaction.user_id,
                    existing["amount"] == transaction.amount,
                    existing["timestamp"] == transaction.timestamp.isoformat(),
                    existing["location"] == transaction.location,
                    existing["device_id"] == transaction.device_id,
                ]
            )

            if same_data:
                return row_to_response(existing, duplicate=True)

            raise HTTPException(
                status_code=409,
                detail="transaction_id already exists with different data",
            )

        score, risk_level, decision, reasons = calculate_risk_score(
            transaction.timestamp
        )

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
                transaction.timestamp.isoformat(),
                transaction.location,
                transaction.device_id,
                score,
                risk_level,
                decision,
                json.dumps(reasons),
            ),
        )
        connection.commit()

        created = connection.execute(
            "SELECT * FROM transactions WHERE transaction_id = ?",
            (transaction.transaction_id,),
        ).fetchone()

    return row_to_response(created)
