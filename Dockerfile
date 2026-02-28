# Build stage
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache ca-certificates git

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /earthmc-scraper ./cmd/worker

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

# Run as non-root
RUN adduser -D -g '' appuser
USER appuser

COPY --from=builder /earthmc-scraper /earthmc-scraper

ENTRYPOINT ["/earthmc-scraper"]
