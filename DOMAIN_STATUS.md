# EVMS Domain Status

**Last Updated:** 2026-06-12 (all domains now have tests — 23/23 Go packages pass)

## Legend
- ✅ Certified — implementation, tests, acceptance criteria all verified
- ⏳ In Progress — implementation exists, tests and/or AC verification in progress
- ❌ Not Started / No Tests — implementation exists but no tests, cannot certify

---

| # | Domain | Code | Tests | AC Verified | Certified | Notes |
|---|--------|------|-------|-------------|-----------|-------|
| 1 | auth | ✓ | ✓ | ⏳ | ❌ | Needs test expansion for SSO, MFA, full LDAP |
| 2 | camera-mgmt | ✓ | ✓ | ⏳ | ❌ | Minimal test coverage |
| 3 | recorder | ✓ | ✓ | ⏳ | ❌ | Needs tests for retention, bookmarks, legal holds |
| 4 | playback | ✓ | ✓ | ⏳ | ❌ | Minimal test coverage |
| 5 | webrtc | ✓ | ✓ | ⏳ | ❌ | Good coverage, not yet AC-verified |
| 6 | camera-control | ✓ | ✓ 56 tests | ✓ | ✅ | PTZ commands, presets, ONVIF/VAPIX/Hikvision protocols tested |
| 7 | thumbnails | ✓ | ✓ 25 tests | ✓ | ✅ | Thumbnail timeline, image serving, cache, path traversal tested |
| 8 | discovery | ✓ | ✓ | ⏳ | ❌ | Good coverage, not yet AC-verified |
| 9 | event-proc | ✓ | ✓ | ⏳ | ❌ | Moderate coverage, needs expansion |
| 10 | api-gateway | ✓ | ✓ | ⏳ | ❌ | Good coverage, needs integration tests |
| 11 | export | ✓ | ✓ | ⏳ | ❌ | Minimal test coverage |
| 12 | audit | ✓ | ✓ 21 tests | ✓ | ✅ | Hash-linked audit chain, log ingestion, chain verification, integrity verification tested |
| 13 | blur (ai-worker) | ✓ | ✓ | ⏳ | ❌ | LPR tested, blur untested |
| 14 | federation | ✓ | ✓ 15 tests | ✓ | ✅ | Cross-site federation CRUD, search, playback proxy, latency tested |
| 15 | ingest | ✓ | ✓ | ⏳ | ❌ | Minimal test coverage |
| 16 | metadata | ✓ | ✓ | ⏳ | ❌ | Minimal test coverage |
| 17 | notification | ✓ | ✓ | ⏳ | ❌ | Minimal test coverage |
| 18 | onvif-events | ✓ | ✓ 22 tests | ✓ | ✅ | Pull-point subscribe/unsubscribe/list, JWT auth, concurrent access, event insert tested |
| 19 | pos-ingest | ✓ | ✓ 8 tests | ✓ | ✅ | POS transaction JSON round-trip, JWT auth, method validation tested |
| 20 | reporting | ✓ | ✓ 22 tests | ✓ | ✅ | Report rendering (all 4 types), data table generation, HTML formatting, config tested |
| 21 | model-registry | ✓ | ✓ 18 tests | ✓ | ✅ | Model CRUD with DB-nil paths, JWT auth, routing, JSON round-trip, Close tested |
| | **pkg/common** | ✓ | ✓ | ✓ | ✅ | Certified |
| | **pkg/onvif** | ✓ | ✓ | ✓ | ✅ | Certified |
| | **web** | ✓ | ✓ | ⏳ | ❌ | 16 tests pass, needs expansion |
