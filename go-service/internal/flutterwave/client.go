package flutterwave

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type Client struct {
	BaseURL    string
	SecretKey  string
	HTTPClient *http.Client
}

type Customer struct {
	Email       string `json:"email"`
	Name        string `json:"name,omitempty"`
	PhoneNumber string `json:"phonenumber,omitempty"`
}

type PaymentRequest struct {
	TxRef       string   `json:"tx_ref"`
	Amount      float64  `json:"amount"`
	Currency    string   `json:"currency"`
	RedirectURL string   `json:"redirect_url"`
	Customer    Customer `json:"customer"`
	Customizations struct {
		Title string `json:"title,omitempty"`
	} `json:"customizations,omitempty"`
}

type PaymentResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Data    struct {
		Link string `json:"link"`
	} `json:"data"`
}

type VerifyResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Data    struct {
		ID            int     `json:"id"`
		TxRef         string  `json:"tx_ref"`
		Amount        float64 `json:"amount"`
		ChargedAmount float64 `json:"charged_amount"`
		Currency      string  `json:"currency"`
		Status        string  `json:"status"`
	} `json:"data"`
}

func NewClient(secretKey string) *Client {
	return &Client{
		BaseURL:   getenv("FLW_BASE_URL", "https://api.flutterwave.com/v3"),
		SecretKey: secretKey,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) CreatePayment(ctx context.Context, payment PaymentRequest) (PaymentResponse, error) {
	var result PaymentResponse
	if err := c.doJSON(ctx, http.MethodPost, "/payments", payment, &result); err != nil {
		return result, err
	}
	return result, nil
}

func (c *Client) VerifyTransaction(ctx context.Context, transactionID string) (VerifyResponse, error) {
	var result VerifyResponse
	if err := c.doJSON(ctx, http.MethodGet, "/transactions/"+transactionID+"/verify", nil, &result); err != nil {
		return result, err
	}
	return result, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, payload any, result any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal Flutterwave request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, body)
	if err != nil {
		return fmt.Errorf("create Flutterwave request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.SecretKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("call Flutterwave: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Flutterwave returned HTTP %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("decode Flutterwave response: %w", err)
	}
	return nil
}

// VerifyWebhookSignature validates the HMAC-SHA256 webhook signature used by
// newer Flutterwave webhook configurations.
func VerifyWebhookSignature(rawBody []byte, signature, secretHash string) bool {
	if signature == "" || secretHash == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secretHash))
	_, _ = mac.Write(rawBody)
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) == 1
}

// VerifyWebhookSecretHash validates the verif-hash header used by Flutterwave
// v3 webhook configurations.
func VerifyWebhookSecretHash(signature, secretHash string) bool {
	if signature == "" || secretHash == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(signature), []byte(secretHash)) == 1
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
