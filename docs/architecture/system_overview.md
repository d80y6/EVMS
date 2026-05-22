# DAM VMS: High-Level System Architecture

## 1. Overview
DAM VMS is a distributed, cloud-native Video Management System designed for extreme scalability and AI-driven intelligence.

## 2. Component Diagram

```mermaid
graph TD
    subgraph "Edge / Site"
        Camera[IP Camera] --> Ingest[Stream Ingest Service]
        Ingest --> Recorder[Recording Engine]
        Ingest --> AIWorker[AI Worker Service]
        Recorder --> Storage[Local Storage / NAS]
    end

    subgraph "Management Plane (Cloud/Data Center)"
        API[API Gateway]
        Auth[Auth/RBAC Service]
        CamMgmt[Camera Mgmt Service]
        EventProc[Event Processing Service]
        Metadata[Metadata/Search Service]
        WebRTC[WebRTC Relay Service]
    end

    subgraph "Infrastructure"
        NATS[NATS JetStream]
        DB[(PostgreSQL / Timescale)]
        Cache[(Redis)]
        Vector[(OpenSearch / pgvector)]
    end

    Ingest -- gRPC/NATS --> EventProc
    AIWorker -- NATS --> Metadata
    API --> Auth
    API --> CamMgmt
    WebRTC -- WebRTC --> Frontend[Web/Mobile UI]
    Metadata --> Vector
```

## 3. Key Services

### 3.1 Stream Ingest Service (Go)
- Handles RTSP/ONVIF ingestion.
- Performs stream health monitoring.
- Provides raw streams to the Recorder and AI Workers.

### 3.2 Recording Engine (Go)
- Manages circular buffers.
- Writes fragmented MP4/MKV to storage.
- Indexes recordings into the database.

### 3.3 AI Analytics Service (Python/Go)
- Orchestrates AI jobs.
- Manages model lifecycle.
- Processes object detection, tracking, and classification.

### 3.4 WebRTC Service (Go)
- Acts as a signaling and STUN/TURN server.
- Provides low-latency live streams to clients via Pion WebRTC.

### 3.5 Event Processing Service (Go)
- Correlates triggers from cameras and AI.
- Executes notification rules (Email, Webhook, Mobile Push).

## 4. Scalability Strategy
- **Horizontal Scaling:** All services are stateless except the Recorder (which binds to storage).
- **Sharding:** Ingest nodes are sharded by camera/site.
- **Federation:** Multiple "Management Planes" can be federated for global visibility.
