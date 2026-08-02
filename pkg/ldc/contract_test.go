package ldc

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testPrivateKey() ed25519.PrivateKey {
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index + 1)
	}
	return ed25519.NewKeyFromSeed(seed)
}

func TestCreateOrderSignsRequestAndResolvesCheckoutURL(t *testing.T) {
	privateKey := testPrivateKey()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.NoError(t, r.ParseForm())

		expected := map[string]string{
			"client_id":    "client-1",
			"type":         PaymentType,
			"out_trade_no": "trade-1",
			"order_name":   "Top-up",
			"money":        "10.00",
			"notify_url":   "https://merchant.example/ldc/notify",
			"return_url":   "https://merchant.example/wallet",
		}
		for key, value := range expected {
			assert.Equal(t, value, r.PostForm.Get(key), key)
		}

		signature, err := base64.StdEncoding.DecodeString(r.PostForm.Get("sign"))
		require.NoError(t, err)
		require.True(t, ed25519.Verify(
			privateKey.Public().(ed25519.PublicKey),
			[]byte(buildSignaturePayload(expected, "secret")),
			signature,
		))
		_, err = io.WriteString(w, `{"data":{"url":"/checkout/trade-1"}}`)
		require.NoError(t, err)
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL:      server.URL + "/epay",
		ClientID:     "client-1",
		ClientSecret: "secret",
		PrivateKey:   base64.StdEncoding.EncodeToString(privateKey.Seed()),
	})
	require.NoError(t, err)

	checkoutURL, err := client.CreateOrder(context.Background(), CreateOrderRequest{
		ServiceTradeNo: "trade-1",
		OrderName:      "Top-up",
		Amount:         "10.00",
		NotifyURL:      "https://merchant.example/ldc/notify",
		ReturnURL:      "https://merchant.example/wallet",
	})
	require.NoError(t, err)
	assert.Equal(t, server.URL+"/checkout/trade-1", checkoutURL)
}

func TestVerifyNotificationRejectsTamperedAmount(t *testing.T) {
	privateKey := testPrivateKey()
	client, err := NewClient(Config{
		ClientID:     "client-1",
		ClientSecret: "secret",
		PrivateKey:   hex.EncodeToString(privateKey.Seed()),
	})
	require.NoError(t, err)

	params := url.Values{
		"pid":          {"client-1"},
		"out_trade_no": {"trade-1"},
		"trade_no":     {"ldc-trade-1"},
		"type":         {PaymentType},
		"name":         {"Top-up"},
		"money":        {"10.00"},
		"trade_status": {TradeStatusOK},
	}
	values := make(map[string]string, len(params))
	for key := range params {
		values[key] = params.Get(key)
	}
	params.Set("sign", signMD5(values, "secret"))

	notification, err := client.VerifyNotification(params)
	require.NoError(t, err)
	assert.Equal(t, "trade-1", notification.ServiceTradeNo)
	assert.Equal(t, "10.00", notification.Money)

	params.Set("money", "1000.00")
	_, err = client.VerifyNotification(params)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "signature"))
}
