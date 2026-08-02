package ldc

import (
	"context"
	"crypto/ed25519"
	"crypto/md5"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	PaymentType    = "ldcpay"
	TradeStatusOK  = "TRADE_SUCCESS"
	DefaultBaseURL = "https://credit.linux.do/epay"
)

type Config struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
	PrivateKey   string
	HTTPClient   *http.Client
}

type Client struct {
	baseURL      *url.URL
	clientID     string
	clientSecret string
	privateKey   ed25519.PrivateKey
	httpClient   *http.Client
}

type CreateOrderRequest struct {
	ServiceTradeNo string
	OrderName      string
	Amount         string
	NotifyURL      string
	ReturnURL      string
}

type Notification struct {
	ClientID       string
	ServiceTradeNo string
	TradeNo        string
	Type           string
	Name           string
	Money          string
	TradeStatus    string
}

func NewClient(cfg Config) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	parsedURL, err := url.Parse(baseURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, fmt.Errorf("invalid LDC base URL")
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("LDC base URL must use http or https")
	}
	if strings.TrimSpace(cfg.ClientID) == "" || strings.TrimSpace(cfg.ClientSecret) == "" {
		return nil, errors.New("LDC client credentials are required")
	}
	privateKey, err := parsePrivateKey(cfg.PrivateKey)
	if err != nil {
		return nil, err
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}

	return &Client{
		baseURL:      parsedURL,
		clientID:     strings.TrimSpace(cfg.ClientID),
		clientSecret: strings.TrimSpace(cfg.ClientSecret),
		privateKey:   privateKey,
		httpClient:   httpClient,
	}, nil
}

func (c *Client) CreateOrder(ctx context.Context, req CreateOrderRequest) (string, error) {
	if c == nil || c.baseURL == nil {
		return "", errors.New("LDC client is not initialized")
	}
	if strings.TrimSpace(req.ServiceTradeNo) == "" || strings.TrimSpace(req.OrderName) == "" || strings.TrimSpace(req.Amount) == "" {
		return "", errors.New("LDC order fields are incomplete")
	}

	params := map[string]string{
		"client_id":    c.clientID,
		"type":         PaymentType,
		"out_trade_no": req.ServiceTradeNo,
		"order_name":   req.OrderName,
		"money":        req.Amount,
		"notify_url":   req.NotifyURL,
		"return_url":   req.ReturnURL,
	}
	form := url.Values{}
	for key, value := range params {
		if value != "" {
			form.Set(key, value)
		}
	}
	form.Set("sign", c.signEd25519(params))

	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/pay/submit.php"
	endpoint.RawQuery = ""
	endpoint.Fragment = ""
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("build LDC order request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json, text/plain, */*")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("send LDC order request: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read LDC order response: %w", err)
	}
	if location := strings.TrimSpace(response.Header.Get("Location")); location != "" && response.StatusCode >= 300 && response.StatusCode < 400 {
		return resolveURL(c.baseURL, location)
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		if location := extractCheckoutURL(body); location != "" {
			return resolveURL(c.baseURL, location)
		}
	}

	message := extractErrorMessage(body)
	if message == "" {
		message = strings.TrimSpace(string(body))
	}
	if len(message) > 512 {
		message = message[:512]
	}
	return "", fmt.Errorf("LDC order request failed: status=%d: %s", response.StatusCode, message)
}

func (c *Client) VerifyNotification(params url.Values) (Notification, error) {
	if c == nil {
		return Notification{}, errors.New("LDC client is not initialized")
	}
	values := make(map[string]string, len(params))
	for key := range params {
		values[key] = params.Get(key)
	}
	signature := values["sign"]
	if signature == "" {
		return Notification{}, errors.New("LDC notification signature is missing")
	}
	expected := signMD5(values, c.clientSecret)
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(signature)), []byte(expected)) != 1 {
		return Notification{}, errors.New("LDC notification signature is invalid")
	}

	notification := Notification{
		ClientID:       values["pid"],
		ServiceTradeNo: values["out_trade_no"],
		TradeNo:        values["trade_no"],
		Type:           values["type"],
		Name:           values["name"],
		Money:          values["money"],
		TradeStatus:    values["trade_status"],
	}
	if notification.ClientID == "" {
		notification.ClientID = values["client_id"]
	}
	if notification.ClientID != c.clientID {
		return Notification{}, errors.New("LDC notification client ID is invalid")
	}
	if notification.ServiceTradeNo == "" || notification.Money == "" {
		return Notification{}, errors.New("LDC notification fields are incomplete")
	}
	return notification, nil
}

