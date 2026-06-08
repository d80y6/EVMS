FROM rust:1.85-slim-bookworm AS builder

RUN apt-get update && apt-get install -y \
    libgstreamer1.0-dev \
    libgstreamer-plugins-base1.0-dev \
    gstreamer1.0-tools \
    gstreamer1.0-plugins-base \
    gstreamer1.0-plugins-good \
    pkg-config \
    cmake \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY Cargo.toml .
COPY src/ ./src/

RUN cargo build --release

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y \
    libgstreamer1.0-0 \
    gstreamer1.0-plugins-base \
    gstreamer1.0-plugins-good \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/target/release/media-processor /usr/local/bin/

EXPOSE 3004

ENV RUST_LOG=info
ENV MEDIA__HTTP_HOST=0.0.0.0
ENV MEDIA__HTTP_PORT=3004

CMD ["media-processor"]
