# EVMS Testing Gap Report

**Generated:** 2026-06-11
**Project:** EVMS (Enterprise Video Management System)
**Module:** `github.com/dam-vms/dam`

---

## Executive Summary

EVMS has 33 Go test files across the backend (`pkg/` and `services/`) providing **partial unit-test coverage** for roughly half the microservices, and **zero frontend tests** of any kind. There are no end-to-end tests, no API contract tests, no fuzz tests, and no CI-mandated coverage gates. Many security-critical services have no tests at all, and several existing test files are minimal (config-defaults only), offering little behavioral verification. The test-to-source ratio is well below industry standard for a production video-surveillance system.

---

## 1. Test Inventory

### 1.1 Backend Tests (Go)

| File | Type | Lines | Quality Assessment |
|------|------|-------|-------------------|
| `pkg/common/auth_test.go` | Unit | 260 | **Good** - covers JWT middleware, claim injection, token-in-query-param, missing/invalid tokens |
| `pkg/common/crypto_test.go` | Unit | 157 | **Good** - encrypt/decrypt round-trip, key missing/invalid, MustEncrypt fallback, logging |
| `pkg/onvif/device_test.go` | Unit | 273 | **Good** - mock HTTP server, tests GetDeviceInformation, GetCapabilities, GetServices, etc. |
| `pkg/onvif/media_test.go` | Unit | 257 | **Good** - mock HTTP server, GetProfiles, GetStreamURI, snapshot URI |
| `pkg/onvif/soap_test.go` | Unit | 232 | **Good** - SOAP client construction, Do with/without auth, error handling |
| `pkg/onvif/auth_test.go` | - | - | ONVIF auth tests (digest, WS-UsernameToken) |
| `pkg/onvif/event_test.go` | - | - | ONVIF event subscription tests |
| `pkg/onvif/ptz_test.go` | - | - | PTZ operations tests |
| `pkg/onvif/imaging_test.go` | - | - | Imaging settings tests |
| `pkg/onvif/provision_test.go` | - | - | Device provisioning tests |
| `pkg/onvif/analytics_test.go` | - | - | Analytics configuration tests |
| `pkg/onvif/recording_test.go` | - | - | ONVIF recording tests |
| `services/auth/main_test.go` | Unit | 328 | **Good** - rate limiter, logout handler, OIDC callback validation, SAML nameID extraction, login rate-limiting |
| `services/playback/security_test.go` | Unit | 175 | **Good** - path traversal, symlink attacks, path cleanup, directory listing prevention |
| `services/playback/main_test.go` | Unit | 28 | **Minimal** - default config + root-not-found validation |
| `services/recorder/main_test.go` | Unit | 38 | **Minimal** - default config + validation (DB URL, NATS URL) |
| `services/recorder/leader_test.go` | Integration | 330 | **Good** - leader election via NATS JetStream KV, failover, context cancellation |
| `services/webrtc/main_test.go` | Unit | 298 | **Good** - JWT auth middleware, offer handler, session cleanup, WebRTC codec setup |
| `services/event-proc/main_test.go` | Unit | 54 | **Minimal** - default config, notification threshold logic, camera ID extraction |
| `services/event-proc/rule_engine_test.go` | Unit | 76 | **Adequate** - simple match, no-match, OR logic, disabled rules |
| `services/event-proc/tripwire_test.go` | Unit | 44 | **Minimal** - geometry functions (cross product, line intersection, direction filter) |
| `services/discovery/orchestrator_test.go` | Unit | 161 | **Good** - mock store/scanner, start+complete, dedup, cancellation |
| `services/discovery/store_test.go` | Integration | 172 | **Good** - Postgres-backed CRUD, pagination, mark imported (requires TEST_DB_URL) |
| `services/discovery/scanner_test.go` | Unit | 81 | **Adequate** - interface compliance, manual IP parsing, CIDR parsing |
| `services/discovery/wsdiscovery_test.go` | Unit | 67 | **Adequate** - probe XML building, name, context cancellation |
| `services/discovery/iprange_test.go` | Unit | 21 | **Minimal** - name test + flaky probe timeout test |
| `services/camera-mgmt/main_test.go` | Unit | 58 | **Minimal** - camera-to-proto mapping only; DB tests `t.Skip` when no `TEST_DB_URL` |
| `services/export/main_test.go` | Unit | 82 | **Minimal** - camera ID sanitization, JSON serialization |
| `services/ingest/main_test.go` | Unit | 28 | **Minimal** - default config + validation |
| `services/metadata/main_test.go` | Unit | 12 | **Trivial** - single default-config assertion |
| `services/notification/main_test.go` | Unit | 24 | **Trivial** - default config + constant checks |
| `services/ai-worker/lpr_test.go` | Unit | 29 | **Trivial** - disabled processor returns nil; hotlist map check |