func (c *Client) signEd25519(params map[string]string) string {
	payload := buildSignaturePayload(params, c.clientSecret)
	signature := ed25519.Sign(c.privateKey, []byte(payload))
	return base64.StdEncoding.EncodeToString(signature)
}

func signMD5(params map[string]string, secret string) string {
	hash := md5.Sum([]byte(buildSignaturePayload(params, secret)))
	return hex.EncodeToString(hash[:])
}

func buildSignaturePayload(params map[string]string, secret string) string {
	keys := make([]string, 0, len(params))
	for key, value := range params {
		if key == "sign" || key == "sign_type" || value == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var builder strings.Builder
	for index, key := range keys {
		if index > 0 {
			builder.WriteByte('&')
		}
		builder.WriteString(key)
		builder.WriteByte('=')
		builder.WriteString(params[key])
	}
	builder.WriteString(secret)
	return builder.String()
}

func parsePrivateKey(value string) (ed25519.PrivateKey, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("LDC private key is required")
	}
	if block, _ := pem.Decode([]byte(value)); block != nil {
		if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
			if privateKey, ok := key.(ed25519.PrivateKey); ok {
				return clonePrivateKey(privateKey), nil
			}
		}
		if privateKey, ok := privateKeyFromBytes(block.Bytes); ok {
			return privateKey, nil
		}
	}

	candidates := make([][]byte, 0, 2)
	if decoded, err := base64.StdEncoding.DecodeString(value); err == nil {
		candidates = append(candidates, decoded)
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(value); err == nil {
		candidates = append(candidates, decoded)
	}
	if decoded, err := base64.RawURLEncoding.DecodeString(value); err == nil {
		candidates = append(candidates, decoded)
	}
	if decoded, err := hex.DecodeString(value); err == nil {
		candidates = append(candidates, decoded)
	}
	for _, candidate := range candidates {
		if key, err := x509.ParsePKCS8PrivateKey(candidate); err == nil {
			if privateKey, ok := key.(ed25519.PrivateKey); ok {
				return clonePrivateKey(privateKey), nil
			}
		}
		if privateKey, ok := privateKeyFromBytes(candidate); ok {
			return privateKey, nil
		}
	}
	return nil, errors.New("LDC private key must be Ed25519 PKCS#8, PEM, base64, or hex")
}

func privateKeyFromBytes(value []byte) (ed25519.PrivateKey, bool) {
	switch len(value) {
	case ed25519.PrivateKeySize:
		return clonePrivateKey(ed25519.PrivateKey(value)), true
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(value), true
	default:
		return nil, false
	}
}

func clonePrivateKey(value ed25519.PrivateKey) ed25519.PrivateKey {
	return append(ed25519.PrivateKey(nil), value...)
}

func resolveURL(baseURL *url.URL, location string) (string, error) {
	parsed, err := url.Parse(location)
	if err != nil {
		return "", fmt.Errorf("invalid LDC checkout URL: %w", err)
	}
	return baseURL.ResolveReference(parsed).String(), nil
}

func extractCheckoutURL(body []byte) string {
	var payload map[string]any
	if common.Unmarshal(body, &payload) != nil {
		return ""
	}
	for _, key := range []string{"url", "pay_url", "checkout_url", "redirect_url"} {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	if data, ok := payload["data"].(map[string]any); ok {
		for _, key := range []string{"url", "pay_url", "checkout_url", "redirect_url"} {
			if value, ok := data[key].(string); ok && strings.TrimSpace(value) != "" {
				return value
			}
		}
	}
	if data, ok := payload["data"].(string); ok {
		return strings.TrimSpace(data)
	}
	return ""
}

func extractErrorMessage(body []byte) string {
	var payload map[string]any
	if common.Unmarshal(body, &payload) != nil {
		return ""
	}
	for _, key := range []string{"error_msg", "message", "msg"} {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
