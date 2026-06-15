# Testing & CI Production Hardening

**Date:** 2026-06-15
**Status:** Design Approved

## Overview

Two parallel tracks to raise Testing score from 15% to 85%: CI/backend improvements and frontend/E2E tests. Tracks are independent and can execute concurrently.

---

## Track A: CI & Backend Tests

### CI Fixes
- Add `npm test` to frontend CI job in `.github/workflows/go-ci.yml`
- Add `-coverprofile=coverage.out` to `go test` in CI
- Add coverage threshold check (min 30% package coverage)

### Go Test Depth

Target services with only `main_test.go` config-validation tests:
- `services/auth` — test password validation, MFA, token lifecycle
- `services/api-gateway` — test route setup, middleware chain
- `services/ingest` — test ffmpeg arg construction, NATS publishing
- `services/playback` — test segment serving, authz checks (already has security_test.go)
- `services/webrtc` — test offer handling, NATS subscription
- `services/recorder` — test retention, tiering, segment indexing

### Integration Test Tags
- Add `//go:build integration` build tags to existing tests that require DB/NATS
- Add CI step to run `go test -tags=integration ./...`

---

## Track B: Frontend Tests & E2E

### Component Tests (Vitest)
- `SettingsPage` — loading state, camera list render, save settings
- `SearchPage` — search filters, results display, empty state
- `WebhooksPage` — webhook list, create/edit, error handling

### Playwright E2E
- Install Playwright, configure `playwright.config.ts`
- Test 1: Login/logout flow (auth)
- Test 2: Camera list page loads and displays cameras
- Test 3: Playback page loads with time range picker
- Test 4: Export flow (select camera, time range, submit)
