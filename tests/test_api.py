from datetime import datetime, timezone

import pytest
from fastapi.testclient import TestClient

from app import database
from app.main import app


@pytest.fixture()
def client(tmp_path, monkeypatch):
    db_path = tmp_path / "test_fraudguard.db"
    monkeypatch.setattr(database, "DATABASE_PATH", db_path)
    database.initialize_database()
    return TestClient(app)


def transaction_payload(transaction_id="txn_test_1"):
    return {
        "transaction_id": transaction_id,
        "user_id": "user_123",
        "amount": 5000,
        "timestamp": datetime(2026, 9, 8, 10, 0, tzinfo=timezone.utc).isoformat(),
        "location": "Lagos",
        "device_id": "device_123",
    }


def test_create_transaction(client):
    response = client.post("/transactions", json=transaction_payload())

    assert response.status_code == 200
    data = response.json()
    assert data["transaction_id"] == "txn_test_1"
    assert data["decision"] == "ALLOW"
    assert data["duplicate"] is False


def test_exact_duplicate_returns_existing_result(client):
    payload = transaction_payload()
    first = client.post("/transactions", json=payload)
    second = client.post("/transactions", json=payload)

    assert first.status_code == 200
    assert second.status_code == 200
    assert second.json()["duplicate"] is True
    assert second.json()["risk_score"] == first.json()["risk_score"]


def test_conflicting_transaction_id_returns_409(client):
    client.post("/transactions", json=transaction_payload())
    payload = transaction_payload()
    payload["amount"] = 9000

    response = client.post("/transactions", json=payload)

    assert response.status_code == 409


def test_invalid_amount_returns_422(client):
    payload = transaction_payload()
    payload["amount"] = 0

    response = client.post("/transactions", json=payload)

    assert response.status_code == 422


def test_get_transaction(client):
    client.post("/transactions", json=transaction_payload())

    response = client.get("/transactions/txn_test_1")

    assert response.status_code == 200
    assert response.json()["transaction_id"] == "txn_test_1"


def test_missing_transaction_returns_404(client):
    response = client.get("/transactions/does-not-exist")

    assert response.status_code == 404


def test_list_and_stats_endpoints(client):
    client.post("/transactions", json=transaction_payload())
    response = client.get("/transactions")
    stats = client.get("/stats")

    assert response.status_code == 200
    assert len(response.json()) == 1
    assert stats.status_code == 200
    assert stats.json()["total_transactions"] == 1


def test_analytics_endpoint(client):
    client.post("/transactions", json=transaction_payload())

    response = client.get("/analytics")

    assert response.status_code == 200
    assert response.json()["transaction_count"] == 1
    assert response.json()["average_amount"] == 5000
