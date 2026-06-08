# EVMS Complete Enterprise VMS Domain Map

## Domain Categories & Master List

| # | Domain | Category | Priority | Source Platforms |
|---|--------|----------|----------|-----------------|
| CORE-01 | Camera Management | Core VMS | Critical | All |
| CORE-02 | Camera Discovery | Core VMS | Critical | All |
| CORE-03 | ONVIF Protocol | Core VMS | Critical | All |
| CORE-04 | Stream Ingestion | Core VMS | Critical | All |
| CORE-05 | Media Pipeline | Core VMS | Critical | All |
| CORE-06 | WebRTC Streaming | Core VMS | Critical | All |
| CORE-07 | Recording Engine | Core VMS | Critical | All |
| CORE-08 | Playback Engine | Core VMS | Critical | All |
| CORE-09 | Timeline Service | Core VMS | Critical | All |
| CORE-10 | Storage Management | Core VMS | Critical | All |
| CORE-11 | PTZ Control | Core VMS | High | All |
| CORE-12 | Thumbnails | Core VMS | High | All |
| CORE-13 | Export Engine | Core VMS | High | All |
| CORE-14 | Bookmarking | Core VMS | Medium | All |
| CORE-15 | Dewarping/Fisheye | Core VMS | Medium | Milestone, Genetec |
| CORE-16 | Frame Analysis/Scrub | Core VMS | Medium | Milestone, Genetec |
| CORE-17 | Multi-Stream Support | Core VMS | High | All |
| CORE-18 | Audio Management | Core VMS | Medium | Genetec, Avigilon |
| CORE-19 | Camera Provisioning | Core VMS | High | All |
| CORE-20 | Retention Management | Core VMS | Critical | All |
| AI-01 | Object Detection | AI Platform | Critical | BriefCam, Avigilon |
| AI-02 | Facial Recognition | AI Platform | High | BriefCam, Avigilon |
| AI-03 | License Plate Recognition | AI Platform | High | BriefCam, Avigilon |
| AI-04 | People Counting | AI Platform | High | BriefCam, Avigilon |
| AI-05 | Heatmap Generation | AI Platform | Medium | BriefCam |
| AI-06 | Tripwire Detection | AI Platform | High | All |
| AI-07 | Intrusion Detection | AI Platform | High | All |
| AI-08 | Object Tracking | AI Platform | Critical | BriefCam, Avigilon |
| AI-09 | Metadata Management | AI Platform | Critical | All |
| AI-10 | Vector Search | AI Platform | High | Avigilon, Verkada |
| AI-11 | Facial Watchlist | AI Platform | Medium | Avigilon |
| AI-12 | Loitering Detection | AI Platform | Medium | Avigilon, Qognify |
| AI-13 | Abandoned Object | AI Platform | Medium | Qognify |
| AI-14 | Crowd Detection | AI Platform | Medium | BriefCam |
| AI-15 | Tailgating Detection | AI Platform | Medium | Avigilon, Qognify |
| AI-16 | Scene Change Detection | AI Platform | Medium | All |
| AI-17 | Audio Detection | AI Platform | Medium | Qognify |
| AI-18 | Predictive Analytics | AI Platform | Low | BriefCam |
| AI-19 | Forensics Search | AI Platform | High | BriefCam, Avigilon |
| AI-20 | AI Model Management | AI Platform | Medium | BriefCam |
| SEC-01 | Authentication | Security | Critical | All |
| SEC-02 | Authorization/RBAC | Security | Critical | All |
| SEC-03 | Multi-Tenancy | Security | Critical | All |
| SEC-04 | Audit Logging | Security | Critical | All |
| SEC-05 | Encryption at Rest | Security | Critical | All |
| SEC-06 | Encryption in Transit | Security | Critical | All |
| SEC-07 | Secrets Management | Security | Critical | All |
| SEC-08 | Password Policies | Security | High | All |
| SEC-09 | MFA/2FA | Security | High | Verkada, Eagle Eye |
| SEC-10 | SSO/SAML/OIDC | Security | High | Genetec, Avigilon |
| SEC-11 | LDAP/AD Integration | Security | Critical | All |
| SEC-12 | API Key Management | Security | High | Verkada, Eagle Eye |
| SEC-13 | IP Allowlisting | Security | Medium | Genetec |
| SEC-14 | Session Management | Security | Critical | All |
| SEC-15 | Rate Limiting | Security | High | All |
| SEC-16 | CSRF Protection | Security | High | All |
| SEC-17 | FIPS Compliance | Security | Medium | Genetec |
| SEC-18 | Video Watermarking | Security | Medium | Milestone |
| SEC-19 | Chain of Custody | Security | Medium | Genetec |
| OPS-01 | Event Management | Operations | Critical | All |
| OPS-02 | Notification System | Operations | Critical | All |
| OPS-03 | Alert Rules Engine | Operations | Critical | All |
| OPS-04 | Observability | Operations | Critical | All |
| OPS-05 | Monitoring/Metrics | Operations | Critical | All |
| OPS-06 | Distributed Tracing | Operations | High | All |
| OPS-07 | Health Checks | Operations | Critical | All |
| OPS-08 | Backup & Recovery | Operations | Critical | All |
| OPS-09 | Audit Trail Review | Operations | High | All |
| OPS-10 | System Logging | Operations | Critical | All |
| OPS-11 | Webhook System | Operations | High | All |
| OPS-12 | Email/SMS/Push | Operations | High | All |
| OPS-13 | Incident Response | Operations | Medium | Genetec, Qognify |
| OPS-14 | SLA Management | Operations | Low | Enterprise |
| OPS-15 | System Configuration Mgmt | Operations | High | All |
| DIST-01 | Federation | Distributed | High | Milestone, Genetec |
| DIST-02 | Edge Nodes | Distributed | High | Verkada, Eagle Eye |
| DIST-03 | Cluster Coordination | Distributed | Critical | All |
| DIST-04 | High Availability | Distributed | Critical | All |
| DIST-05 | Failover | Distributed | Critical | All |
| DIST-06 | Load Balancing | Distributed | High | All |
| DIST-07 | Data Replication | Distributed | High | All |
| DIST-08 | Offline/Store-Forward | Distributed | High | Verkada, Eagle Eye |
| DIST-09 | WAN Optimization | Distributed | Medium | Milestone |
| DIST-10 | Hybrid Cloud | Distributed | Medium | Verkada, Eagle Eye |
| INFRA-01 | API Gateway | Infrastructure | Critical | All |
| INFRA-02 | Service Mesh | Infrastructure | High | Genetec |
| INFRA-03 | Container Orchestration | Infrastructure | Critical | All |
| INFRA-04 | CI/CD Pipeline | Infrastructure | Critical | All |
| INFRA-05 | Infrastructure as Code | Infrastructure | High | All |
| INFRA-06 | Service Discovery | Infrastructure | Critical | All |
| INFRA-07 | Secret Store | Infrastructure | Critical | All |
| INFRA-08 | Certificate Management | Infrastructure | High | All |
| ENT-01 | Licensing | Enterprise | High | All |
| ENT-02 | Fleet Management | Enterprise | Medium | Genetec, Eagle Eye |
| ENT-03 | Device Health | Enterprise | High | All |
| ENT-04 | Video Walls | Enterprise | Medium | Milestone, Genetec |
| ENT-05 | Incident Management | Enterprise | Medium | Genetec, Qognify |
| ENT-06 | Evidence Management | Enterprise | High | Genetec, Milestone |
| ENT-07 | Reporting Engine | Enterprise | Medium | All |
| ENT-08 | Compliance Management | Enterprise | Medium | Genetec |
| ENT-09 | Workflow Automation | Enterprise | Medium | Genetec, Qognify |
| ENT-10 | Maps & GIS | Enterprise | High | All |
| ENT-11 | Integrations Platform | Enterprise | High | All |
| ENT-12 | Audio Platform | Enterprise | Medium | Genetec, Avigilon |
| ENT-13 | Intercom/Two-Way Audio | Enterprise | Medium | Avigilon, Eagle Eye |
| ENT-14 | Access Control Integration | Enterprise | Medium | Genetec, Avigilon |
| ENT-15 | POS/POS Integration | Enterprise | Medium | All |
| ENT-16 | Dashboard & BI | Enterprise | Medium | All |
| ENT-17 | Tenant Self-Service | Enterprise | Medium | Verkada, Eagle Eye |
| ENT-18 | API Public/Partner | Enterprise | High | Verkada, Eagle Eye |
| ENT-19 | Webhook Platform | Enterprise | High | Verkada |
| ENT-20 | Mobile Client | Enterprise | High | All |
| ENT-21 | Desktop Client | Enterprise | High | Milestone, Genetec |
| ENT-22 | SDK/Plugin System | Enterprise | Medium | Milestone, Genetec |
| ENT-23 | Data Export/Import | Enterprise | Medium | All |
| ENT-24 | Custom Dashboards | Enterprise | Medium | Genetec |
| ENT-25 | Alarm Management | Enterprise | High | All |
