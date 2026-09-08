package fraudguard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Transaction struct {
	TransactionID string    `json:"transaction_id"`
	UserID        string    `json:"user_id"`
	Amount        float64   `json:"amount"`
	Timestamp     time.Time `json:"timestamp"`
	Location      string    `json:"location"`
	DeviceID      string    `json:"device_id"`
}

type RiskResponse struct {
	TransactionID string   `json:"transaction_id"`
	RiskScore     int      `json:"risk_score"`
	RiskLevel     string   `json:"risk_level"`
	Decision      string   `json:"decision"`
	Reasons       []string `json:"reasons"`
	Duplicate     bool     `json:"duplicate"`
}

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (c *Client) CheckTransaction(ctx context.Context, transaction Transaction) (RiskResponse, error) {
	body, err := json.Marshal(transaction)
	if err != nil {
		return RiskResponse{}, fmt.Errorf("marshal fraud request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.BaseURL+"/transactions",
		bytes.NewReader(body),
	)
	if err != nil {
		return RiskResponse{}, fmt.Errorf("create fraud request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return RiskResponse{}, fmt.Errorf("call FraudGuard: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return RiskResponse{}, fmt.Errorf("FraudGuard returned HTTP %d", resp.StatusCode)
	}

	var result RiskResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return RiskResponse{}, fmt.Errorf("decode FraudGuard response: %w", err)
	}

	return result, nil
}
