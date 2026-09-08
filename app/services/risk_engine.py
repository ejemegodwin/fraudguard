from datetime import datetime, timezone


def calculate_risk_score(timestamp: datetime) -> tuple[int, str, str, list[str]]:
    """Calculate the initial risk score using the time-of-day rule.

    This first rule is intentionally simple. The remaining rules will be added
    as FraudGuard grows.
    """
    if timestamp.tzinfo is None:
        timestamp = timestamp.replace(tzinfo=timezone.utc)

    hour = timestamp.hour
    score = 0
    reasons: list[str] = []

    # MVP rule: transactions between midnight and 5:00 AM receive a small
    # risk increase because this is outside a typical daytime shopping period.
    if 0 <= hour < 5:
        score += 10
        reasons.append("Transaction occurred during an unusual time")

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
