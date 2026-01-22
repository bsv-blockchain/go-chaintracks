# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files first for layer caching
COPY go.mod go.sum ./

# Download dependencies - allow newer go version requirement
RUN GOTOOLCHAIN=go1.23.12+auto go mod download

# Copy source code
COPY . .

# Build the binary with static linking
RUN GOTOOLCHAIN=auto CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o server ./cmd/server

# Production stage
FROM alpine:3.21

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
