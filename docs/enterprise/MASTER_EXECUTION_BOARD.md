# EVMS Master Execution Board

## Program Overview

| Dimension | Value |
|-----------|-------|
| Total Domains | 82 |
| Production Candidate | 35 (43%) |
| Beta | 48 (59%) |
| Alpha | 6 (7%) |
| Not Started | 28 (34%) |
| Enterprise Ready | 0 |
| Waves | 5 |
| Verified Blockers (Wave 1) | 8 |

## Execution Model

- **Wave**: Self-contained unit of work with independent deployability
- **Gate**: Wave cannot begin until previous wave gate criteria are met
- **Verification**: Each domain within a wave has specific exit and verification criteria
- **Ownership**: Each domain has a designated owner

---

## Dependency Graph

```
Wave 1 (Foundation Security)
  |
  v
Wave 2 (Production Safety)
  |
  v
Wave 3 (State & Persistence)
  |
  v
Wave 4 (Observability & Operations)
  |
  v
Wave 5 (Enterprise & Scale)
```

### Key Dependency Map
```
SEC-01 (Auth) ──> SEC-02 (RBAC) ──> SEC-03 (Multi-Tenancy)
                    │
CORE-03 (ONVIF) ──> CORE-04 (Ingest) ──> CORE-07 (Recording) ──> CORE-08 (Playback)
                    │                                           │
                    └──> AI-01 (Detection) ──> AI-06/07/12/13    │
                                                                  │
                    CORE-07 ──────────────────────────────────────┘
                    │
                    ├──> CORE-13 (Export) ──> ENT-06 (Evidence)
                    ├──> CORE-09 (Timeline)
                    ├──> CORE-16 (Frame Analysis)
                    └──> CORE-20 (Retention)

OPS-01 (Events) ──> OPS-03 (Alerts) ──> ENT-05 (Incidents)
                    OPS-02 (Notifications) ──> OPS-12 (Channels)

DIST-04 (HA) ──> DIST-05 (Failover) ──> DIST-03 (Cluster)
                                        DIST-01 (Federation)

INFRA-01 (Gateway) ──> SEC-15 (Rate Limiting) ──> SEC-13 (Allowlisting)
```

---

## Wave 1: Foundation Security & Critical Compilation

**Status: COMPLETED**
**Owner**: Security Engineering
**Gate**: Code freeze on all services

| ID | Domain | Fix | Effort | Verification |
|----|--------|-----|--------|-------------|
| CB01 | CORE-13 Export Engine | Missing imports compilation | 1h | `go build ./services/export/` |
| CB02 | SEC-05 Encryption at Rest | MustEncrypt/MustDecrypt silent plaintext | 2h | 12 tests pass |
| CB03 | SEC-01 Authentication | JWT secret cached empty via sync.Once | 1h | Auth tests pass |
| CB04 | SEC-14 Session Management | JWT middleware discards claims | 2h | Context injection tests pass |
| CB08 | SEC-14 Session Management | Missing logout endpoint | 3h | Handler tests pass |
| CB07 | SEC-15 Rate Limiting | IP-based login rate limiting | 3h | 5 rate limiter tests pass |
| BB01 | SEC-10 SSO/SAML/OIDC | OIDC state validation on callback | 2h | 3 OIDC callback tests pass |
| BB07 | SEC-10 SSO/SAML/OIDC | SAML string parsing → encoding/xml | 2h | 5 SAML test cases pass |

**Verification Gate**: `go test ./...` passes on all 14 test suites, `go build ./...` clean

---

## Wave 2: Production Safety

**Status**: NOT STARTED
**Owner**: Platform Engineering
**Dependencies**: Wave 1
**Gate**: All Wave 1 blockers verified closed

### SEC-01: WebRTC Authentication (Critical)
| Field | Value |
|-------|-------|
| Priority | **Critical** |
| Effort | 2 days |
| Dependencies | None |
| Exit Criteria | WebRTC peer connection requires JWT |
| Verification | WebRTC endpoints reject unauthenticated requests |

Implementation:
- Add `JWTAuthMiddleware` to WebRTC HTTP signaling handler
- Validate JWT token during SDP offer exchange
- Verify camera ID in token claims matches requested stream
- Replace hardcoded `http://camera-control:8088` with configurable, authenticated call

