# Build stage
FROM golang:1.27-alpine@sha256:fa3d596898d9f126af7c01d14ee7597bab3f39cccf392ef36b69fe77cd681f18 AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files first for layer caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary with static linking
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o server ./cmd/server

# Production stage
FROM alpine:3.24@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a

WORKDIR /app

# Install runtime dependencies and create non-root user for security
RUN apk --no-cache add ca-certificates wget && \
    addgroup -g 1000 chaintracks && \
    adduser -u 1000 -G chaintracks -s /bin/sh -D chaintracks

# Copy binary from builder
COPY --from=builder /app/server .

# Copy default config if needed
COPY --from=builder /app/cmd/server/config.example.yaml ./config.yaml

# Create storage directory and set ownership
RUN mkdir -p /data/chaintracks && \
    chown -R chaintracks:chaintracks /app /data/chaintracks

# Expose API port (same as TypeScript service)
EXPOSE 3011

# Expose CDN port (same as TypeScript service)
EXPOSE 3012

# Switch to non-root user
USER chaintracks

# Health check using /v2/network endpoint
HEALTHCHECK --interval=30s --timeout=10s --retries=3 --start-period=60s \
  CMD wget --quiet --tries=1 --spider http://localhost:3011/v2/network || exit 1

# Run the server
CMD ["./server"]
