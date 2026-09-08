package fraudguard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCheckTransaction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/transactions" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		var input Transaction
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if input.TransactionID != "txn_test_1" || input.UserID != "user_1" || input.Amount != 5000 {
			t.Fatalf("unexpected transaction: %+v", input)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(RiskResponse{
			TransactionID: input.TransactionID,
			RiskScore:     10,
			RiskLevel:     "LOW",
			Decision:      "ALLOW",
			Reasons:       []string{"test"},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	result, err := client.CheckTransaction(context.Background(), Transaction{
		TransactionID: "txn_test_1",
		UserID:        "user_1",
		Amount:        5000,
		Timestamp:     time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC),
		Location:      "Port Harcourt",
		DeviceID:      "device_1",
	})
	if err != nil {
		t.Fatalf("CheckTransaction returned error: %v", err)
	}
	if result.Decision != "ALLOW" || result.RiskScore != 10 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestCheckTransactionRejectsNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.CheckTransaction(context.Background(), Transaction{TransactionID: "txn_test"})
	if err == nil {
		t.Fatal("expected error for non-2xx response")
	}
}
