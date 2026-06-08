# EVMS Domain Audit Report

## Audit Methodology
Each domain assessed as: **Exists** (fully implemented), **Partial** (partially implemented), **Missing** (not implemented)

---

## CORE VMS DOMAINS

### CORE-01: Camera Management
**Status: Exists** | Service: `camera-mgmt` (gRPC :50051)
- Full CRUD via gRPC CameraService API
- Site grouping, camera status reporting
- Smart search capability
- Frontend: CamerasPage, CameraCard components
- Gateway routes: `/api/cameras`, `/api/sites`
- **Gap:** No bulk operations, no firmware management, no camera health dashboard

### CORE-02: Camera Discovery
**Status: Exists** | Service: `discovery` (:8091)
- ONVIF WS-Discovery probe
- Subnet scanning
- Discovery results stored in DB (migration 010)
- Frontend: DiscoveryPage
- Gateway: `/api/discovery/*`
- **Gap:** No automatic scheduling, no credential testing at discovery time, no discovery profiles

### CORE-03: ONVIF Protocol
**Status: Exists** | Package: `pkg/onvif/`
- Full ONVIF client: Device, Media, PTZ, Analytics, Events, Imaging, Recording, Auth, Provisioning
- SOAP client with auth support
- Tests for all subpackages
- **Gap:** ONVIF Profile G (edge storage), Profile M (metadata), Profile T (advanced)

### CORE-04: Stream Ingestion
**Status: Exists** | Services: `ingest`, Rust `apps/ingest-service`
- Go RTSP ingestion via FFmpeg
- Rust high-performance RTSP/RTP engine
- Segment creation, NATS frame publishing
- Tests exist for Go service
- **Gap:** No MPEG-DASH/HLS ingestion, no multicast support, no adaptive bitrate ingestion

### CORE-05: Media Pipeline
**Status: Partial** | Rust `apps/media-processing-service`
- GStreamer-based processing pipeline (Rust)
- FFmpeg thumbnail generation
- Dewarping in recorder service
- **Gap:** No transcoding pipeline, no video analytics pipeline integration, no GPU acceleration pipeline

### CORE-06: WebRTC Streaming
**Status: Exists** | Service: `webrtc` (:8082)
- Pion WebRTC implementation
- STUN/TURN support
- NATS frame subscription
- Frontend: CameraView, PtzOverlay, useStreamSelector hook
- Gateway: `/api/webrtc/*`
- Tests exist
- **Gap:** No simulcast, no SVC support, no adaptive quality

### CORE-07: Recording Engine
**Status: Exists** | Service: `recorder` (:8087)
- Ingestion, indexing, retention
- Legal holds, bookmarks
- Tiered storage (hot/warm/cold)
- Leader election (NATS KV-based)
- Dewarping support
- Gateway: `/api/recordings`, `/api/bookmarks`, `/api/legal-holds`, `/api/storage/*`, `/api/dewarp`
- Tests exist
- **Gap:** No edge recording management, no continuous vs. motion-only recording scheduling

### CORE-08: Playback Engine
**Status: Exists** | Service: `playback` (:8086)
- Static file serving of recordings
- Range request support for seeking
- Frontend: SyncPlaybackView, TimelineScrubber
- Gateway: `/api/playback/*`
- Tests exist (including security tests)
- **Gap:** No multi-camera sync playback, no speed control, no smart search during playback

### CORE-09: Timeline Service
**Status: Missing** | Rust `apps/timeline-service` exists but is incomplete
- Basic Rust service defined
- **Gap:** No timeline aggregation, no event overlay on timeline, no bookmark visualization

### CORE-10: Storage Management
**Status: Partial** | Service: `recorder` storage tiering
- Hot/Warm/Cold tiering support
- Storage metrics in Prometheus
- Frontend: StoragePage
- Gateway: `/api/storage/*`
- **Gap:** No storage forecasting, no quota management, no per-camera retention policies, no cloud tier integration

