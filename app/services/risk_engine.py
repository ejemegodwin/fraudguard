from datetime import datetime, timedelta, timezone
from statistics import mean
from typing import Iterable


TIME_WINDOW_MINUTES = 10
VELOCITY_LIMIT = 5
MIN_HISTORY_FOR_BEHAVIOR = 3
AMOUNT_DEVIATION_MULTIPLIER = 3.0


NORMALIZE_REASONS = {
    "amount": "Unusually high transaction amount",
    "velocity": "High transaction velocity detected",
    "cold_start": "Limited transaction history for this user",
    "device": "New device detected",
    "location": "New location detected",
    "time": "Transaction occurred during an unusual time",
}


def _ensure_timezone(timestamp: datetime) -> datetime:
    if timestamp.tzinfo is None:
        return timestamp.replace(tzinfo=timezone.utc)
    return timestamp


def _parse_timestamp(value: str) -> datetime:
    return _ensure_timezone(datetime.fromisoformat(value))


def calculate_risk_score(
    timestamp: datetime,
    amount: float,
    historical_transactions: Iterable[dict],
) -> tuple[int, str, str, list[str]]:
    """Calculate an explainable MVP fraud score from transaction history."""
    timestamp = _ensure_timezone(timestamp)
    history = list(historical_transactions)

    score = 0
    reasons: list[str] = []

    # Rule 1: time of day.
    if 0 <= timestamp.hour < 5:
        score += 10
        reasons.append(NORMALIZE_REASONS["time"])

    # Rule 2: cold start. We do not make strong behavioral claims without
    # enough history, so this is only a small informational risk increase.
    if len(history) < MIN_HISTORY_FOR_BEHAVIOR:
        score += 5
        reasons.append(NORMALIZE_REASONS["cold_start"])
    else:
        amounts = [float(row["amount"]) for row in history]
        average_amount = mean(amounts)

        # Rule 3: amount deviation. A transaction at least 3x the user's
        # historical average is treated as unusually large for this MVP.
        if average_amount > 0 and amount >= average_amount * AMOUNT_DEVIATION_MULTIPLIER:
            score += 30
            reasons.append(NORMALIZE_REASONS["amount"])

        # Rule 4: behavioral consistency.
        known_devices = {row["device_id"] for row in history}
        known_locations = {row["location"] for row in history}

        current_device = history[0]["_current_device"] if history and "_current_device" in history[0].keys() else None
        current_location = history[0]["_current_location"] if history and "_current_location" in history[0].keys() else None

        if current_device and current_device not in known_devices:
            score += 15
            reasons.append(NORMALIZE_REASONS["device"])

        if current_location and current_location not in known_locations:
            score += 15
            reasons.append(NORMALIZE_REASONS["location"])

        # Rule 5: velocity. Because the current transaction is not stored yet,
        # only previous transactions are counted here.
        window_start = timestamp - timedelta(minutes=TIME_WINDOW_MINUTES)
        recent_count = sum(
            1
            for row in history
            if window_start <= _parse_timestamp(row["timestamp"]) <= timestamp
        )
        if recent_count >= VELOCITY_LIMIT:
            score += 25
            reasons.append(NORMALIZE_REASONS["velocity"])

    score = min(score, 100)

    if score >= 70:
        risk_level = "HIGH"
        decision = "REJECT"
    elif score >= 30:
        risk_level = "MEDIUM"
        decision = "REVIEW"
    else:
        risk_level = "LOW"
        decision = "ALLOW"

    return score, risk_level, decision, reasons
