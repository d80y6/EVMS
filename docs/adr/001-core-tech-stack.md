# ADR 001: Core Technology Stack

## Status
Proposed

## Context
We need to select a technology stack for DAM VMS that ensures high performance, scalability, maintainability, and enterprise-readiness.

## Decision

### 1. Data Plane (Streaming, Recording, Ingestion)
- **Language:** Go
- **Reasoning:** Go provides excellent concurrency primitives (goroutines), high performance, and a mature ecosystem for networking and media handling (Pion WebRTC, etc.).
- **Media Framework:** GStreamer (via bindings) or FFmpeg. GStreamer is preferred for complex, dynamic pipelines.

### 2. Control Plane (Microservices)
- **Language:** Go
- **Reasoning:** Consistency with the data plane, excellent gRPC/REST support, and small binary sizes for edge deployment.

### 3. AI Worker Nodes
- **Language:** Python / C++
- **Reasoning:** Python for ease of integration with AI frameworks (PyTorch, ONNX). C++ for high-performance TensorRT/OpenVINO inference wrappers.

### 4. Frontend
- **Framework:** React with TypeScript
- **Styling:** Tailwind CSS + shadcn/ui
- **State Management:** TanStack Query + Zustand

### 5. Databases
- **Relational/Metadata:** PostgreSQL
- **Time-Series:** TimescaleDB (extension of PostgreSQL)
- **Caching/PubSub:** Redis
- **Search/Vector:** OpenSearch or pgvector

### 6. Messaging / Event Bus
- **Choice:** NATS JetStream
- **Reasoning:** Extremely lightweight, high performance, and supports both pub/sub and request/reply patterns. Native support for edge-to-cloud clustering.

### 7. Orchestration
- **Choice:** Kubernetes
- **Reasoning:** De facto standard for distributed systems.

## Consequences
- Requires expertise in Go and GStreamer.
- Management of multiple specialized databases.
- Strong focus on gRPC for inter-service communication.
