import json
import sqlite3
import os
import urllib.error
import urllib.request
import urllib.parse
from datetime import timezone
from pathlib import Path

from fastapi import FastAPI, HTTPException, Query, Request
from fastapi.responses import FileResponse, JSONResponse

from app.database import (check_database,get_connection,get_stats,get_transaction,get_user_transactions,initialize_database,list_transactions)
from app.schemas import TransactionCreate, TransactionResponse
from app.services.analytics import summarize_transactions
from app.services.anomaly_model import detect_anomalies
from app.services.risk_engine import calculate_risk_score

app=FastAPI(title="FraudGuard",version="1.3.1")
initialize_database()
BASE_DIR=Path(__file__).resolve().parent.parent
DASHBOARD_PATH=BASE_DIR/"static"/"dashboard.html";ANALYZE_PATH=BASE_DIR/"static"/"analyze.html";INVESTIGATE_PATH=BASE_DIR/"static"/"investigate.html";STORE_PATH=BASE_DIR/"static"/"store.html";REVIEW_PATH=BASE_DIR/"static"/"review.html"
FRAUDGUARD_API_KEY=os.getenv("FRAUDGUARD_API_KEY","");GO_SERVICE_URL=os.getenv("GO_SERVICE_URL","http://127.0.0.1:8080").rstrip("/")

def normalized_timestamp(transaction:TransactionCreate)->str:
    timestamp=transaction.timestamp
    if timestamp.tzinfo is None:return timestamp.replace(tzinfo=timezone.utc).isoformat()
    return timestamp.astimezone(timezone.utc).isoformat()

def row_to_response(row,duplicate=False)->TransactionResponse:
    return TransactionResponse(transaction_id=row["transaction_id"],user_id=row["user_id"],amount=row["amount"],timestamp=row["timestamp"],location=row["location"],device_id=row["device_id"],risk_score=row["risk_score"],risk_level=row["risk_level"],decision=row["decision"],reasons=json.loads(row["reasons"]),duplicate=duplicate)

def same_transaction_data(row,transaction:TransactionCreate)->bool:return all([row["user_id"]==transaction.user_id,row["amount"]==transaction.amount,row["timestamp"]==normalized_timestamp(transaction),row["location"]==transaction.location,row["device_id"]==transaction.device_id])
def transaction_from_history(row)->dict:return {"amount":row["amount"],"timestamp":row["timestamp"],"location":row["location"],"device_id":row["device_id"]}
def row_to_dict(row)->dict:return dict(row)
def require_service_auth(request:Request)->None:
    if not FRAUDGUARD_API_KEY:return
    import hmac
    if not hmac.compare_digest(request.headers.get("X-API-Key","").encode(),FRAUDGUARD_API_KEY.encode()):raise HTTPException(status_code=401,detail="invalid API credentials")

def proxy_go(path:str,payload:dict|None=None,method:str="POST",headers:dict|None=None)->JSONResponse:
    body=json.dumps(payload).encode("utf-8") if payload is not None else None;req_headers={"Content-Type":"application/json"}
    if headers:req_headers.update(headers)
    req=urllib.request.Request(f"{GO_SERVICE_URL}{path}",data=body,headers=req_headers,method=method)
    try:
        with urllib.request.urlopen(req,timeout=12) as response:
            raw=response.read().decode("utf-8");return JSONResponse(content=json.loads(raw) if raw else {},status_code=response.status)
    except urllib.error.HTTPError as exc:
        try:data=json.loads(exc.read().decode("utf-8"))
        except (json.JSONDecodeError,UnicodeDecodeError):data={"detail":"E-commerce service rejected the request"}
        return JSONResponse(content=data,status_code=exc.code)
    except (urllib.error.URLError,TimeoutError):return JSONResponse(content={"detail":"E-commerce service is unavailable"},status_code=503)

@app.get("/")
def root():return FileResponse(DASHBOARD_PATH)
@app.get("/health")
def health():return {"status":"healthy"}
@app.get("/ready")
def ready():
    if not check_database():raise HTTPException(status_code=503,detail="database is not ready")
    return {"status":"ready","database":"ok"}
@app.get("/dashboard")
def dashboard():return FileResponse(DASHBOARD_PATH)
@app.get("/analyze")
def analyze():return FileResponse(ANALYZE_PATH)
@app.get("/investigate/{transaction_id}")
def investigate(transaction_id:str):return FileResponse(INVESTIGATE_PATH)
@app.get("/store")
def store():return FileResponse(STORE_PATH)
@app.get("/review")
def review():return FileResponse(REVIEW_PATH)
@app.post("/ecommerce/checkout")
def ecommerce_checkout(payload:dict):
    required=("user_id","amount","email","location","device_id");missing=[field for field in required if field not in payload or payload[field] in (None,"")]
    if missing:raise HTTPException(status_code=400,detail=f"missing fields: {', '.join(missing)}")
    return proxy_go("/checkout",payload)
