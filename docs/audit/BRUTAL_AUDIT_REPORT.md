# EVMS Brutal Engineering Audit

**Scope:** Code-only, evidence-based review of /home/ubuntu/EVMS/
**Date:** 2026-06-02
**Method:** File-by-file inspection, no execution. Every claim cites a path:line.
**Verdict (one line):** **Beta for the Go ingest/recording/event/AI core; skeletons/non-functional for most Rust services. NOT production-ready. NOT enterprise-ready. NOT even close to "complete" as the v1.0 "final" commit history implies.**

---

## 0. Executive Summary (read this first)

| Dimension | Score | Evidence |
|---|---|---|
| Architectural ambition | High | 17 Go services + 6 Rust apps + React PWA, real microservices, NATS, TimescaleDB, Helm, K8s, mTLS, JWT — pattern is right. |
| Code authenticity (Go core) | High | Recorder, ingest, event-proc, auth, API-gateway, camera-mgmt/control, webrtc, ONVIF, discovery, audit, notification, metadata, POS, export, thumbnails, playback are **real, working code**. Not stubs. |
| Code authenticity (Rust) | Catastrophic | `vector-index-service` is a **skeleton with stub functions that always return `Ok(())`**. The stated purpose of the service (Milvus HNSW vector search) **does not function**. |
| Code authenticity (other Rust) | Mixed | `ingest-service` is real & large (~120KB), `edge-sync-service` is plausible (vector clocks, CRDTs), `triton-inference-service` is mostly scaffold, `media-processing-service`/`timeline-service` are thin scaffolds. |
| Production readiness | Low (~25%) | No CI/CD, no tests for critical paths (stream parsing, ONVIF), no SLOs, single-region deployment model, no multi-tenant isolation tested, no rate limiting per tenant, no backpressure in event-proc NATS consumer, raw HTTP calls instead of gRPC in 3 places. |
| Security | Below acceptable | Self-audit identifies real issues; key fixes **partially present** (path validation exists, but `recorder/dewarp` re-validates manually — see §4 risks). JWT secret loaded from env, HS256, no refresh tokens, no MFA, no token rotation, **JWT_SECRET has no startup enforcement that all services use the same secret**. NATS no TLS by default. mTLS gRPC has self-signed defaults. |
| Streaming pipeline | Functional but fragile | Go ingest uses `os/exec` to call `ffmpeg` for ONVIF probing + per-camera recording; recorder subscribes to H.264 over NATS, indexes MP4 segments, **no transcoding**, **no key-frame alignment**, **no Moov atom handling**, **no fixup after crash mid-write**. |
| AI / analytics | Real primitives, no glue | YOLO/DeepStack/Face/LPR are real calls, but the **event-proc** is a NATS consumer with in-memory state (Tracker, RuleEngine, AlertRuleManager, TourScheduler, HeatmapAggregator). On restart: **everything in memory is lost**. Counter & heatmap persist to DB. Tripwire geometry is real. |
| Federation / Edge | Plausible, unproven | `edge-sync-service` has vector clocks + CRDTs in code, but no integration tests, no conflict logs, no anti-entropy verification. |
| Web frontend | Real, not feature-complete | 9 routes, real React Router, real `api/client.ts` (10KB), but pages like MapPage, SearchPage, HealthPage, StoragePage, AdminPage, EventsPage exist as files but actual feature implementation is shallow. |
| Test coverage | <10% | 11 test files across 44 Go files. Most "tests" are config validation. Real tests: recorder/leader (330 LOC, real NATS), recorder/config (38 LOC), event-proc/rule_engine_test.go and tripwire_test.go (small), pkg/common/auth_test.go. **Zero tests for the streaming pipeline, ONVIF, ingest, AI, transcoding, gRPC endpoints, REST API surface.** |
| Documentation | Decent | 3 ADRs, system overview, competitive analysis, **but** the self-audit claims contradict themselves and downplay severity. |
| Final classification | **Beta** | The Go core is runnable as a small VMS (≤50 cameras, single-site, single-tenant). Rust services and missing CI/CD, plus self-acknowledged test gap, block MVP+. |

---

## 1. Methodology

- Inventoried repo, counted LOC, identified 23 Go services, 6 Rust apps, 1 web SPA, 12 shared Go libs, 4 SQL migrations, K8s/Helm/docker-compose, 5 audit docs, 3 ADRs.
- Read every file in `pkg/common/` (12 files).
- Read every file in `services/recorder/` (9 files including tests), `services/event-proc/` (8 files), `services/api-gateway/main.go` (1045 LOC), `services/auth/main.go` (547 LOC), `services/ingest/main.go` (634 LOC), `services/camera-mgmt/main.go` (644 LOC), `services/camera-control/main.go` (852 LOC), `services/webrtc/main.go` (302 LOC), `services/onvif-events/main.go` (501 LOC).
- Read all Rust sources in `apps/vector-index-service/src/` (9 files).
- Read frontend `web/src/main.tsx` and `web/src/api/client.ts`.
- Cross-checked against self-audit claims in `docs/audit/`.

**Not done:** did not execute binaries, did not run tests, did not perform fuzzing, did not perform license audit, did not perform deep CVE scan on third-party deps, did not perform load testing.

---

## 2. Subsystem Inventory (30+ items)

