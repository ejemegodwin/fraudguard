package orders

import (
	"path/filepath"
	"testing"
)

func TestStorePersistsAndReloads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "orders.json")
	store, err := NewStore(path)
	if err != nil { t.Fatal(err) }

	want := Order{OrderID: "ord_1", TransactionID: "txn_1", UserID: "user_1", Amount: 1000, Currency: "NGN", Status: StatusPending, PaymentStatus: StatusPending}
	if err := store.Create(want); err != nil { t.Fatal(err) }

	reloaded, err := NewStore(path)
	if err != nil { t.Fatal(err) }
	got, ok := reloaded.Get("ord_1")
	if !ok { t.Fatal("expected order after reload") }
	if got.TransactionID != want.TransactionID || got.Amount != want.Amount || got.Status != want.Status { t.Fatalf("unexpected order: %+v", got) }
}

func TestOrderStatusUpdates(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "orders.json"))
	if err != nil { t.Fatal(err) }
	if err := store.Create(Order{OrderID: "ord_2", TransactionID: "txn_2", Status: StatusPending, PaymentStatus: StatusPending}); err != nil { t.Fatal(err) }
	if _, err := store.UpdateStatus("ord_2", StatusPaid); err != nil { t.Fatal(err) }
	if _, err := store.UpdatePaymentStatus("ord_2", StatusPaid); err != nil { t.Fatal(err) }
	got, _ := store.Get("ord_2")
	if got.Status != StatusPaid || got.PaymentStatus != StatusPaid { t.Fatalf("unexpected statuses: %+v", got) }
}
