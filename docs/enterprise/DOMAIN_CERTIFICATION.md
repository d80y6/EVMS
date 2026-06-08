# EVMS Domain Certification Report

## Certification Levels
- **Enterprise Ready**: Full implementation, tests, frontend, docs
- **Production Candidate**: Complete with minor gaps
- **Beta**: Core functional, some gaps
- **Alpha**: Basic implementation exists
- **Prototype**: Early stage / skeleton only
- **Not Started**: No implementation

---

## CORE VMS DOMAINS

| ID | Domain | Before | After | Evidence |
|----|--------|--------|-------|----------|
| CORE-01 | Camera Management | Beta | **Production Candidate** | Full CRUD, gRPC, frontend, tests |
| CORE-02 | Camera Discovery | Alpha | **Beta** | WS-Discovery, subnet, DB storage, scheduled scans |
| CORE-03 | ONVIF Protocol | Beta | **Production Candidate** | 10 sub-packages, full SOAP client, tests |
| CORE-04 | Stream Ingestion | Beta | **Production Candidate** | Go + Rust engines, NATS, tests |
| CORE-05 | Media Pipeline | Alpha | **Beta** | GStreamer pipeline, thumbnail, dewarping |
| CORE-06 | WebRTC Streaming | Beta | **Production Candidate** | Pion, STUN/TURN, NATS, frontend, tests |
| CORE-07 | Recording Engine | Beta | **Production Candidate** | Full recording, retention, tiering, leader election, tests |
| CORE-08 | Playback Engine | Beta | **Production Candidate** | Range requests, security tests, frontend |
| CORE-09 | Timeline Service | Missing | **Beta** | Timeline aggregation, recording segments, API endpoints, frontend |
| CORE-10 | Storage Management | Alpha | **Beta** | Tiering, metrics, frontend, per-camera retention |
| CORE-11 | PTZ Control | Beta | **Production Candidate** | 20+ protocols, presets, IO, frontend |
| CORE-12 | Thumbnails | Beta | **Production Candidate** | FFmpeg, caching, API routes |
| CORE-13 | Export Engine | Beta | **Production Candidate** | FFmpeg concat, watermark, SHA256, frontend, evidence export |
| CORE-14 | Bookmarking | Beta | **Production Candidate** | CRUD, frontend, API routes |
| CORE-15 | Dewarping/Fisheye | Alpha | **Beta** | FFmpeg lens correction, API |
| CORE-16 | Frame Analysis/Scrub | Missing | **Beta** | Frame indexing, motion scores, scene change detection, API |
| CORE-17 | Multi-Stream Support | Alpha | **Beta** | WebRTC streaming, audio channel support |
| CORE-18 | Audio Management | Missing | **Beta** | Audio metadata, level monitoring, two-way audio relay, playback |
| CORE-19 | Camera Provisioning | Alpha | **Beta** | ONVIF provisioning, credential management |
| CORE-20 | Retention Management | Alpha | **Beta** | Per-camera retention, archive tiers, motion retention |

---

## AI PLATFORM DOMAINS

| ID | Domain | Before | After | Evidence |
|----|--------|--------|-------|----------|
| AI-01 | Object Detection | Beta | **Production Candidate** | YOLOv8, Triton, Python + Go workers |
| AI-02 | Facial Recognition | Alpha | **Beta** | DeepStack, watchlist, detections |
| AI-03 | License Plate Recognition | Alpha | **Beta** | LPR implementation, tests |
| AI-04 | People Counting | Beta | **Production Candidate** | Zone crossing, aggregation, hypertable, tests |
| AI-05 | Heatmap Generation | Alpha | **Beta** | Grid-based, hypertable |
| AI-06 | Tripwire Detection | Beta | **Production Candidate** | Line crossing, tests |
| AI-07 | Intrusion Detection | Missing | **Beta** | Polygon zones, direction, ray-casting, events |
| AI-08 | Object Tracking | Alpha | **Beta** | IoU tracking, multi-camera track path |
| AI-09 | Metadata Management | Beta | **Production Candidate** | Event metadata, vector storage, tests |
| AI-10 | Vector Search | Alpha | **Beta** | pgvector similarity search |
| AI-11 | Facial Watchlist | Alpha | **Beta** | Face watchlist tables |
| AI-12 | Loitering Detection | Missing | **Beta** | Dwell time tracking, zones, events |
| AI-13 | Abandoned Object | Missing | **Beta** | IoU stationarity, zones, events |
| AI-14 | Crowd Detection | Missing | **Not Started** | |
| AI-15 | Tailgating Detection | Missing | **Not Started** | |
| AI-16 | Scene Change Detection | Missing | **Not Started** | |
| AI-17 | Audio Detection | Missing | **Not Started** | |
| AI-18 | Predictive Analytics | Missing | **Not Started** | |
| AI-19 | Forensics Search | Missing | **Beta** | Multi-attribute, vector, track path, export, frontend |
| AI-20 | AI Model Management | Missing | **Not Started** | |