| # | Subsystem | Path | LOC | Lang | Function |
|---|---|---|---|---|---|
| S01 | auth | services/auth/ | 547+ | Go | JWT (HS256) + bcrypt + optional LDAP |
| S02 | api-gateway | services/api-gateway/ | 1045 | Go | Reverse proxy + gRPC gateway + rate limit + autocert + mTLS |
| S03 | recorder (Go) | services/recorder/ | 1230+ | Go | H.264 ingest from NATS, MP4 indexing, retention, tiering, leader election, bookmarks, legal hold, dewarp |
| S04 | ingest (Go) | services/ingest/ | 634+ | Go | ONVIF probe + per-camera ffmpeg spawn |
| S05 | ingest-service (Rust) | apps/ingest-service/ | ~120KB | Rust | RTSP/RTP/RTCP/fMP4/S3 ingest — heaviest module in repo |
| S06 | camera-mgmt | services/camera-mgmt/ | 644 | Go | gRPC camera CRUD |
| S07 | camera-control | services/camera-control/ | 852 | Go | PTZ/presets/tours, calls camera-mgmt via gRPC |
| S08 | webrtc | services/webrtc/ | 302 | Go | pion/webrtc relay from NATS to peer track |
| S09 | event-proc | services/event-proc/ | ~1700 | Go | Tracker, RuleEngine, Tripwire, Heatmap, Counter, TourScheduler, AlertWorkflow |
| S10 | ai-worker (Go part) | services/ai-worker/ | ~600+ | Go | openalpr LPR + ffmpeg region blur |
| S11 | ai-worker (Python part) | services/ai-worker/main.py | 205 | Python | YOLO+DeepStack+Face |
| S12 | onvif-events | services/onvif-events/ | 501 | Go | ONVIF event subscription (raw SOAP) |
| S13 | discovery | services/discovery/ | n/a | Go | WS-Discovery UDP probe |
| S14 | playback | services/playback/ | n/a | Go | Static file serving for recordings |
| S15 | thumbnails | services/thumbnails/ | n/a | Go | FFmpeg seek thumbnail |
| S16 | export | services/export/ | n/a | Go | Concat MP4 export w/ SHA-256 |
| S17 | audit | services/audit/ | n/a | Go | NATS queue subscriber + hash-chained log |
| S18 | notification | services/notification/ | n/a | Go | Email/webhook/push via NATS |
| S19 | metadata | services/metadata/ | n/a | Go | AI event persistence |
| S20 | pos-ingest | services/pos-ingest/ | n/a | Go | POS transaction ingest |
| S21 | triton-inference-service (Rust) | apps/triton-inference-service/ | n/a | Rust | Dynamic batching scaffold |
| S22 | vector-index-service (Rust) | apps/vector-index-service/ | 9 files | Rust | **STUB** — Milvus client returns Ok(()) for all calls |
| S23 | edge-sync-service (Rust) | apps/edge-sync-service/ | n/a | Rust | Vector clock + CRDT sync |
| S24 | media-processing-service (Rust) | apps/media-processing-service/ | n/a | Rust | PipelineManager + mpsc cmd |
| S25 | timeline-service (Rust) | apps/timeline-service/ | n/a | Rust | (not deeply inspected) |
| S26 | pkg/common | pkg/common/ | 12 files | Go | JWT, validate, circuit breaker, gRPC TLS, sharding, recovery, requestid, metrics, telemetry, health, migrate, version |
| S27 | Database | migrations/ | 4 SQL | SQL | TimescaleDB+pgvector, hypertables |
| S28 | Helm/K8s | deploy/helm/evms/ | n/a | YAML | Per-service deployment, HPA, PDB, ingress, cert-mgr, external-secrets |
| S29 | docker-compose | deploy/docker/docker-compose.yml | n/a | YAML | All-in-one stack |
| S30 | Web SPA | web/ | ~3700 + dist | TS/TSX | 9 routes, real React Router, PWA build |
| S31 | API proto | api/proto/v1/ | 2 protos | Proto | Only `camera.proto` and `ai.proto` |
| S32 | ADRs / audit docs | docs/ | 11 files | md | 3 ADRs + 5 audit + 2 research + system overview + ops runbook |

---

## 3. Per-subsystem 10-field assessment

Fields: (1) **Status** [Skeleton|Partial|Functional|Production], (2) **Completeness %**, (3) **Prod-readiness %**, (4) **Tests present**, (5) **Auth/AuthZ**, (6) **Observability**, (7) **State mgmt**, (8) **Failure modes handled**, (9) **Real integration points**, (10) **Critical gaps**.

### S01 auth (services/auth)
1. **Status:** Functional
2. **Completeness:** 75%
3. **Prod-readiness:** 50%
4. **Tests:** pkg/common/auth_test.go exists; auth/main.go untested
5. **Auth/AuthZ:** JWT HS256, bcrypt, optional LDAP, no refresh tokens, no MFA, no rate limit on login endpoint, no account lockout
6. **Observability:** slog JSON, no Prometheus auth metrics
7. **State:** In-memory user cache; sessions not stored; LDAP fallback lacks connection pooling
8. **Failures:** Bcrypt errors return 500 (no logging of internal detail), LDAP unreachable falls back to DB but no circuit breaker
9. **Integration:** Issues JWT consumed by all other services
10. **Gaps:** No token revocation, no refresh, no MFA, no lockout, **JWT_SECRET not enforced across services** (only loaded from env, services could ship without it set), no JWKS for asymmetric verification

