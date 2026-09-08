from datetime import datetime

from pydantic import BaseModel, Field


class TransactionCreate(BaseModel):
    transaction_id: str = Field(min_length=1)
    user_id: str = Field(min_length=1)
    amount: float = Field(gt=0)
    timestamp: datetime
    location: str = Field(min_length=1)
    device_id: str = Field(min_length=1)


class TransactionResponse(TransactionCreate):
    risk_score: int
    risk_level: str
    decision: str
    reasons: list[str]
    duplicate: bool = False