### 1.2 Frontend Tests (TypeScript/React)

| Test Infrastructure | Status |
|---------------------|--------|
| Test runner config (jest.config, vitest.config) | **Not present** |
| Test files (*.test.ts, *.test.tsx, *.spec.ts, *.spec.tsx) | **Zero** |
| Component tests | **Zero** |
| Page tests | **Zero** |
| Hook tests | **Zero** |
| API client tests | **Zero** |
| Auth context tests | **Zero** |
| Cypress / Playwright config | **Not present** |

### 1.3 Test Files by Category

| Category | Count |
|----------|-------|
| **Total Go test files** | **33** |
| -- pkg/ (library tests) | 12 |
| -- services/ (service tests) | 21 |
| **Total Frontend test files** | **0** |
| **Integration tests** (require external DB/NATS) | 3 |
| **Unit tests** | 30 |
| **E2E tests** | 0 |
| **API/contract tests** | 0 |
| **Fuzz tests** | 0 |
| **Security-specific tests** | 1 (playback/security_test.go) |

---

## 2. Test Coverage by Service Area

### 2.1 Services WITH Tests

| Service | Test Quality | What's Tested | What's MISSING |
|---------|-------------|---------------|----------------|
| **auth** | Good | Rate limiting, OIDC callback, SAML parsing, logout, login rate-limit | Token issuance/refresh, password hashing, LDAP auth, MFA flows, session management, RBAC enforcement |
| **playback** | Security: Good / Rest: Minimal | Path traversal, symlink attacks, config defaults | Actual video streaming, seek operations, HLS/DASH delivery, playback authorization per tenant |
| **recorder** | Integration: Good / Config: Minimal | Leader election via NATS KV, config validation | Recording segment upload, retention enforcement, storage tiering, failure recovery, segment indexing |
| **webrtc** | Good | JWT auth middleware, offer handler, codec negotiation, session cleanup (non-panics) | Actual ICE negotiation, STUN/TURN integration, reconnection, multi-stream handling |
| **event-proc** | Adequate | Notification thresholds, rule engine (match/no-match/OR/disabled), tripwire geometry, camera ID extraction | Complex rules, temporal conditions, zone-based logic, deduplication, event persistence |
| **discovery** | Good | Orchestrator lifecycle, dedup, cancellation, store CRUD, pagination, WS-Discovery probe, IP range scanner | Real network scanning (mocked), ONVIF device probing over network, multi-site discovery |
| **camera-mgmt** | Minimal | Camera-to-proto mapping | CRUD operations, site management, camera health, firmware management, bulk import |
| **export** | Minimal | Camera ID sanitization, JSON serde | Actual export pipeline, video transcoding, watermark application, download auth, async job management |
| **ingest** | Minimal | Config defaults + validation | RTSP stream ingestion, segment writing, stream health monitoring, reconnection logic |
| **metadata** | Trivial | Default config (1 assertion) | Metadata indexing, search, storage, retrieval APIs |
| **notification** | Trivial | Default config + constants | Email dispatch, webhook delivery, push notifications, retry logic, template rendering |
| **ai-worker** | Trivial | LPR disabled state, hotlist map | AI inference pipeline, model loading, image analysis, alert generation |

### 2.2 Services WITHOUT Tests (9 of 21 - 43%)

| Service | Directory | Risk | Key Untested Logic |
|---------|-----------|------|-------------------|
| **api-gateway** | `services/api-gateway/` | **CRITICAL** | Route routing, auth forwarding, rate limiting, TLS termination, CORS, request validation, WebSocket upgrade |
| **audit** | `services/audit/` | **HIGH** | Audit log ingestion, tamper-proof storage, query/filter, retention, compliance exports |
| **camera-control** | `services/camera-control/` | **HIGH** | PTZ command dispatch, absolute/move/zoom, privacy masking, preset management |
| **federation** | `services/federation/` | **CRITICAL** | Cross-site trust, stream sharing, token exchange, access control across tenants |
| **onvif-events** | `services/onvif-events/` | **HIGH** | Event subscription, pull-point polling, event parsing, reconnection |
| **pos-ingest** | `services/pos-ingest/` | **MEDIUM** | POS transaction ingestion, data transformation, metadata extraction |
| **reporting** | `services/reporting/` | **MEDIUM** | Report generation, data aggregation, scheduling, export formats |
| **model-registry** | `services/model-registry/` | **MEDIUM** | Model versioning, deployment, rollout, rollback |
| **thumbnails** | `services/thumbnails/` | **MEDIUM** | Thumbnail generation, caching, on-demand resizing |

---

## 3. Critical Untested Paths

### 3.1 Authentication & Authorization (HIGH RISK)