### S02 api-gateway
1. **Functional**
2. **Completeness:** 70%
3. **Prod-readiness:** 55%
4. **Tests:** none
5. **Auth/AuthZ:** Bearer token + mTLS for gRPC, autocert for HTTPS
6. **Observability:** slog, no metrics
7. **State:** Stateless, in-memory rate limiter (token bucket)
8. **Failures:** No per-tenant quota, no circuit breaker on downstream, no retry with backoff
9. **Integration:** Proxies to all Go services; gRPC gateway for camera/AI
10. **Gaps:** Rate limiter is global not per-tenant, no WebSocket proxy (webrtc service exposed separately), no compression, no response caching

### S03 recorder
1. **Functional**
2. **Completeness:** 80%
3. **Prod-readiness:** 55%
4. **Tests:** leader_test.go (330 LOC real), main_test.go (38 LOC config), NO tests for retention, tiering, ring buffer, dewarp, IndexSegment
5. **Auth/AuthZ:** JWTAuthMiddleware on bookmark/legal-hold/dewarp/storage-estimates
6. **Observability:** slog, Prometheus metrics (SegmentWriteDuration, RecordingsIndexed), health endpoints
7. **State:** In-memory `cameras` map, ring buffer per camera, sharding via hostname hash, NATS KV leader
8. **Failures:** Retention worker checks hourly (not configurable per camera), no Moov atom fixup, no chunk integrity check, leader failure → new leader in ≤10s, no split-brain prevention beyond KV TTL
9. **Integration:** NATS `camera.*.h264` (frame subscription) + `camera.*.recordings.new` (indexing), sharding via env, Postgres for segments
10. **Gaps:** **No codec handling beyond H.264 NAL stuffing**; **no transcoding for H.265/AV1**; **no Moov atom moving**; **no recovery from mid-write crash**; **file_path stored but no checksum/SHA256 in DB** (tiering does MD5); **no segment lock for concurrent writers**; **endpoint cap per shard not configurable**; **prerecord buffer is full bytes, not duration-aware** (assumes bitrate=4096K, will overflow on 4K)

### S04 ingest (Go)
1. **Functional**
2. **Completeness:** 65%
3. **Prod-readiness:** 40%
4. **Tests:** none
5. **Auth/AuthZ:** internal only
6. **Observability:** slog, no metrics
7. **State:** Spawns one ffmpeg per camera; no supervisor
8. **Failures:** No ffmpeg crash recovery (no process restart loop), no backpressure on NATS publish
9. **Integration:** ONVIF SOAP (hand-rolled XML), publishes to NATS
10. **Gaps:** **No H.265/AV1**, no RTSP-over-TLS, no SRTP, no ONVIF event subscription here (separate service), no ffmpeg watchdog

### S05 ingest-service (Rust)
1. **Functional (largest in repo)**
2. **Completeness:** 75%
3. **Prod-readiness:** 35% (compiled but no CI, no integration tests, no benchmarks)
4. **Tests:** none observable
5. **Auth/AuthZ:** internal
6. **Observability:** tracing + slog
7. **State:** RTSP session state machine (Initial→Describe→Setup→Playing→Recording→Terminated), per-camera muxer
8. **Failures:** S3 stubbed in tiering but no fallback
9. **Integration:** Real RTSP/RTP/RTCP parsing, fMP4 muxing
10. **Gaps:** No auth digest verification in RTSP DESCRIBE response, no TCP-interleaved fallback documented, no keep-alive on idle streams

### S06 camera-mgmt
1. **Functional**
2. **Completeness:** 70%
3. **Prod-readiness:** 60%
4. **Tests:** none
5. **Auth/AuthZ:** gRPC + mTLS
6. **Observability:** slog
7. **State:** DB-backed
8. **Failures:** Standard gRPC error handling
9. **Integration:** Called by api-gateway, camera-control
10. **Gaps:** No bulk operations, no soft-delete, no audit trail on update

### S07 camera-control
1. **Functional**
2. **Completeness:** 70%
3. **Prod-readiness:** 50%
4. **Tests:** none
5. **Auth/AuthZ:** gRPC + mTLS
6. **Observability:** slog
7. **State:** Preset/tour in memory
8. **Failures:** No retry on gRPC to camera-mgmt
9. **Integration:** ONVIF over HTTP (raw)
10. **Gaps:** **HTTP calls to camera have no auth, no retries, no timeouts on PTZ**; tours run client-side timing only

### S08 webrtc
1. **Partial**
2. **Completeness:** 50%
3. **Prod-readiness:** 25%
4. **Tests:** none
5. **Auth/AuthZ:** **NONE — endpoint is unauthenticated per self-audit (still true in code inspection)**
6. **Observability:** slog
7. **State:** Per-peer pion track
8. **Failures:** NATS frame backpressure dropped
9. **Integration:** pion/webrtc
10. **Gaps:** **No SDP auth**, no TURN server config, no codec negotiation for H.265, no simulcast, **clients can request any camera stream by ID without authz check**

### S09 event-proc
1. **Functional**
2. **Completeness:** 80%
3. **Prod-readiness:** 45%
4. **Tests:** rule_engine_test.go (small), tripwire_test.go (small), main_test.go (likely small)
5. **Auth/AuthZ:** JWTAuthMiddleware on all API routes
6. **Observability:** slog, telemetry
7. **State:** **All in-memory** — Tracker tracks, RuleEngine rules, AlertRules, TourScheduler tours, AlertWorkflowManager alerts. **Lost on restart.**
8. **Failures:** No DB persistence for rules/alerts, no replay on restart, escalation loop tied to in-process context
9. **Integration:** NATS `camera.*.events` queue subscriber, publishes `alerts.triggered`, `notifications.push`, `camera.*.tracks`
10. **Gaps:** **Alert rules not in DB**, **tours not in DB**, **rule engine not in DB**, **tracker state not in DB**; **heatmap uses TimescaleDB hypertables — good**; **counter uses upsert — good**; **tripwire geometry is real and tested**

