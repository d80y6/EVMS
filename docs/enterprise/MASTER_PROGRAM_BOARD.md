# EVMS Master Program Board

## Program Status Overview

| Metric | Value |
|--------|-------|
| Total Domains | 82 |
| Enterprise Ready | 0 |
| Production Candidate | 33 |
| Beta | 32 |
| Alpha | 4 |
| Prototype | 0 |
| Not Started | 13 |
| Backlog Items Implemented | 64 |
| New Go Files | 37 |
| New Frontend Pages | 12 |
| New Migrations | 14 |
| Total Lines of New Code | ~14,300 |

---

## Domain Status Matrix

### CORE VMS (20 domains)
| ID | Domain | Certification | Risk | Dependencies | Backlog Status |
|----|--------|---------------|------|--------------|----------------|
| CORE-01 | Camera Management | Production Candidate | Low | None | Closed |
| CORE-02 | Camera Discovery | Beta | Low | CORE-03 | Closed |
| CORE-03 | ONVIF Protocol | Production Candidate | Low | None | Closed |
| CORE-04 | Stream Ingestion | Production Candidate | Low | CORE-03 | Closed |
| CORE-05 | Media Pipeline | Beta | Medium | CORE-04 | Open (transcoding) |
| CORE-06 | WebRTC Streaming | Production Candidate | Low | CORE-04 | Closed |
| CORE-07 | Recording Engine | Production Candidate | Low | CORE-04, CORE-20 | Closed |
| CORE-08 | Playback Engine | Production Candidate | Low | CORE-07 | Closed |
| CORE-09 | Timeline Service | Beta | Low | CORE-07 | Closed |
| CORE-10 | Storage Management | Beta | Low | CORE-07 | Open (forecasting) |
| CORE-11 | PTZ Control | Production Candidate | Low | CORE-03 | Closed |
| CORE-12 | Thumbnails | Production Candidate | Low | CORE-07 | Closed |
| CORE-13 | Export Engine | Production Candidate | Low | CORE-07 | Closed |
| CORE-14 | Bookmarking | Production Candidate | Low | CORE-07 | Closed |
| CORE-15 | Dewarping/Fisheye | Beta | Low | CORE-05 | Open (real-time) |
| CORE-16 | Frame Analysis/Scrub | Beta | Low | CORE-07 | Closed |
| CORE-17 | Multi-Stream Support | Beta | Low | CORE-06 | Open (adaptive) |
| CORE-18 | Audio Management | Beta | Low | CORE-06 | Closed |
| CORE-19 | Camera Provisioning | Beta | Low | CORE-03 | Open (bulk) |
| CORE-20 | Retention Management | Beta | Low | CORE-07 | Closed |

### AI PLATFORM (20 domains)
| ID | Domain | Certification | Risk | Dependencies | Backlog Status |
|----|--------|---------------|------|--------------|----------------|
| AI-01 | Object Detection | Production Candidate | Low | CORE-04 | Closed |
| AI-02 | Facial Recognition | Beta | Low | AI-01 | Closed |
| AI-03 | License Plate Recognition | Beta | Low | AI-01 | Open (watchlist) |
| AI-04 | People Counting | Production Candidate | Low | AI-01 | Closed |
| AI-05 | Heatmap Generation | Beta | Medium | AI-04 | Open (frontend) |
| AI-06 | Tripwire Detection | Production Candidate | Low | AI-01 | Closed |
| AI-07 | Intrusion Detection | Beta | Low | AI-01 | Closed |
| AI-08 | Object Tracking | Beta | Low | AI-01 | Open (ReID) |
| AI-09 | Metadata Management | Production Candidate | Low | AI-01 | Closed |
| AI-10 | Vector Search | Beta | Low | AI-09 | Open (NLP) |
| AI-11 | Facial Watchlist | Beta | Low | AI-02 | Open (import) |
| AI-12 | Loitering Detection | Beta | Low | AI-01 | Closed |
| AI-13 | Abandoned Object | Beta | Low | AI-01 | Closed |
| AI-14 | Crowd Detection | Not Started | Medium | AI-01 | - |
| AI-15 | Tailgating Detection | Not Started | Medium | AI-08 | - |
| AI-16 | Scene Change Detection | Not Started | Low | AI-01 | - |
| AI-17 | Audio Detection | Not Started | Medium | CORE-18 | - |
| AI-18 | Predictive Analytics | Not Started | High | AI-20 | - |
| AI-19 | Forensics Search | Beta | Low | AI-09, AI-10 | Closed |
| AI-20 | AI Model Management | Not Started | High | AI-01 | - |

