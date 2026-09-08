package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"fraudguard-go/internal/flutterwave"
	"fraudguard-go/internal/fraudguard"
)

type RiskCheckRequest struct {
	UserID   string  `json:"user_id"`
	Amount   float64 `json:"amount"`
	Location string  `json:"location"`
	DeviceID string  `json:"device_id"`
}

type CheckoutRequest struct {
	RiskCheckRequest
	Email       string `json:"email"`
	Name        string `json:"name"`
	PhoneNumber string `json:"phone_number"`
	Currency    string `json:"currency"`
}

type pendingPayment struct {
	Amount   float64
	Currency string
}

var pendingPayments = struct {
	sync.RWMutex
	items map[string]pendingPayment
}{items: make(map[string]pendingPayment)}

func main() {
	fraudGuardURL := getenv("FRAUDGUARD_URL", "http://127.0.0.1:8000")
	port := getenv("PORT", "8080")
	flwSecretKey := os.Getenv("FLW_SECRET_KEY")

	fraudClient := fraudguard.NewClient(fraudGuardURL)
	flwClient := flutterwave.NewClient(flwSecretKey)

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
	})

	http.HandleFunc("/risk-check", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		input, ok := decodeRiskRequest(w, r)
		if !ok {
			return
		}

		transaction := newTransaction(input)
		risk, err := checkRisk(r.Context(), fraudClient, transaction)
		if err != nil {
			log.Printf("risk check failed: %v", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "FraudGuard unavailable"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"transaction_id": transaction.TransactionID,
			"risk":           risk,
		})
	})

	http.HandleFunc("/checkout", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		var input CheckoutRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}

		if input.Email == "" || input.UserID == "" || input.Amount <= 0 || input.Location == "" || input.DeviceID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id, amount, email, location and device_id are required"})
			return
		}

		transaction := newTransaction(input.RiskCheckRequest)
		risk, err := checkRisk(r.Context(), fraudClient, transaction)
		if err != nil {
			log.Printf("risk check failed: %v", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "FraudGuard unavailable"})
			return
		}

		switch risk.Decision {
		case "REJECT":
			writeJSON(w, http.StatusForbidden, map[string]any{
				"transaction_id": transaction.TransactionID,
				"decision":       risk.Decision,
				"risk":           risk,
			})
			return
		case "REVIEW":
			writeJSON(w, http.StatusAccepted, map[string]any{
				"transaction_id": transaction.TransactionID,
				"decision":       risk.Decision,
				"message":        "order requires risk review before payment",
				"risk":           risk,
			})
			return
		}

		if flwSecretKey == "" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "FLW_SECRET_KEY is not configured"})
			return
		}

		currency := input.Currency
		if currency == "" {
			currency = "NGN"
		}

		redirectURL := getenv("FLW_REDIRECT_URL", "http://localhost:8080/payment/callback")
		payment, err := flwClient.CreatePayment(r.Context(), flutterwave.PaymentRequest{
			TxRef:       transaction.TransactionID,
			Amount:      transaction.Amount,
			Currency:    currency,
			RedirectURL: redirectURL,
			Customer: flutterwave.Customer{
				Email:       input.Email,
				Name:        input.Name,
				PhoneNumber: input.PhoneNumber,
			},
			Customizations: struct {
				Title string `json:"title,omitempty"`
			}{Title: "FraudGuard Store"},
		})
		if err != nil {
			log.Printf("Flutterwave payment initialization failed: %v", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "payment initialization failed"})
			return
		}

		pendingPayments.Lock()
		pendingPayments.items[transaction.TransactionID] = pendingPayment{Amount: transaction.Amount, Currency: currency}
		pendingPayments.Unlock()

		writeJSON(w, http.StatusOK, map[string]any{
			"transaction_id": transaction.TransactionID,
			"decision":       risk.Decision,
			"risk":           risk,
			"payment_link":   payment.Data.Link,
		})
	})

	http.HandleFunc("/payment/callback", func(w http.ResponseWriter, r *http.Request) {
		internalRef := r.URL.Query().Get("tx_ref")
		flutterwaveID := r.URL.Query().Get("transaction_id")
		callbackStatus := r.URL.Query().Get("status")

		if internalRef == "" || flutterwaveID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tx_ref and transaction_id are required"})
			return
		}
		if flwSecretKey == "" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "FLW_SECRET_KEY is not configured"})
			return
		}

		pendingPayments.RLock()
		expected, exists := pendingPayments.items[internalRef]
		pendingPayments.RUnlock()
		if !exists {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "pending transaction not found"})
			return
		}

		verification, err := flwClient.VerifyTransaction(r.Context(), flutterwaveID)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "payment verification failed"})
			return
		}

		verified := verification.Data.Status == "successful" &&
			verification.Data.TxRef == internalRef &&
			verification.Data.Currency == expected.Currency &&
			verification.Data.ChargedAmount >= expected.Amount

		if verified {
			pendingPayments.Lock()
			delete(pendingPayments.items, internalRef)
			pendingPayments.Unlock()
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"callback_status": callbackStatus,
			"verified":        verified,
			"transaction":     verification.Data,
		})
	})

	http.HandleFunc("/webhooks/flutterwave", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		secretHash := os.Getenv("FLW_SECRET_HASH")
		if secretHash == "" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "FLW_SECRET_HASH is not configured"})
			return
		}

		rawBody, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unable to read webhook"})
			return
		}

		signature := r.Header.Get("flutterwave-signature")
		valid := flutterwave.VerifyWebhookSignature(rawBody, signature, secretHash)
		if !valid {
			signature = r.Header.Get("verif-hash")
			valid = flutterwave.VerifyWebhookSecretHash(signature, secretHash)
		}
		if !valid {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid webhook signature"})
			return
		}

		var event struct {
			ID   string `json:"id"`
			Type string `json:"type"`
			Data struct {
				ID       string  `json:"id"`
				TxRef    string  `json:"tx_ref"`
				Amount   float64 `json:"amount"`
				Currency string  `json:"currency"`
				Status   string  `json:"status"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rawBody, &event); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid webhook JSON"})
			return
		}

		// Acknowledge quickly. Production fulfillment should enqueue this event,
		// deduplicate it, then re-query Flutterwave and verify amount/currency/ref.
		log.Printf("Flutterwave webhook received: id=%s type=%s tx_ref=%s status=%s", event.ID, event.Type, event.Data.TxRef, event.Data.Status)
		writeJSON(w, http.StatusOK, map[string]string{"status": "received"})
	})

	log.Printf("Go e-commerce service listening on :%s; FraudGuard=%s", port, fraudGuardURL)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func decodeRiskRequest(w http.ResponseWriter, r *http.Request) (RiskCheckRequest, bool) {
	var input RiskCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return RiskCheckRequest{}, false
	}
	if input.UserID == "" || input.Amount <= 0 || input.Location == "" || input.DeviceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id, amount, location and device_id are required"})
		return RiskCheckRequest{}, false
	}
	return input, true
}

func newTransaction(input RiskCheckRequest) fraudguard.Transaction {
	return fraudguard.Transaction{
		TransactionID: fmt.Sprintf("txn_%d", time.Now().UnixNano()),
		UserID:        input.UserID,
		Amount:        input.Amount,
		Timestamp:     time.Now().UTC(),
		Location:      input.Location,
		DeviceID:      input.DeviceID,
	}
}

func checkRisk(ctx context.Context, client *fraudguard.Client, transaction fraudguard.Transaction) (fraudguard.RiskResponse, error) {
	checkCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	return client.CheckTransaction(checkCtx, transaction)
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
