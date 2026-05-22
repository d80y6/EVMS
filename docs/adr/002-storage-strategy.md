# ADR 002: Storage Strategy

## Status
Proposed

## Context
High-performance video recording requires efficient storage management, handling high write throughput while allowing fast random access for playback.

## Decision
1. **File Format:** Use Fragmented MP4 (fMP4) for recordings. This allows playback without needing to re-index the entire file if the recording is interrupted.
2. **Indexing:** Store segment metadata (start time, duration, file path, keyframe offsets) in **TimescaleDB**.
3. **Storage Layers:**
    - **Tier 1 (Hot):** Local NVMe/SSD for circular buffers and recent recordings (0-24h).
    - **Tier 2 (Warm):** HDD / NAS for short-term retention (1-30 days).
    - **Tier 3 (Cold):** S3 / Glacier for long-term archival.
4. **Consistency:** Use a write-ahead log (WAL) for recording metadata to ensure consistency after crashes.

## Consequences
- Requires a robust background process for moving data between tiers.
- High database write load for segment indexing (mitigated by TimescaleDB).