### CORE-11: PTZ Control
**Status: Exists** | Service: `camera-control` (:8088)
- Full PTZ control (20+ protocols)
- ONVIF PTZ, presets, IO ports
- Frontend: PtzOverlay component
- Gateway: `/api/cameras/{id}/ptz/*`, `/api/cameras/{id}/io`
- **Gap:** No PTZ patrol patterns, no PTZ scheduling, no PTZ tour interruption handling

### CORE-12: Thumbnails
**Status: Exists** | Service: `thumbnails` (:8089)
- FFmpeg screenshot extraction
- Caching support
- Gateway: `/api/thumbnails/*`
- **Gap:** No configurable thumbnail intervals, no AI-powered thumbnail selection

### CORE-13: Export Engine
**Status: Exists** | Service: `export` (:8094)
- FFmpeg concatenation
- Watermarking support
- SHA256 checksums
- Frontend: ExportPage
- Gateway: `/api/export`
- **Gap:** No multi-format export, no trim UI, no bulk export, no evidence export with metadata

### CORE-14: Bookmarking
**Status: Exists** | Service: `recorder/bookmarks.go`
- CRUD operations
- Frontend: BookmarksPage
- Gateway: `/api/bookmarks`
- **Gap:** No bookmark sharing, no bookmark categories, no bookmark notes/annotations

### CORE-15: Dewarping/Fisheye
**Status: Exists** | Service: `recorder` (dewarp)
- FFmpeg lens correction
- Gateway: `/api/dewarp`
- **Gap:** No real-time dewarping, no multiple dewarping modes (360, dual, quad)

### CORE-16: Frame Analysis/Scrub
**Status: Missing**
- **Gap:** No frame-by-frame analysis, no timeline scrubbing at frame level, no smart frame indexing

### CORE-17: Multi-Stream Support
**Status: Partial**
- WebRTC streaming exists
- **Gap:** No profile-based stream selection (main/sub/third), no adaptive bitrate streaming

### CORE-18: Audio Management
**Status: Missing**
- **Gap:** No audio recording/playback, no audio level monitoring, no two-way audio, no audio codec management

### CORE-19: Camera Provisioning
**Status: Partial** | Package: `pkg/onvif/provision.go`
- ONVIF provisioning support
- Migration 003 for ONVIF credentials
- **Gap:** No bulk provisioning, no provisioning templates, no automatic configuration upon discovery

### CORE-20: Retention Management
**Status: Partial** | Service: `recorder` tiering
- Retention policies exist in tiering
- **Gap:** No per-camera retention policies, no event-based retention, no retention templates

---

## AI PLATFORM DOMAINS

### AI-01: Object Detection
**Status: Exists** | Service: `ai-worker` (Python)
- YOLOv8 inference
- Triton gRPC client
- Rust Triton inference proxy
- **Gap:** No custom model support, no model retraining pipeline

### AI-02: Facial Recognition
**Status: Exists** | Service: `ai-worker` (Python) + Migration 005
- DeepStack integration
- Face watchlist tables
- Face detections hypertable
- **Gap:** No face matching confidence tuning, no watchlist import/export

### AI-03: License Plate Recognition
**Status: Exists** | Service: `ai-worker/lpr.go`
- LPR implementation
- Go-based LPR tests exist
- **Gap:** No LPR watchlist, no LPR alerting, no ANPR configuration

### AI-04: People Counting
**Status: Exists** | Service: `event-proc/counter.go`
- Zone crossing counting, aggregation
- People counters hypertable (migration 002)
- **Gap:** No real-time occupancy dashboard, no hourly/daily/weekly reporting

### AI-05: Heatmap Generation
**Status: Exists** | Service: `event-proc/heatmap.go`
- Grid-based crowd heatmaps
- Crowd heatmaps hypertable
- **Gap:** No temporal heatmap comparison, no interactive heatmap in frontend

### AI-06: Tripwire Detection
**Status: Exists** | Service: `event-proc/tripwire.go`
- Line crossing detection
- Tests exist
- **Gap:** No zone-based tripwires, no bidirectional counting

### AI-07: Intrusion Detection
**Status: Missing**
- **Gap:** No virtual perimeter detection, no zone intrusion, no stay-out/stay-in zones

