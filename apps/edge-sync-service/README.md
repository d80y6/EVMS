# Edge Sync Service

Offline-first synchronization service with conflict resolution for edge devices.

## Features

- **Offline-First Architecture**: Queue operations when disconnected, sync when reconnected
- **Vector Clocks**: Track causality across distributed nodes
- **CRDT Support**: Last-Writer-Wins registers, G-Counters, PN-Counters, OR-Sets, LWW-Maps
- **Conflict Resolution**: Pluggable strategies (LWW, merge, custom resolvers)
- **Multiple Storage Backends**: RocksDB (production), in-memory (testing)
- **gRPC API**: Efficient streaming synchronization
- **REST API**: Health checks, metrics, management endpoints
- **Metrics**: Prometheus-compatible metrics export

## Architecture

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│   Sync Engine   │────▶│  Offline Queue   │────▶│  Conflict Res.  │
└────────┬────────┘     └──────────────────┘     └────────┬────────┘
         │                                                │
         ▼                                                ▼
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│  Vector Clock   │     │  Storage Backend │     │  Replication    │
└─────────────────┘     └──────────────────┘     └─────────────────┘
```

## Quick Start

### Build

```bash
cd apps/edge-sync-service
cargo build --release
```

### Run

```bash
export EDGE_SYNC__DEVICE_ID=edge-device-1
export EDGE_SYNC__STORAGE_PATH=./data
export EDGE_SYNC__GRPC_PORT=50051
export EDGE_SYNC__HTTP_PORT=8080
cargo run
```

### Configuration

| Environment Variable | Description | Default |
|---------------------|-------------|---------|
| `EDGE_SYNC__DEVICE_ID` | Unique device identifier | Auto-generated |
| `EDGE_SYNC__STORAGE_PATH` | Path to storage backend | `./data/edge-sync` |
| `EDGE_SYNC__GRPC_PORT` | gRPC server port | `50051` |
| `EDGE_SYNC__HTTP_PORT` | HTTP server port | `8080` |
| `EDGE_SYNC__BIND_ADDRESS` | Bind address | `0.0.0.0` |
| `EDGE_SYNC__SYNC_INTERVAL_SECS` | Sync interval | `30` |
| `EDGE_SYNC__BATCH_SIZE` | Batch size for sync | `100` |
| `EDGE_SYNC__MAX_QUEUE_SIZE` | Max offline queue size | `10000` |
| `EDGE_SYNC__CONFLICT_STRATEGY` | Conflict strategy | `last_write_wins` |

### API Endpoints

#### REST

- `GET /health` - Health check
- `GET /metrics` - Prometheus metrics
- `GET /status` - Service status
- `POST /sync` - Trigger manual sync
- `GET /queue` - Queue status
- `POST /queue/purge` - Purge completed items
- `GET /peers` - List known peers
- `POST /peers` - Register peer
- `GET /conflicts` - List pending conflicts
- `POST /conflicts/:id/resolve` - Resolve conflict
- `GET /replication/stats` - Replication statistics

#### gRPC

- `Sync` - Batch synchronization
- `SyncStream` - Streaming synchronization
- `GetQueueState` - Get offline queue state
- `ResolveConflict` - Resolve a conflict

## CRDT Types

### Last-Writer-Wins Register

```rust
use edge_sync::crdt::LWWRegister;

let mut reg = LWWRegister::new(42, timestamp, "device1".to_string());
reg.set(100, later_timestamp, "device2".to_string());
assert_eq!(reg.get(), Some(&100));
```

### G-Counter (Grow-only Counter)

```rust
use edge_sync::crdt::GCounter;

let mut counter = GCounter::new();
counter.increment("node1", 5);
counter.increment("node2", 3);
assert_eq!(counter.value(), 8);
```

### OR-Set (Observed-Remove Set)

```rust
use edge_sync::crdt::ORSet;

let mut set = ORSet::new();
set.add("item", "device1".to_string(), timestamp);
assert!(set.contains(&"item"));
```

## Testing

```bash
cargo test
```

## License

MIT