### SEC-15: Rate Limiting Enhancement (High)
| Field | Value |
|-------|-------|
| Priority | High |
| Effort | 3 days |
| Dependencies | SEC-01 |
| Exit Criteria | Per-tenant rate limiting on API gateway |
| Verification | Load test shows tenant isolation |

Implementation:
- Add per-tenant rate limiter to API gateway (distributed, Redis-backed)
- Add per-user rate limiter to auth service login endpoint
- Rate limit tiers: tenant-level, user-level, endpoint-level
- Expose rate limit headers (`X-RateLimit-Remaining`, `X-RateLimit-Reset`)

### SEC-17: FIPS Enforcement (High)
| Field | Value |
|-------|-------|
| Priority | High |
| Effort | 2 days |
| Dependencies | None |
| Exit Criteria | All services enforce FIPS-compatible crypto |
| Verification | `make fips-test` passes |

Implementation:
- Add startup `os.Exit(1)` if `JWT_SECRET` is empty in all services
- Audit all `crypto/...` imports for FIPS compliance
- Replace `math/rand` with `crypto/rand` where used for security
- Verify all services use `crypto/` not `crypto/unsafe` for hashing
- Add FIPS self-test at startup (already exists in `cmd/fips.go`)

### SEC-05: NATS TLS Enforcement (High)
| Field | Value |
|-------|-------|
| Priority | High |
| Effort | 3 days |
| Dependencies | None |
| Exit Criteria | All NATS connections use TLS |
| Verification | Packet capture confirms encrypted NATS traffic |

Implementation:
- Add NATS TLS configuration in all services
- Generate internal CA + service certs in Helm
- Update docker-compose with NATS TLS
- Add NATS TLS config to `deploy/nats/nats.conf`
- Enable mTLS for NATS connections

### INFRA-01: API Gateway Stability (Medium)
| Field | Value |
|-------|-------|
| Priority | Medium |
| Effort | 3 days |
| Dependencies | None |
| Exit Criteria | Circuit breakers on all downstream services |
| Verification | Downstream failure returns 503, not hang |

Implementation:
- Add HTTP circuit breaker to API gateway for each backend service
- Add retry with backoff for transient failures (3 retries, exponential)
- Add timeout per backend (default 10s)
- Add bulkhead isolation per tenant for critical endpoints

### CORE-04: Ingest Supervisor (High)
| Field | Value |
|-------|-------|
| Priority | High |
| Effort | 3 days |
| Dependencies | None |
| Exit Criteria | FFmpeg crash auto-restarts per camera |
| Verification | Kill ffmpeg → auto restart within 5s |

Implementation:
- Add ffmpeg process supervisor with backoff (1s, 5s, 30s, 5min cap)
- Add per-camera health endpoint (`/health/camera/{id}`)
- Add process-exit monitoring with signal handling
- Add rate-limited restart loop to prevent crash loops

### CORE-08: Playback Moov Atom Fixup (Medium)
| Field | Value |
|-------|-------|
| Priority | Medium |
| Effort | 3 days |
| Dependencies | None |
| Exit Criteria | All recordings seekable in standard players |
| Verification | 100% of test recordings seek to start/middle/end |

Implementation:
- Add `ffmpeg -c copy -movflags +faststart` on segment finalization
- Add Go-native MP4 moov atom relocation as fallback
- Add segment integrity check before serving (valid header, non-zero duration)

---

## Wave 3: State & Persistence

**Status**: NOT STARTED
**Owner**: Backend Engineering
**Dependencies**: Wave 2
**Gate**: All Wave 2 items verified + all production services crash-safe

### AI-01: Event Processing Persistence (Critical)
| Field | Value |
|-------|-------|
| Priority | **Critical** |
| Effort | 5 days |
| Dependencies | None |
| Exit Criteria | All event-proc state survives restart |
| Verification | Restart event-proc → rules, alerts, tours restored |

Implementation:
- Persist `RuleEngine` rules to DB (migration 026)
- Persist `AlertRules` to DB (migration 027)
- Persist `TourScheduler` tours to DB (migration 028)
- Persist `AlertWorkflowManager` state to DB
- Add DB-backed tracker state with periodic snapshot
- Add NATS JetStream consumer with durable name + replay on restart

