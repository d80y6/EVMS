# FIPS-compliant builder using Go with BoringCrypto
FROM golang:1.24-bullseye AS builder

# Install FIPS-compliant OpenSSL
RUN apt-get update && apt-get install -y \
    openssl \
    libssl-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build with FIPS mode
ENV CGO_ENABLED=1
ENV GOEXPERIMENT=opensslcrypto
RUN go build -tags fips -o /fips-verify ./cmd/fips.go
RUN go build -tags fips -o /auth-service ./services/auth/

FROM debian:bullseye-slim
RUN apt-get update && apt-get install -y \
    openssl \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /fips-verify /usr/local/bin/
COPY --from=builder /auth-service /usr/local/bin/

CMD ["fips-verify"]