@app.get("/ecommerce/orders")
def ecommerce_orders(user_id:str|None=None,status:str|None=None,limit:int=Query(default=50,ge=1,le=100),offset:int=Query(default=0,ge=0)):
    query=[]
    if user_id:query.append("user_id="+urllib.parse.quote(user_id))
    if status:query.append("status="+urllib.parse.quote(status))
    query += [f"limit={limit}",f"offset={offset}"]
    return proxy_go("/orders?"+"&".join(query),method="GET")
@app.post("/ecommerce/admin/orders/{order_id}/{action}")
def ecommerce_admin_order(order_id:str,action:str,request:Request):
    key=request.headers.get("X-Admin-API-Key","")
    if not key:raise HTTPException(status_code=401,detail="admin API key required")
    if action not in ("approve","reject"):raise HTTPException(status_code=404,detail="unknown admin action")
    return proxy_go(f"/admin/orders/{urllib.parse.quote(order_id,safe='')}/{action}",method="POST",headers={"X-Admin-API-Key":key})

@app.post("/transactions",response_model=TransactionResponse)
def create_transaction(transaction:TransactionCreate,request:Request):
    require_service_auth(request);existing=get_transaction(transaction.transaction_id)
    if existing:
        if same_transaction_data(existing,transaction):return row_to_response(existing,duplicate=True)
        raise HTTPException(status_code=409,detail="transaction_id already exists with different data")
    history=[transaction_from_history(row) for row in get_user_transactions(transaction.user_id)]
    score,risk_level,decision,reasons=calculate_risk_score(timestamp=transaction.timestamp,amount=transaction.amount,device_id=transaction.device_id,location=transaction.location,historical_transactions=history);stored_timestamp=normalized_timestamp(transaction)
    try:
        with get_connection() as connection:
            connection.execute("INSERT INTO transactions (transaction_id,user_id,amount,timestamp,location,device_id,risk_score,risk_level,decision,reasons) VALUES (?,?,?,?,?,?,?,?,?,?)",(transaction.transaction_id,transaction.user_id,transaction.amount,stored_timestamp,transaction.location,transaction.device_id,score,risk_level,decision,json.dumps(reasons)));connection.commit()
    except sqlite3.IntegrityError:
        existing=get_transaction(transaction.transaction_id)
        if existing and same_transaction_data(existing,transaction):return row_to_response(existing,duplicate=True)
        raise HTTPException(status_code=409,detail="transaction_id already exists with different data")
    created=get_transaction(transaction.transaction_id)
    if created is None:raise HTTPException(status_code=500,detail="transaction could not be stored")
    return row_to_response(created)
@app.get("/transactions",response_model=list[TransactionResponse])
def get_transactions(user_id:str|None=None,limit:int=Query(default=50,ge=1,le=100),offset:int=Query(default=0,ge=0)):
    return [row_to_response(row) for row in list_transactions(user_id=user_id,limit=limit,offset=offset)]
@app.get("/transactions/{transaction_id}",response_model=TransactionResponse)
def get_transaction_by_id(transaction_id:str):
    row=get_transaction(transaction_id)
    if row is None:raise HTTPException(status_code=404,detail="transaction not found")
    return row_to_response(row)
@app.get("/stats")
def stats():
    data=get_stats();data["high_risk_transactions"]=[{"transaction_id":row["transaction_id"],"user_id":row["user_id"],"amount":row["amount"],"risk_score":row["risk_score"],"risk_level":row["risk_level"],"decision":row["decision"],"reasons":json.loads(row["reasons"]),"timestamp":row["timestamp"]} for row in data["high_risk_transactions"]];return data
@app.get("/analytics")
def analytics(user_id:str|None=None,limit:int=Query(default=5000,ge=1,le=10000)):
    rows=list_transactions(user_id=user_id,limit=limit,offset=0);summary=summarize_transactions([row_to_dict(row) for row in rows]);summary["user_id"]=user_id;return summary
@app.get("/analytics/anomalies")
def anomaly_analysis(user_id:str|None=None,limit:int=Query(default=5000,ge=1,le=10000)):
    rows=list_transactions(user_id=user_id,limit=limit,offset=0);anomalies=detect_anomalies([row_to_dict(row) for row in rows]);return {"user_id":user_id,"minimum_training_rows":20,"anomalies":anomalies,"model_used":"IsolationForest" if len(rows)>=20 else None}