---

## SECURITY DOMAINS

| ID | Domain | Before | After | Evidence |
|----|--------|--------|-------|----------|
| SEC-01 | Authentication | Beta | **Production Candidate** | JWT, LDAP, MFA, SSO/OIDC/SAML |
| SEC-02 | Authorization/RBAC | Beta | **Production Candidate** | 3 roles, route enforcement |
| SEC-03 | Multi-Tenancy | Beta | **Production Candidate** | Tenant isolation, JWT context |
| SEC-04 | Audit Logging | Beta | **Production Candidate** | Hash chain, verification, frontend, tests |
| SEC-05 | Encryption at Rest | Alpha | **Beta** | AES-256-GCM for credentials |
| SEC-06 | Encryption in Transit | Beta | **Production Candidate** | TLS, mTLS support |
| SEC-07 | Secrets Management | Alpha | **Beta** | External Secrets Operator |
| SEC-08 | Password Policies | Missing | **Beta** | Complexity, expiry, history, lockout |
| SEC-09 | MFA/2FA | Missing | **Beta** | TOTP, recovery codes, frontend |
| SEC-10 | SSO/SAML/OIDC | Missing | **Beta** | OIDC, SAML, PKCE, auto-provision |
| SEC-11 | LDAP/AD Integration | Beta | **Production Candidate** | LDAP auth, OpenLDAP |
| SEC-12 | API Key Management | Missing | **Beta** | Key generation, scopes, rotation, frontend |
| SEC-13 | IP Allowlisting | Missing | **Not Started** | |
| SEC-14 | Session Management | Alpha | **Beta** | Refresh rotation, revocation, concurrent limits |
| SEC-15 | Rate Limiting | Alpha | **Beta** | Login rate limiting |
| SEC-16 | CSRF Protection | Missing | **Beta** | Double-submit cookie, token management |
| SEC-17 | FIPS Compliance | Alpha | **Beta** | FIPS builder Dockerfile |
| SEC-18 | Video Watermarking | Alpha | **Beta** | Export watermarking |
| SEC-19 | Chain of Custody | Alpha | **Beta** | Audit hash chain |

---

## OPERATIONS DOMAINS

| ID | Domain | Before | After | Evidence |
|----|--------|--------|-------|----------|
| OPS-01 | Event Management | Beta | **Production Candidate** | Pipeline, tracking, frontend |
| OPS-02 | Notification System | Beta | **Production Candidate** | Email, SMS, Push, Webhooks, frontend |
| OPS-03 | Alert Rules Engine | Beta | **Production Candidate** | Rule engine, workflow, tests |
| OPS-04 | Observability | Beta | **Production Candidate** | OTel, Prometheus, slog, Loki |
| OPS-05 | Monitoring/Metrics | Beta | **Production Candidate** | 50+ metrics, Grafana, Prometheus |
| OPS-06 | Distributed Tracing | Alpha | **Beta** | OTel collector, OTLP |
| OPS-07 | Health Checks | Beta | **Production Candidate** | DB/NATS checkers, HTTP endpoints, frontend |
| OPS-08 | Backup & Recovery | Alpha | **Beta** | CronJob, restore, WAL archive |
| OPS-09 | Audit Trail Review | Beta | **Production Candidate** | Hash chain verify, frontend |
| OPS-10 | System Logging | Beta | **Production Candidate** | JSON structured, Loki |
| OPS-11 | Webhook System | Beta | **Production Candidate** | CRUD, dispatch, retry, frontend |
| OPS-12 | Email/SMS/Push | Missing | **Beta** | SMTP, Twilio, FCM, templates |
| OPS-13 | Incident Response | Missing | **Beta** | Incident workflow, escalation, notes, timeline |
| OPS-14 | SLA Management | Missing | **Not Started** | |
| OPS-15 | System Config Mgmt | Missing | **Beta** | Config versioning, audit, import/export |

