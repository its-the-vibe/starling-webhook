# Build stage
FROM golang:1.25.5-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o starling-webhook .

# Final stage
FROM scratch

WORKDIR /root/

# Copy the binary from builder
COPY --from=builder /app/starling-webhook .

# Expose port
EXPOSE 8080

# Run the binary
CMD ["./starling-webhook"]
