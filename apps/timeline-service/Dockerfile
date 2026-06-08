FROM rust:1.85-slim-bookworm AS builder

WORKDIR /app
COPY Cargo.toml .
COPY src/ ./src/

RUN cargo build --release

FROM debian:bookworm-slim

COPY --from=builder /app/target/release/timeline-service /usr/local/bin/

EXPOSE 3007

ENV RUST_LOG=info
ENV TIMELINE__HTTP_HOST=0.0.0.0
ENV TIMELINE__HTTP_PORT=3007

CMD ["timeline-service"]
