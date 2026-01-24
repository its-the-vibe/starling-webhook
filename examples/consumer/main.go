package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/redis/go-redis/v9"
)

func main() {
	// Get Redis configuration from environment
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	redisChannel := os.Getenv("REDIS_CHANNEL")
	if redisChannel == "" {
		redisChannel = "starling_events"
	}

	// Create Redis client
	rdb := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	// Test connection
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	log.Printf("Connected to Redis at %s", redisAddr)
	log.Printf("Subscribing to channel: %s", redisChannel)

	// Subscribe to the channel
	pubsub := rdb.Subscribe(ctx, redisChannel)
	defer pubsub.Close()

	// Wait for subscription confirmation
	_, err := pubsub.Receive(ctx)
	if err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}

	log.Println("Waiting for messages... (Press Ctrl+C to exit)")

	// Create channel for messages
	ch := pubsub.Channel()

	// Handle graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Process messages
	for {
		select {
		case msg := <-ch:
			fmt.Printf("\n=== Received Event ===\n")
			fmt.Printf("Channel: %s\n", msg.Channel)
			fmt.Printf("Payload:\n%s\n", msg.Payload)
			fmt.Println("===================")
		case <-quit:
			log.Println("Shutting down consumer...")
			return
		}
	}
}
