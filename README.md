# starling-webhook

A simple Go web service which consumes Starling Bank webhook events and publishes them to Redis pub/sub.

## Overview

This service receives webhook notifications from Starling Bank, verifies their authenticity using HMAC signature verification, and publishes the events to a Redis pub/sub channel for downstream processing.

## Features

- ✅ Receives Starling Bank v2 webhook events via HTTP POST
- ✅ Verifies webhook signatures using HMAC SHA-512
- ✅ Publishes events to Redis pub/sub for downstream consumers
- ✅ Health check endpoint
- ✅ Graceful shutdown
- ✅ Configurable via environment variables

## Prerequisites

- Go 1.21 or higher
- Redis server running

## Configuration

The service is configured via environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | HTTP server port | `8080` |
| `REDIS_ADDR` | Redis server address | `localhost:6379` |
| `REDIS_CHANNEL` | Redis channel name for publishing events | `starling_events` |
| `WEBHOOK_SECRET` | Starling webhook secret for signature verification | _(empty, verification skipped)_ |

## Installation

```bash
# Clone the repository
git clone https://github.com/its-the-vibe/starling-webhook.git
cd starling-webhook

# Build the binary
go build -o starling-webhook

# Or run directly
go run main.go
```

## Usage

### Running the service

```bash
# Set environment variables
export REDIS_ADDR="localhost:6379"
export REDIS_CHANNEL="starling_events"
export WEBHOOK_SECRET="your-webhook-secret"
export PORT="8080"

# Run the service
./starling-webhook
```

### Using Docker

```bash
docker run -d \
  -p 8080:8080 \
  -e REDIS_ADDR="redis:6379" \
  -e REDIS_CHANNEL="starling_events" \
  -e WEBHOOK_SECRET="your-webhook-secret" \
  starling-webhook
```

### Configuring Starling Bank Webhook

1. Log in to the [Starling Developer Portal](https://developer.starlingbank.com/)
2. Navigate to your application settings
3. Set your webhook URL to: `https://your-domain.com/webhook`
4. Configure your webhook secret (use the same value in `WEBHOOK_SECRET`)
5. Select the event types you want to receive

## Endpoints

### POST /webhook

Receives Starling Bank webhook events and publishes them to Redis.

**Headers:**
- `X-Hook-Signature`: HMAC SHA-512 signature (base64 encoded)

**Example Event:**
```json
{
  "eventType": "TRANSACTION_FEED_ITEM_CREATED",
  "timestamp": "2023-06-17T10:43:17.892Z",
  "content": {
    "feedItemUid": "abc-123",
    "amount": {
      "currency": "GBP",
      "minorUnits": 1000
    },
    "transactionTime": "2023-06-17T10:43:17.892Z",
    "source": "FASTER_PAYMENTS_IN",
    "status": "SETTLED"
  },
  "accountHolderUid": "xyz-789"
}
```

### GET /health

Health check endpoint that verifies Redis connectivity.

**Response:**
- `200 OK` - Service is healthy
- `503 Service Unavailable` - Redis is unreachable

## Consuming Events

To consume events from Redis:

```go
package main

import (
    "context"
    "fmt"
    "github.com/redis/go-redis/v9"
)

func main() {
    rdb := redis.NewClient(&redis.Options{
        Addr: "localhost:6379",
    })

    pubsub := rdb.Subscribe(context.Background(), "starling_events")
    ch := pubsub.Channel()

    for msg := range ch {
        fmt.Printf("Received event: %s\n", msg.Payload)
    }
}
```

A complete example consumer is available in the `examples/consumer` directory.

## Development

### Running Tests

```bash
go test ./...
```

### Linting

```bash
go fmt ./...
go vet ./...
```

## Architecture

This service follows a simple webhook-to-pubsub pattern:

```
Starling Bank → [Webhook] → starling-webhook → Redis Pub/Sub → Consumers
```

The service acts as a bridge between Starling Bank's webhook notifications and your internal event processing pipeline.

## Security

- **Signature Verification**: All webhook requests are verified using HMAC SHA-512 to ensure they originate from Starling Bank
- **TLS**: It's recommended to deploy this service behind a reverse proxy (nginx, Caddy) with TLS enabled
- **Secret Management**: Store `WEBHOOK_SECRET` securely (e.g., using environment variables, secrets management systems)

## License

MIT

## Related Projects

- [github-webhook](https://github.com/its-the-vibe/github-webhook) - Similar service for GitHub webhooks
