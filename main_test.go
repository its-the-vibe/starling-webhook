package main

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestVerifySignature(t *testing.T) {
	// Generate an RSA key pair for testing
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA key: %v", err)
	}

	config := &Config{
		// WebhookSecret must be non-empty to trigger RSA verification; the
		// actual signing/verification uses the RSA key pair set on the server.
		WebhookSecret: "not-empty",
	}
	server := &Server{
		config:    config,
		publicKey: &privateKey.PublicKey,
	}

	payload := []byte(`{"eventType":"TEST"}`)

	// Create a valid RSA PKCS1v15 SHA-512 signature
	hash := sha512.Sum512(payload)
	sigBytes, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA512, hash[:])
	if err != nil {
		t.Fatalf("Failed to sign payload: %v", err)
	}
	validSignature := base64.StdEncoding.EncodeToString(sigBytes)

	// Test valid signature
	if !server.verifySignature(payload, validSignature) {
		t.Error("Valid signature was rejected")
	}

	// Test invalid signature: valid base64 ("invalid") but wrong RSA bytes
	if server.verifySignature(payload, "aW52YWxpZA==") {
		t.Error("Invalid signature was accepted")
	}

	// Test non-base64 signature
	if server.verifySignature(payload, "not-valid-base64!!!") {
		t.Error("Non-base64 signature was accepted")
	}
}

func TestVerifySignatureNoSecret(t *testing.T) {
	config := &Config{
		WebhookSecret: "",
	}
	server := &Server{
		config: config,
	}

	payload := []byte(`{"eventType":"TEST"}`)

	// Should accept any signature when no secret is configured
	if !server.verifySignature(payload, "any-signature") {
		t.Error("Signature check should pass when no secret is configured")
	}
}

func TestHandleWebhookInvalidMethod(t *testing.T) {
	config := &Config{
		RedisChannel: "test",
	}
	server := &Server{
		config: config,
	}

	req := httptest.NewRequest(http.MethodGet, "/webhook", nil)
	w := httptest.NewRecorder()

	server.handleWebhook(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestHandleWebhookInvalidJSON(t *testing.T) {
	config := &Config{
		RedisChannel:  "test",
		WebhookSecret: "",
	}
	server := &Server{
		config: config,
	}

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBufferString("invalid json"))
	w := httptest.NewRecorder()

	server.handleWebhook(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandleHealth(t *testing.T) {
	// Create a mock Redis client (note: this will fail without a real Redis instance)
	// For a real test, you would use a Redis mock or test container
	config := &Config{
		RedisAddr: "localhost:6379",
	}

	server := &Server{
		config:      config,
		redisClient: redis.NewClient(&redis.Options{Addr: config.RedisAddr}),
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	// This will fail if Redis is not running, which is expected in a unit test environment
	// In a real scenario, you'd use a mock or integration test
	if w.Code != http.StatusOK && w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d or %d, got %d", http.StatusOK, http.StatusServiceUnavailable, w.Code)
	}
}

func TestLoadConfig(t *testing.T) {
	// Save original env vars
	origPort := getEnv("PORT", "")
	origRedisAddr := getEnv("REDIS_ADDR", "")
	origRedisChannel := getEnv("REDIS_CHANNEL", "")
	origWebhookSecret := getEnv("WEBHOOK_SECRET", "")

	// Clean up after test
	defer func() {
		if origPort != "" {
			t.Setenv("PORT", origPort)
		}
		if origRedisAddr != "" {
			t.Setenv("REDIS_ADDR", origRedisAddr)
		}
		if origRedisChannel != "" {
			t.Setenv("REDIS_CHANNEL", origRedisChannel)
		}
		if origWebhookSecret != "" {
			t.Setenv("WEBHOOK_SECRET", origWebhookSecret)
		}
	}()

	config := loadConfig()

	if config.Port == "" {
		t.Error("Port should have a default value")
	}
	if config.RedisAddr == "" {
		t.Error("RedisAddr should have a default value")
	}
	if config.RedisChannel == "" {
		t.Error("RedisChannel should have a default value")
	}
}

func TestWebhookEventParsing(t *testing.T) {
	eventJSON := `{
		"webhookType": "TRANSACTION_FEED_ITEM_CREATED",
		"eventTimestamp": "2023-06-17T10:43:17.892Z",
		"content": {
			"feedItemUid": "abc-123",
			"amount": {
				"currency": "GBP",
				"minorUnits": 1000
			}
		},
		"accountHolderUid": "xyz-789"
	}`

	var event WebhookEvent
	if err := json.Unmarshal([]byte(eventJSON), &event); err != nil {
		t.Fatalf("Failed to parse webhook event: %v", err)
	}

	if event.EventType != "TRANSACTION_FEED_ITEM_CREATED" {
		t.Errorf("Expected eventType TRANSACTION_FEED_ITEM_CREATED, got %s", event.EventType)
	}

	if event.AccountHolderUID != "xyz-789" {
		t.Errorf("Expected accountHolderUid xyz-789, got %s", event.AccountHolderUID)
	}

	// Parse timestamp
	_, err := time.Parse(time.RFC3339, event.Timestamp)
	if err != nil {
		t.Errorf("Failed to parse timestamp: %v", err)
	}
}

func TestGetEnv(t *testing.T) {
	// Test with default value
	value := getEnv("NONEXISTENT_VAR", "default")
	if value != "default" {
		t.Errorf("Expected 'default', got '%s'", value)
	}

	// Test with set value
	t.Setenv("TEST_VAR", "test-value")
	value = getEnv("TEST_VAR", "default")
	if value != "test-value" {
		t.Errorf("Expected 'test-value', got '%s'", value)
	}
}
