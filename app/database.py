import sqlite3
from pathlib import Path


DATABASE_PATH = Path("fraudguard.db")


def get_connection() -> sqlite3.Connection:
    connection = sqlite3.connect(DATABASE_PATH)
    connection.row_factory = sqlite3.Row
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
        connection.commit()
