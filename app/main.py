from fastapi import FastAPI

from app.schemas import TransactionCreate, TransactionResponse

app = FastAPI(title="FraudGuard", version="0.1.0")


@app.get("/")
def root():
    return {"service": "FraudGuard", "status": "running"}


@app.get("/health")
def health():
    return {"status": "healthy"}


@app.post("/transactions", response_model=TransactionResponse)
def create_transaction(transaction: TransactionCreate):
    return TransactionResponse(
        **transaction.model_dump(),
        risk_score=0,
        risk_level="LOW",
        decision="ALLOW",
        reasons=[],
    )
