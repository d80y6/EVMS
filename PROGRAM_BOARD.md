# EVMS Program Board — Production Candidate Tracker

**Date:** 2026-06-12
**Status:** Alpha → Production Candidate gate in progress
**Build:** All 14 Makefile services compile
**Test Suites:** 23/23 Go packages pass | 3/3 web test files pass
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
| 6. camera-control | ✓ 1 Go file | ✓ 56 tests | ✓ | ✓ | ✓ | ✅ |
| 7. thumbnails | ✓ 1 Go file | ✓ 25 tests | ✓ | ✓ | ✓ | ✅ |
| 8. discovery | ✓ 15 Go files | ✓ 5 files | ✓ | ⏳ | ⏳ | ❌ |
| 9. event-proc | ✓ 15 Go files 30K | ✓ 3 files | ✓ | ⏳ | ⏳ | ❌ |
| 10. api-gateway | ✓ 2 Go files 95K | ✓ 1 file 772 lines | ✓ | ⏳ | ⏳ | ❌ |
| 11. export | ✓ 3 Go files | ✓ 1 file | ✓ | ⏳ | ⏳ | ❌ |
| 12. audit | ✓ 1 Go file | ✓ 21 tests | ✓ | ✓ | ✓ | ✅ |
| 13. blur (ai-worker) | ✓ 3 Go + py files | ✓ 1 file | ✓ | ⏳ | ⏳ | ❌ |
| 14. federation | ✓ 1 Go file | ✓ 15 tests | ✓ | ✓ | ✓ | ✅ |
| 15. ingest | ✓ 2 Go files 23K | ✓ 1 file | ✓ | ⏳ | ⏳ | ❌ |
| 16. metadata | ✓ 2 Go files | ✓ 1 file | ✓ | ⏳ | ⏳ | ❌ |
| 17. notification | ✓ 4 Go files | ✓ 1 file | ✓ | ⏳ | ⏳ | ❌ |
| 18. onvif-events | ✓ 1 Go file 12K | ✓ 22 tests | ✓ | ✓ | ✓ | ✅ |
| 19. pos-ingest | ✓ 1 Go file | ✓ 8 tests | ✓ | ✓ | ✓ | ✅ |
| 20. reporting | ✓ 1 Go file | ✓ 22 tests | ✓ | ✓ | ✓ | ✅ |
| 21. model-registry | ✓ 1 Go file | ✓ 18 tests | ✓ | ✓ | ✓ | ✅ |
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
| camera-control | 1 file | 1 (56 tests) | 1,800 | PTZ commands, presets, ONVIF/VAPIX/Hikvision — certified |
| audit | 1 file | 1 (21 tests) | 600 | Hash-linked audit chain, log ingestion, chain verification — certified |

### Tier 3: Minimal Domains (need test coverage)
| Domain | Service LOC | Test Files | Key Features |
|--------|-------------|------------|--------------|
| export | 3 files | 1 | Evidence export |
| thumbnails | 1 file | 1 (25 tests) | 400 | Thumbnail generation from recordings — certified |
| notification | 4 files | 1 | Channels (email, SMS, webhook), system config |
| metadata | 2 files | 1 | Metadata storage/retrieval |
| ai-worker/blur | 3 Go + Python | 1 | LPR (license plate), blur worker, Python ML inference |
| federation | 1 file | 1 (15 tests) | 450 | Cross-site federation CRUD, search, proxy — certified |
| onvif-events | 12K | 1 (22 tests) | 500 | ONVIF pull-point subscribe/unsubscribe/list — certified |
| pos-ingest | 3.7K | 1 (8 tests) | 350 | POS transaction JSON API — certified |
| reporting | 1 file | 1 (22 tests) | 600 | Report configs, render, HTML table generation — certified |
| model-registry | 1 file | 1 (18 tests) | 550 | AI model CRUD, activation, canary, rollback — certified |

### Tier 4: Skeletal (no tests, single file)
| Domain | Service LOC | Key Features |
|--------|-------------|--------------|


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
| **camera-control** | ✓ | 56 subtests | Good — covers PTZ move, zoom, stop, presets, protocols (ONVIF/VAPIX/Hikvision), router, validation, error handling | Certified |
| **audit** | ✓ | 21 | Good — covers hash chain, log entry, dedup, verify endpoints | Certified |
| **federation** | ✓ | 15 | Good — covers routing, config, JSON helpers, JWT auth, latency, shutdown | Certified |
| **onvif-events** | ✓ | 22 | Good — covers subscribe/unsubscribe/list, JWT auth, concurrent access, Close | Certified |
| **pos-ingest** | ✓ | 8 | Good — covers JWT auth, JSON round-trip, method validation, jsonError | Certified |
| **reporting** | ✓ | 22 | Good — covers all 4 report type renderers, HTML formatting, config, data types | Certified |
| **model-registry** | ✓ | 18 | Good — covers DB-nil error paths, JWT auth, routing, JSON round-trip, Close | Certified |
| **thumbnails** | ✓ | 25 subtests | Good — covers timeline, image serving, findRecording, cache, path traversal, format validation | Certified |
| **web** | ✓ | 3 files / 16 tests | Good — smoke, API client, AuthContext | Needs expansion |

---

## Build Verification

| Check | Status | Details |
|-------|--------|---------|
| `make build` (14 services) | ✓ PASS | All 14 Makefile services compile |
| Additional services (8) | ✓ PASS | ingest, metadata, notification, onvif-events, pos-ingest, reporting, model-registry, ai-worker all compile |
| `make test` (Go) | ✓ PASS | 23 packages pass, 0 packages with no test files |
| `npm test` (web) | ✓ PASS | 3 test files, 16 tests pass |
| `npm run build` (web) | ✓ PASS | Production build succeeds (642KB JS bundle) |
| `make lint` | ⚠️ Partial | golangci-lint not installed in CI |