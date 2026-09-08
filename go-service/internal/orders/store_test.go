package orders

import (
	"path/filepath"
	"testing"
	"time"
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

func TestListFiltersAndSortsNewestFirst(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "orders.json"))
	if err != nil { t.Fatal(err) }

	ordersToCreate := []Order{
		{OrderID: "ord_old", TransactionID: "txn_old", UserID: "user_1", Status: StatusPaid},
		{OrderID: "ord_middle", TransactionID: "txn_middle", UserID: "user_2", Status: StatusReview},
		{OrderID: "ord_new", TransactionID: "txn_new", UserID: "user_1", Status: StatusPaid},
	}
	for _, order := range ordersToCreate {
		if err := store.Create(order); err != nil { t.Fatal(err) }
		time.Sleep(time.Millisecond)
	}

	got := store.List("user_1", StatusPaid, 0, 0)
	if len(got) != 2 { t.Fatalf("expected 2 orders, got %d", len(got)) }
	if got[0].OrderID != "ord_new" || got[1].OrderID != "ord_old" { t.Fatalf("unexpected order order: %+v", got) }
}

func TestListPagination(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "orders.json"))
	if err != nil { t.Fatal(err) }
	for i := 1; i <= 4; i++ {
		if err := store.Create(Order{OrderID: "ord_" + string(rune('0'+i)), TransactionID: "txn_" + string(rune('0'+i)), UserID: "user_1", Status: StatusPending}); err != nil { t.Fatal(err) }
		time.Sleep(time.Millisecond)
	}

	got := store.List("", "", 2, 1)
	if len(got) != 2 { t.Fatalf("expected 2 orders, got %d", len(got)) }
	if got[0].OrderID != "ord_3" || got[1].OrderID != "ord_2" { t.Fatalf("unexpected page: %+v", got) }
}
