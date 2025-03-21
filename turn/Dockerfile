# SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
# SPDX-License-Identifier: MIT

# why we need a multi-stage build:
# - minimizes final image size
# - separates build dependencies from runtime
# - produces a clean runtime environment
FROM golang:alpine as builder

WORKDIR /build

# why we need these build dependencies:
# - git for fetching dependencies
# - gcc and musl-dev for any cgo dependencies
RUN apk add --no-cache git gcc musl-dev

# Copy our local implementation
COPY . .

# Download dependencies
RUN go mod download

# Build static binary
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-w -s" -o turn-server main.go

# Final stage
FROM alpine:latest
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/turn-server .

# why we need an entrypoint script:
# - properly handles environment variables
# - ensures correct argument passing
# - enables verbose logging
COPY entrypoint.sh /app/
RUN chmod +x /app/entrypoint.sh

ENTRYPOINT ["/app/entrypoint.sh"] 