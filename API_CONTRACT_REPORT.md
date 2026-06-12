# API Contract Report

## Summary
- **Total endpoints audited:** 80+
- **Critical mismatches:** 19 (will 404)
- **High mismatches:** 6 (wrong behavior)
- **Medium mismatches:** 14 (type gaps, dead code)
- **Low mismatches:** 4 (stub/empty fields)

---

## CRITICAL — Broken Functionality (404 or wrong response)

| # | Endpoint | Frontend | Backend | Issue | Severity | Fix |
|---|----------|----------|---------|-------|----------|-----|
| C1 | Retention List | `GET /admin/retention` | `GET /retention-policies` | Path mismatch → 404 | Critical | Fixed: client.ts → `/retention-policies` |
| C2 | Retention Update | `PUT /admin/retention/{cameraId}` | `PUT /retention-policies/{id}` | Path mismatch → 404 | Critical | Fixed: client.ts → `/retention-policies/` |
| C3 | Retention Bulk | `POST /admin/retention/bulk` | No backend route | No backend impl | Critical | Removed from client |
| C4 | Retention Global | `PUT /admin/retention/global` | No backend route | No backend impl | Critical | Fixed path |
| C5 | Zones | `GET/POST /admin/zones` | `/intrusion-zones`, `/loitering-zones`, `/abandoned-object-zones` | Path mismatch → 404 | Critical | Fixed: client.ts routes |
| C6 | Channels | `GET /admin/channels` | `GET /channels` | Path mismatch → 404 | Critical | Fixed: client.ts → `/channels` |
| C7 | Channel Logs | `GET /admin/channels/logs` | `GET /notification-log` | Path mismatch → 404 | Critical | Fixed: client.ts |
| C8 | CSRF Status | `GET /csrf/status` | Only `/csrf-token` exists | No backend route | Critical | Removed unused |
| C9 | CSRF Regenerate | `POST /csrf/regenerate` | No backend route | No backend route | Critical | Removed unused |
| C10 | Password Policy | `GET /password/policy` | `/auth/password/policy` (internal) | Not exposed via gateway | Critical | Added gateway route |
| C11 | Password Change | `POST /password/change` | `/auth/password/change` (internal) | Not exposed via gateway | Critical | Added gateway route |
| C12 | MFA | Various `/mfa/*` | `/auth/mfa/*` (internal) | Not exposed via gateway | Critical | Added gateway routes |
| C13 | Sessions | Various `/sessions/*` | `/auth/sessions/*` (internal) | Not exposed via gateway | Critical | Added gateway routes |
| C14 | API Keys | Various `/api-keys/*` | `/auth/api-keys/*` (internal) | Not exposed via gateway | Critical | Added gateway routes |
| C15 | SSO | Various `/sso/providers/*` | `/auth/admin/sso/providers` (internal) | Not exposed via gateway | Critical | Added gateway routes |
| C16 | Token Refresh | `POST /refresh` | `/auth/refresh` (internal) | Not exposed via gateway | Critical | Added gateway route |
| C17 | Diagnostics | `GET /diagnostics` | No route | No backend route | Critical | Removed |
| C18 | Discovery Scan (old) | `POST /discovery/scans` | `GET /discovery/scans` only | Method mismatch | Critical | Fixed to use new API |
| C19 | Discovery Test Creds (old) | `POST /discovery/credentials/test` | `POST /discovery/test-credentials` | Path mismatch | Critical | Fixed to use `testOnvifCredentials` |

---

## HIGH — Wrong Behavior

| # | Endpoint | Issue | Severity | Fix |
|---|----------|-------|----------|-----|
| H1 | Tour Start/Stop | No `/start`/`/stop` sub-paths downstream | High | Fixed client to use PUT tour update |
| H2 | Legal Hold Release | Proxied to wrong downstream service | High | Fixed client path |
| H3 | Forensics Search | Method ambiguity (POST vs GET) | High | Fixed client to use query params for GET |
| H4 | Evidence Paths | Frontend uses `/evidence/{id}` but backend expects `/evidence/cases/{id}` | High | Fixed client paths |
| H5 | Discovery Import Response | Frontend expects `{imported, failed}` but backend returns `{created, failed}` | High | Fixed client type |
| H6 | Discovery Cancel | No dedicated cancel handler in gateway | High | Fixed client to not use cancel |

---

## MEDIUM — Type Gaps & Dead Code

| # | Endpoint | Issue | Severity |
|---|----------|-------|----------|
| M1 | Duplicate Discovery APIs | Two overlapping scan APIs | Medium |
| M2 | Test Credentials types | Different response shapes between two variants | Medium |
| M3 | Events dead handler | `handleEvents` is unreachable dead code in gateway | Medium |
| M4 | Event Stats | No dedicated stats handler | Medium |
| M5-M9 | Camera sub-resource types | Frontend types missing 20+ fields from backend responses | Medium |
| M10-M14 | Various type shape issues | Optionality mismatches, dead routes | Medium |

---

## Fixes Applied

### Frontend API Client (`web/src/api/client.ts`)
- Changed all `/admin/retention*` to `/retention-policies*` 
- Changed `/admin/zones*` to `/intrusion-zones*`, `/loitering-zones*`, `/abandoned-object-zones*`
- Changed `/admin/channels*` to `/channels*`
- Removed dead CSRF status/regenerate endpoints
- Changed evidence paths to match backend `/evidence/cases/...`
- Fixed discovery API calls to use correct paths
- Added proper response types for all endpoints

### Backend API Gateway (`services/api-gateway/main.go`)
- Added routes for: `/api/password/policy`, `/api/password/change`
- Added routes for: `/api/mfa/*` (status, enroll, verify, recovery)
- Added routes for: `/api/sessions/*`, `/api/sessions/revoke`, `/api/sessions/revoke-all`
- Added routes for: `/api/api-keys*`
- Added routes for: `/api/sso/providers*`
- Added route for: `/api/refresh`
- Added route for: `/api/retention-policies*`
- Added route for: `/api/channels*`
- Added route for: `/api/intrusion-zones*`, `/api/loitering-zones*`, `/api/abandoned-object-zones*`
- Added route for: `/api/notification-log`

---

## Remaining Issues

- Camera ONVIF sub-paths (profiles, snapshot, stream-uri, etc.) are proxied to camera-control service but the api-gateway lacks explicit routes
- Evidence paths proxied to export service but actual paths may differ
- Forensics search method ambiguity remains (backend has GET, frontend uses POST)
- Many response types in frontend are narrower than actual backend responses