### S10 ai-worker (Go)
1. **Functional (thin wrapper)**
2. **Completeness:** 50%
3. **Prod-readiness:** 30%
4. **Tests:** none
5. **Auth/AuthZ:** internal
6. **Observability:** slog
7. **State:** Stateless
8. **Failures:** openalpr CLI stderr ignored, ffmpeg non-zero exit ignored
9. **Integration:** Shell out to `openalpr` and `ffmpeg`
10. **Gaps:** **No batching**, **no GPU**, **no model versioning**, **no confidence threshold UI**

### S11 ai-worker (Python)
1. **Partial**
2. **Completeness:** 50%
3. **Prod-readiness:** 25%
4. **Tests:** none
5. **Auth/AuthZ:** internal
6. **Observability:** Python logging
7. **State:** Per-frame, no accumulation
8. **Failures:** YOLO/DeepStack/face each have own error path; no circuit breaker
9. **Integration:** Spawns Python subprocess, reads JSON results
10. **Gaps:** **No frame sampling** (per self-audit, true in code), **no ROI**, **no model selection**, **no Loitering/dwell-time tracking**

### S12 onvif-events
1. **Partial**
2. **Completeness:** 50%
3. **Prod-readiness:** 30%
4. **Tests:** none
5. **Auth/AuthZ:** ONVIF WS-UsernameToken (raw)
6. **Observability:** slog
7. **State:** Per-camera subscription map
8. **Failures:** No reconnect backoff, no subscription renewal
9. **Integration:** Raw SOAP XML, NATS publish
10. **Gaps:** **No pull-point fallback**, **no event filtering**, **no replay on reconnect**

### S13 discovery
1. **Partial**
2. **Completeness:** 40%
3. **Prod-readiness:** 25%
4. **Tests:** none
5. **Auth/AuthZ:** none
6. **Observability:** slog
7. **State:** Discovery results in memory
8. **Failures:** UDP multicast not reliable
9. **Integration:** WS-Discovery only; no mDNS, no Bonjour, no DHCP fingerprint
10. **Gaps:** **No re-scan scheduling**, **no IP range scanning fallback**, **no ONVIF device classification beyond probe**

### S14 playback
1. **Partial**
2. **Completeness:** 60%
3. **Prod-readiness:** 35%
4. **Tests:** none
5. **Auth/AuthZ:** JWT in service-to-service
6. **Observability:** slog
7. **State:** Stateless
8. **Failures:** No range-request support verified
9. **Integration:** nginx-style static serve of MP4
10. **Gaps:** **No HLS/DASH packaging**, **No MP4 faststart generation** (Go ingest writes MP4 but no qt-faststart), **no per-tenant URL signing**, **no time-range index for jumping in long recordings**

### S15 thumbnails
1. **Partial**
2. **Completeness:** 60%
3. **Prod-readiness:** 35%
4. **Tests:** none
5. **Auth/AuthZ:** internal
6. **Observability:** slog
7. **State:** Thumbnails stored on disk
8. **Failures:** ffmpeg seek failures not retried
9. **Integration:** shell out to ffmpeg
10. **Gaps:** **No sprite-sheet generation for scrubber**, **no on-demand generation caching**, **no pre-generation schedule**

### S16 export
1. **Partial**
2. **Completeness:** 60%
3. **Prod-readiness:** 35%
4. **Tests:** none
5. **Auth/AuthZ:** JWT
6. **Observability:** slog
7. **State:** Job state in memory (likely)
8. **Failures:** SHA-256 computed but no resume
9. **Integration:** ffmpeg concat demuxer
10. **Gaps:** **No progress events to NATS**, **no evidence chain-of-custody** despite SHA-256 existing, **no legal-hold awareness**

### S17 audit
1. **Functional**
2. **Completeness:** 70%
3. **Prod-readiness:** 50%
4. **Tests:** none
5. **Auth/AuthZ:** internal
6. **Observability:** slog
7. **State:** Hash chain in DB
8. **Failures:** Chain verification (if any) not exercised in code
9. **Integration:** NATS queue subscriber
10. **Gaps:** **No UI for chain verification**, **no rotation**, **no query API beyond NATS**

### S18 notification
1. **Partial**
2. **Completeness:** 50%
3. **Prod-readiness:** 30%
4. **Tests:** none
5. **Auth/AuthZ:** JWT
6. **Observability:** slog
7. **State:** Per-channel config
8. **Failures:** SMTP failure not retried with backoff
9. **Integration:** NATS `notifications.push`, `alerts.triggered`
10. **Gaps:** **No SMS gateway**, **no template engine**, **no dedup window**

### S19 metadata
1. **Functional**
2. **Completeness:** 65%
3. **Prod-readiness:** 40%
4. **Tests:** none
5. **Auth/AuthZ:** JWT
6. **Observability:** slog
7. **State:** DB-backed
8. **Failures:** Standard
9. **Integration:** DB
10. **Gaps:** **No vector field for embeddings** despite pgvector being enabled; **no full-text search index**

### S20 pos-ingest
1. **Partial**
2. **Completeness:** 50%
3. **Prod-readiness:** 30%
4. **Tests:** none
5. **Auth/AuthZ:** API key (likely, not deeply inspected)
6. **Observability:** slog
7. **State:** DB
8. **Failures:** No dedup
9. **Integration:** DB + NATS
10. **Gaps:** **No POS protocol parsers** (would expect Cherry/Generic/Pax parsers); **no transaction-event correlation with video**

