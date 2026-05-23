# DAM VMS: Architecture Review

## 1. Current State
The system follows a microservices architecture using Go for the data plane and Python for the AI plane. NATS acts as the central message bus.

## 2. Architectural Debt
- **Ingest Service Responsibility:** The ingest service handles too many tasks (RTSP pull, segmentation, MJPEG transcoding, H264 extraction). This should be more modular or use a dedicated media server (e.g., MediaMTX, GStreamer).
- **Event Consistency:** Events published to NATS are not persisted in a way that guarantees "exactly-once" or "at-least-once" delivery to the metadata service in case of service restarts.
- **Storage Coupling:** The recorder and playback services are tightly coupled to the local filesystem path.

## 3. Target Architecture
- Transition to GStreamer for more robust media pipelines.
- Implement NATS JetStream consumers for all critical event paths.
- Abstract storage via an S3-compatible interface for cloud-native scalability.
