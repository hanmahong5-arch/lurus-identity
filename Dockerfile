FROM oven/bun:1 AS frontend

WORKDIR /web
COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile
COPY web/ .
RUN bun run build

FROM golang:1.25-alpine AS builder

ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

WORKDIR /build

# Download dependencies first (cache layer)
COPY go.mod go.sum ./
RUN go mod download

# Copy frontend build output
COPY --from=frontend /web/dist ./web/dist

# Build
COPY . .
RUN go build -ldflags="-s -w" -trimpath -o lurus-identity ./cmd/server

# Minimal final image
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /build/lurus-identity /lurus-identity

EXPOSE 18104

ENTRYPOINT ["/lurus-identity"]