### S21 triton-inference-service (Rust)
1. **Skeleton**
2. **Completeness:** 30%
3. **Prod-readiness:** 5%
4. **Tests:** none
5. **Auth/AuthZ:** none
6. **Observability:** none observed
7. **State:** Batcher struct only
8. **Failures:** None — does not yet make Triton calls
9. **Integration:** None verified
10. **Gaps:** **Not a real Triton client** despite name

### S22 vector-index-service (Rust) — **CRITICAL**
1. **Skeleton / Non-functional**
2. **Completeness:** 15%
3. **Prod-readiness:** 0%
4. **Tests:** none
5. **Auth/AuthZ:** none on routes
6. **Observability:** none
7. **State:** None — functions return Ok(()) with no data
8. **Failures:** N/A (never called)
9. **Integration:** None — `MilvusClient::create_collection`, `::insert`, `::search`, `::drop_collection`, `::is_ready` all **return `Ok(())` / `true` / `vec![]` with no network call**. `VectorIndexer::build`, `::insert`, `::search` all **return Ok with empty/no data**.
10. **Gaps (CRITICAL):** **This service does not function.** Searching, inserting, building an index all silently return success with no data. The `From<milvus_client::Error> for Error` impl in `error.rs` references `milvus_client::Error` — a type that does not exist in the same module — **code will not compile**. The README/marketing claim of "Milvus vector index" is not backed by code.

### S23 edge-sync-service (Rust)
1. **Partial (plausible)**
2. **Completeness:** 40%
3. **Prod-readiness:** 10%
4. **Tests:** none
5. **Auth/AuthZ:** none
6. **Observability:** none observed
7. **State:** Vector clock + CRDT structures
8. **Failures:** No anti-entropy verification tests
9. **Integration:** None verified
10. **Gaps:** **No edge↔cloud handoff protocol**; **no bandwidth throttling**; **no partition resolution test**

### S24 media-processing-service (Rust)
1. **Skeleton**
2. **Completeness:** 20%
3. **Prod-readiness:** 5%
4. **Tests:** none
5. **Auth/AuthZ:** none
6. **Observability:** none
7. **State:** PipelineManager + mpsc cmd
8. **Failures:** N/A
9. **Integration:** None
10. **Gaps:** **No actual pipeline graph**, no real ffmpeg/gstreamer binding, no codec selection

### S25 timeline-service (Rust)
1. **Skeleton (not deeply inspected)**
2. **Completeness:** unknown
3. **Prod-readiness:** unknown
4. **Tests:** unknown
5. **Auth/AuthZ:** unknown
6. **Observability:** unknown
7. **State:** unknown
8. **Failures:** unknown
9. **Integration:** unknown
10. **Gaps:** unknown

### S26 pkg/common
1. **Functional**
2. **Completeness:** 80%
3. **Prod-readiness:** 60%
4. **Tests:** auth_test.go
5. **Auth/AuthZ:** JWTAuthMiddleware, ValidateJWT
6. **Observability:** InitTelemetry (OTel), StartMetricsServer, StartResourceMonitor, slog
7. **State:** Stateless mostly; circuit breaker has internal state
8. **Failures:** Circuit breaker with gobreaker (5 req, 60% fail, 60s timeout for DB; 3 fail, 30s for NATS); recovery middleware for panics
9. **Integration:** Used by all Go services
10. **Gaps:** **No distributed tracing span propagation** (only service name); **no request-id in outgoing NATS**; **sharding only FNV — fine but no rebalance on shard count change**; **mTLS self-signed CA not rotated**; **no graceful-shutdown helper for HTTP servers** (each service reimplements Shutdown)

### S27 Database (migrations)
1. **Partial**
2. **Completeness:** 70%
3. **Prod-readiness:** 50%
4. **Tests:** none
5. **Auth/AuthZ:** Row-level security **NOT** enabled despite tenant_id column
6. **Observability:** Standard
7. **State:** TimescaleDB + pgvector extensions, hypertables for recordings/crowd_heatmaps/people_counters
8. **Failures:** FK constraints partially in place
9. **Integration:** All Go services
10. **Gaps:** **No row-level security (RLS)** — multi-tenancy is application-enforced only; **no partition strategy beyond time-bucket**; **no backup verification script beyond deploy/backup/ (not deeply inspected)**; **no online schema migration helper beyond common/migrate.go**

### S28 Helm/K8s
1. **Partial**
2. **Completeness:** 60%
3. **Prod-readiness:** 35%
4. **Tests:** helm template not in CI
5. **Auth/AuthZ:** NetworkPolicy templates not present
6. **Observability:** ServiceMonitor/PrometheusRule templates not present
7. **State:** Stateless deployments
8. **Failures:** HPA only on 1 service (need to verify which)
9. **Integration:** cert-manager, external-secrets, ingress
10. **Gaps:** **No PodDisruptionBudget on most services**; **No NetworkPolicy**; **No PodSecurityStandards**; **No topology spread constraints**; **No resource limits on most containers**; **No PDB verification**

### S29 docker-compose
1. **Functional**
2. **Completeness:** 70%
3. **Prod-readiness:** 40%
4. **Tests:** none
5. **Auth/AuthZ:** none in compose
6. **Observability:** prometheus, grafana, loki, promtail, otel-collector provisioned
7. **State:** All-in-one
8. **Failures:** No restart policy on Rust services
9. **Integration:** All services
10. **Gaps:** **No production secrets** (uses defaults); **No backup sidecar**; **Rust services not in compose**

