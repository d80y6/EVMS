# Timeline Service

Time synchronization and multi-stream alignment service for synchronized video playback.

## Features

- Hybrid Logical Clock (HLC) implementation
- NTP-based clock synchronization
- Multi-stream alignment with cross-correlation
- Segment management and virtual segment creation
- Gap detection in recorded streams
- Prometheus metrics export

## Quick Start

```bash
cargo run
```

## API Endpoints

- `GET /health` - Health check
- `GET /metrics` - Prometheus metrics
- `GET /sync/status` - Current sync status
- `POST /sync/update` - Update clock offset
- `GET /align/plan` - Get alignment plan
- `POST /align/set_offset` - Set stream offset
- `POST /segments` - Add a segment
- `GET /segments/:stream_id` - Get segments by stream
- `POST /virtual/create` - Create virtual segment

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| TIMELINE__HTTP_HOST | 0.0.0.0 | HTTP bind address |
| TIMELINE__HTTP_PORT | 3007 | HTTP port |
| TIMELINE__NTP_SERVERS | pool.ntp.org | Comma-separated NTP servers |
| TIMELINE__SYNC_INTERVAL | 60 | Sync interval in seconds |
| TIMELINE__MAX_DRIFT_MS | 100.0 | Maximum allowed drift |
| TIMELINE__ALIGNMENT_WINDOW | 500 | Alignment window in ms |
