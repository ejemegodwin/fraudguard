from typing import Iterable

import pandas as pd


def transactions_to_dataframe(transactions: Iterable[dict]) -> pd.DataFrame:
    """Convert transaction records into a pandas DataFrame."""
    records = list(transactions)
    if not records:
        return pd.DataFrame(
            columns=["transaction_id", "user_id", "amount", "timestamp", "location", "device_id", "risk_score"]
        )

    frame = pd.DataFrame(records)
    frame["timestamp"] = pd.to_datetime(frame["timestamp"], utc=True)
    frame["amount"] = pd.to_numeric(frame["amount"], errors="coerce")
    frame["risk_score"] = pd.to_numeric(frame["risk_score"], errors="coerce")
    return frame


def summarize_transactions(transactions: Iterable[dict]) -> dict:
    """Return useful historical transaction statistics."""
    frame = transactions_to_dataframe(transactions)
    if frame.empty:
        return {
            "transaction_count": 0,
            "average_amount": 0,
            "median_amount": 0,
            "maximum_amount": 0,
            "average_risk_score": 0,
        }

    return {
        "transaction_count": int(len(frame)),
        "average_amount": round(float(frame["amount"].mean()), 2),
        "median_amount": round(float(frame["amount"].median()), 2),
        "maximum_amount": round(float(frame["amount"].max()), 2),
        "average_risk_score": round(float(frame["risk_score"].mean()), 2),
    }
