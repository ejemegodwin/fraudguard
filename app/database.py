import sqlite3
from datetime import datetime
from pathlib import Path


DATABASE_PATH = Path("fraudguard.db")


def get_connection() -> sqlite3.Connection:
    connection = sqlite3.connect(DATABASE_PATH, timeout=10)
    connection.row_factory = sqlite3.Row
    connection.execute("PRAGMA busy_timeout = 10000")
    connection.execute("PRAGMA journal_mode = WAL")
    connection.execute("PRAGMA synchronous = NORMAL")
    return connection


def initialize_database() -> None:
    with get_connection() as connection:
        connection.execute(
            """
            CREATE TABLE IF NOT EXISTS transactions (
                transaction_id TEXT PRIMARY KEY,
                user_id TEXT NOT NULL,
                amount REAL NOT NULL,
                timestamp TEXT NOT NULL,
                location TEXT NOT NULL,
                device_id TEXT NOT NULL,
                risk_score INTEGER NOT NULL DEFAULT 0,
                risk_level TEXT NOT NULL DEFAULT 'LOW',
                decision TEXT NOT NULL DEFAULT 'ALLOW',
                reasons TEXT NOT NULL DEFAULT '[]'
            )
            """
        )
        connection.execute(
            "CREATE INDEX IF NOT EXISTS idx_transactions_user_time "
            "ON transactions(user_id, timestamp)"
        )
        connection.commit()


def check_database() -> bool:
    try:
        with get_connection() as connection:
            connection.execute("SELECT 1").fetchone()
        return True
    except sqlite3.Error:
        return False


def get_transaction(transaction_id: str) -> sqlite3.Row | None:
    with get_connection() as connection:
        return connection.execute(
            "SELECT * FROM transactions WHERE transaction_id = ?",
            (transaction_id,),
        ).fetchone()


def get_user_transactions(user_id: str) -> list[sqlite3.Row]:
    with get_connection() as connection:
        return connection.execute(
            """
            SELECT * FROM transactions
            WHERE user_id = ?
            ORDER BY timestamp DESC
            """,
            (user_id,),
        ).fetchall()


def get_recent_user_transactions(
    user_id: str, start_time: datetime, end_time: datetime
) -> list[sqlite3.Row]:
    with get_connection() as connection:
        return connection.execute(
            """
            SELECT * FROM transactions
            WHERE user_id = ?
              AND timestamp >= ?
              AND timestamp <= ?
            ORDER BY timestamp DESC
            """,
            (user_id, start_time.isoformat(), end_time.isoformat()),
        ).fetchall()


def list_transactions(
    user_id: str | None = None, limit: int = 50, offset: int = 0
) -> list[sqlite3.Row]:
    with get_connection() as connection:
        if user_id:
            return connection.execute(
                """
                SELECT * FROM transactions
                WHERE user_id = ?
                ORDER BY timestamp DESC
                LIMIT ? OFFSET ?
                """,
                (user_id, limit, offset),
            ).fetchall()

        return connection.execute(
            """
            SELECT * FROM transactions
            ORDER BY timestamp DESC
            LIMIT ? OFFSET ?
            """,
            (limit, offset),
        ).fetchall()


def get_stats() -> dict:
    with get_connection() as connection:
        totals = connection.execute(
            """
            SELECT
                COUNT(*) AS total_transactions,
                COALESCE(AVG(risk_score), 0) AS average_risk_score,
                COALESCE(SUM(CASE WHEN decision = 'ALLOW' THEN 1 ELSE 0 END), 0) AS allowed,
                COALESCE(SUM(CASE WHEN decision = 'REVIEW' THEN 1 ELSE 0 END), 0) AS review,
                COALESCE(SUM(CASE WHEN decision = 'REJECT' THEN 1 ELSE 0 END), 0) AS rejected
            FROM transactions
            """
        ).fetchone()

        high_risk = connection.execute(
            """
            SELECT transaction_id, user_id, amount, risk_score, risk_level, decision, reasons, timestamp
            FROM transactions
            WHERE risk_level = 'HIGH'
            ORDER BY timestamp DESC
            LIMIT 10
            """
        ).fetchall()

    return {
        "total_transactions": totals["total_transactions"],
        "allowed": totals["allowed"],
        "review": totals["review"],
        "rejected": totals["rejected"],
        "average_risk_score": round(totals["average_risk_score"], 2),
        "high_risk_transactions": high_risk,
    }
