# DAM VMS: Architecture Plan of Record

## Section 1: Architecture Decision Records (ADRs)

ADR-001: Service decomposition model
Status: ACCEPTED
Date: 2025-05-15
Context: The system must scale to thousands of cameras and concurrent users while maintaining low latency. A monolithic approach would create bottlenecks in media processing and AI inference. We considered a macro-services model with combined ingest/recording, but concluded it limited horizontal scalability of compute-heavy AI tasks.
Decision: We adopt a microservices architecture sharded by functional domain. Services communicate via gRPC for control plane operations and NATS JetStream for high-throughput data plane (frames, events) transport.
Consequences:
  Positive: Independent scaling of AI vs. Ingest; failure isolation (ingest crash doesn't stop playback); technology flexibility (Python for AI, Go for Data).
  Negative: Operational complexity of managing 10+ services; overhead of network serialization; eventual consistency between service states.
  Risks: Increased latency due to network hops; service discovery complexity.

ADR-002: RTSP ingest and stream distribution architecture
Status: ACCEPTED
Date: 2025-05-15
Context: High-performance ingestion is required for varied camera types. Low-latency live viewing is a core requirement (<500ms). Alternatives included using a centralized media server (e.g., Wowza or Ant Media), but custom Go-based ingest provided tighter integration with our NATS-based event bus.
Decision: Use a dedicated Ingest Service utilizing FFmpeg for robust protocol handling. Raw H264 NAL units and MJPEG frames are published to NATS subjects. WebRTC Relay service acts as a consumer for browser-based distribution.
Consequences:
  Positive: Low-latency streaming; support for thousands of RTSP sources; decoupled distribution.
  Negative: High bandwidth requirement on the internal NATS cluster; FFmpeg process management overhead.
  Risks: NATS saturation under heavy frame load; resource leakage in FFmpeg subprocesses.

ADR-003: Recording segment strategy and crash recovery
Status: ACCEPTED
Date: 2025-05-15
Context: Video recordings must be durable and playable even after ungraceful shutdowns. We evaluated continuous MP4 recording which often results in corrupt headers during crashes. HLS was also considered but fragmented MP4 provided better random access performance for our playback engine.
Decision: Implement a fragmented MP4 (fMP4) recording strategy with 60-second segments. The Recorder service indexes segments into TimescaleDB only after a successful filesystem 'close' and 'sync' operation.
Consequences:
  Positive: Recordings are playable up to the last finished fragment; standard container format; high-performance time-series queries via TimescaleDB.
  Negative: High I/O ops for small file writes; significant database record growth.
  Risks: Filesystem metadata corruption; DB indexing lag.

ADR-004: Multi-tenancy isolation model
Status: ACCEPTED
Date: 2025-05-15
Context: Enterprise customers require data isolation. We evaluated DB-per-tenant, which offers the best isolation but highest cost, and Schema-per-tenant, which complicates migrations. Shared schema with row-level security was chosen for its balance of cost and operational simplicity.
Decision: Adopt a shared-database, shared-schema model using Row-Level Security (RLS) in PostgreSQL. Every table contains a `tenant_id` column, and application-level middleware enforces this context.
Consequences:
  Positive: Lower operational cost; simplified migrations; easier cross-tenant reporting for platform admins.
  Negative: "Noisy neighbor" risk for shared resources; logical isolation bugs could lead to data leaks.
  Risks: Performance degradation as tenant count scales; accidental exclusion of `WHERE tenant_id = ?` in custom queries.

ADR-005: Auth/AuthZ model
Status: ACCEPTED
Date: 2025-05-15
Context: Secure access to video streams and management APIs is critical. We considered session-based auth and OAuth2/OIDC. JWT-based RS256 signing was selected for its statelessness and ability to be validated at the edge without constant database lookups.
Decision: Implement a centralized JWT-based Authentication service using RS256 signing. Authorization uses a Role-Based Access Control (RBAC) model (Admin, Operator, Viewer) enforced at the API Gateway and service middleware.
Consequences:
  Positive: Stateless validation across services; standard industry integration; reduced DB load for auth checks.
  Negative: Token revocation is difficult (requires blocklisting); client-side secret management.
  Risks: Key compromise; token theft.

ADR-006: Storage tiering and retention enforcement
Status: ACCEPTED
Date: 2025-05-15
Context: Storing 4K video at scale is expensive. Tiered storage is necessary for cost-efficiency. We evaluated a single-tier NAS approach but found it either too slow for ingest or too expensive for long-term storage, leading to our three-tier decision.
Decision: A 3-tier storage model: Tier 1 (Hot/NVMe) for 24h buffer; Tier 2 (Warm/HDD) for 30-day retention; Tier 3 (Cold/S3) for long-term archival. A background worker in the Recorder service manages data movement and deletion.
Consequences:
  Positive: Significant cost reduction; predictable performance for recent playback.
  Negative: Complexity in managing data movement jobs; potential latency for "cold" retrievals.
  Risks: Data loss during transfer; retention worker failure leading to disk exhaustion.

ADR-007: AI analytics pipeline
Status: ACCEPTED
Date: 2025-05-15
Context: AI processing should not impact the stability of the primary ingestion and recording path. Synchronous AI processing was rejected due to its potential to block stream ingest if the GPU was saturated. An asynchronous NATS-based pipeline provides the necessary decoupling.
Decision: An asynchronous AI pipeline where the Ingest service publishes a secondary low-resolution MJPEG stream to NATS. AI Workers subscribe to this stream, perform inference, and publish metadata results back to NATS for the Event Processor.
Consequences:
  Positive: Decoupled scaling of AI; no impact on recording if AI workers are slow; ability to run multiple models on one stream.
  Negative: Added latency between frame capture and event generation; redundant frame transport.
  Risks: High memory usage in NATS for image payloads; synchronization issues between video and metadata.

ADR-008: Kubernetes topology
Status: ACCEPTED
Date: 2025-05-15
Context: Standardizing deployment across edge and cloud environments. We considered Bare Metal and Nomad for edge deployments but standardizing on Kubernetes allowed us to leverage a unified deployment model and the mature Helm ecosystem.
Decision: Deploy all services to Kubernetes using Helm. Use Linkerd service mesh for mTLS and observability. Use StatefulSets for the Recorder service to ensure stable storage mapping.
Consequences:
  Positive: Consistent environments; automated healing; fine-grained resource control.
  Negative: Higher management overhead; networking complexity.
  Risks: Misconfigured resource limits causing OOM kills; mesh-induced latency.

ADR-009: Observability strategy
Status: ACCEPTED
Date: 2025-05-15
Context: Monitoring stream health and AI performance in a distributed environment. We evaluated self-managed ELK vs. Managed Prometheus/Grafana. We chose the Prometheus/Loki/Jaeger stack for its lower footprint and better integration with Kubernetes metrics.
Decision: Centralized observability using Prometheus for metrics, Jaeger for distributed tracing, and Loki for log aggregation. Every service exposes a `/metrics` endpoint with custom domain metrics (e.g., frame_drop_rate).
Consequences:
  Positive: Fast incident response; performance bottleneck identification; visibility into stream health.
  Negative: Storage overhead for telemetry data; instrumentation effort.
  Risks: Cardinality explosion for metrics with `camera_id` labels.

ADR-010: HA and failover model
Status: ACCEPTED
Date: 2025-05-15
Context: The VMS is a mission-critical system requiring high availability. We considered a cold-standby model but the recovery times exceeded our RTO. An Active-Active/Active-Passive hybrid approach was chosen to minimize downtime for critical ingest paths.
Decision: Implement an Active-Active model for stateless services (API, Auth) and Active-Passive for stateful/media services (Ingest, Recorder) using Kubernetes liveness/readiness probes and leader election where necessary. Target RTO is <30s and RPO is <60s (one segment).
Consequences:
  Positive: Resilient to single node/pod failures; automated recovery.
  Negative: Complexity in managing stateful failovers; potential for duplicate recording segments during transition.
  Risks: Split-brain scenarios; storage contention during failover.

---

## Section 2: Dependency-Ordered Milestone Map

MILESTONE-01: Infrastructure skeleton
Depends on: NONE
Scope:
  - All 10 microservices containerized and bootable.
  - NATS JetStream, PostgreSQL, and Redis clusters deployed and reachable.
Frozen interfaces: Internal service DNS names, DB connection schemas.
Gate condition: The infrastructure cluster reports all 10 core services as "UP" when queried via a centralized health aggregator, measurable by the CI health-check suite.
Definition of done:
  - Gate condition passes in CI
  - No regressions in dependent milestones
  - All frozen interfaces remain unchanged
  - Observability: metric 'vms_service_up' is emitted and visible in dashboard
Rollback condition: Any core infra component (NATS/DB) fails to stabilize within 5 minutes of deployment.

MILESTONE-02: RTSP ingest primitive
Depends on: MILESTONE-01
Scope:
  - Ingest service connects to a single RTSP source and publishes frames to NATS.
Frozen interfaces: NATS subject naming convention (camera.<id>.frames).
Gate condition: The ingest service connects to a valid RTSP source within 3s and begins publishing frames when a stream is started, measurable by a NATS subscription benchmark tool reporting >20 FPS.
Definition of done:
  - Gate condition passes in CI
  - No regressions in dependent milestones
  - All frozen interfaces remain unchanged
  - Observability: metric 'vms_frames_processed_total' is emitted and visible in dashboard
Rollback condition: FFmpeg process crashes repeatedly or fails to connect to simulator.

MILESTONE-03: Recording primitive
Depends on: MILESTONE-02
Scope:
  - Recorder service consumes NATS signals and writes fMP4 segments to local disk.
Frozen interfaces: Recording segment filesystem path structure.
Gate condition: The recorder service writes a valid 60-second fMP4 segment to the designated storage path when a stream is active, measurable by the existence of a new DB record in the 'recordings' table matching the file metadata.
Definition of done:
  - Gate condition passes in CI
  - No regressions in dependent milestones
  - All frozen interfaces remain unchanged
  - Observability: metric 'vms_recordings_indexed_total' is emitted and visible in dashboard
Rollback condition: Corrupt MP4 files or missing DB records for written files.

MILESTONE-04: Stream distribution
Depends on: MILESTONE-02
Scope:
  - WebRTC service establishes a peer connection and relays H264 data from NATS to a client.
Frozen interfaces: WebRTC signaling protocol (SDP exchange format).
Gate condition: The WebRTC service establishes a peer connection and relays video data when a client requests a session, measurable by a Playwright test confirming a frame change on the client within 500ms of ingest.
Definition of done:
  - Gate condition passes in CI
  - No regressions in dependent milestones
  - All frozen interfaces remain unchanged
  - Observability: metric 'vms_webrtc_sessions_active' is emitted and visible in dashboard
Rollback condition: Peer connection failure rate > 10%.

MILESTONE-05: Multi-tenancy skeleton
Depends on: MILESTONE-01
Scope:
  - Database RLS policies active and 'tenant_id' context propagated through API.
Frozen interfaces: Tenant-aware API request headers.
Gate condition: The API service rejects requests for resources belonging to Tenant B when authenticated as Tenant A, measurable by an automated security test suite returning 403 Forbidden for cross-tenant access.
Definition of done:
  - Gate condition passes in CI
  - No regressions in dependent milestones
  - All frozen interfaces remain unchanged
  - Observability: metric 'vms_unauthorized_access_attempts' is emitted
Rollback condition: Any test case successfully leaks data between tenants.

MILESTONE-06: Auth primitive
Depends on: MILESTONE-05
Scope:
  - Auth service issues JWTs; services validate tokens for all protected routes.
Frozen interfaces: JWT claim structure.
Gate condition: The system rejects unauthenticated requests for protected video and management endpoints, measurable by the API Gateway returning 401 Unauthorized for all requests without a valid Bearer token.
Definition of done:
  - Gate condition passes in CI
  - No regressions in dependent milestones
  - All frozen interfaces remain unchanged
  - Observability: metric 'vms_auth_failures_total' is emitted
Rollback condition: Token validation latency > 50ms.

MILESTONE-07: Storage tiering
Depends on: MILESTONE-03
Scope:
  - Retention worker moves/deletes files based on configured policy.
Frozen interfaces: Retention policy JSON schema.
Gate condition: The retention worker removes expired video segments from the filesystem when the retention period is reached, measurable by a disk usage check and DB query for segments older than the policy threshold.
Definition of done:
  - Gate condition passes in CI
  - No regressions in dependent milestones
  - All frozen interfaces remain unchanged
  - Observability: metric 'vms_retention_deleted_bytes' is emitted
Rollback condition: Retention worker deletes files that are within the retention period.

MILESTONE-08: AI analytics pipeline
Depends on: MILESTONE-02
Scope:
  - AI Worker detects objects and publishes results to 'ai_events' table.
Frozen interfaces: AI event JSON schema.
Gate condition: The AI worker identifies objects in a synthetic video stream when frames are published to NATS, measurable by the insertion of detected object metadata into the 'ai_events' table within 2s of the frame timestamp.
Definition of done:
  - Gate condition passes in CI
  - No regressions in dependent milestones
  - All frozen interfaces remain unchanged
  - Observability: metric 'vms_inference_duration_seconds' is emitted
Rollback condition: AI worker CPU/GPU utilization exceeds 90% for a single stream.

MILESTONE-09: Observability baseline
Depends on: MILESTONE-01
Scope:
  - Prometheus, Loki, and Grafana dashboards are populated with data from all services.
Frozen interfaces: Common metric label names.
Gate condition: The observability stack displays real-time telemetry from all 10 services when queried via the Grafana API, measurable by the 'data_freshness' metric remaining above 99% for all configured dashboards.
Definition of done:
  - Gate condition passes in CI
  - No regressions in dependent milestones
  - All frozen interfaces remain unchanged
  - Observability: All dashboards show 100% data freshness
Rollback condition: Metrics collection latency > 30s.

MILESTONE-10: HA primitive
Depends on: MILESTONE-03
Scope:
  - Automated recovery of Ingest and Recorder services on node failure.
Frozen interfaces: Liveness/Readiness probe configurations.
Gate condition: The Kubernetes scheduler restores ingest service availability when a pod is terminated, measurable by a "failover latency" metric reporting <30s from pod death to stream resumption.
Definition of done:
  - Gate condition passes in CI
  - No regressions in dependent milestones
  - All frozen interfaces remain unchanged
  - Observability: metric 'vms_failover_duration_seconds' is emitted
Rollback condition: Failover takes > 60s or results in persistent data corruption.

MILESTONE-11: End-to-end integration
Depends on: MILESTONE-02, MILESTONE-03, MILESTONE-04, MILESTONE-05, MILESTONE-06, MILESTONE-07, MILESTONE-08, MILESTONE-09, MILESTONE-10
Scope:
  - Full system operational from camera ingest to frontend viewing and AI alerts.
Frozen interfaces: All previously frozen interfaces.
Gate condition: A user successfully completes the "Live to Archive" journey when the system is under nominal load, measurable by a Playwright E2E suite verifying login, stream view, and playback functionality in a single pass.
Definition of done:
  - Gate condition passes in CI
  - No regressions in dependent milestones
  - All frozen interfaces remain unchanged
  - Observability: All system metrics show healthy green state
Rollback condition: Failure of any core user journey (Live/Playback/AI).

MILESTONE-12: Load baseline
Depends on: MILESTONE-11
Scope:
  - System performance verified under target load (e.g., 100 cameras).
Frozen interfaces: Resource allocation limits.
Gate condition: The system maintains target performance when handling 100 concurrent 1080p streams, measurable by the 'frame_loss_percentage' remaining below 10% across the entire cluster for 1 hour.
Definition of done:
  - Gate condition passes in CI
  - No regressions in dependent milestones
  - All frozen interfaces remain unchanged
  - Observability: Dashboard shows performance within SLA limits
Rollback condition: System-wide crash or latency exceeding SLA by 2x.

---

## Section 3: Frozen Decision Registry

FROZEN: Go 1.24+ as primary language for Data Plane
Rationale: Concurrency primitives and performance required for media handling.
Superseded by: NONE

FROZEN: NATS JetStream as the internal messaging backbone
Rationale: Low-latency pub/sub with persistence for events and frames.
Superseded by: NONE

FROZEN: Fragmented MP4 (fMP4) for all video recordings
Rationale: Ensures playability after crashes and compatibility with Web browsers.
Superseded by: NONE

FROZEN: Asynchronous AI frame processing via secondary low-res stream
Rationale: Prevents AI load from impacting recording stability.
Superseded by: NONE

FROZEN: TimescaleDB for metadata and event storage
Rationale: Optimized for time-series queries and scalable indexing.
Superseded by: NONE

---

## Section 4: Warning Flags (Self-Audit)

### Gate Condition Audit

| Milestone | Gate Automatable? | Blocking Risk | What's Missing |
| :--- | :--- | :---: | :--- |
| **M-01** | YES | High | CI health-check script to aggregate `docker-compose` health statuses. |
| **M-02** | PARTIAL | **Critical** | RTSP Simulator (e.g., MediaMTX); NATS sub benchmark tool. |
| **M-03** | PARTIAL | High | Integration test script to verify DB record matching specific disk file hash. |
| **M-04** | PARTIAL | Medium | Playwright-based frontend visual regression/latency script. |
| **M-05** | NO | Medium | Implementation of PostgreSQL RLS policies; automated security test suite. |
| **M-06** | PARTIAL | Medium | Automated auth test suite for 401/403 validation. |
| **M-07** | PARTIAL | Low | Scripted file age simulator (moving system clock or `touch`ing files back in time). |
| **M-08** | PARTIAL | Medium | E2E harness with synthetic video for deterministic object detection. |
| **M-09** | PARTIAL | Low | JSON dashboard definitions; Grafana API validation script. |
| **M-10** | NO | High | Production-grade K8s manifests; Chaos mesh or custom 'pod kill' script. |
| **M-11** | NO | Medium | Full Playwright E2E scenario suite. |
| **M-12** | NO | Low | k6 load scripts for RTSP/WebRTC/NATS; high-scale load simulator. |

### Audit Questions

1. **Which milestones have gate conditions that cannot currently be automated? List them and explain why.**
   - MILESTONE-05 (Multi-tenancy): Missing the actual RLS implementation in the DB schema to test against.
   - MILESTONE-10 (HA): Missing the Kubernetes infrastructure and manifests required to run pod-level failover tests.
   - MILESTONE-11 (E2E) & MILESTONE-12 (Load): These rely on the completion of all preceding manual/partial gates and require complex orchestration not present in the current environment.

2. **Which ADRs have decisions that are underspecified — where a reasonable engineer could make two different implementation choices and both would satisfy the ADR?**
   - ADR-010 (HA and failover model): Does not specify the *mechanism* for leader election (e.g., NATS KV vs. K8s Leases vs. custom sidecar). Both would satisfy the "Active-Passive" requirement but have different operational profiles.
   - ADR-008 (Kubernetes topology): Does not specify the CNI or storage provisioner requirements, which are critical for the Recorder service's performance.

3. **Which dependencies between milestones create the highest risk of blocking the entire project?**
   - **MILESTONE-02 (RTSP Ingest):** As the primary data producer for Recording (M-03), Streaming (M-04), and AI (M-08), it is a single point of failure for the entire functional roadmap.

4. **What is the earliest gate condition that proves the multi-tenancy isolation model works correctly?**
   - MILESTONE-05: Multi-tenancy skeleton. This is the first gate that explicitly tests cross-tenant data leakage.

5. **List any component from the codebase that does NOT map cleanly to any milestone — these are prototype accumulations that need explicit triage decisions.**

### Prototype Triage Registry

| Component | Triage Decision | Targeted Milestone Gate (for PROMOTE) |
| :--- | :--- | :--- |
| `services/camera-mgmt` | **PROMOTE** | Satisfies MILESTONE-01 (Infrastructure skeleton) via its health endpoint and MILESTONE-05 (Multi-tenancy skeleton) by acting as the primary tenant-aware resource gateway. |
| `services/metadata` | **PROMOTE** | Satisfies MILESTONE-08 (AI analytics pipeline) gate by persisting AI detections and embeddings to the database for verification. |
| `services/playback` | **PROMOTE** | Satisfies MILESTONE-11 (End-to-end integration) gate by providing the HTTP interface for the "archive playback" user journey. |
| `services/event-proc` | **REWRITE** | Logic is too brittle (hardcoded confidence/labels). Requires a rewrite to support dynamic rules before it can satisfy MILESTONE-11. |
| `services/notification` | **DEFER** | Logic is currently just logging. Post-MVP feature that belongs in a future "Enterprise Alerting" milestone. |
| `pkg/common/retry.go` | **PROMOTE** | Satisfies MILESTONE-01 (Infrastructure skeleton) by enabling services to survive and recover during sequential cluster startup. |