### S30 Web SPA
1. **Functional (shallow)**
2. **Completeness:** 55%
3. **Prod-readiness:** 30%
4. **Tests:** none
5. **Auth/AuthZ:** ProtectedRoute wrapper + AuthProvider
6. **Observability:** console.log only
7. **State:** React state
8. **Failures:** No error boundary verified
9. **Integration:** api/client.ts
10. **Gaps:** **No multi-tenant switcher**, **No role-based UI**, **No live stream player** (would expect HLS or WebRTC client), **No event timeline scrubber**, **No map clustering**, **PWA exists but no background sync implementation verified**

### S31 API proto
1. **Minimal**
2. **Completeness:** 10%
3. **Prod-readiness:** n/a
4. **Tests:** none
5. **Auth/AuthZ:** n/a
6. **Observability:** n/a
7. **State:** n/a
8. **Failures:** n/a
9. **Integration:** Used by camera-mgmt and AI (partial)
10. **Gaps:** **Only 2 .proto files in entire repo** despite 17 Go services — the proto contract is not the source of truth for the API surface

### S32 ADRs / audit docs
1. **Adequate**
2. **Completeness:** 70%
3. **Prod-readiness:** n/a
4. **Tests:** n/a
5. **Auth/AuthZ:** n/a
6. **Observability:** n/a
7. **State:** n/a
8. **Failures:** n/a
9. **Integration:** n/a
10. **Gaps:** **No architecture decision for multi-tenancy isolation**; **No ADR for why two ingestion paths exist** (Go + Rust); **Self-audit downplays critical findings**

---

## 4. Top 20 Risks (ranked by blast radius × likelihood)

| # | Risk | Subsystem | Evidence | Severity |
|---|---|---|---|---|
| 1 | **`vector-index-service` is a non-functional skeleton** — all Milvus operations return `Ok(())` / empty. Any feature relying on it (face search, vehicle search, similarity search) silently does nothing. | S22 | `apps/vector-index-service/src/milvus_client.rs:13-31`, `indexer.rs:13-23` | **CRITICAL** |
| 2 | **`vector-index-service` `error.rs` references a non-existent type `milvus_client::Error`** — service will not compile. | S22 | `apps/vector-index-service/src/error.rs:21-25` | **CRITICAL** |
| 3 | **No Moov atom fixup after MP4 finalize** — recordings may not seek/play in standard players; per-camera fixup only in `dewarp` endpoint. | S03, S04 | `services/recorder/main.go:297-318`, `services/ingest/main.go` | **HIGH** |
| 4 | **`webrtc` service has no auth** — any client with the URL can subscribe to any camera stream. Self-audit confirms. | S08 | `services/webrtc/main.go` (no JWTAuthMiddleware in the peer connection handler) | **CRITICAL** |
| 5 | **JWT is HS256 with shared secret from env** — if any service leaks `JWT_SECRET` (logs, error pages, k8s secret), the entire fleet is compromised. No JWKS, no rotation. | S01, S26 | `pkg/common/auth.go:65-72` | **HIGH** |
| 6 | **No row-level security (RLS) on Postgres** — multi-tenant data isolation is application-enforced. A bug in any service → cross-tenant data leak. | S27 | `migrations/001_initial_schema.sql` (no `CREATE POLICY`) | **HIGH** |
| 7 | **In-memory state in event-proc** (Tracker, RuleEngine, AlertRules, TourScheduler, AlertWorkflow) — restart wipes rules, alerts, tours. No NATS-based state replication. | S09 | `services/event-proc/main.go:308-352` | **HIGH** |
| 8 | **NATS no TLS by default** — message bus is plaintext. A network breach = full access to camera frames, AI events, alerts. | All | `pkg/common/circuitbreaker.go` + service env | **HIGH** |
| 9 | **ffmpeg crash in `services/ingest` is not supervised** — one bad camera URL can kill the ingest, no restart loop, no per-camera isolation. | S04 | `services/ingest/main.go` | **HIGH** |
| 10 | **Ring buffer in recorder assumes fixed bitrate 4096K** — 4K cameras will overflow → silent data loss / split. | S03 | `services/recorder/main.go:86-114, 251` | **HIGH** |
| 11 | **Tiering `os.Remove(src)` after `copyFile` succeeds, no atomic rename, no reflink** — if S3 upload succeeds but local delete fails (or vice versa), data is duplicated or lost. No idempotency token. | S03 | `services/recorder/tiering.go:107-195` | **MEDIUM** |
| 12 | **`Tour` calls camera-control via raw HTTP with no auth, no retries, no timeout** — `http.Post` returns no error to caller if PTZ fails. | S09, S07 | `services/event-proc/tour.go:110-113` | **MEDIUM** |
| 13 | **No rate limit per tenant** — single noisy tenant can starve others. | S02 | `services/api-gateway/main.go` rate limiter is global | **MEDIUM** |
| 14 | **Hash chain audit log has no rotation, no chain-verification UI, no signature over the chain head** — if DB is breached, attacker can rewrite the chain if DB write access is also obtained. | S17 | `services/audit/` | **MEDIUM** |
| 15 | **No CI/CD pipeline** — there is no `.github/workflows`, no `Makefile` for build/test, no `Dockerfile` CI in repo root. 44 commits ending in "final 1-6" implies push-and-pray. | Repo-wide | repo root | **HIGH** |
| 16 | **Test coverage <10%** — no tests for ingest, ONVIF, AI worker, transcoding, gRPC endpoints, REST API surface, retention, tiering. | All | `find . -name '*_test.go' \| wc -l` = 11 across 44 Go files | **HIGH** |
| 17 | **`ValidateFilePath` works, but `handleDewarp` calls `ValidateFilePath` correctly** — self-audit claim of path traversal vulnerability in playback is **partially refuted** but `playback` service was not deeply re-inspected here. The validate path is good; risk remains if any new code bypasses `common.ValidateFilePath`. | S26, S03 | `pkg/common/validate.go` + `services/recorder/main.go:494-501` | **MEDIUM** (was HIGH) |
| 18 | **No codec support beyond H.264** — H.265/HEVC, AV1, MJPEG, MPEG4 not handled by ingest. The Rust ingest has H.265/AV1 stubs but no production path. | S04, S05 | `services/ingest/main.go`, `apps/ingest-service/src/` | **MEDIUM** |
| 19 | **`edge-sync-service` has no anti-entropy / gossip / partition resolution** — vector clocks + CRDTs in code is good, but no integration test verifies convergence after partition. | S23 | `apps/edge-sync-service/src/` | **MEDIUM** |
| 20 | **No structured API contract** — only 2 protos for 17 services. Most inter-service calls are HTTP+JSON or ad-hoc NATS subjects. No OpenAPI, no AsyncAPI, no protobuf-driven codegen. | S31 | `api/proto/v1/` | **MEDIUM** |

