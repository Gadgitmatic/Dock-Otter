# Build stage
FROM golang:1.22-alpine AS builder

# Install build dependencies and security updates
RUN apk add --no-cache git ca-certificates tzdata && \
    apk upgrade --no-cache

# Create non-root user for security
RUN adduser -D -s /bin/sh -u 1001 appuser

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary with 2025 security best practices
ARG VERSION=dev
ARG BUILD_TIME
ARG GIT_COMMIT=unknown

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -extldflags '-static' \
    -X main.version=${VERSION} \
    -X main.buildTime=${BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)} \
    -X main.gitCommit=${GIT_COMMIT}" \
    -a -installsuffix cgo \
    -trimpath \
    -o dock-otter .

# Final stage - distroless for security
FROM gcr.io/distroless/static-debian12:nonroot

# Copy CA certificates and timezone data
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy the binary
COPY --from=builder /app/dock-otter /dock-otter

# Use non-root user
USER nonroot:nonroot

# Health check port
EXPOSE 8080

# Add health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD ["/dock-otter", "--health-check"] || exit 1

# Run the binary
ENTRYPOINT ["/dock-otter"]