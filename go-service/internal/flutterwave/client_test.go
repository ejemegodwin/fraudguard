package flutterwave

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreatePayment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/payments" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Fatalf("missing authorization header")
		}

		var request PaymentRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.TxRef != "txn_123" || request.Amount != 1000 || request.Currency != "NGN" {
			t.Fatalf("unexpected payment request: %+v", request)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"message": "Hosted Link",
			"data": map[string]string{"link": "https://checkout.example/test"},
		})
	}))
	defer server.Close()

	client := NewClient("test-secret")
	client.BaseURL = server.URL
	result, err := client.CreatePayment(context.Background(), PaymentRequest{
		TxRef: "txn_123", Amount: 1000, Currency: "NGN", RedirectURL: "http://localhost/callback",
		Customer: Customer{Email: "customer@example.com"},
	})
	if err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}
	if result.Data.Link != "https://checkout.example/test" {
		t.Fatalf("unexpected payment link: %s", result.Data.Link)
	}
}

func TestVerifyWebhookSignature(t *testing.T) {
	body := []byte(`{"event":"charge.completed"}`)
	secret := "webhook-secret"
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	if !VerifyWebhookSignature(body, signature, secret) {
		t.Fatal("expected valid signature")
	}
	if VerifyWebhookSignature(body, "invalid", secret) {
		t.Fatal("expected invalid signature to fail")
	}
}

func TestVerifyWebhookSecretHash(t *testing.T) {
	if !VerifyWebhookSecretHash("secret-hash", "secret-hash") {
		t.Fatal("expected matching secret hash")
	}
	if VerifyWebhookSecretHash("wrong", "secret-hash") {
		t.Fatal("expected mismatched secret hash to fail")
	}
}
