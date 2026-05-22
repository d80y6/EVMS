# ADR 003: AI Pipeline Architecture

## Status
Proposed

## Context
AI processing is resource-intensive and needs to be decoupled from the primary streaming/recording path.

## Decision
1. **Asynchronous Processing:** The Stream Ingest Service will publish low-resolution frames (via shared memory or NATS) to a dedicated AI Worker pool.
2. **Worker Pool:** AI Workers are independent services (containers) that subscribe to frames, perform inference, and publish metadata results.
3. **Hardware Acceleration:** Standardize on **ONNX Runtime** to support multiple backends (CUDA, TensorRT, OpenVINO, DirectML).
4. **Metadata Store:** AI results (bounding boxes, classifications, embeddings) are stored in **PostgreSQL** with **pgvector** for similarity search.

## Consequences
- Added latency between frame capture and AI result (typically <100ms).
- Increased network bandwidth if frames are sent over the network (mitigated by local processing or low-res streams).