### AI-08: Object Tracking
**Status: Partial** | Service: `event-proc`
- IoU-based tracking in event-proc
- **Gap:** No ReID (re-identification) across cameras, no long-term trajectory tracking

### AI-09: Metadata Management
**Status: Exists** | Service: `metadata` (:8089)
- AI event metadata storage
- Vector embedding storage (pgvector)
- Tests exist
- **Gap:** No metadata schema versioning, no metadata lifecycle management

### AI-10: Vector Search
**Status: Partial**
- pgvector embeddings stored
- **Gap:** No similarity search UI, no natural language search, no image-based search

### AI-11: Facial Watchlist
**Status: Exists** (Partial) | Migration 005
- Face watchlist table exists
- **Gap:** No watchlist import/export, no watchlist sharing, no alert on watchlist match UI

### AI-12: Loitering Detection
**Status: Missing**
- **Gap:** No dwell time detection, no loitering zone configuration

### AI-13: Abandoned Object
**Status: Missing**
- **Gap:** No unattended object detection, no object removal detection

### AI-14: Crowd Detection
**Status: Missing**
- **Gap:** No crowd density estimation, no crowd formation detection

### AI-15: Tailgating Detection
**Status: Missing**
- **Gap:** No tailgating logic, no paired entry/exit tracking

### AI-16: Scene Change Detection
**Status: Missing**
- **Gap:** No scene comparison, no camera tamper detection via AI

### AI-17: Audio Detection
**Status: Missing**
- **Gap:** No gunshot detection, no glass break detection, no scream detection

### AI-18: Predictive Analytics
**Status: Missing**
- **Gap:** No anomaly prediction, no behavior prediction

### AI-19: Forensics Search
**Status: Missing**
- **Gap:** No forensic search UI, no combined attribute search (clothing, color, direction)

### AI-20: AI Model Management
**Status: Missing**
- **Gap:** No model registry, no model versioning, no A/B testing of models

---

## SECURITY DOMAINS

### SEC-01: Authentication
**Status: Exists**
- JWT generation/validation
- Local user auth
- LDAP/AD integration
- Rate limiting on login
- **Gap:** No MFA, no SSO/SAML, no OIDC

### SEC-02: Authorization/RBAC
**Status: Exists**
- Roles: viewer, operator, admin
- JWT role extraction in middleware
- Route-level role enforcement in gateway
- **Gap:** No fine-grained permissions, no resource-level ACLs, no custom roles

### SEC-03: Multi-Tenancy
**Status: Exists**
- Tenant isolation via tenant_id column
- JWT tenant context propagation
- Tenants table with CRUD
- **Gap:** No tenant-level resource quotas, no tenant self-service portal

### SEC-04: Audit Logging
**Status: Exists** | Service: `audit` (:8093)
- Tamper-evident hash chain
- NATS event subscription
- All audit_logs columns (actor, resource, previous_hash, hash)
- Gateway: `/api/audit/*`
- Frontend: AuditPage
- **Gap:** No audit report generation, no audit retention policies

### SEC-05: Encryption at Rest
**Status: Partial**
- AES-256-GCM for ONVIF credentials (pkg/common/crypto.go)
- Encryption migration (009)
- **Gap:** No database-level TDE, no file-level recording encryption

### SEC-06: Encryption in Transit
**Status: Exists**
- TLS for gRPC (pkg/common/grpc_tls.go)
- HTTPS support via ingress
- **Gap:** No mTLS enforcement between services

### SEC-07: Secrets Management
**Status: Partial**
- External Secrets Operator support
- Helm secrets template
- **Gap:** No vault integration, no automatic secret rotation

### SEC-08: Password Policies
**Status: Missing**
- **Gap:** No password complexity rules, no password expiry, no history enforcement

### SEC-09: MFA/2FA
**Status: Missing**
- **Gap:** No TOTP, no SMS 2FA, no hardware key support

### SEC-10: SSO/SAML/OIDC
**Status: Missing**
- **Gap:** No SAML IdP integration, no OIDC/OAuth2 provider support

### SEC-11: LDAP/AD Integration
**Status: Exists**
- LDAP authentication in auth service
- OpenLDAP in docker-compose
- **Gap:** No LDAP synchronization, no group mapping

