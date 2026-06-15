# EVMS Program Board — Production Candidate Tracker

**Date:** 2026-06-12
**Status:** Alpha → Production Candidate gate in progress
**Build:** All 14 Makefile services compile
**Test Suites:** 17/17 Go packages pass | 3/3 web test files pass
**Total Code:** 35,417 Go LOC (services + pkg) + 16,114 TS/TSX (web) = 51,531 LOC

---

## Domain Completion Matrix

| Domain | Code | Tests | Build | Runtime | AC | Certified |
|--------|------|-------|-------|---------|----|-----------|
| 1. auth | ✓ 7 Go files 33K | ✓ 1 file 328 lines | ✓ | ⏳ | ⏳ | ❌ |
| 2. camera-mgmt | ✓ 2 Go files | ✓ 1 file | ✓ | ⏳ | ⏳ | ❌ |
| 3. recorder | ✓ 12 Go files 24K | ✓ 2 files | ✓ | ⏳ | ⏳ | ❌ |
| 4. playback | ✓ 3 Go files | ✓ 2 files | ✓ | ⏳ | ⏳ | ❌ |
| 5. webrtc | ✓ 2 Go files | ✓ 1 file | ✓ | ⏳ | ⏳ | ❌ |
| 6. camera-control | ✓ 1 Go file | ❌ 0 tests | ✓ | ⏳ | ⏳ | ❌ |
| 7. thumbnails | ✓ 1 Go file | ❌ 0 tests | ✓ | ⏳ | ⏳ | ❌ |
| 8. discovery | ✓ 15 Go files | ✓ 5 files | ✓ | ⏳ | ⏳ | ❌ |
| 9. event-proc | ✓ 15 Go files 30K | ✓ 3 files | ✓ | ⏳ | ⏳ | ❌ |
| 10. api-gateway | ✓ 2 Go files 95K | ✓ 1 file 772 lines | ✓ | ⏳ | ⏳ | ❌ |
| 11. export | ✓ 3 Go files | ✓ 1 file | ✓ | ⏳ | ⏳ | ❌ |
| 12. audit | ✓ 1 Go file | ❌ 0 tests | ✓ | ⏳ | ⏳ | ❌ |
| 13. blur (ai-worker) | ✓ 3 Go + py files | ✓ 1 file | ✓ | ⏳ | ⏳ | ❌ |
| 14. federation | ✓ 1 Go file | ❌ 0 tests | ✓ | ⏳ | ⏳ | ❌ |
| 15. ingest | ✓ 2 Go files 23K | ✓ 1 file | ✓ | ⏳ | ⏳ | ❌ |
| 16. metadata | ✓ 2 Go files | ✓ 1 file | ✓ | ⏳ | ⏳ | ❌ |
| 17. notification | ✓ 4 Go files | ✓ 1 file | ✓ | ⏳ | ⏳ | ❌ |
| 18. onvif-events | ✓ 1 Go file 12K | ❌ 0 tests | ✓ | ⏳ | ⏳ | ❌ |
| 19. pos-ingest | ✓ 1 Go file | ❌ 0 tests | ✓ | ⏳ | ⏳ | ❌ |
| 20. reporting | ✓ 1 Go file | ❌ 0 tests | ✓ | ⏳ | ⏳ | ❌ |
| 21. model-registry | ✓ 1 Go file | ❌ 0 tests | ✓ | ⏳ | ⏳ | ❌ |
| **pkg/common** | ✓ Shared lib | ✓ 13 tests 11 files | ✓ | ✓ | ✓ | ✓ |
| **pkg/onvif** | ✓ ONVIF lib 20 files | ✓ 11 tests | ✓ | ✓ | ✓ | ✓ |
| **web** | ✓ 16K TSX 34 pages | ✓ 3 files 16 tests | ✓ | ⏳ | ⏳ | ❌ |

---

## Phase 1 Audit — Summary by Tier

### Tier 1: Core Infrastructure (certified)
| Domain | Go LOC | Tests | Test LOC | Notes |
|--------|--------|-------|----------|-------|
| pkg/common | Shared | 13 tests | ~300 | JWT, crypto, env helpers — all pass |
| pkg/onvif | 20 files | 11 tests | ~800 | Device discovery, media, PTZ, events, auth, analytics, recording — all pass |

