FROM golang:1.25-alpine AS builder

ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

WORKDIR /build

# Download dependencies first (cache layer)
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .
RUN go build -ldflags="-s -w" -trimpath -o lurus-identity ./cmd/server

# Minimal final image
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /build/lurus-identity /lurus-identity

EXPOSE 18104

ENTRYPOINT ["/lurus-identity"]
