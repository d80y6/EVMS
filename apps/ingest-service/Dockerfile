FROM rust:1.75-slim-bookworm AS builder

WORKDIR /app

# Install build dependencies
RUN apt-get update && apt-get install -y \
    pkg-config \
    libssl-dev \
    cmake \
    protobuf-compiler \
    && rm -rf /var/lib/apt/lists/*

# Copy manifests
COPY Cargo.toml Cargo.lock ./

# Create dummy source for dependency caching
RUN mkdir -p src && echo "fn main() {}" > src/main.rs

# Build dependencies only (caching layer)
RUN cargo build --release --bin ingest-service || true
RUN rm -rf src

# Copy actual source
COPY . .

# Build the application
RUN cargo build --release --bin ingest-service

# Runtime stage
FROM debian:bookworm-slim

# Install runtime dependencies
RUN apt-get update && apt-get install -y \
    ca-certificates \
    libssl3 \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN useradd -r -u 1000 -m ingest-user

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/target/release/ingest-service .

# Set ownership
RUN chown -R ingest-user:ingest-user /app

USER ingest-user

# Expose ports
EXPOSE 554 8080

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

ENTRYPOINT ["./ingest-service"]
