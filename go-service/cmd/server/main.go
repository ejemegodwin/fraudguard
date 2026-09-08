package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"fraudguard-go/internal/flutterwave"
	"fraudguard-go/internal/fraudguard"
	"fraudguard-go/internal/orders"
	"fraudguard-go/internal/payments"
)

type RiskCheckRequest struct {
	UserID string `json:"user_id"`
	Amount float64 `json:"amount"`
	Location string `json:"location"`
	DeviceID string `json:"device_id"`
}

type CheckoutRequest struct {
	RiskCheckRequest
	Email string `json:"email"`
	Name string `json:"name"`
	PhoneNumber string `json:"phone_number"`
	Currency string `json:"currency"`
}

func main() {
	fraudGuardURL := getenv("FRAUDGUARD_URL", "http://127.0.0.1:8000")
	port := getenv("PORT", "8080")
	flwSecretKey := os.Getenv("FLW_SECRET_KEY")
	fraudClient := fraudguard.NewClient(fraudGuardURL)
	flwClient := flutterwave.NewClient(flwSecretKey)

	paymentStore, err := payments.NewStore(getenv("PAYMENT_STORE_PATH", "payments.json"))
	if err != nil { log.Fatalf("initialize payment store: %v", err) }
	orderStore, err := orders.NewStore(getenv("ORDER_STORE_PATH", "orders.json"))
	if err != nil { log.Fatalf("initialize order store: %v", err) }

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
	})

	http.HandleFunc("/risk-check", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost { writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"}); return }
		input, ok := decodeRiskRequest(w, r); if !ok { return }
		transaction := newTransaction(input)
		risk, err := checkRisk(r.Context(), fraudClient, transaction)
		if err != nil { log.Printf("risk check failed: %v", err); writeJSON(w, http.StatusBadGateway, map[string]string{"error": "FraudGuard unavailable"}); return }
		writeJSON(w, http.StatusOK, map[string]any{"transaction_id": transaction.TransactionID, "risk": risk})
	})

	http.HandleFunc("/checkout", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost { writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"}); return }
		var input CheckoutRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"}); return }
		if input.Email == "" || input.UserID == "" || input.Amount <= 0 || input.Location == "" || input.DeviceID == "" { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id, amount, email, location and device_id are required"}); return }

		currency := input.Currency; if currency == "" { currency = "NGN" }
		transaction := newTransaction(input.RiskCheckRequest)
		risk, err := checkRisk(r.Context(), fraudClient, transaction)
		if err != nil { log.Printf("risk check failed: %v", err); writeJSON(w, http.StatusBadGateway, map[string]string{"error": "FraudGuard unavailable"}); return }
		orderID := fmt.Sprintf("ord_%d", time.Now().UnixNano())

		createOrder := func(status, paymentStatus string) bool {
			err := orderStore.Create(orders.Order{OrderID: orderID, TransactionID: transaction.TransactionID, UserID: input.UserID, Amount: input.Amount, Currency: currency, Status: status, PaymentStatus: paymentStatus})
			if err != nil { log.Printf("create order: %v", err); writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to create order"}); return false }
			return true
		}

		switch risk.Decision {
		case "REJECT":
			if !createOrder(orders.StatusRejected, payments.StatusRejected) { return }
			writeJSON(w, http.StatusForbidden, map[string]any{"order_id": orderID, "transaction_id": transaction.TransactionID, "decision": risk.Decision, "order_status": orders.StatusRejected, "risk": risk}); return
		case "REVIEW":
			if !createOrder(orders.StatusReview, payments.StatusReview) { return }
			writeJSON(w, http.StatusAccepted, map[string]any{"order_id": orderID, "transaction_id": transaction.TransactionID, "decision": risk.Decision, "order_status": orders.StatusReview, "message": "order requires risk review before payment", "risk": risk}); return
		}

		if flwSecretKey == "" { writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "FLW_SECRET_KEY is not configured"}); return }
		if !createOrder(orders.StatusPending, payments.StatusPending) { return }
		if err := paymentStore.Create(payments.Payment{TransactionID: transaction.TransactionID, Amount: transaction.Amount, Currency: currency, Status: payments.StatusPending}); err != nil {
			log.Printf("create pending payment: %v", err); _, _ = orderStore.UpdateStatus(orderID, orders.StatusFailed); writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to create payment state"}); return
		}

		redirectURL := getenv("FLW_REDIRECT_URL", "http://localhost:8080/payment/callback")
		payment, err := flwClient.CreatePayment(r.Context(), flutterwave.PaymentRequest{TxRef: transaction.TransactionID, Amount: transaction.Amount, Currency: currency, RedirectURL: redirectURL, Customer: flutterwave.Customer{Email: input.Email, Name: input.Name, PhoneNumber: input.PhoneNumber}, Customizations: struct { Title string `json:"title,omitempty"` }{Title: "FraudGuard Store"}})
		if err != nil {
			_, _ = paymentStore.UpdateStatus(transaction.TransactionID, payments.StatusFailed); _, _ = orderStore.UpdatePaymentStatus(orderID, orders.StatusFailed); _, _ = orderStore.UpdateStatus(orderID, orders.StatusFailed)
			log.Printf("Flutterwave payment initialization failed: %v", err); writeJSON(w, http.StatusBadGateway, map[string]string{"error": "payment initialization failed"}); return
		}
		if _, err := paymentStore.UpdateStatus(transaction.TransactionID, payments.StatusPaymentInitialized); err != nil { _, _ = orderStore.UpdateStatus(orderID, orders.StatusFailed); writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to persist payment state"}); return }
		if _, err := orderStore.UpdateStatus(orderID, orders.StatusPayment); err != nil { writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to persist order state"}); return }
		if _, err := orderStore.UpdatePaymentStatus(orderID, orders.StatusPayment); err != nil { log.Printf("mark order payment status: %v", err) }
		writeJSON(w, http.StatusOK, map[string]any{"order_id": orderID, "transaction_id": transaction.TransactionID, "decision": risk.Decision, "order_status": orders.StatusPayment, "payment_status": payments.StatusPaymentInitialized, "risk": risk, "payment_link": payment.Data.Link})
	})

	http.HandleFunc("/orders", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet { writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"}); return }
		orderID, transactionID := r.URL.Query().Get("id"), r.URL.Query().Get("transaction_id")
		var order orders.Order; var ok bool
		if orderID != "" { order, ok = orderStore.Get(orderID) } else if transactionID != "" { order, ok = orderStore.GetByTransaction(transactionID) } else { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id or transaction_id is required"}); return }
		if !ok { writeJSON(w, http.StatusNotFound, map[string]string{"error": "order not found"}); return }
		writeJSON(w, http.StatusOK, order)
	})

	http.HandleFunc("/payment/callback", func(w http.ResponseWriter, r *http.Request) {
		internalRef, flutterwaveID := r.URL.Query().Get("tx_ref"), r.URL.Query().Get("transaction_id")
		callbackStatus := r.URL.Query().Get("status")
		if internalRef == "" || flutterwaveID == "" { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tx_ref and transaction_id are required"}); return }
		if flwSecretKey == "" { writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "FLW_SECRET_KEY is not configured"}); return }
		expected, exists := paymentStore.Get(internalRef); if !exists { writeJSON(w, http.StatusNotFound, map[string]string{"error": "transaction not found"}); return }
		verification, err := flwClient.VerifyTransaction(r.Context(), flutterwaveID); if err != nil { writeJSON(w, http.StatusBadGateway, map[string]string{"error": "payment verification failed"}); return }
		verified := verification.Data.Status == "successful" && verification.Data.TxRef == internalRef && verification.Data.Currency == expected.Currency && verification.Data.ChargedAmount >= expected.Amount
		order, orderExists := orderStore.GetByTransaction(internalRef)
		if verified {
			if _, err := paymentStore.MarkPaid(internalRef, fmt.Sprintf("%d", verification.Data.ID)); err != nil { writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to persist paid state"}); return }
			if orderExists { _, _ = orderStore.UpdatePaymentStatus(order.OrderID, orders.StatusPaid); _, _ = orderStore.UpdateStatus(order.OrderID, orders.StatusPaid) }
		} else if verification.Data.Status == "failed" || verification.Data.Status == "cancelled" {
			_, _ = paymentStore.UpdateStatus(internalRef, payments.StatusFailed); if orderExists { _, _ = orderStore.UpdatePaymentStatus(order.OrderID, orders.StatusFailed); _, _ = orderStore.UpdateStatus(order.OrderID, orders.StatusFailed) }
		}
		updated, _ := paymentStore.Get(internalRef); orderStatus := ""; if orderExists { updatedOrder, _ := orderStore.Get(order.OrderID); orderStatus = updatedOrder.Status }
		writeJSON(w, http.StatusOK, map[string]any{"callback_status": callbackStatus, "verified": verified, "order_status": orderStatus, "payment_status": updated.Status, "transaction": verification.Data})
	})

	http.HandleFunc("/webhooks/flutterwave", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost { writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"}); return }
		secretHash := os.Getenv("FLW_SECRET_HASH"); if secretHash == "" { writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "FLW_SECRET_HASH is not configured"}); return }
		rawBody, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)); if err != nil { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unable to read webhook"}); return }
		signature := r.Header.Get("flutterwave-signature"); valid := flutterwave.VerifyWebhookSignature(rawBody, signature, secretHash); if !valid { signature = r.Header.Get("verif-hash"); valid = flutterwave.VerifyWebhookSecretHash(signature, secretHash) }; if !valid { writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid webhook signature"}); return }
		var event struct { ID string `json:"id"`; Type string `json:"type"`; Data struct { ID string `json:"id"`; TxRef string `json:"tx_ref"`; Amount float64 `json:"amount"`; Currency string `json:"currency"`; Status string `json:"status"` } `json:"data"` }
		if err := json.Unmarshal(rawBody, &event); err != nil { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid webhook JSON"}); return }
		payment, exists := paymentStore.Get(event.Data.TxRef); if !exists { log.Printf("Flutterwave webhook for unknown transaction: tx_ref=%s", event.Data.TxRef); writeJSON(w, http.StatusOK, map[string]string{"status": "received"}); return }
		if payment.Status == payments.StatusPaid { writeJSON(w, http.StatusOK, map[string]string{"status": "already_processed"}); return }
		order, orderExists := orderStore.GetByTransaction(payment.TransactionID)
		if event.Data.Status == "successful" && event.Data.ID != "" {
			verification, verifyErr := flwClient.VerifyTransaction(r.Context(), event.Data.ID); if verifyErr != nil { log.Printf("webhook verification failed: %v", verifyErr); writeJSON(w, http.StatusBadGateway, map[string]string{"error": "payment verification failed"}); return }
			validPayment := verification.Data.Status == "successful" && verification.Data.TxRef == payment.TransactionID && verification.Data.Currency == payment.Currency && verification.Data.ChargedAmount >= payment.Amount
			if validPayment { if _, err := paymentStore.MarkPaid(payment.TransactionID, fmt.Sprintf("%d", verification.Data.ID)); err != nil { writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to persist payment state"}); return }; if orderExists { _, _ = orderStore.UpdatePaymentStatus(order.OrderID, orders.StatusPaid); _, _ = orderStore.UpdateStatus(order.OrderID, orders.StatusPaid) } }
		} else if event.Data.Status == "failed" || event.Data.Status == "cancelled" { _, _ = paymentStore.UpdateStatus(payment.TransactionID, payments.StatusFailed); if orderExists { _, _ = orderStore.UpdatePaymentStatus(order.OrderID, orders.StatusFailed); _, _ = orderStore.UpdateStatus(order.OrderID, orders.StatusFailed) } }
		log.Printf("Flutterwave webhook processed: id=%s type=%s tx_ref=%s status=%s", event.ID, event.Type, event.Data.TxRef, event.Data.Status)
		writeJSON(w, http.StatusOK, map[string]string{"status": "received"})
	})

	log.Printf("Go e-commerce service listening on :%s; FraudGuard=%s", port, fraudGuardURL)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func decodeRiskRequest(w http.ResponseWriter, r *http.Request) (RiskCheckRequest, bool) {
	var input RiskCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"}); return RiskCheckRequest{}, false }
	if input.UserID == "" || input.Amount <= 0 || input.Location == "" || input.DeviceID == "" { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id, amount, location and device_id are required"}); return RiskCheckRequest{}, false }
	return input, true
}

func newTransaction(input RiskCheckRequest) fraudguard.Transaction { return fraudguard.Transaction{TransactionID: fmt.Sprintf("txn_%d", time.Now().UnixNano()), UserID: input.UserID, Amount: input.Amount, Timestamp: time.Now().UTC(), Location: input.Location, DeviceID: input.DeviceID} }
func checkRisk(ctx context.Context, client *fraudguard.Client, transaction fraudguard.Transaction) (fraudguard.RiskResponse, error) { checkCtx, cancel := context.WithTimeout(ctx, 6*time.Second); defer cancel(); return client.CheckTransaction(checkCtx, transaction) }
func getenv(key, fallback string) string { if value := os.Getenv(key); value != "" { return value }; return fallback }
func writeJSON(w http.ResponseWriter, status int, value any) { w.Header().Set("Content-Type", "application/json"); w.WriteHeader(status); _ = json.NewEncoder(w).Encode(value) }