### SEC-03: Row-Level Security (High)
| Field | Value |
|-------|-------|
| Priority | High |
| Effort | 3 days |
| Dependencies | SEC-03 |
| Exit Criteria | All multi-tenant tables have RLS policies |
| Verification | Cross-tenant query returns empty result set |

Implementation:
- Add `CREATE POLICY tenant_isolation ON ... USING (tenant_id = current_setting('app.tenant_id'))` to all tenant-scoped tables
- Enable `ALTER TABLE ... ENABLE ROW LEVEL SECURITY` on all tenant tables
- Set `app.tenant_id` in connection pool based on JWT claims
- Add migration 029 for RLS policies
- Add cross-tenant verification test

### DIST-05: Service Failover (Medium)
| Field | Value |
|-------|-------|
| Priority | Medium |
| Effort | 5 days |
| Dependencies | DIST-04 |
| Exit Criteria | All critical services have tested failover |
| Verification | Kill primary → secondary takes over in <30s |

Implementation:
- Add NATS-based leader election to all stateful services (recorder exists)
- Add graceful shutdown handler to all HTTP servers (shared helper in pkg/common)
- Add PodDisruptionBudget to all services in Helm
- Add liveness/readiness probes to all services
- Test failover: kill leader, verify takeover time

### CORE-10: Storage Forecasting (Medium)
| Field | Value |
|-------|-------|
| Priority | Medium |
| Effort | 3 days |
| Dependencies | CORE-07 |
| Exit Criteria | Storage forecasting endpoint returns accurate predictions |
| Verification | Forecasting within 10% of actual consumption over 7 days |

Implementation:
- Add ingestion-rate tracking per camera (bytes/sec over sliding window)
- Add storage consumption forecasting (linear projection based on 7d history)
- Add quota enforcement per camera/per site
- Expose storage estimates API with per-camera breakdown
- Add storage threshold alerts

### OPS-08: Backup Verification (Medium)
| Field | Value |
|-------|-------|
| Priority | Medium |
| Effort | 2 days |
| Dependencies | None |
| Exit Criteria | Automated backup verification runs weekly |
| Verification | `pg_restore --test` on latest backup returns success |

Implementation:
- Add `pg_restore --test` to backup script
- Add backup freshness alert (no successful backup in 48h → alert)
- Add point-in-time recovery test procedure
- Add backup retention policy (daily for 7d, weekly for 4w, monthly for 12m)

---

## Wave 4: Observability & Operations

**Status**: NOT STARTED
**Owner**: DevOps / Platform
**Dependencies**: Wave 3
**Gate**: All services crash-safe + state persists across restarts

### INFRA-04: CI/CD Pipeline (Critical)
| Field | Value |
|-------|-------|
| Priority | **Critical** |
| Effort | 5 days |
| Dependencies | None |
| Exit Criteria | Full CI pipeline runs on every PR |
| Verification | PR pushed → lint + test + build + security scan pass |

Implementation:
- Add `go vet ./...` to CI
- Add `golangci-lint run` to CI
- Add `go test -race ./pkg/... ./services/...` to CI
- Add `tsc --noEmit` for frontend
- Add `vite build` to CI
- Add Trivy container scan to CI
- Add `cargo check` for Rust services
- Add Helm template validation to CI
- Add deployment pipeline (staging → prod with manual approval)
- Add integration test stage using docker-compose

### OPS-05: Production Monitoring (High)
| Field | Value |
|-------|-------|
| Priority | High |
| Effort | 4 days |
| Dependencies | None |
| Exit Criteria | All services emit metrics; dashboards cover critical paths |
| Verification | Grafana dashboards for streaming, recording, AI, auth, infra |

Implementation:
- Add Prometheus metrics to all services (latency histograms, error counters, throughput)
- Add ServiceMonitor CRDs to Helm for all services
- Add PrometheusRule alerts to Helm (recording lag, ingest down, auth errors > threshold)
- Add Grafana dashboards checked into repo:
  - Streaming pipeline (ingest → NATS → recorder → playback)
  - AI pipeline (frames → detection → event → alert)
  - Auth (login success/failure rate, token issuance)
  - Infrastructure (CPU, memory, disk, network per service)
