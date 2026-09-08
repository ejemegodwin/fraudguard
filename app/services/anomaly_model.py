from typing import Iterable

import pandas as pd
from sklearn.ensemble import IsolationForest


MINIMUM_TRAINING_ROWS = 20


def detect_anomalies(transactions: Iterable[dict]) -> list[dict]:
    """Use Isolation Forest to flag unusual historical transactions.

    The model is deliberately separate from the rules engine. Rules remain the
    authoritative, explainable MVP decision path while ML is used for anomaly
    analysis once enough historical data exists.
    """
    records = list(transactions)
    if len(records) < MINIMUM_TRAINING_ROWS:
        return []

    frame = pd.DataFrame(records).copy()
    frame["amount"] = pd.to_numeric(frame["amount"], errors="coerce")
    frame["risk_score"] = pd.to_numeric(frame["risk_score"], errors="coerce")
    frame["timestamp"] = pd.to_datetime(frame["timestamp"], utc=True)
    frame = frame.dropna(subset=["amount", "risk_score", "timestamp"])

    if len(frame) < MINIMUM_TRAINING_ROWS:
        return []

    frame["hour"] = frame["timestamp"].dt.hour
    frame["day_of_week"] = frame["timestamp"].dt.dayofweek

    features = frame[["amount", "risk_score", "hour", "day_of_week"]]

    model = IsolationForest(
        n_estimators=100,
        contamination="auto",
        random_state=42,
    )
    frame["anomaly"] = model.fit_predict(features)
    frame["anomaly_score"] = model.decision_function(features)

    anomalies = frame[frame["anomaly"] == -1]
    return [
        {
            "transaction_id": row["transaction_id"],
            "user_id": row["user_id"],
            "amount": float(row["amount"]),
            "risk_score": int(row["risk_score"]),
            "anomaly_score": round(float(row["anomaly_score"]), 4),
            "timestamp": row["timestamp"].isoformat(),
        }
        for _, row in anomalies.iterrows()
    ]
