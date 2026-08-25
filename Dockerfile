# Build stage
FROM golang:1.27-alpine@sha256:4c9fe60190a2a3350ddc51de80d0224b8a6698d12bdfc999fee45ea9d6c46dbc AS builder

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
FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

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
