package orders

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	StatusPending   = "PENDING"
	StatusReview    = "REVIEW"
	StatusRejected  = "REJECTED"
	StatusPayment   = "PAYMENT_INITIALIZED"
	StatusPaid      = "PAID"
	StatusFailed    = "FAILED"
)

type Order struct {
	OrderID       string    `json:"order_id"`
	TransactionID string    `json:"transaction_id"`
	UserID        string    `json:"user_id"`
	Amount        float64   `json:"amount"`
	Currency      string    `json:"currency"`
	Status        string    `json:"status"`
	PaymentStatus string    `json:"payment_status"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Store struct {
	mu     sync.RWMutex
	path   string
	orders map[string]Order
}

func NewStore(path string) (*Store, error) {
	if path == "" {
		path = "orders.json"
	}
	s := &Store{path: path, orders: make(map[string]Order)}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read order store: %w", err)
	}
	if len(data) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(data, &s.orders); err != nil {
		return nil, fmt.Errorf("decode order store: %w", err)
	}
	return s, nil
}

func (s *Store) Create(order Order) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.orders[order.OrderID]; exists {
		return fmt.Errorf("order %s already exists", order.OrderID)
	}
	now := time.Now().UTC()
	order.CreatedAt = now
	order.UpdatedAt = now
	s.orders[order.OrderID] = order
	return s.persistLocked()
}

func (s *Store) Get(orderID string) (Order, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	order, ok := s.orders[orderID]
	return order, ok
}

func (s *Store) GetByTransaction(transactionID string) (Order, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, order := range s.orders {
		if order.TransactionID == transactionID {
			return order, true
		}
	}
	return Order{}, false
}

// List returns orders filtered by user and/or status, newest first.
// limit <= 0 means no explicit limit; offset < 0 is treated as zero.
func (s *Store) List(userID, status string, limit, offset int) []Order {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if offset < 0 {
		offset = 0
	}

	result := make([]Order, 0, len(s.orders))
	for _, order := range s.orders {
		if userID != "" && order.UserID != userID {
			continue
		}
		if status != "" && order.Status != status {
			continue
		}
		result = append(result, order)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	if offset >= len(result) {
		return []Order{}
	}
	result = result[offset:]
	if limit > 0 && limit < len(result) {
		result = result[:limit]
	}
	return result
}

func (s *Store) UpdateStatus(orderID, status string) (Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[orderID]
	if !ok {
		return Order{}, fmt.Errorf("order %s not found", orderID)
	}
	order.Status = status
	order.UpdatedAt = time.Now().UTC()
	s.orders[orderID] = order
	if err := s.persistLocked(); err != nil {
		return Order{}, err
	}
	return order, nil
}

func (s *Store) UpdatePaymentStatus(orderID, status string) (Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[orderID]
	if !ok {
		return Order{}, fmt.Errorf("order %s not found", orderID)
	}
	order.PaymentStatus = status
	order.UpdatedAt = time.Now().UTC()
	s.orders[orderID] = order
	if err := s.persistLocked(); err != nil {
		return Order{}, err
	}
	return order, nil
}

func (s *Store) persistLocked() error {
	if dir := filepath.Dir(s.path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create order store directory: %w", err)
		}
	}
	data, err := json.MarshalIndent(s.orders, "", "  ")
	if err != nil {
		return fmt.Errorf("encode order store: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write order store: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("replace order store: %w", err)
	}
	return nil
}