- Add RED method metrics: Rate, Errors, Duration for all HTTP endpoints

### OPS-06: Distributed Tracing (Medium)
| Field | Value |
|-------|-------|
| Priority | Medium |
| Effort | 3 days |
| Dependencies | OPS-04 |
| Exit Criteria | End-to-end trace from camera → ingest → recorder → playback |
| Verification | Jaeger shows complete span tree for streaming path |

Implementation:
- Propagate trace context through NATS messages (W3C traceparent)
- Add span tagging standards (camera_id, tenant_id, stream_id)
- Add trace sampling configuration (head-based: 10% default, 100% for errors)
- Add OTel collector configuration for span processing
- Add Jaeger deployment to docker-compose

### OPS-10: Log Management (Medium)
| Field | Value |
|-------|-------|
| Priority | Medium |
| Effort | 2 days |
| Dependencies | OPS-04 |
| Exit Criteria | All logs structured, aggregated, searchable | 
| Verification | LogQL query for error rate across all services returns results |

Implementation:
- Ensure all services use structured JSON logging via `slog`
- Add log level configuration per service via environment variable
- Add Loki log retention policy (30d hot, 90d cold)
- Add Promtail pipeline stages for extracting metrics from logs
- Add log-based alerting (error rate spikes)

### INFRA-05: Infrastructure as Code (Medium)
| Field | Value |
|-------|-------|
| Priority | Medium |
| Effort | 5 days |
| Dependencies | INFRA-03 |
| Exit Criteria | Full environment bootstrap with single command |
| Verification | `terraform apply` → 3-node K8s cluster + EVMS deployed |

Implementation:
- Add Terraform module for AWS EKS deployment (or GKE/AKS)
- Add Terraform module for TimescaleDB RDS
- Add Terraform module for NATS cluster
- Add environment bootstrapping script (`deploy/bootstrap.sh`)
- Add `deploy/terraform/` directory with modules for:
  - VPC, subnets, security groups
  - EKS cluster with node groups
  - RDS TimescaleDB
  - ElastiCache Redis
  - NATS JetStream cluster
  - S3 buckets for recordings and backups
  - IAM roles for service accounts

### OPS-04: Unified Observability (Low)
| Field | Value |
|-------|-------|
| Priority | Low |
| Effort | 2 days |
| Dependencies | OPS-05, OPS-06 |
| Exit Criteria | Single pane of glass for all observability data |
| Verification | Metrics, logs, and traces linkable from a single dashboard |

Implementation:
- Add exemplar support to Prometheus metrics (link traces)
- Add derived fields in Loki to link trace IDs
- Add Grafana Explore configuration for cross-signal navigation
- Add service dependency graph visualization

---

## Wave 5: Enterprise & Scale

**Status**: NOT STARTED
**Owner**: Enterprise Engineering
**Dependencies**: Wave 4
**Gate**: Production monitoring active + CI/CD enforced + failover tested

### AI-20: AI Model Management (High)
| Field | Value |
|-------|-------|
| Priority | High |
| Effort | 8 days |
| Dependencies | AI-01 |
| Exit Criteria | Model registry with versioning, A/B testing |
| Verification | Deploy new model → canary → 100% → rollback |

Implementation:
- Add model registry (Postgres-backed) with versioning
- Add model deployment API (activate/deactivate/promote)
- Add canary deployment (route X% of frames to new model)
- Add model metrics comparison (precision, recall, latency)
- Add model rollback (one-click to previous version)

### ENT-07: Reporting Engine (Medium)
| Field | Value |
|-------|-------|
| Priority | Medium |
| Effort | 5 days |
| Dependencies | OPS-02, OPS-12 |
| Exit Criteria | Scheduled report delivery via email |
| Verification | Generate PDF report with camera stats, events, storage |

Implementation:
- Add report template engine (Go templates + wkhtmltopdf)
- Add scheduled report delivery (daily, weekly, monthly)
- Add report types: audit trail, event summary, storage usage, system health
- Add report API (generate, list, download)
- Add frontend report configuration page