### SEC-12: API Key Management
**Status: Missing**
- **Gap:** No API key generation, no key rotation, no key-scoped access

### SEC-13: IP Allowlisting
**Status: Missing**
- **Gap:** No IP-based access control for admin endpoints

### SEC-14: Session Management
**Status: Exists**
- JWT-based sessions
- **Gap:** No session revocation, no refresh token rotation, no concurrent session limits

### SEC-15: Rate Limiting
**Status: Exists**
- Rate limiting on `/api/login`
- **Gap:** No per-user rate limiting, no per-tenant rate limiting

### SEC-16: CSRF Protection
**Status: Missing**
- **Gap:** No CSRF tokens, no SameSite cookie enforcement

### SEC-17: FIPS Compliance
**Status: Partial**
- FIPS builder Dockerfile exists
- **Gap:** No FIPS-validated crypto across all services

### SEC-18: Video Watermarking
**Status: Partial**
- Export watermarking exists
- **Gap:** No real-time watermarking on live streams, no forensic watermarking

### SEC-19: Chain of Custody
**Status: Partial**
- Audit hash chain provides tamper evidence
- **Gap:** No formal chain-of-custody documentation, no evidence handoff tracking

---

## OPERATIONS DOMAINS

### OPS-01: Event Management
**Status: Exists** | Service: `event-proc` (:8093)
- AI event processing pipeline
- Nested event tracking
- Gateway: `/api/events`
- Frontend: EventsPage
- **Gap:** No event correlation, no event suppression, no event lifecycle management

### OPS-02: Notification System
**Status: Exists** | Service: `notification` (:8090)
- Webhook management & dispatch
- NATS event dispatch
- Push notifications via NATS
- Gateway: `/api/webhooks/*`
- Frontend: WebhooksPage
- Tests exist
- **Gap:** No email notifications, no SMS notifications, no push notification to mobile

### OPS-03: Alert Rules Engine
**Status: Exists** | Service: `event-proc/rule_engine.go`
- Rule evaluation engine
- Alert workflows
- Gateway: `/api/alerts`, `/api/rules`
- Frontend: AlertsPage
- Tests exist (rule_engine_test.go)
- **Gap:** No alert escalation chains, no alert suppression rules, no SLA on alerts

### OPS-04: Observability
**Status: Exists**
- OpenTelemetry OTLP tracing
- 50+ Prometheus metrics
- JSON structured logging (slog)
- **Gap:** No unified observability dashboard, no service dependency graph

### OPS-05: Monitoring/Metrics
**Status: Exists**
- Prometheus metrics across all services
- Grafana dashboards (in docker-compose)
- Loki log aggregation
- Promtail log shipping
- **Gap:** No custom Grafana dashboards checked into repo, no anomaly detection on metrics

### OPS-06: Distributed Tracing
**Status: Exists**
- OpenTelemetry collector in docker-compose
- OTLP exporter in pkg/common/telemetry.go
- **Gap:** No trace sampling configuration, no span tagging standards

### OPS-07: Health Checks
**Status: Exists** | Package: `pkg/common/health.go`
- DB health checker
- NATS health checker
- HTTP health endpoints (`/api/health`, `/api/ready`)
- Health frontend page
- **Gap:** No detailed health reporting, no health history

### OPS-08: Backup & Recovery
**Status: Exists**
- Backup CronJob
- Restore Job
- WAL archiving configured
- PVC for persistent data
- **Gap:** No backup verification, no point-in-time recovery tested, no backup retention policies

### OPS-09: Audit Trail Review
**Status: Exists**
- Audit service with hash chain verification
- Frontend: AuditPage
- Gateway: `/api/audit/log`, `/api/audit/chain`, `/api/audit/verify`
- **Gap:** No audit trail filtering, no audit export

### OPS-10: System Logging
**Status: Exists**
- JSON structured logging (slog)
- Loki aggregation
- **Gap:** No log retention policies, no log level configuration per service

