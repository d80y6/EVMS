FROM rust:1.85-slim-bookworm AS builder
WORKDIR /app
RUN apt-get update && apt-get install -y protobuf-compiler pkg-config libssl-dev && rm -rf /var/lib/apt/lists/*
COPY Cargo.toml build.rs ./
COPY Cargo.lock* ./
COPY proto/ proto/
RUN mkdir src && echo "fn main() {}" > src/main.rs
RUN cargo build --release 2>/dev/null || true
COPY src/ src/
RUN cargo build --release

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates libssl3 && rm -rf /var/lib/apt/lists/*
COPY --from=builder /app/target/release/triton-inference-service /usr/local/bin/
EXPOSE 3000 3001
CMD ["triton-inference-service"]
