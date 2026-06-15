# EVMS RBAC Audit Matrix

**Date:** 2026-06-14
**Scope:** Full-stack RBAC analysis (frontend route guards, component-level checks, backend gateway middleware, backend service middleware)

---

## Roles Identified

| Role       | Level | Description                                          |
|------------|-------|------------------------------------------------------|
| `viewer`   | 1     | Read-only access to cameras, recordings, events      |
| `operator` | 2     | Can control cameras, export, run discovery, update cameras |
| `admin`    | 3     | Full system administration, user/SSO/config management |

**Not found in codebase:** `investigator`, `auditor` (documented but not implemented).

---

## Role Hierarchy (Backend Enforcement)

Defined in `services/api-gateway/main.go`:

```go
roleLevels := map[string]int{
    "viewer":   1,
    "operator": 2,
    "admin":    3,
}
```

The `requireRole(minRole)` middleware enforces that the user's role level is >= the required level.

---

## RBAC Matrix

### Critical Actions — ALL RESOLVED

| # | Action | Role Required | Backend Protection | Status |
|---|--------|--------------|--------------------|--------|
| 1 | **Delete Evidence (Case)** | `admin` | `requireRole("admin")` on evidence mutations | **FIXED** |
| 2 | **Export Evidence (bundle)** | `admin` | Covered by same route block as evidence mutations | **FIXED** |
| 3 | **Delete Site** | `admin` | DELETE /api/sites/{id} route with `requireRole("admin")` | **FIXED** |
| 4 | **Webhook Management (CRUD)** | `admin` | `requireRole("admin")` on webhooks route | **FIXED** |

### Secondary Actions — ALL RESOLVED

| # | Action | Role Required | Backend Protection | Status |
|---|--------|--------------|--------------------|--------|
| 5 | **Delete Incident** | `admin` | `requireRole("admin")` on DELETE | **FIXED** |
| 6 | **Incident Status Changes** | `operator` | `requireRole("operator")` on POST/PUT | **FIXED** |
| 7 | **Alert/Rule Management** | `operator` | `requireRole("operator")` on POST/PUT/DELETE | **FIXED** |
| 8 | **Tour Management** | `operator` | `requireRole("operator")` on POST/PUT/DELETE | **FIXED** |
| 9 | **Report Management (CRUD)** | `operator` | `requireRole("operator")` on POST/PUT/DELETE reports | **FIXED** |

### Partial Gaps (Frontend Only — Backend Protected)

| # | Action | Backend Role | Frontend |
|---|--------|-------------|----------|
| 10 | Delete Camera | admin | Not gated |
| 11 | Create Camera | admin | Not gated |
| 12 | Update Camera | operator | Not gated |
| 13 | Export Recording | operator | Not gated |
| 14 | Global Retention Settings | admin | Not gated |
| 15 | Per-Camera Retention | operator | Not gated |
| 16 | PTZ Control | operator | Not gated |
| 17 | Discovery Scan/Import | operator | Not gated |

---

## Gap Summary

### Critical Gaps — ALL REMEDIATED
| Gap ID | Action | Status | Fix |
|--------|--------|--------|-----|
| G-01 | Delete Evidence | **FIXED** | `requireRole("admin")` on evidence mutations |
| G-02 | Evidence Export | **FIXED** | Covered by evidence mutations route block |
| G-03 | Delete Site | **FIXED** | DELETE /api/sites/{id} with admin check |
| G-04 | Webhook Management | **FIXED** | `requireRole("admin")` on webhooks |

### Moderate Gaps — ALL REMEDIATED
| Gap ID | Action | Status | Fix |
|--------|--------|--------|-----|
| G-05 | Delete Incident | **FIXED** | `requireRole("admin")` on DELETE |
| G-06 | Incident Status Changes | **FIXED** | `requireRole("operator")` on POST/PUT |
| G-07 | Alert/Rule Management | **FIXED** | `requireRole("operator")` on mutations |
| G-08 | Tour Management | **FIXED** | `requireRole("operator")` on mutations |
| - | Report Management | **FIXED** | `requireRole("operator")` on reports mutations |

### Partial Gaps (Frontend Only)
8 items remain where backend is protected but frontend UI does not conditionally gate controls based on role. These are defense-in-depth gaps, not vulnerabilities.

---

## Backend Protection Summary

- **Evidence mutations (POST/PUT/DELETE):** `requireRole("admin")`
- **Evidence GET (list/view):** `authMiddleware` only
- **Webhooks (ALL):** `requireRole("admin")`
- **Incident DELETE:** `requireRole("admin")`
- **Incident POST/PUT:** `requireRole("operator")`
- **Alert/Rule mutations:** `requireRole("operator")`
- **Tour mutations:** `requireRole("operator")`
- **Report mutations:** `requireRole("operator")`
- **Retention-policies:** `requireRole("admin")`
- **Camera create:** `requireRole("admin")`
- **Camera delete:** `requireRole("admin")`
- **Camera update:** `requireRole("operator")`
- **Export recording:** `requireRole("operator")`
- **PTZ control:** `requireRole("operator")`
- **Discovery scan/import:** `requireRole("operator")`
- **Legal holds:** `requireRole("admin")`
- **IP allowlist:** `requireRole("admin")`
- **User management:** `requireRole("admin")` + auth service `adminOnly`
- **SSO management:** `requireRole("admin")` + auth service `adminOnly`
- **Plugin registration/update/delete:** `requireRole("admin")`
- **Admin config:** `requireRole("admin")`
