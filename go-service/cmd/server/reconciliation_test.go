package main

import (
	"context"
	"testing"

	"go-service/internal/flutterwave"
	"go-service/internal/orders"
	"go-service/internal/payments"
)

type reconciliationGateway struct {
	verify flutterwave.VerifyResponse
	err    error
	calls  int
}

func (g *reconciliationGateway) CreatePayment(context.Context, flutterwave.PaymentRequest) (flutterwave.PaymentResponse, error) {
	return flutterwave.PaymentResponse{}, nil
}

func (g *reconciliationGateway) VerifyTransaction(context.Context, string) (flutterwave.VerifyResponse, error) {
	g.calls++
	return g.verify, g.err
}

func newReconciliationStores(t *testing.T) (*payments.Store, *orders.Store) {
	t.Helper()
	ps, err := payments.NewStore(t.TempDir() + "/payments.json")
	if err != nil { t.Fatal(err) }
	os, err := orders.NewStore(t.TempDir() + "/orders.json")
	if err != nil { t.Fatal(err) }
	return ps, os
}

func seedReconciliationState(t *testing.T, ps *payments.Store, os *orders.Store) orders.Order {
	t.Helper()
	order := orders.Order{
		OrderID: "ord_reconcile_1", TransactionID: "txn_reconcile_1", UserID: "user_1",
		Amount: 15000, Currency: "NGN", Status: orders.StatusPayment,
		PaymentStatus: payments.StatusPaymentInitialized,
	}
	if err := os.Create(order); err != nil { t.Fatal(err) }
	if err := ps.Create(payments.Payment{
		TransactionID: order.TransactionID, Amount: order.Amount, Currency: order.Currency,
		Status: payments.StatusPaymentInitialized, FlutterwaveTransactionID: "987654",
	}); err != nil { t.Fatal(err) }
	return order
}

func TestReconcilePaymentMarksVerifiedSuccessPaid(t *testing.T) {
	ps, os := newReconciliationStores(t)
	order := seedReconciliationState(t, ps, os)
	gateway := &reconciliationGateway{}
	gateway.verify.Data.ID = 987654
	gateway.verify.Data.TxRef = order.TransactionID
	gateway.verify.Data.Currency = order.Currency
	gateway.verify.Data.ChargedAmount = order.Amount
	gateway.verify.Data.Status = "successful"

	s := newServer(nil, gateway, ps, os, "secret", "admin", "http://localhost/callback")
	result, err := s.reconcilePayment(context.Background(), order)
	if err != nil { t.Fatal(err) }
	if result["state_changed"] != true { t.Fatalf("expected state_changed=true, got %#v", result["state_changed"]) }
	if result["reconciled"] != true { t.Fatalf("expected reconciled=true") }
	p, _ := ps.Get(order.TransactionID)
	if p.Status != payments.StatusPaid { t.Fatalf("payment status = %s", p.Status) }
	o, _ := os.Get(order.OrderID)
	if o.Status != orders.StatusPaid || o.PaymentStatus != orders.StatusPaid { t.Fatalf("order state = %s/%s", o.Status, o.PaymentStatus) }
	if gateway.calls != 1 { t.Fatalf("verify calls = %d", gateway.calls) }
}

func TestReconcilePaymentRejectsProviderMismatchWithoutChangingState(t *testing.T) {
	ps, os := newReconciliationStores(t)
	order := seedReconciliationState(t, ps, os)
	gateway := &reconciliationGateway{}
	gateway.verify.Data.TxRef = "different_tx"
	gateway.verify.Data.Currency = order.Currency
	gateway.verify.Data.ChargedAmount = order.Amount
	gateway.verify.Data.Status = "successful"

	s := newServer(nil, gateway, ps, os, "secret", "admin", "http://localhost/callback")
	result, err := s.reconcilePayment(context.Background(), order)
	if err != nil { t.Fatal(err) }
	if result["reconciled"] != false { t.Fatalf("expected reconciled=false") }
	if result["state_changed"] != false { t.Fatalf("expected state_changed=false") }
	p, _ := ps.Get(order.TransactionID)
	if p.Status != payments.StatusPaymentInitialized { t.Fatalf("payment changed to %s", p.Status) }
	o, _ := os.Get(order.OrderID)
	if o.Status != orders.StatusPayment { t.Fatalf("order changed to %s", o.Status) }
}

func TestReconcilePaymentLeavesPendingProviderStateUntouched(t *testing.T) {
	ps, os := newReconciliationStores(t)
	order := seedReconciliationState(t, ps, os)
	gateway := &reconciliationGateway{}
	gateway.verify.Data.TxRef = order.TransactionID
	gateway.verify.Data.Currency = order.Currency
	gateway.verify.Data.ChargedAmount = order.Amount
	gateway.verify.Data.Status = "pending"

	s := newServer(nil, gateway, ps, os, "secret", "admin", "http://localhost/callback")
	result, err := s.reconcilePayment(context.Background(), order)
	if err != nil { t.Fatal(err) }
	if result["reconciled"] != false { t.Fatalf("expected reconciled=false") }
	p, _ := ps.Get(order.TransactionID)
	if p.Status != payments.StatusPaymentInitialized { t.Fatalf("payment changed to %s", p.Status) }
}