### OPS-11: Webhook System
**Status: Exists**
- Webhook CRUD in notification service
- Event dispatch via NATS
- Frontend: WebhooksPage
- **Gap:** No webhook retry policies, no webhook health monitoring, no webhook secret signing

### OPS-12: Email/SMS/Push
**Status: Missing**
- **Gap:** No email gateway integration, no SMS provider (Twilio), no mobile push

### OPS-13: Incident Response
**Status: Missing**
- **Gap:** No incident creation workflow, no incident assignment, no incident timeline

### OPS-14: SLA Management
**Status: Missing**
- **Gap:** No SLA tracking, no uptime monitoring, no SLA breach notification

### OPS-15: System Configuration Management
**Status: Missing**
- **Gap:** No config versioning, no config audit, no config rollback

---

## DISTRIBUTED SYSTEMS DOMAINS

### DIST-01: Federation
**Status: Missing**
- **Gap:** No multi-site federation, no cross-site recording search, no federated user management

### DIST-02: Edge Nodes
**Status: Partial** | Rust `apps/edge-sync-service`
- CRDT-based conflict resolution
- Sled/RocksDB local storage
- Parquet export, SQLx queries
- **Gap:** No edge node registration, no edge health monitoring, no edge software update

### DIST-03: Cluster Coordination
**Status: Partial**
- NATS-based leader election (recorder)
- Sharding configuration
- **Gap:** No full cluster membership, no distributed consensus, no node state management

### DIST-04: High Availability
**Status: Partial**
- K8s deployment (replicas)
- Health check probes
- PDB (Pod Disruption Budget)
- HPA (Horizontal Pod Autoscaler)
- **Gap:** No stateful HA for database, no multi-region HA

### DIST-05: Failover
**Status: Partial**
- Leader election for recorder
- NATS JetStream for queue durability
- **Gap:** No automatic failover for all services, no failover testing

### DIST-06: Load Balancing
**Status: Exists**
- K8s Service load balancing
- API Gateway reverse proxy
- **Gap:** No gRPC load balancing, no adaptive load balancing

### DIST-07: Data Replication
**Status: Missing**
- **Gap:** No cross-region replication, no database replication configuration, no multi-master

### DIST-08: Offline/Store-Forward
**Status: Partial** | Edge sync service
- CRDT-based sync
- **Gap:** No queue management for offline periods, no bandwidth management

### DIST-09: WAN Optimization
**Status: Missing**
- **Gap:** No stream compression for WAN, no adaptive streaming based on bandwidth

### DIST-10: Hybrid Cloud
**Status: Missing**
- **Gap:** No cloud storage tier, no cloud bursting, no hybrid deployment model

---

## INFRASTRUCTURE DOMAINS

### INFRA-01: API Gateway
**Status: Exists** | Service: `api-gateway` (:8090)
- Reverse proxy to all services
- JWT auth middleware
- Rate limiting
- Route dispatch
- **Gap:** No API versioning, no request/response transformation, no API documentation (OpenAPI/Swagger)

### INFRA-02: Service Mesh
**Status: Missing**
- **Gap:** No Istio/Linkerd, no mTLS mesh, no traffic policies, no canary deployments

### INFRA-03: Container Orchestration
**Status: Exists**
- Docker Compose for dev
- Helm charts for K8s
- All services dockerized
- **Gap:** No K8s operator for EVMS, no automated rollback

### INFRA-04: CI/CD Pipeline
**Status: Exists**
- GitHub Actions (go-ci.yml)
- 3 parallel jobs (backend, triton, frontend)
- Docker image build & push
- Trivy security scanning
- **Gap:** No deployment pipeline, no staging/prod environments, no integration tests in CI

### INFRA-05: Infrastructure as Code
**Status: Exists**
- Helm charts
- Docker Compose
- **Gap:** No Terraform/Pulumi configs, no cloud provisioning, no environment bootstrapping

### INFRA-06: Service Discovery
**Status: Partial**
- K8s DNS-based discovery
- Docker Compose service names
- **Gap:** No service mesh discovery, no health-based service selection

### INFRA-07: Secret Store
**Status: Partial**
- External Secrets Operator support
- **Gap:** No HashiCorp Vault, no automatic secret injection for all services