### SECURITY (19 domains)
| ID | Domain | Certification | Risk | Dependencies | Backlog Status |
|----|--------|---------------|------|--------------|----------------|
| SEC-01 | Authentication | Production Candidate | Low | None | Closed |
| SEC-02 | Authorization/RBAC | Production Candidate | Low | SEC-01 | Closed |
| SEC-03 | Multi-Tenancy | Production Candidate | Low | SEC-01 | Closed |
| SEC-04 | Audit Logging | Production Candidate | Low | SEC-01 | Closed |
| SEC-05 | Encryption at Rest | Beta | Low | None | Open (TDE) |
| SEC-06 | Encryption in Transit | Production Candidate | Low | None | Closed |
| SEC-07 | Secrets Management | Beta | Low | None | Open (Vault) |
| SEC-08 | Password Policies | Beta | Low | SEC-01 | Closed |
| SEC-09 | MFA/2FA | Beta | Low | SEC-01 | Closed |
| SEC-10 | SSO/SAML/OIDC | Beta | Low | SEC-01 | Closed |
| SEC-11 | LDAP/AD Integration | Production Candidate | Low | SEC-01 | Closed |
| SEC-12 | API Key Management | Beta | Low | SEC-01 | Closed |
| SEC-13 | IP Allowlisting | Not Started | Low | SEC-01 | - |
| SEC-14 | Session Management | Beta | Low | SEC-01 | Closed |
| SEC-15 | Rate Limiting | Beta | Low | INFRA-01 | Open (per-user) |
| SEC-16 | CSRF Protection | Beta | Low | INFRA-01 | Closed |
| SEC-17 | FIPS Compliance | Beta | Medium | SEC-05 | Open (FIPS crypto) |
| SEC-18 | Video Watermarking | Beta | Low | CORE-13 | Open (live) |
| SEC-19 | Chain of Custody | Beta | Low | SEC-04 | Open (formal) |

### OPERATIONS (15 domains)
| ID | Domain | Certification | Risk | Dependencies | Backlog Status |
|----|--------|---------------|------|--------------|----------------|
| OPS-01 | Event Management | Production Candidate | Low | CORE-07, AI-01 | Closed |
| OPS-02 | Notification System | Production Candidate | Low | OPS-03 | Closed |
| OPS-03 | Alert Rules Engine | Production Candidate | Low | OPS-01 | Closed |
| OPS-04 | Observability | Production Candidate | Low | None | Closed |
| OPS-05 | Monitoring/Metrics | Production Candidate | Low | OPS-04 | Closed |
| OPS-06 | Distributed Tracing | Beta | Low | OPS-04 | Open (sampling) |
| OPS-07 | Health Checks | Production Candidate | Low | None | Closed |
| OPS-08 | Backup & Recovery | Beta | Medium | None | Open (verify) |
| OPS-09 | Audit Trail Review | Production Candidate | Low | SEC-04 | Closed |
| OPS-10 | System Logging | Production Candidate | Low | OPS-04 | Closed |
| OPS-11 | Webhook System | Production Candidate | Low | OPS-02 | Closed |
| OPS-12 | Email/SMS/Push | Beta | Low | OPS-02 | Closed |
| OPS-13 | Incident Response | Beta | Low | OPS-01 | Closed |
| OPS-14 | SLA Management | Not Started | Medium | OPS-04 | - |
| OPS-15 | System Config Mgmt | Beta | Low | None | Closed |

### DISTRIBUTED SYSTEMS (10 domains)
| ID | Domain | Certification | Risk | Dependencies | Backlog Status |
|----|--------|---------------|------|--------------|----------------|
| DIST-01 | Federation | Not Started | High | CORE-07, SEC-01 | - |
| DIST-02 | Edge Nodes | Alpha | Medium | DIST-08 | Open (registration) |
| DIST-03 | Cluster Coordination | Alpha | Medium | DIST-04 | Open (consensus) |
| DIST-04 | High Availability | Alpha | Medium | K8s | Open (multi-region) |
| DIST-05 | Failover | Alpha | Medium | DIST-03 | Open (testing) |
| DIST-06 | Load Balancing | Beta | Low | INFRA-01 | Closed |
| DIST-07 | Data Replication | Not Started | High | DIST-10 | - |
| DIST-08 | Offline/Store-Forward | Alpha | Medium | DIST-02 | Open (queue mgmt) |
| DIST-09 | WAN Optimization | Not Started | Medium | CORE-04 | - |
| DIST-10 | Hybrid Cloud | Not Started | High | CORE-10 | - |

### INFRASTRUCTURE (8 domains)
| ID | Domain | Certification | Risk | Dependencies | Backlog Status |
|----|--------|---------------|------|--------------|----------------|
| INFRA-01 | API Gateway | Production Candidate | Low | None | Closed |
| INFRA-02 | Service Mesh | Not Started | Medium | K8s | - |
| INFRA-03 | Container Orchestration | Production Candidate | Low | None | Closed |
| INFRA-04 | CI/CD Pipeline | Production Candidate | Low | None | Closed |
| INFRA-05 | Infrastructure as Code | Beta | Low | INFRA-03 | Open (Terraform) |
| INFRA-06 | Service Discovery | Beta | Low | INFRA-03 | Closed |
| INFRA-07 | Secret Store | Beta | Low | INFRA-03 | Open (Vault) |
| INFRA-08 | Certificate Management | Beta | Low | INFRA-03 | Open (cert-manager) |

