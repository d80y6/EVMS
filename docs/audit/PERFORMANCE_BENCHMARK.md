# DAM VMS: Performance Benchmark & Scalability Analysis

## 1. Benchmarking (Predicted/Theoretical)

### 1.1 Ingest Density
- **Current:** ~10-15 HD streams per node (CPU bound due to FFmpeg MJPEG transcoding).
- **Target:** 50+ HD streams per node using GPU-accelerated transcoding (NVENC).

### 1.2 AI Throughput
- **Current:** 100% frame processing. Latency increases linearly with camera count.
- **Bottleneck:** CPU/GPU saturation on the AI worker node.
- **Mitigation:** Implement frame sampling and batch inference.

## 2. Scalability Limits
- **NATS:** Can handle thousands of cameras, but subject wildcards must be optimized.
- **PostgreSQL:** TimescaleDB handles metadata well, but the `recordings` table will require aggressive partitioning and indexing optimization as it grows to billions of rows.
- **Frontend:** React rendering of 16+ simultaneous WebRTC streams will hit browser hardware acceleration limits.