### Tier 2: Well-Equipped Domains (ready for AC definition)
| Domain | Service LOC | Test Files | Test LOC | Key Features |
|--------|-------------|------------|----------|--------------|
| auth | 7 files / 33K | 1 (main_test.go) | 328 | JWT, MFA, OIDC, LDAP, SSO, API keys, password policy, rate limiting |
| api-gateway | 2 files / 95K | 1 (main_test.go) | 772 | Reverse proxy, rate limiting, circuit breaker, TLS, gRPC, ACME, multi-tenant |
| recorder | 12 files / 24K | 2 | 1,696 | Recording pipeline, retention, tiering, storage, audio, bookmarks, legal hold, leader election, timeline, frame analysis |
| event-proc | 15 files / 30K | 3 | 1,823 | Rule engine, tripwire, intrusion, loitering, abandoned object, forensics, heatmap, incident management, alert workflow |
| discovery | 15 files | 5 | 2,006 | ONVIF WS-Discovery, mDNS, IP range scanning, manual add, orchestrator, scheduler, store |
| ingest | 2 files / 23K | 1 | 689 | RTSP negotiator, FFmpeg pipeline, ONVIF profile discovery, recording segments |
| webrtc | 2 files | 1 | 288 | WebRTC offer/answer, ICE, SDP, auth middleware |
| playback | 3 files | 2 | 206 | Media streaming, security check, HLS/DASH segments |
| camera-mgmt | 2 files | 1 | 100 | CRUD operations |

### Tier 3: Minimal Domains (need test coverage)
| Domain | Service LOC | Test Files | Key Features |
|--------|-------------|------------|--------------|
| camera-control | 1.8K | 0 | PTZ commands |
| export | 3 files | 1 | Evidence export |
| notification | 4 files | 1 | Channels (email, SMS, webhook), system config |
| metadata | 2 files | 1 | Metadata storage/retrieval |
| ai-worker/blur | 3 Go + Python | 1 | LPR (license plate), blur worker, Python ML inference |

### Tier 4: Skeletal (no tests, single file)
| Domain | Service LOC | Key Features |
|--------|-------------|--------------|
| audit | 1 Go file | Audit logging endpoint |
| federation | 1 Go file | Cross-site federation |
| onvif-events | 12K | ONVIF pull-point subscription handling |
| pos-ingest | 3.7K | POS data ingestion |
| reporting | 1 Go file | Report generation |
| model-registry | 1 Go file | AI model registry |
| thumbnails | 1 Go file | Thumbnail service |

---

## Test Gap Analysis

| Domain | Has Tests? | Test Count | Quality | Status |
|--------|-----------|------------|---------|--------|
| pkg/common | ✓ | 13 subtests | Good — covers JWT, crypto, env | Certified |
| pkg/onvif | ✓ | ~40 subtests | Excellent — covers all ONVIF operations | Certified |
| auth | ✓ | ~15 subtests | Adequate — HTTP handlers, config, JWT | Needs expansion |
| recorder | ✓ | 9 subtests | Good — leader election, config, KV store | Needs expansion |
| event-proc | ✓ | ~8 subtests | Adequate — rule engine, tripwire | Needs expansion |
| discovery | ✓ | ~15 subtests | Good — scanner, store, orchestrator | Needs expansion |
| api-gateway | ✓ | ~25 subtests | Good — routing, auth, proxy | Needs expansion |
| webrtc | ✓ | 11 subtests | Good — offer/answer, auth, sessions | Needs expansion |
| playback | ✓ | 2 subtests | Minimal — config, error path | Needs expansion |
| ingest | ✓ | ~5 subtests | Minimal — config | Needs expansion |
| export | ✓ | 1 test | Minimal | Needs expansion |
| metadata | ✓ | 1 test | Minimal | Needs expansion |
| notification | ✓ | 1 test | Minimal | Needs expansion |
| camera-mgmt | ✓ | 1 test | Minimal | Needs expansion |
| ai-worker/lpr | ✓ | 1 test | Minimal — LPR path | Needs expansion |
| **camera-control** | ❌ | 0 | — | Must add tests |
| **audit** | ❌ | 0 | — | Must add tests |
| **federation** | ❌ | 0 | — | Must add tests |
| **onvif-events** | ❌ | 0 | — | Must add tests |
| **pos-ingest** | ❌ | 0 | — | Must add tests |
| **reporting** | ❌ | 0 | — | Must add tests |
| **model-registry** | ❌ | 0 | — | Must add tests |
| **thumbnails** | ❌ | 0 | — | Must add tests |
| **web** | ✓ | 3 files / 16 tests | Good — smoke, API client, AuthContext | Needs expansion |

---

## Build Verification

| Check | Status | Details |
|-------|--------|---------|
| `make build` (14 services) | ✓ PASS | All 14 Makefile services compile |
| Additional services (8) | ✓ PASS | ingest, metadata, notification, onvif-events, pos-ingest, reporting, model-registry, ai-worker all compile |
| `make test` (Go) | ✓ PASS | 17 packages pass, 8 packages with no test files |
| `npm test` (web) | ✓ PASS | 3 test files, 16 tests pass |
| `npm run build` (web) | ✓ PASS | Production build succeeds (642KB JS bundle) |
| `make lint` | ⚠️ Partial | golangci-lint not installed in CI |