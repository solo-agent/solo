# =============================================================================
# Solo - Multi-stage Dockerfile
# =============================================================================
# Stage 1: Build Go binaries
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Cache module downloads
COPY go.mod go.sum ./
RUN go mod download

# Copy full source
COPY . .

# Build both server and daemon binaries with stripped debug info
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/server ./cmd/server/
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/daemon ./cmd/daemon/

# =============================================================================
# Stage 2: Runtime image — reuse golang:alpine (already cached)
FROM golang:1.22-alpine

# Create a non-root user for running the services
RUN addgroup -S solo && adduser -S -G solo solo

COPY --from=builder /app/server /usr/local/bin/server
COPY --from=builder /app/daemon /usr/local/bin/daemon

RUN chown solo:solo /usr/local/bin/server /usr/local/bin/daemon

USER solo

EXPOSE 8080 8081

# Default to running the server. Use "daemon" as the command to run the daemon.
ENTRYPOINT ["server"]
CMD []
