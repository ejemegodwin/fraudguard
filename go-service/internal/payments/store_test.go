package payments

import (
	"path/filepath"
	"testing"
)

func TestStorePersistsPaymentState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "payments.json")

	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Create(Payment{
		TransactionID: "txn_test_1",
		Amount:        15000,
		Currency:      "NGN",
		Status:        StatusPending,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := store.UpdateStatus("txn_test_1", StatusPaymentInitialized); err != nil {
		t.Fatal(err)
	}

	if _, err := store.MarkPaid("txn_test_1", "987654"); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}

	payment, ok := reloaded.Get("txn_test_1")
	if !ok {
		t.Fatal("expected persisted payment")
	}
	if payment.Status != StatusPaid {
		t.Fatalf("expected %s, got %s", StatusPaid, payment.Status)
	}
	if payment.FlutterwaveTransactionID != "987654" {
		t.Fatalf("expected Flutterwave transaction ID to persist")
	}
}

func TestStoreRejectsDuplicateTransaction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "payments.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}

	payment := Payment{TransactionID: "txn_duplicate", Amount: 100, Currency: "NGN", Status: StatusPending}
	if err := store.Create(payment); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(payment); err == nil {
		t.Fatal("expected duplicate payment to be rejected")
	}
}

func TestWebhookEventIsPersistentAndIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "payments.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}

	claimed, err := store.ClaimWebhookEvent("flw_123")
	if err != nil || !claimed {
		t.Fatalf("expected first claim, claimed=%v err=%v", claimed, err)
	}

	claimed, err = store.ClaimWebhookEvent("flw_123")
	if err != nil || claimed {
		t.Fatalf("expected duplicate claim to be rejected, claimed=%v err=%v", claimed, err)
	}

	if err := store.CompleteWebhookEvent("flw_123"); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err = reloaded.ClaimWebhookEvent("flw_123")
	if err != nil || claimed {
		t.Fatalf("expected persisted processed event to remain idempotent, claimed=%v err=%v", claimed, err)
	}
}

func TestWebhookEventCanBeReleasedAfterFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "payments.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimWebhookEvent("flw_retry")
	if err != nil || !claimed {
		t.Fatal("expected initial webhook claim")
	}
	if err := store.ReleaseWebhookEvent("flw_retry"); err != nil {
		t.Fatal(err)
	}
	claimed, err = store.ClaimWebhookEvent("flw_retry")
	if err != nil || !claimed {
		t.Fatalf("expected released event to be claimable again, claimed=%v err=%v", claimed, err)
	}
}
