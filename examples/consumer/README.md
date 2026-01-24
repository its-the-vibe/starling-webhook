# Example Consumer

This is a simple example consumer that subscribes to the Redis pub/sub channel and prints received Starling webhook events.

## Usage

```bash
# Build the consumer
cd examples/consumer
go build -o consumer

# Run the consumer
./consumer

# Or with custom configuration
REDIS_ADDR=localhost:6379 REDIS_CHANNEL=starling_events ./consumer
```

## Output

When a webhook event is received, the consumer will print:

```
=== Received Event ===
Channel: starling_events
Payload:
{
  "eventType": "TRANSACTION_FEED_ITEM_CREATED",
  "timestamp": "2023-06-17T10:43:17.892Z",
  ...
}
===================
```