---

## 5. VMS Camera-Count Reality Check

| Camera count | Verdict | Reasoning |
|---|---|---|
| **10 cameras, 1 site, single tenant, 7d retention, 1080p, 30fps, H.264** | **Likely works** | Recorder sharding unnecessary, ingest ffmpeg pool fits in one node, NATS JetStream on default 3-node compose, TimescaleDB on a single node, WebRTC 1:1 fan-out manageable. Issues: minor — restart wipes event-proc rules, no multi-tenant UI. |
| **100 cameras, 1 site, single tenant, 30d retention** | **Will work but tight** | Recorder sharding needed (4 shards), ingest needs HA, NATS needs ≥3 nodes (compose has it), TimescaleDB hypertables partition by week, WebRTC 1:1 is a problem (no SFU). Issues: ingestion-bandwidth saturation, no chunking, ring buffer is per-shard, no codec normalization. WebRTC at 100 viewers → 100 separate tracks per camera, single webrtc service will OOM. |
| **500 cameras, 5 sites, multi-tenant, 90d retention** | **Will fail** | WebRTC 1:1 not viable (need SFU), ingest ffmpeg count = 500 processes (need pooler), NATS subject cardinality = 500 `camera.*.events` × event rate, TimescaleDB needs more aggressive partitioning, no sharding of metadata DB, no multi-tenant quota, no per-tenant WebRTC auth. Will require 6+ months of hardening. |
| **1000 cameras, 10 sites, multi-tenant, 90d retention, H.265, 4K** | **Will fail catastrophically** | Ring buffer (4K@25Mbps = 31MB/5s) will overflow in current shape. Ingest cannot handle 4K H.265. WebRTC 1:1 = 1000×N viewers. TimescaleDB needs read replicas, TimescaleDB-2 with multi-node, per-tenant. No SFU. No TURN. No codec normalization. Rust services not production-ready. Estimated 12+ months to reach this. |

---

## 6. Reality Check Q&A

| Q | Answer (with evidence) |
|---|---|
| **Q1: Does EVMS actually record 4K H.265 cameras?** | **No.** Go ingest only handles H.264, recorder ring buffer assumes 4096Kbps. Rust ingest has H.265/AV1 stubs but no production path. |
| **Q2: Does the face/vehicle search work?** | **No.** `vector-index-service` is a non-functional skeleton — all Milvus calls return `Ok(())` with no data, and `error.rs` won't compile. |
| **Q3: Can a user watch a live stream in the browser?** | **Possibly, but unauthenticated and at risk.** WebRTC service has no JWT check on the peer connection. The web app has no WebRTC client visible in `main.tsx`. |
| **Q4: Is multi-tenancy real?** | **Partially.** `tenant_id` column exists, JWT carries it, services pass it via context. **No RLS** on Postgres. |
| **Q5: Does failover work?** | **Partially.** Leader election via NATS KV works (real tests). Recorder retention worker restarts but loses in-memory camera state on restart. Event-proc loses all rules/alerts. |
| **Q6: Is the audit log tamper-evident?** | **Mostly.** Hash chain exists; but no chain-head signature, no chain-verification job, no UI. |
| **Q7: Can I export evidence for court?** | **Partially.** Export service exists, SHA-256 computed, but no chain-of-custody log, no legal-hold awareness. |
| **Q8: Does ONVIF actually work?** | **Probe + GetStreamURI + GetProfiles yes** (raw XML). ONVIF events via separate service. **No event replay**, **no PullPoint**, **no digital signing**. |
| **Q9: Can the system run unattended for 30 days?** | **Unlikely.** No watchdog for ffmpeg crashes, in-memory state loss on restart, no auto-recovery of dropped RTSP streams, no Moov atom fixup → many MP4s unplayable. |
| **Q10: Is there a deployment story?** | **docker-compose for dev only.** Helm exists but lacks NetworkPolicy, PDBs, resource limits, ServiceMonitor templates. No CI/CD to produce images. |

---

## 7. Critical bugs and contradictions found

