package main

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
)

// Config holds the application configuration
type Config struct {
	Port          string
	RedisAddr     string
	RedisChannel  string
	WebhookSecret string
	RedisPassword string
}

// WebhookEvent represents a generic Starling webhook event
type WebhookEvent struct {
	EventType        string          `json:"eventType"`
	Timestamp        string          `json:"timestamp"`
	Content          json.RawMessage `json:"content"`
	AccountHolderUID string          `json:"accountHolderUid,omitempty"`
	EventID          string          `json:"eventId,omitempty"`
}

// Server handles HTTP requests and publishes to Redis
type Server struct {
	config      *Config
	redisClient *redis.Client
	publicKey   *rsa.PublicKey
}

// NewServer creates a new Server instance
func NewServer(config *Config) (*Server, error) {
	redisClient := redis.NewClient(&redis.Options{
		Addr:     config.RedisAddr,
		Password: config.RedisPassword,
	})

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := redisClient.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	log.Printf("Connected to Redis at %s", config.RedisAddr)

	return &Server{
		config:      config,
		redisClient: redisClient,
	}, nil
}

// Close gracefully shuts down the server
func (s *Server) Close() error {
	return s.redisClient.Close()
}

func (s *Server) initialiseKey() error {
	// Decode the Base64 string from your config
	der, err := base64.StdEncoding.DecodeString(s.config.WebhookSecret)
	if err != nil {
		return err
	}

	// Parse the PKIX/X.509 format
	pub, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return err
	}

	// Assert that it is indeed an RSA key
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return errors.New("not an RSA public key")
	}

	s.publicKey = rsaPub
	return nil
}

func (s *Server) verifySignature(payload []byte, signature string) bool {
	if s.config.WebhookSecret == "" {
		return true
	}

	// 1. Decode the signature from the header
	sigBytes, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return false
	}

	// 2. Hash the payload
	// Most providers use SHA256 for RSA signatures
	hash := sha256.Sum256(payload)

	// 3. Verify using the Public Key
	// We use rsa.VerifyPKCS1v15 for standard RSA signatures
	err = rsa.VerifyPKCS1v15(s.publicKey, crypto.SHA256, hash[:], sigBytes)

	return err == nil
}

// handleWebhook processes incoming webhook requests
func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading request body: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Verify the signature
	signature := r.Header.Get("X-Hook-Signature")
	if !s.verifySignature(body, signature) {
		log.Printf("Invalid signature for webhook")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse the webhook event
	var event WebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		log.Printf("Error parsing webhook event: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Publish to Redis
	ctx := context.Background()
	if err := s.redisClient.Publish(ctx, s.config.RedisChannel, body).Err(); err != nil {
		log.Printf("Error publishing to Redis: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	log.Printf("Published event type %s to Redis channel %s", event.EventType, s.config.RedisChannel)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// handleHealth provides a health check endpoint
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := s.redisClient.Ping(ctx).Err(); err != nil {
		http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// loadConfig loads configuration from environment variables
func loadConfig() *Config {
	config := &Config{
		Port:          getEnv("PORT", "8080"),
		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		RedisChannel:  getEnv("REDIS_CHANNEL", "starling_events"),
		WebhookSecret: getEnv("WEBHOOK_SECRET", ""),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
	}

	if config.WebhookSecret == "" {
		log.Println("Warning: WEBHOOK_SECRET not set. Webhook signature verification will be skipped.")
	}

	if config.RedisPassword == "" {
		log.Println("Warning: REDIS_PASSWORD not set. Redis connection will be attempted without a password.")
	}

	return config
}

// getEnv gets an environment variable with a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Load configuration
	config := loadConfig()

	// Create server
	server, err := NewServer(config)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}
	defer server.Close()

	if err := server.initialiseKey(); err != nil {
		log.Fatalf("Failed to initialise key: %v", err)
	}

	// Set up HTTP handlers
	http.HandleFunc("/webhook", server.handleWebhook)
	http.HandleFunc("/health", server.handleHealth)

	// Create HTTP server
	httpServer := &http.Server{
		Addr:         ":" + config.Port,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Starting server on port %s", config.Port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}