### SEC-13: IP Allowlisting (Low)
| Field | Value |
|-------|-------|
| Priority | Low |
| Effort | 2 days |
| Dependencies | INFRA-01 |
| Exit Criteria | Admin endpoints restricted by IP |
| Verification | Request from non-allowlisted IP returns 403 |

Implementation:
- Add CIDR-based allowlist to API gateway
- Add IP allowlist CRUD API
- Apply to admin endpoints only (by default)
- Add X-Forwarded-For awareness for proxy deployments

### DIST-01: Federation (High)
| Field | Value |
|-------|-------|
| Priority | High |
| Effort | 10 days |
| Dependencies | CORE-07, SEC-01, WAVE-3 |
| Exit Criteria | Cross-site recording search and playback |
| Verification | Site A can search and play recordings from Site B |

Implementation:
- Add federation API: site registration, trust establishment
- Add cross-site recording search via NATS bridging
- Add federated user management (shared identity provider)
- Add cross-site playback (proxy recordings from remote site)
- Add WAN-aware streaming (adaptive bitrate for remote sites)

### ENT-20: Mobile Client (High)
| Field | Value |
|-------|-------|
| Priority | High |
| Effort | 8 days |
| Dependencies | CORE-06, SEC-01 |
| Exit Criteria | Mobile-responsive PWA with live view |
| Verification | Demonstrated on iOS Safari and Android Chrome |

Implementation:
- Enhance PWA for mobile: responsive layout, touch gestures
- Add mobile-optimized live view (WebRTC with adaptive quality)
- Add push notification support (Web Push API)
- Add offline support (service worker cache strategy)
- Add mobile camera grid view (1, 2x2, 3x3 layouts)

### ENT-22: SDK / Plugin System (High)
| Field | Value |
|-------|-------|
| Priority | High |
| Effort | 10 days |
| Dependencies | SEC-12 |
| Exit Criteria | Third-party plugin can receive events and control cameras |
| Verification | Sample plugin receives event, sends PTZ command |

Implementation:
- Define plugin interface (gRPC-based, sidecar process)
- Add plugin registry (name, version, permissions, status)
- Add plugin lifecycle management (install, enable, disable, uninstall)
- Add event subscription API for plugins (filtered event stream)
- Add camera control API for plugins (with permission scoping)
- Add SDK package with Go client library + examples

### INFRA-07: Secret Store (Medium)
| Field | Value |
|-------|-------|
| Priority | Medium |
| Effort | 4 days |
| Dependencies | INFRA-03 |
| Exit Criteria | All secrets managed by Vault |
| Verification | Vault restart → secrets still available |

Implementation:
- Deploy HashiCorp Vault in K8s
- Migrate from External Secrets Operator to Vault + CSI provider
- Add Vault agent injector for all services
- Add secret rotation policy (30 days for JWT, 90 days for DB creds)
- Add unseal workflow (auto-unseal via KMS)

### INFRA-08: Certificate Management (Medium)
| Field | Value |
|-------|-------|
| Priority | Medium |
| Effort | 2 days |
| Dependencies | INFRA-03 |
| Exit Criteria | All certificates auto-renewed |
| Verification | Expired cert test → cert-manager auto-renews within 24h |

Implementation:
- Deploy cert-manager to K8s cluster
- Add Let's Encrypt ClusterIssuer for public endpoints
- Add internal CA Issuer for service-to-service TLS
- Add certificate renewal monitoring and alerting

### ENT-01: Licensing (High)
| Field | Value |
|-------|-------|
| Priority | High |
| Effort | 5 days |
| Dependencies | SEC-01 |
| Exit Criteria | License enforcement for camera count + features |
| Verification | Exceed camera count → new cameras locked |

Implementation:
- Add license key generation (signed JWT with claims)
- Add license enforcement in API gateway (camera count, feature flags)
- Add license management API (activate, validate, list)
- Add trial license support (time-limited, full features)
- Add license expiration alerting

### ENT-10: Maps & GIS Enhancement (Low)
| Field | Value |
|-------|-------|
| Priority | Low |
| Effort | 3 days |
| Dependencies | None |
| Exit Criteria | Indoor mapping with floor plan overlay |
| Verification | Upload SVG floor plan → cameras positioned correctly |