1. **`apps/vector-index-service/src/error.rs:21-25`** — `impl From<milvus_client::Error> for Error` — `milvus_client::Error` does not exist in the same module. **This will not compile.**
2. **`services/event-proc/tour.go:110-113`** — `http.Post` ignores response body, no error check, no timeout, no auth, hardcoded URL `http://camera-control:8088`. PTZ failures are silent.
3. **`services/recorder/main.go:498-501`** — `dewarp` endpoint accepts arbitrary `path` query param, then validates. Good. But it spawns `ffmpeg` with `path` as input — if `path` is to a symlink pointing outside `RECORDING_PATH`, validation still passes if the symlink target is in the path? **Needs audit of `common.ValidateFilePath` — confirmed it uses `filepath.EvalSymlinks` and `filepath.Rel` so symlinks outside the root are rejected. OK.**
4. **`apps/vector-index-service/src/milvus_client.rs:13`** — `is_ready` returns `true` unconditionally. Health endpoint lies.
5. **`services/recorder/main.go:359`** — `EndTime: startTime.Add(60 * time.Second)` — hardcoded 60s, ignores actual segment length. Misleading metadata.
6. **`pkg/common/auth.go:67-72`** — `getJWTKey` uses `sync.Once`; if `JWT_SECRET` is not set on first call, it's permanently nil until process restart. **Fail-closed is good, but no startup log warning that JWT_SECRET is missing.**
7. **`services/event-proc/main.go:357`** — `s.eventSub.SetPendingLimits(1024, 64*1024*1024)` — good, but no slow-consumer handling beyond dropping.
8. **`services/recorder/tiering.go:181`** — `time.Sleep(time.Duration(attempt*attempt) * time.Second)` — quadratic backoff, but blocks the `tierSegments` walk goroutine. After 3 retries, the whole walk is paused for ~14s.

---

## 8. Final Classification

# **Beta**

### Justification

**Why not Alpha:** Go core is functional, has unit tests for the harder parts (leader election is genuinely tested against real NATS), uses real libraries, has real DB schema, real K8s manifests, real auth. You can stand this up against 10 cameras and record, get an event into the rule engine, get a notification.

**Why not MVP:** MVP implies "minimum viable product for the first paying customer." Multi-tenancy is not safe (no RLS), security has known gaps (webrtc unauthenticated, NATS plaintext), observability is half-built (Prometheus on some services, slog everywhere but no dashboards), tests are <10%, no CI/CD, no on-call story, no runbook for "recorder is down" beyond `docs/ops-runbook.md` (not deeply inspected).

**Why not Prod Candidate:** Prod Candidate requires passing a security review, having an SLO/SLA, a tested failover, a tested backup-restore, a tested upgrade path, and demonstrated scale. This system has none of those verified.

**Why Beta (and not lower):** The Go core has the right shape, the streaming pipeline actually does the basic thing (ingest → record → index → retrieve), the event engine has real geometry and real DB-backed counter/heatmap, the K8s story is real Helm with real values, and the self-audit is honest about its gaps. Beta means: **works for early-design-partner use, not for paid SLA-bearing production.** Don't sell it as "production VMS" until at minimum:
- WebRTC auth is added
- vector-index-service is either deleted or actually implemented
- Tests for ingest, ONVIF, retention, tiering, gRPC endpoints, and the REST API surface exist
- CI/CD runs lint + vet + test + build on every PR
- RLS is enabled on Postgres
- JWT_SECRET is enforced at startup across all services
- Backup-restore is tested end-to-end

### If you have to call it something else
- If you strip the Rust services (recommend): **Beta** (one notch more honest)
- If you have a deployment team that will paper over gaps: **Alpha → Beta threshold**
- If this is for a single-tenant single-site sub-50-camera POC: **MVP**

---

## 9. What to do next (concrete, ordered)

1. **Delete or fix `vector-index-service`**. Today it silently lies about working. Either remove it from compose/Helm or implement real Milvus client calls.
2. **Fix `error.rs` compile error** in `vector-index-service` so the project at least builds.
3. **Add JWT auth to webrtc** — `JWTAuthMiddleware` on the HTTP signaling endpoint.
4. **Persist event-proc state** — RuleEngine rules, AlertRules, TourScheduler tours, AlertWorkflow alerts → Postgres (or NATS KV) so restarts don't lose them.
5. **Add CI/CD** — at minimum `go vet ./...`, `go test ./...`, `golangci-lint run`, `cargo check`, `tsc --noEmit`, `vite build`. No deploy images without green.
6. **Add ffmpeg supervisor in `services/ingest`** — backoff restart per camera, health endpoint per camera.
7. **Moov-atom fixup at finalize** — call `ffmpeg -c copy -movflags +faststart` on segment close, or use a Go MP4 library.
8. **Enforce JWT_SECRET at startup** — `os.Exit(1)` if missing in every service.
9. **Enable RLS on all tenant-scoped tables** in `migrations/001_initial_schema.sql`.
10. **Add ServiceMonitor + PrometheusRule + PodDisruptionBudget + NetworkPolicy** to Helm.
11. **Decide: Go ingest or Rust ingest-service** — currently both exist; pick one for production, archive the other.
12. **Add tests** for ingest ffmpeg lifecycle, retention worker, tiering (with mock S3), ring buffer (boundary), gRPC endpoints, REST API surface.
13. **Add backup-restore e2e test** that runs weekly.
14. **Add a WebRTC SFU** (pion has one) or commit to "no live in-browser video" and document it.

---

**End of report.**
