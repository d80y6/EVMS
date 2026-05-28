# Ingest Service

Rust-based RTSP/RTP ingestion service with WebRTC support, fMP4 muxing, and S3 storage.

## Features

- **RTSP Server**: Full RTSP protocol support (DESCRIBE, SETUP, PLAY, TEARDOWN, RECORD)
- **RTP Jitter Buffer**: Packet reordering with configurable latency tolerance
- **RTCP Handler**: Sender/Receiver reports, NACK support, quality metrics
- **fMP4 Muxer**: Fragmented MP4 segment generation
- **S3 Storage**: Multipart upload support for large segments
- **WebRTC Signaling**: SDP offer/answer, ICE candidate exchange
- **Metrics**: Prometheus-compatible metrics endpoint
- **API**: REST and GraphQL APIs for control and monitoring

## Quick Start

```bash
# Copy example configuration
cp .env.example .env

# Edit configuration
vim .env

# Build
cargo build --release

# Run
cargo run --release
```

## Configuration

All configuration is done via environment variables with the `INGEST__` prefix:

| Variable | Default | Description |
|----------|---------|-------------|
| `INGEST__RTSP_PORT` | 554 | RTSP server port |
| `INGEST__RTP_BUFFER_SIZE` | 1024 | RTP jitter buffer size (packets) |
| `INGEST__RTP_MAX_LATENCY_MS` | 500 | Maximum packet latency (ms) |
| `INGEST__SEGMENT_DURATION_MS` | 2000 | fMP4 segment duration (ms) |
| `INGEST__S3_BUCKET` | - | S3 bucket name |
| `INGEST__S3_REGION` | us-east-1 | S3 region |
| `INGEST__S3_ENDPOINT` | - | S3 endpoint (for MinIO, etc.) |
| `INGEST__METRICS_BIND_ADDR` | 0.0.0.0:9090 | Metrics endpoint |
| `INGEST__API_BIND_ADDR` | 0.0.0.0:8080 | API server address |

## API Endpoints

### REST

- `GET /health` - Health check
- `GET /metrics` - Prometheus metrics
- `GET /streams/:stream_id` - Stream statistics
- `POST /webrtc/offer` - Create WebRTC offer
- `POST /webrtc/answer` - Handle WebRTC answer
- `POST /webrtc/ice` - Add ICE candidate

### GraphQL

Available at `/graphql` (when enabled):

```graphql
query {
  health {
    status
  }
  stream(ssrc: 12345) {
    ssrc
    packetsReceived
    packetsLost
  }
}
```

## Metrics

Prometheus metrics available at `/metrics`:

- `rtp_packets_received_total` - Total RTP packets received
- `rtp_bytes_received_total` - Total RTP bytes received
- `rtp_packets_dropped_total` - Dropped packets (by reason)
- `rtp_buffer_occupancy` - Current buffer occupancy
- `rtcp_packets_received_total` - RTCP packets received
- `segments_muxed_total` - Segments muxed
- `s3_uploads_total` - S3 uploads completed
- `rtsp_sessions_active` - Active RTSP sessions
- `webrtc_offers_created_total` - WebRTC offers created

## Architecture

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│   RTSP      │────▶│   RTP        │────▶│   fMP4      │
│   Session   │     │   Jitter     │     │   Muxer     │
│   Manager   │     │   Buffer     │     │             │
└─────────────┘     └──────────────┘     └──────┬──────┘
                                                │
┌─────────────┐     ┌──────────────┐           ▼
│   WebRTC    │────▶│   RTCP       │     ┌─────────────┐
│   Signaling │     │   Handler    │     │   S3        │
│             │     │              │     │   Uploader  │
└─────────────┘     └──────────────┘     └─────────────┘
```

## License

MIT