### ENTERPRISE (25 domains)
| ID | Domain | Certification | Risk | Dependencies | Backlog Status |
|----|--------|---------------|------|--------------|----------------|
| ENT-01 | Licensing | Not Started | High | SEC-01 | - |
| ENT-02 | Fleet Management | Not Started | High | CORE-01 | - |
| ENT-03 | Device Health | Not Started | Medium | CORE-01 | - |
| ENT-04 | Video Walls | Not Started | Medium | CORE-06 | - |
| ENT-05 | Incident Management | Beta | Low | OPS-01 | Closed |
| ENT-06 | Evidence Management | Beta | Low | CORE-07, CORE-13 | Closed |
| ENT-07 | Reporting Engine | Not Started | Medium | All | - |
| ENT-08 | Compliance Management | Not Started | High | ENT-07 | - |
| ENT-09 | Workflow Automation | Not Started | High | OPS-03 | - |
| ENT-10 | Maps & GIS | Beta | Low | None | Open (indoor) |
| ENT-11 | Integrations Platform | Not Started | High | SEC-12 | - |
| ENT-12 | Audio Platform | Beta | Low | CORE-18 | Closed |
| ENT-13 | Intercom/Two-Way Audio | Beta | Low | CORE-18 | Closed |
| ENT-14 | Access Control Integration | Not Started | High | CORE-01 | - |
| ENT-15 | POS/POS Integration | Beta | Low | None | Open (correlation) |
| ENT-16 | Dashboard & BI | Not Started | Medium | All | - |
| ENT-17 | Tenant Self-Service | Not Started | Medium | SEC-03 | - |
| ENT-18 | API Public/Partner | Not Started | Medium | SEC-12 | - |
| ENT-19 | Webhook Platform | Production Candidate | Low | OPS-11 | Closed |
| ENT-20 | Mobile Client | Not Started | High | CORE-06 | - |
| ENT-21 | Desktop Client | Not Started | Medium | CORE-06 | - |
| ENT-22 | SDK/Plugin System | Not Started | High | SEC-12 | - |
| ENT-23 | Data Export/Import | Not Started | Medium | CORE-13 | - |
| ENT-24 | Custom Dashboards | Not Started | Medium | All | - |
| ENT-25 | Alarm Management | Beta | Low | OPS-03 | Open (escalation) |

---

## Risk Register

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| Distributed consensus complexity | High | Medium | Use proven technology (NATS, Raft) |
| AI model management requires ML infra | High | Medium | Start with model registry; add training later |
| Federation requires cross-site networking | High | Medium | Design federation API for NATS bridging |
| Multi-region HA is expensive to test | Medium | Medium | Test in cloud provider first with 2 regions |
| Mobile client native development | High | Medium | Start with PWA enhancement; native later |
| Access control integration is proprietary | Medium | High | Build generic integration framework first |

---

## Dependency Graph (Critical Path)

```
SEC-01 (Auth) ──> SEC-02 (RBAC) ──> SEC-03 (Multi-Tenancy)
                                        │
CORE-03 (ONVIF) ──> CORE-04 (Ingest) ──> CORE-07 (Recording) ──> CORE-08 (Playback)
                     │                                           │
                     └──> AI-01 (Detection) ──> AI-06 (Tripwire) │
                                                AI-07 (Intrusion)│
                                                AI-12 (Loitering)│
                                                AI-19 (Forensics)│
                                                                  │
                     CORE-07 ─────────────────────────────────────┘
                     │
                     └──> CORE-13 (Export) ──> ENT-06 (Evidence)
                     └──> CORE-09 (Timeline)
                     └──> CORE-16 (Frame Analysis)
                     └──> CORE-20 (Retention)

OPS-01 (Events) ──> OPS-03 (Alerts) ──> ENT-05 (Incidents)
                                        OPS-02 (Notifications) ──> OPS-12 (Channels)
```

## Next Wave Priorities

### Wave 5 (Next): Advanced AI & Enterprise
1. AI-20: AI Model Management (model registry, versioning)
2. ENT-07: Reporting Engine (report templates, scheduling)
3. AI-14: Crowd Detection 
4. AI-15: Tailgating Detection
5. SEC-13: IP Allowlisting

### Wave 6: Distributed Systems
1. DIST-01: Federation (cross-site)
2. DIST-07: Data Replication
3. DIST-04: HA enhancement (multi-region)
4. DIST-09: WAN Optimization

### Wave 7: Platform Expansion
1. ENT-01: Licensing
2. ENT-20: Mobile Client (PWA enhancement)
3. ENT-22: SDK/Plugin System
4. ENT-04: Video Walls
5. INFRA-02: Service Mesh