Implementation:
- Add indoor map support (SVG floor plan upload)
- Add GIS data import (GeoJSON, KML)
- Add camera overlay on indoor maps
- Add heatmap overlay on map (from people counting data)

---

## Domain Ownership

| Category | Owner Team | Domains |
|----------|------------|---------|
| Core VMS | Streaming Engineering | CORE-01 through CORE-20 |
| AI Platform | AI Engineering | AI-01 through AI-20 |
| Security | Security Engineering | SEC-01 through SEC-19 |
| Operations | Platform Engineering | OPS-01 through OPS-15 |
| Distributed Systems | Infrastructure Engineering | DIST-01 through DIST-10 |
| Infrastructure | DevOps | INFRA-01 through INFRA-08 |
| Enterprise | Enterprise Engineering | ENT-01 through ENT-25 |

## Verification Gates

| Gate | Criteria | Measured By |
|------|----------|-------------|
| Wave 1 Gate | All 8 blockers fixed, committed, verified | `go test ./...` passes, `go build ./...` clean |
| Wave 2 Gate | All production safety items verified | `make beta-verify` passes, security audit confirms fixes |
| Wave 3 Gate | All state persisted, failover tested | `make test-e2e` passes (restart → data survives) |
| Wave 4 Gate | CI/CD enforced, monitoring active | PR merge → staging deploy → e2e tests pass |
| Wave 5 Gate | Enterprise features verified | `make enterprise-verify` passes (multi-tenant, federated, licensed) |

## Certification Gates

| Level | Criteria | Wave |
|-------|----------|------|
| Production Candidate | All blockers fixed, tests pass, builds clean | Wave 1 |
| Beta+ (Safe for Pilot) | Auth on all endpoints, persistence survives restart, basic monitoring | Wave 3 |
| Production Ready | CI/CD, full monitoring, failover tested, backup verified | Wave 4 |
| Enterprise Ready | Multi-tenant RLS, federation, mobile, SDK, licensing | Wave 5 |

## Risk Register

| Risk | Impact | Probability | Wave | Mitigation |
|------|--------|-------------|------|------------|
| NATS TLS rollout breaks existing deployments | Medium | Medium | 2 | Add TLS as optional first, then enforce |
| RLS migration causes query performance regression | High | Low | 3 | Load test before/after, add indexes |
| CI/CD rollout blocks developer velocity | Medium | Medium | 4 | Start with non-blocking checks, escalate over 2 weeks |
| Federation requires significant cross-site networking | High | Medium | 5 | Design for NATS bridging, cloud-native first |
| Mobile client native development vs PWA | Medium | Medium | 5 | PWA enhancement first, native later |
| Vault migration disrupts secret injection | High | Low | 5 | Side-by-side migration with fallback |

## Wave Effort Summary

| Wave | Focus | Estimated Effort | Domains Impacted | Risk |
|------|-------|-----------------|-----------------|------|
| 1 | Foundation Security | ~16h (DONE) | 5 | Low |
| 2 | Production Safety | ~21 days | 6 | High (security gaps) |
| 3 | State & Persistence | ~18 days | 4 | Medium (data loss) |
| 4 | Observability & Operations | ~21 days | 3 | Medium (blindness) |
| 5 | Enterprise & Scale | ~57 days | 10 | High (complexity) |
| **Total** | | **~117 days** | **28 domains** | |

## Key Milestones

| Milestone | Wave | Timeline | Exit Criteria |
|-----------|------|----------|---------------|
| M1: Security Baseline | Wave 1 | Day 1 | All 8 blockers verified |
| M2: Production Safe | Wave 2 | Week 3-4 | Webrtc auth + NATS TLS + FFmpeg supervisor + circuit breakers |
| M3: Stateful & Resilient | Wave 3 | Week 5-7 | Event-proc persistence + RLS + failover tested |
| M4: Observable & Automated | Wave 4 | Week 8-10 | CI/CD + full monitoring + dashboards |
| M5: Enterprise Ready | Wave 5 | Week 11-16 | Federation + mobile + SDK + licensing |
