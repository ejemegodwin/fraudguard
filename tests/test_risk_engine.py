from datetime import datetime, timezone, timedelta

from app.services.risk_engine import calculate_risk_score


def history_row(amount, timestamp, location="Lagos", device_id="device-1"):
    return {
        "amount": amount,
        "timestamp": timestamp.isoformat(),
        "location": location,
        "device_id": device_id,
    }


def test_unusual_time_adds_risk():
    score, level, decision, reasons = calculate_risk_score(
        datetime(2026, 9, 8, 2, 0, tzinfo=timezone.utc),
        5000,
        "device-1",
        "Lagos",
        [],
    )

    assert score == 15
    assert level == "LOW"
    assert decision == "ALLOW"
    assert "Transaction occurred during an unusual time" in reasons


def test_amount_deviation_is_detected():
    base = datetime(2026, 9, 8, 10, 0, tzinfo=timezone.utc)
    history = [
        history_row(5000, base - timedelta(days=3)),
        history_row(8000, base - timedelta(days=2)),
        history_row(7000, base - timedelta(days=1)),
    ]

    score, level, decision, reasons = calculate_risk_score(
        base,
        50000,
        "device-1",
        "Lagos",
        history,
    )

    assert score == 30
    assert level == "MEDIUM"
    assert decision == "REVIEW"
    assert "Unusually high transaction amount" in reasons


def test_new_device_and_location_are_detected():
    base = datetime(2026, 9, 8, 10, 0, tzinfo=timezone.utc)
    history = [
        history_row(5000, base - timedelta(days=3)),
        history_row(8000, base - timedelta(days=2)),
        history_row(7000, base - timedelta(days=1)),
    ]

    score, level, decision, reasons = calculate_risk_score(
        base,
        7000,
        "new-device",
        "Abuja",
        history,
    )

    assert score == 30
    assert level == "MEDIUM"
    assert decision == "REVIEW"
    assert "New device detected" in reasons
    assert "New location detected" in reasons


def test_velocity_is_detected():
    base = datetime(2026, 9, 8, 10, 10, tzinfo=timezone.utc)
    history = [
        history_row(1000, base - timedelta(minutes=1)),
        history_row(1000, base - timedelta(minutes=2)),
        history_row(1000, base - timedelta(minutes=3)),
        history_row(1000, base - timedelta(minutes=4)),
        history_row(1000, base - timedelta(minutes=5)),
    ]

    score, level, decision, reasons = calculate_risk_score(
        base,
        1000,
        "device-1",
        "Lagos",
        history,
    )

    assert score == 30
    assert level == "MEDIUM"
    assert decision == "REVIEW"
    assert "High transaction velocity detected" in reasons


def test_score_is_capped_at_100():
    base = datetime(2026, 9, 8, 2, 0, tzinfo=timezone.utc)
    history = [
        history_row(1000, base - timedelta(minutes=1)),
        history_row(1000, base - timedelta(minutes=2)),
        history_row(1000, base - timedelta(minutes=3)),
        history_row(1000, base - timedelta(minutes=4)),
        history_row(1000, base - timedelta(minutes=5)),
    ]

    score, level, decision, _ = calculate_risk_score(
        base,
        100000,
        "new-device",
        "Abuja",
        history,
    )

    assert score <= 100
    assert level == "HIGH"
    assert decision == "REJECT"
