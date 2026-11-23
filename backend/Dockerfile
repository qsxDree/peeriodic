# --- Build Stage ---
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache ca-certificates git

# Cache Go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and embedded migrations
COPY . .

# Build statically linked binary with stripped debug symbols
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o /app/bin/server main.go

# --- Final Runtime Stage ---
FROM alpine:3.20

WORKDIR /app

# Install security certificates & timezone data
RUN apk add --no-cache ca-certificates tzdata wget && \
    addgroup -S appgroup && adduser -S appuser -G appgroup

# Copy compiled binary from builder
COPY --from=builder /app/bin/server /app/server

# Set permissions for non-root execution
RUN chown -R appuser:appgroup /app
USER appuser

EXPOSE 5000

HEALTHCHECK --interval=15s --timeout=5s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:5000/healthz || exit 1

ENTRYPOINT ["/app/server"]
