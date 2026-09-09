package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"go-service/internal/orders"
	"go-service/internal/payments"
)

// reconcilePayment verifies the current provider state against the local payment
// record and applies only a safe, verified terminal transition.
func (s *Server) reconcilePayment(ctx context.Context, order orders.Order) (map[string]any, error) {
	p, ok := s.payments.Get(order.TransactionID)
	if !ok {
		return nil, fmt.Errorf("payment state not found")
	}
	if p.FlutterwaveTransactionID == "" {
		return nil, fmt.Errorf("payment has no Flutterwave transaction ID")
	}

	v, err := s.flw.VerifyTransaction(ctx, p.FlutterwaveTransactionID)
	if err != nil {
		return nil, fmt.Errorf("payment verification failed")
	}

	result := map[string]any{
		"order_id":                 order.OrderID,
		"transaction_id":           order.TransactionID,
		"provider_transaction_id":  p.FlutterwaveTransactionID,
		"provider_status":          v.Data.Status,
		"local_payment_status":     p.Status,
		"local_order_status":       order.Status,
		"reconciled":               false,
		"state_changed":            false,
	}

	identityOK := v.Data.TxRef == p.TransactionID &&
		strings.EqualFold(v.Data.Currency, p.Currency) &&
		v.Data.ChargedAmount >= p.Amount
	if !identityOK {
		result["reason"] = "provider transaction does not match local payment"
		return result, nil
	}

	switch strings.ToLower(v.Data.Status) {
	case "successful":
		if p.Status != payments.StatusPaid {
			if _, err := s.payments.MarkPaid(p.TransactionID, p.FlutterwaveTransactionID); err != nil {
				return nil, fmt.Errorf("unable to mark payment paid: %w", err)
			}
			if _, err := s.orders.UpdatePaymentStatus(order.OrderID, orders.StatusPaid); err != nil {
				return nil, fmt.Errorf("unable to update order payment status: %w", err)
			}
			if _, err := s.orders.UpdateStatus(order.OrderID, orders.StatusPaid); err != nil {
				return nil, fmt.Errorf("unable to update order status: %w", err)
			}
			result["state_changed"] = true
		}
	case "failed", "cancelled":
		if p.Status != payments.StatusFailed {
			if _, err := s.payments.UpdateStatus(p.TransactionID, payments.StatusFailed); err != nil {
				return nil, fmt.Errorf("unable to mark payment failed: %w", err)
			}
			if _, err := s.orders.UpdatePaymentStatus(order.OrderID, orders.StatusFailed); err != nil {
				return nil, fmt.Errorf("unable to update order payment status: %w", err)
			}
			if _, err := s.orders.UpdateStatus(order.OrderID, orders.StatusFailed); err != nil {
				return nil, fmt.Errorf("unable to update order status: %w", err)
			}
			result["state_changed"] = true
		}
	default:
		result["reason"] = "provider payment is not in a terminal state"
		return result, nil
	}

	result["reconciled"] = true
	updatedPayment, _ := s.payments.Get(order.TransactionID)
	updatedOrder, _ := s.orders.Get(order.OrderID)
	result["local_payment_status"] = updatedPayment.Status
	result["local_order_status"] = updatedOrder.Status
	return result, nil
}

// reconcileOrderHandler exposes an authenticated manual reconciliation operation.
// It is intentionally explicit rather than automatic so operators can inspect
// and trigger recovery without introducing an unbounded background worker.
func (s *Server) reconcileOrderHandler(w http.ResponseWriter, r *http.Request, orderID string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !s.authorizeAdmin(w, r) {
		return
	}
	order, ok := s.orders.Get(orderID)
	if !ok {
		writeError(w, http.StatusNotFound, "order not found")
		return
	}
	result, err := s.reconcilePayment(r.Context(), order)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	status := http.StatusOK
	if reconciled, ok := result["reconciled"].(bool); ok && !reconciled {
		status = http.StatusConflict
	}
	writeJSON(w, status, result)
}
