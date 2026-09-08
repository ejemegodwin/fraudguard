package payments

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	StatusPending = "PENDING"
	StatusPaymentInitialized = "PAYMENT_INITIALIZED"
	StatusPaid = "PAID"
	StatusFailed = "FAILED"
	StatusReview = "REVIEW"
	StatusRejected = "REJECTED"
)

type Payment struct { TransactionID string `json:"transaction_id"`; Amount float64 `json:"amount"`; Currency string `json:"currency"`; Status string `json:"status"`; PaymentLink string `json:"payment_link,omitempty"`; FlutterwaveTransactionID string `json:"flutterwave_transaction_id,omitempty"`; UpdatedAt time.Time `json:"updated_at"` }
type Store struct { mu sync.RWMutex; path string; payments map[string]Payment }

func NewStore(path string)(*Store,error){if path==""{path="payments.json"};s:=&Store{path:path,payments:make(map[string]Payment)};data,err:=os.ReadFile(path);if errors.Is(err,os.ErrNotExist){return s,nil};if err!=nil{return nil,fmt.Errorf("read payment store: %w",err)};if len(data)==0{return s,nil};if err:=json.Unmarshal(data,&s.payments);err!=nil{return nil,fmt.Errorf("decode payment store: %w",err)};return s,nil}
func(s *Store)Create(p Payment)error{s.mu.Lock();defer s.mu.Unlock();if p.TransactionID==""{return errors.New("transaction ID is required")};if _,ok:=s.payments[p.TransactionID];ok{return fmt.Errorf("payment %s already exists",p.TransactionID)};p.UpdatedAt=time.Now().UTC();s.payments[p.TransactionID]=p;return s.persistLocked()}
func(s *Store)Get(id string)(Payment,bool){s.mu.RLock();defer s.mu.RUnlock();p,ok:=s.payments[id];return p,ok}
func canTransition(from,to string)bool{if from==to{return true};switch from{case StatusPending:return to==StatusPaymentInitialized||to==StatusFailed||to==StatusRejected;case StatusPaymentInitialized:return to==StatusPaid||to==StatusFailed;case StatusReview:return to==StatusRejected||to==StatusPending;case StatusPaid,StatusFailed,StatusRejected:return false};return false}
func(s *Store)UpdateStatus(id,status string)(Payment,error){s.mu.Lock();defer s.mu.Unlock();p,ok:=s.payments[id];if !ok{return Payment{},fmt.Errorf("payment %s not found",id)};if !canTransition(p.Status,status){return Payment{},fmt.Errorf("invalid payment transition %s -> %s",p.Status,status)};p.Status=status;p.UpdatedAt=time.Now().UTC();s.payments[id]=p;if err:=s.persistLocked();err!=nil{return Payment{},err};return p,nil}
func(s *Store)SetPaymentLink(id,link string)(Payment,error){s.mu.Lock();defer s.mu.Unlock();p,ok:=s.payments[id];if !ok{return Payment{},fmt.Errorf("payment %s not found",id)};p.PaymentLink=link;p.UpdatedAt=time.Now().UTC();s.payments[id]=p;if err:=s.persistLocked();err!=nil{return Payment{},err};return p,nil}
func(s *Store)MarkPaid(id,flwID string)(Payment,error){s.mu.Lock();defer s.mu.Unlock();p,ok:=s.payments[id];if !ok{return Payment{},fmt.Errorf("payment %s not found",id)};if p.Status==StatusPaid{return p,nil};if !canTransition(p.Status,StatusPaid){return Payment{},fmt.Errorf("invalid payment transition %s -> PAID",p.Status)};p.Status=StatusPaid;p.FlutterwaveTransactionID=flwID;p.UpdatedAt=time.Now().UTC();s.payments[id]=p;if err:=s.persistLocked();err!=nil{return Payment{},err};return p,nil}
func(s *Store)persistLocked()error{if dir:=filepath.Dir(s.path);dir!="."{if err:=os.MkdirAll(dir,0o755);err!=nil{return fmt.Errorf("create payment store directory: %w",err)}};data,err:=json.MarshalIndent(s.payments,"","  ");if err!=nil{return fmt.Errorf("encode payment store: %w",err)};tmp:=s.path+".tmp";if err:=os.WriteFile(tmp,data,0o600);err!=nil{return fmt.Errorf("write payment store: %w",err)};if err:=os.Rename(tmp,s.path);err!=nil{return fmt.Errorf("replace payment store: %w",err)};return nil}
