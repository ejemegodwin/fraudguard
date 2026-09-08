package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"fraudguard-go/internal/fraudguard"
)

type RiskCheckRequest struct {
	UserID    string  `json:"user_id"`
	Amount    float64 `json:"amount"`
	Location  string  `json:"location"`
	DeviceID  string  `json:"device_id"`
}

func main() {
	fraudGuardURL := getenv("FRAUDGUARD_URL", "http://127.0.0.1:8000")
	port := getenv("PORT", "8080")

	client := fraudguard.NewClient(fraudGuardURL)

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
	})

	http.HandleFunc("/risk-check", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		var input RiskCheckRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}

		if input.UserID == "" || input.Amount <= 0 || input.Location == "" || input.DeviceID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id, amount, location and device_id are required"})
			return
		}

		transaction := fraudguard.Transaction{
			TransactionID: fmt.Sprintf("txn_%d", time.Now().UnixNano()),
			UserID:        input.UserID,
			Amount:        input.Amount,
			Timestamp:     time.Now().UTC(),
			Location:      input.Location,
			DeviceID:      input.DeviceID,
		}

		ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
		defer cancel()

		risk, err := client.CheckTransaction(ctx, transaction)
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

	log.Printf("Go e-commerce service listening on :%s; FraudGuard=%s", port, fraudGuardURL)
	log.Fatal(http.ListenAndServe(":"+port, nil))
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