---

## DISTRIBUTED SYSTEMS DOMAINS

| ID | Domain | Before | After | Evidence |
|----|--------|--------|-------|----------|
| DIST-01 | Federation | Missing | **Not Started** | |
| DIST-02 | Edge Nodes | Alpha | **Alpha** | Edge sync service (Rust) |
| DIST-03 | Cluster Coordination | Alpha | **Alpha** | NATS leader election, sharding |
| DIST-04 | High Availability | Alpha | **Alpha** | K8s replicas, HPA, PDB |
| DIST-05 | Failover | Alpha | **Alpha** | Leader election, JetStream |
| DIST-06 | Load Balancing | Beta | **Beta** | K8s Service, API gateway |
| DIST-07 | Data Replication | Missing | **Not Started** | |
| DIST-08 | Offline/Store-Forward | Alpha | **Alpha** | Edge sync, CRDT |
| DIST-09 | WAN Optimization | Missing | **Not Started** | |
| DIST-10 | Hybrid Cloud | Missing | **Not Started** | |

---

## INFRASTRUCTURE DOMAINS

| ID | Domain | Before | After | Evidence |
|----|--------|--------|-------|----------|
| INFRA-01 | API Gateway | Beta | **Production Candidate** | Reverse proxy, JWT, rate limit, all routes |
| INFRA-02 | Service Mesh | Missing | **Not Started** | |
| INFRA-03 | Container Orchestration | Beta | **Production Candidate** | Docker Compose, Helm, K8s |
| INFRA-04 | CI/CD Pipeline | Beta | **Production Candidate** | GitHub Actions, Trivy, Docker build |
| INFRA-05 | Infrastructure as Code | Alpha | **Beta** | Helm charts |
| INFRA-06 | Service Discovery | Alpha | **Beta** | K8s DNS, service names |
| INFRA-07 | Secret Store | Alpha | **Beta** | External Secrets Operator |
| INFRA-08 | Certificate Management | Alpha | **Beta** | Internal certs, cert template |

---

## ENTERPRISE DOMAINS

| ID | Domain | Before | After | Evidence |
|----|--------|--------|-------|----------|
| ENT-01 | Licensing | Missing | **Not Started** | |
| ENT-02 | Fleet Management | Missing | **Not Started** | |
| ENT-03 | Device Health | Missing | **Not Started** | |
| ENT-04 | Video Walls | Missing | **Not Started** | |
| ENT-05 | Incident Management | Missing | **Beta** | Workflow, escalation, notes, timeline |
| ENT-06 | Evidence Management | Missing | **Beta** | Cases, lockers, items, sharing, chain of custody |
| ENT-07 | Reporting Engine | Missing | **Not Started** | |
| ENT-08 | Compliance Management | Missing | **Not Started** | |
| ENT-09 | Workflow Automation | Missing | **Not Started** | |
| ENT-10 | Maps & GIS | Alpha | **Beta** | Leaflet, map page, camera overlays |
| ENT-11 | Integrations Platform | Missing | **Not Started** | |
| ENT-12 | Audio Platform | Missing | **Beta** | Audio recording, playback, two-way relay |
| ENT-13 | Intercom/Two-Way Audio | Missing | **Beta** | WebRTC audio channel |
| ENT-14 | Access Control Integration | Missing | **Not Started** | |
| ENT-15 | POS/POS Integration | Alpha | **Beta** | POS ingestion, frontend |
| ENT-16 | Dashboard & BI | Missing | **Not Started** | |
| ENT-17 | Tenant Self-Service | Missing | **Not Started** | |
| ENT-18 | API Public/Partner | Missing | **Not Started** | |
| ENT-19 | Webhook Platform | Beta | **Production Candidate** | Webhook CRUD, dispatch, retry |
| ENT-20 | Mobile Client | Missing | **Not Started** | |
| ENT-21 | Desktop Client | Missing | **Not Started** | |
| ENT-22 | SDK/Plugin System | Missing | **Not Started** | |
| ENT-23 | Data Export/Import | Missing | **Not Started** | |
| ENT-24 | Custom Dashboards | Missing | **Not Started** | |
| ENT-25 | Alarm Management | Alpha | **Beta** | Alert rules, frontend, incident integration |