| Path | Source | Risk |
|------|--------|------|
| JWT token issuance (login endpoint) | `services/auth/` | Unauthorized access |
| Password hashing verification (bcrypt cost, comparison) | `services/auth/` | Credential compromise |
| LDAP bind and user search | `services/auth/` | Auth bypass |
| MFA TOTP validation | `services/auth/` | Account takeover |
| OAuth2/OIDC full token exchange | `services/auth/` | SSO bypass |
| RBAC enforcement (role/permission checks) | `pkg/common/` | Privilege escalation |
| Refresh token rotation | `services/auth/` | Session hijacking |
| Session revocation/invalidation | `services/auth/` | Persistent unauthorized access |

### 3.2 API Gateway (CRITICAL)

| Path | Source | Risk |
|------|--------|------|
| Request routing and service dispatch | `services/api-gateway/main.go` | Misrouting/SSRF |
| Auth token validation on inbound requests | `services/api-gateway/` | Auth bypass at edge |
| Rate limiting configuration enforcement | `services/api-gateway/` | DoS vulnerability |
| CORS policy enforcement | `services/api-gateway/` | Cross-origin data leak |
| Request body size limits | `services/api-gateway/` | Resource exhaustion |

### 3.3 Federation (CRITICAL)

| Path | Source | Risk |
|------|--------|------|
| Cross-site trust verification | `services/federation/` | Unauthorized cross-site access |
| Token exchange between sites | `services/federation/` | Token theft/replay |
| Tenant isolation across sites | `services/federation/` | Data leakage between tenants |
| Stream sharing authorization | `services/federation/` | Unauthorized video stream access |

### 3.4 Recorder & Storage

| Path | Source | Risk |
|------|--------|------|
| Recording segment encryption at rest | `services/recorder/` | Data breach |
| Retention enforcement / deletion | `services/recorder/` | Compliance failure |
| Storage tiering logic | `services/recorder/` | Data loss |
| Segment corruption detection | `services/recorder/` | Silent data corruption |

### 3.5 AI Worker (LPR & Analytics)

| Path | Source | Risk |
|------|--------|------|
| LPR license plate recognition pipeline | `services/ai-worker/` | False positives/negatives |
| Model inference error handling | `services/ai-worker/` | Service crash |
| Hotlist matching / alert generation | `services/ai-worker/` | Missed security alerts |
| Image preprocessing / scaling | `services/ai-worker/` | Processing failures |

---

## 4. Security-Sensitive Paths Without Tests

| Vulnerability Class | Untested Code | Impact |
|--------------------|--------------|--------|
| **SQL Injection** | All DB queries across `services/` (discovery, camera-mgmt, audit, recorder) | Data exfiltration |
| **Path Traversal** | `services/export/` - export file output paths | Arbitrary file write |
| **Command Injection** | `services/thumbnails/` - ffmpeg execution | RCE |
| **XXE / XML Injection** | `services/onvif-events/`, `pkg/onvif/` - SOAP/XML parsing | SSRF, data leak |
| **JWT None Algorithm** | `pkg/common/auth.go` - JWT parsing | Auth bypass |
| **Insecure Deserialization** | Any JSON/Protobuf deserialization entry points | RCE / DoS |
| **SSRF** | `services/discovery/`, `pkg/onvif/` - probe external URLs | Internal network scanning |
| **Rate Limiting Bypass** | `services/api-gateway/` (no tests), `services/auth/` (partial) | Brute force |
| **Audit Log Tampering** | `services/audit/` (no tests) | Compliance failure |
| **TLS Configuration** | Services with gRPC/TLS | MitM, weak ciphers |

---

## 5. High-Risk Workflows Without Tests

1. **User Registration and MFA Enrollment** - No tests for the signup, MFA setup, or recovery workflows
2. **Camera Onboarding (Discovery -> Provision -> Stream)** - No integration tests connecting discovery results to camera management
3. **Cross-Tenant Federation** - Entirely untested; no trust validation or token exchange tests
4. **Video Export with Watermarking** - No tests for the actual export pipeline, transcoding, or watermark application
5. **AI Alert Pipeline (Detection -> Event -> Notification)** - No end-to-end test covering the full inference-to-alert flow
6. **Audit Log Generation and Querying** - No tests for audit record creation, tamper-evidence, or export
7. **Recording Playback with Authorization** - No tests verifying tenant isolation during playback
8. **Legal Hold / Evidence Export** - No tests for evidence locking, chain-of-custody, or export integrity

---

## 6. Frontend Testing Status

The frontend (`web/`) has **zero testing infrastructure**:

- No test runner configured (no Jest, Vitest, or similar)
- No testing library dependencies (no React Testing Library, no `@testing-library/react`)
- No test files of any type
- No E2E framework (no Cypress, Playwright, or Selenium)
- No linting for test-related rules
- The `package.json` has no `test` script at all

Frontend code that is entirely untested includes:

- **40 page components** (Login, Cameras, Playback, Admin, etc.)
- **12 shared components** (CameraCard, MapView, PtzOverlay, TimelineScrubber, etc.)
- **5 custom hooks** (useMapCameras, useSyncPlayback, useTouchGestures, etc.)
- **1 API client** (client.ts - all HTTP calls to backend)
- **1 Auth context** (AuthContext.tsx - authentication state management)
- **1 ProtectedRoute component** (route guard logic)

This means **all frontend rendering logic, state management, auth flows, API interactions, and user event handling are entirely untested**.

---

## 7. Recommendations

### 7.1 Immediate (Week 1-2)

1. **Add Jest/Vitest + React Testing Library to the frontend**:
   - Install `vitest`, `@testing-library/react`, `@testing-library/jest-dom`
   - Add `test` script to `package.json`
   - Create a `vitest.config.ts`

2. **Write critical-path backend unit tests for services with zero coverage**:
   - `services/api-gateway/main.go` (highest priority - edge router)
   - `services/audit/` (compliance-critical)
   - `services/camera-control/` (physical security risk)

3. **Add JWT validation edge-case tests in `pkg/common/auth.go`**:
   - None algorithm acceptance test
   - Expired token handling
   - Wrong-signing-key test

### 7.2 Short-Term (Week 3-4)

4. **Integration smoke tests for all services with DB/NATS dependencies**:
   - Use `docker compose` or `testcontainers` to spin up Postgres + NATS in CI
   - Convert `t.Skip` tests in `camera-mgmt` to real integration tests

5. **API contract tests**:
   - Use a tool like `httptest` or `hapitest` to verify API responses match documented schemas
   - Validate error response formats, status codes, and content types

6. **Authentication end-to-end test**:
   - Login -> token issuance -> authenticated request -> token refresh -> logout flow
   - SSO OIDC callback full flow (with mock provider)

### 7.3 Medium-Term (Month 2-3)

7. **Frontend component tests for critical pages**:
   - `LoginPage.tsx` - form validation, error states, redirect after login
   - `CamerasPage.tsx` - camera list rendering, filter, search
   - `ProtectedRoute.tsx` - redirect on unauthenticated, render on authenticated

8. **Security fuzz tests**:
   - ONVIF SOAP/XML parser fuzzing
   - JWT token fuzzing
   - File path sanitization fuzzing for export/playback

9. **Performance / load tests for critical paths**:
   - Concurrent WebRTC connections
   - Concurrent recording playback
   - Camera discovery at scale (1000+ cameras)

### 7.4 Long-Term (Month 3+)

10. **E2E tests**:
    - Playwright or Cypress suite for critical user journeys
    - Camera discovery -> import -> view stream -> export recording workflow

11. **Coverage gates in CI**:
    - Require >= 60% package coverage for all new services
    - Require >= 80% coverage for security-critical `pkg/common/`

12. **Regular security testing**:
    - Dependency vulnerability scanning (Dependabot/Trivy)
    - SAST integration (golangci-lint with security rules)

---

## 8. Summary Statistics

| Metric | Value |
|--------|-------|
| Total Go test files | 33 |
| Total Go source files (approx.) | ~120+ |
| Services with meaningful tests | 7 of 21 (33%) |
| Services with minimal/trivial tests | 5 of 21 (24%) |
| Services with zero tests | 9 of 21 (43%) |
| Frontend test files | 0 |
| Frontend components/pages | 52+ |
| Integration tests (require external infra) | 3 (all t.Skip-able) |
| E2E tests | 0 |
| Security-specific test suites | 1 |
| Fuzz tests | 0 |
| Test coverage (estimated) | ~15-20% of backend, 0% of frontend |

---

## 9. Conclusion

EVMS has a solid foundation of ONVIF protocol tests (12 files) and a handful of well-written service-level tests (auth, playback security, recorder leader election, WebRTC auth), but the overall test coverage is critically low for a production security/surveillance system. **43% of microservices have zero tests.** The frontend has no testing infrastructure at all. Security-sensitive paths like authentication token issuance, federation, API gateway routing, PTZ camera control, and audit logging are entirely untested.

The most urgent gaps are:
1. **Zero frontend tests** (any framework)
2. **API gateway without tests** (the system edge router)
3. **Federation service without tests** (cross-site security boundary)
4. **Audit service without tests** (compliance and tamper-evidence)
5. **No E2E tests covering critical user workflows**

Given the physical security context (video surveillance, access control, evidence handling), the lack of test coverage represents a significant operational and security risk.