### INFRA-08: Certificate Management
**Status: Partial**
- Internal cert generation in Helm templates
- TLS cert support
- **Gap:** No cert-manager integration, no automatic renewal

---

## ENTERPRISE DOMAINS

### ENT-01: Licensing
**Status: Missing**
- **Gap:** No license key generation, no license enforcement, no trial management, no feature tiering

### ENT-02: Fleet Management
**Status: Missing**
- **Gap:** No multi-tenant device fleet management, no firmware update orchestration

### ENT-03: Device Health
**Status: Missing**
- **Gap:** No device health dashboard, no device uptime tracking, no device diagnostics

### ENT-04: Video Walls
**Status: Missing**
- **Gap:** No video wall layout editor, no multi-screen support, no video wall scheduling

### ENT-05: Incident Management
**Status: Missing**
- **Gap:** No incident creation, no incident workflow, no incident reporting

### ENT-06: Evidence Management
**Status: Missing**
- **Gap:** No evidence locker, no evidence metadata, no evidence sharing, no evidence retention

### ENT-07: Reporting Engine
**Status: Missing**
- **Gap:** No report templates, no scheduled reports, no custom report builder

### ENT-08: Compliance Management
**Status: Missing**
- **Gap:** No compliance framework mapping (GDPR, HIPAA, PCI), no compliance reporting

### ENT-09: Workflow Automation
**Status: Missing**
- **Gap:** No workflow builder, no trigger-action rules, no approval workflows

### ENT-10: Maps & GIS
**Status: Exists**
- Leaflet integration in frontend
- MapPage component
- useMapCameras hook
- FloorPlanView component
- **Gap:** No GIS data import, no indoor mapping, no heatmap overlay on map

### ENT-11: Integrations Platform
**Status: Missing**
- **Gap:** No REST API documentation, no SDK/plugin SDK, no integration marketplace

### ENT-12: Audio Platform
**Status: Missing**
- **Gap:** No audio recording, no audio playback, no audio analytics

### ENT-13: Intercom/Two-Way Audio
**Status: Missing**
- **Gap:** No two-way audio support, no intercom integration

### ENT-14: Access Control Integration
**Status: Missing**
- **Gap:** No access control system integration, no door event correlation with video

### ENT-15: POS/POS Integration
**Status: Exists** | Service: `pos-ingest` (:8096)
- POS transaction ingestion from NATS
- POS transactions table (migration 006)
- Frontend: POSPage
- Gateway: `/api/pos/*`
- **Gap:** No POS transaction-to-video correlation, no POS alerting

### ENT-16: Dashboard & BI
**Status: Missing**
- **Gap:** No customizable dashboard, no BI reporting, no data export for external BI tools

### ENT-17: Tenant Self-Service
**Status: Missing**
- **Gap:** No tenant portal, no tenant user management UI, no tenant usage stats

### ENT-18: API Public/Partner
**Status: Missing**
- **Gap:** No public API documentation, no rate limit tiers, no API usage analytics

### ENT-19: Webhook Platform
**Status: Exists** | Notification service
- Webhook CRUD and dispatch
- **Gap:** No webhook retry with backoff, no webhook event filtering, no webhook logs

### ENT-20: Mobile Client
**Status: Missing**
- **Gap:** No iOS app, no Android app, no mobile-optimized web UI, no push notifications

### ENT-21: Desktop Client
**Status: Missing**
- **Gap:** No Electron/native desktop app, no multi-monitor support

### ENT-22: SDK/Plugin System
**Status: Missing**
- **Gap:** No plugin SDK, no extension API, no third-party integration framework

### ENT-23: Data Export/Import
**Status: Missing**
- **Gap:** No bulk data export, no CSV/JSON export for reports, no configuration import/export

### ENT-24: Custom Dashboards
**Status: Missing**
- **Gap:** No drag-and-drop dashboard builder, no widget system, no dashboard sharing

### ENT-25: Alarm Management
**Status: Partial**
- AlertsPage frontend exists
- Alert rules in event-proc
- **Gap:** No alarm acknowledgment workflow, no alarm escalation, no alarm priority matrix
