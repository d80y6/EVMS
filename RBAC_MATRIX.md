# EVMS RBAC Audit Matrix

**Date:** 2026-06-11
**Scope:** Full-stack RBAC analysis (frontend route guards, component-level checks, backend gateway middleware, backend service middleware)

---

## Roles Identified

| Role       | Level | Description                                          |
|------------|-------|------------------------------------------------------|
| `viewer`   | 1     | Read-only access to cameras, recordings, events      |
| `operator` | 2     | Can control cameras, export, run discovery, update cameras |
| `admin`    | 3     | Full system administration, user/SSO/config management |

**Not found in codebase:** `investigator`, `auditor` (these roles were considered but are not implemented in the current codebase).

---

## Role Hierarchy (Backend Enforcement)

Defined in `services/api-gateway/main.go` (line 525-529):

```go
roleLevels := map[string]int{
    "viewer":   1,
    "operator": 2,
    "admin":    3,
}
```

The `requireRole(minRole)` middleware enforces that the user's role level is >= the required level.

---

## Frontend Protection Mechanisms

### 1. Route-Level (`ProtectedRoute.tsx`)
- Checks `isAuthenticated` only (any authenticated user passes)
- No role-based route gating

### 2. Navigation-Level (`Layout.tsx`)
- Admin nav item hidden unless `role === 'admin'` (line 69)

### 3. Page-Level (component-mounted checks)
| Page           | Check                     | What it protects                     |
|----------------|---------------------------|---------------------------------------|
| AdminPage      | `role !== 'admin'`        | User management + API keys + sites   |
| ConfigPage     | `role !== 'admin'`        | System configuration CRUD             |
| SsoPage        | `role !== 'admin'`        | SSO provider management               |
| MapPage        | `role === 'admin' \|\| 'operator'` | Map edit capabilities        |
| MapsPage       | `role === 'admin' \|\| 'operator'` | GIS edit capabilities       |

### 4. Pages WITHOUT role checks
- CamerasPage (add/edit/delete visible to all roles)
- WebhooksPage (CRUD visible to all roles)
- EvidencePage (create/delete/export visible to all roles)
- IncidentsPage (create/update/status visible to all roles)
- RetentionPage (global + per-camera retention visible to all roles)
- ExportPage (recording export visible to all roles)
- SettingsPage (tour management, per-camera retention visible to all roles)

---

## Backend Protection Mechanisms

### 1. Gateway `authMiddleware` (line 464)
- Validates JWT, extracts claims, sets context headers
- Used for: any authenticated user, no role discrimination

### 2. Gateway `requireRole(minRole)` (line 502)
- Validates JWT, checks role level against minimum
- Used for: operations requiring specific role clearance

### 3. Auth Service `adminOnly` (auth/main.go, line 774)
- Second layer of defense for admin operations
- Checks `claims.Role == "admin"` directly
- Protects: user CRUD, SSO provider admin

### 4. gRPC-level (camera-mgmt service)
- No role checks at gRPC level
- Only tenant-scoping for multi-tenant isolation
- Relies entirely on gateway for authz

---

## RBAC Matrix

### Critical Actions

| # | Action | Role Required | Frontend Protection | Backend Protection | Status |
|---|--------|--------------|---------------------|--------------------|--------|
| 1 | **Delete Camera** | `admin` | **None** - CamerasPage shows delete button for all authenticated users | `requireRole("admin")` at gateway (line 2210) | **Partial Gap** - Backend protected, frontend does not gate |
| 2 | **Delete Site** | `admin` | **None** - No delete UI exists on AdminPage; only "Add Site" is available | **Route does not exist** - camera-mgmt has gRPC `DeleteSite`, but gateway has NO HTTP route for DELETE /api/sites/{id} | **MISSING** - Capability exists in gRPC but is not exposed via gateway |
| 3 | **Delete Evidence (Case)** | `admin` (expected) | **None** - EvidencePage shows Delete button for all authenticated users | `authMiddleware` only (line 2066) - **no requireRole** | **Gap** - Any authenticated user (incl. viewers) can delete evidence |
| 4 | **Delete Incident** | `operator` (expected) | **None** - No explicit delete button in UI | `authMiddleware` only (line 2071) - **no requireRole** | **Gap** - No role enforcement on incident backend |
| 5 | **Export Evidence (bundle)** | `operator` (expected) | **None** - EvidencePage shows Export Bundle for all users | `authMiddleware` only (line 2066) - `/api/evidence/cases/{id}/export` has **no requireRole**. Compare: `/api/export` (recording export) correctly uses `requireRole("operator")` (line 2037) | **Gap** - Evidence bundle export lacks role check despite recording export being protected |
| 6 | **Export Recording** | `operator` | **None** - ExportPage has no role check | `requireRole("operator")` at gateway (line 2037) | **Partial Gap** - Backend protected, frontend does not gate |
| 7 | **System Settings Changes** | `admin` | **Protected** - ConfigPage checks `role !== 'admin'` and denies access | `requireRole("admin")` at gateway (line 2057) | **Protected** |
| 8 | **Retention Settings (global)** | `admin` | **None** - RetentionPage has no role check | `requireRole("admin")` at gateway (line 1997) | **Partial Gap** - Backend protected, frontend does not gate |
| 9 | **Retention Settings (per-camera)** | `operator` | **None** - SettingsPage has no role check | `requireRole("operator")` at gateway (line 1962) for /api/cameras/{id}/config | **Partial Gap** - Backend protected, frontend does not gate |
| 10 | **Webhook Management (CRUD)** | `admin` (expected) | **None** - WebhooksPage has no role check | `authMiddleware` only (line 1946) - **no requireRole** | **Gap** - Any authenticated user can manage webhooks |
| 11 | **SSO Provider Management** | `admin` | **Protected** - SsoPage checks `role !== 'admin'` at page level | Gateway: `authMiddleware` only (line 2153); **Auth service**: `adminOnly` on `/auth/admin/sso/providers` (auth/main.go:1014) - defense in depth | **Protected** (by auth service layer) |
| 12 | **MFA Administration** | `admin` (for enforcement) | **None** - MfaPage is user-level self-service only | Gateway: `authMiddleware` only (line 2138); Auth service: `authMiddleware` only on `/auth/mfa/` | **Acceptable** - User self-service only; no admin MFA enforcement UI/API exists |
| 13 | **User Management (CRUD)** | `admin` | **Protected** - AdminPage checks `role !== 'admin'` at page level | Gateway: `requireRole("admin")` (line 1974); Auth service: `adminOnly` on `/auth/admin/users/` (auth/main.go:1030-1049) | **Protected** |

### Secondary Actions

| # | Action | Role Required | Frontend Protection | Backend Protection | Status |
|---|--------|--------------|---------------------|--------------------|--------|
| 14 | **Create Camera** | `admin` | **None** - CamerasPage shows +Add Camera for all users | `requireRole("admin")` at gateway (line 2206) | **Partial Gap** - Backend protected, frontend does not gate |
| 15 | **Update Camera** | `operator` | **None** - CamerasPage shows Edit for all users | `requireRole("operator")` at gateway (line 2208) | **Partial Gap** - Backend protected, frontend does not gate |
| 16 | **Create Site** | `admin` | **Protected** - AdminPage gates at page level (admin check) | `requireRole("admin")` at gateway (line 1978) | **Protected** |
| 17 | **PTZ Control** | `operator` | **None** - PTZ controls visible to all | `requireRole("operator")` at gateway (lines 1958-1960) | **Partial Gap** - Backend protected, frontend does not gate |
| 18 | **Discovery Scan** | `operator` | **None** - Discovery wizard from CamerasPage | `requireRole("operator")` at gateway (line 2185) | **Partial Gap** - Backend protected, frontend does not gate |
| 19 | **Discovery Import** | `operator` | **None** | `requireRole("operator")` at gateway (line 2195) | **Partial Gap** - Backend protected, frontend does not gate |
| 20 | **Legal Holds** | `admin` | **Protected** - Behind AdminPage page-level admin gate | `requireRole("admin")` at gateway (line 2032) | **Protected** |
| 21 | **IP Allowlist Management** | `admin` | **None** (no dedicated UI page found) | `requireRole("admin")` at gateway (lines 1968-1972) | **Partial Gap** - Backend protected, frontend UI may not exist |
| 22 | **API Key Management** | `admin` | **Protected** - Behind AdminPage page-level admin gate | Gateway: `authMiddleware` (line 2148); Auth service: `authMiddleware` on `/auth/api-keys` | **Partial Gap** - No backend role check on API key endpoints in auth service, but UI is gated |
| 23 | **Tour Management** | `operator` (expected) | **None** - SettingsPage has no role check | `authMiddleware` only on `/api/tours` (line 2083) | **Gap** - No role enforcement |
| 24 | **Audit Log Access** | `viewer` (read) | **None** | `authMiddleware` only on `/api/audit/` (lines 2107-2122) | **Acceptable** - Read-only, all authenticated users |
| 25 | **Incident Status Changes** | `operator` (expected) | **None** - Status dropdown visible to all | `authMiddleware` only (line 2071) | **Gap** - Any user can modify incident status |
| 26 | **Alert/Rule Management** | `operator` (expected) | **None** | `authMiddleware` only on `/api/alerts`, `/api/rules` (lines 2075-2082) | **Gap** - No role enforcement |

---

## Gap Summary

### Critical Gaps (Requires Immediate Attention)

| Gap ID | Action | Severity | Detail |
|--------|--------|----------|--------|
| G-01 | **Delete Evidence** | **Critical** | Any authenticated user (viewer) can delete evidence cases. No backend role check (`authMiddleware` only) and no frontend gate. |
| G-02 | **Evidence Export (bundle)** | **Critical** | `/api/evidence/cases/{id}/export` only uses `authMiddleware`. Vulnerable to evidence exfiltration by viewers. Inconsistent with `/api/export` which correctly requires operator. |
| G-03 | **Delete Site** | **Critical** | The `DeleteSite` gRPC exists in camera-mgmt but the gateway has **no HTTP route** for it. Cannot be performed via API at all - blocked, but also no audit trail at gateway level. |
| G-04 | **Webhook Management** | **High** | Any authenticated user can create/delete webhooks. Webhooks can exfiltrate event data to external systems. |

### Moderate Gaps

| Gap ID | Action | Severity | Detail |
|--------|--------|----------|--------|
| G-05 | **Delete Incident** | **High** | No backend role check; any authenticated user could potentially delete incidents. |
| G-06 | **Incident Status Changes** | **Medium** | Any authenticated user can change incident status. No audit of who changed status. |
| G-07 | **Alert/Rule Management** | **Medium** | Any authenticated user can create/modify alert rules. |
| G-08 | **Tour Management** | **Medium** | Any authenticated user can create/delete PTZ tours. |

### Partial Gaps (Frontend Only - Backend Protected)

| Gap ID | Action | Severity | Detail |
|--------|--------|----------|--------|
| G-09 | **Delete Camera** | **Medium** | Backend admin-protected, but frontend shows delete to all roles. |
| G-10 | **Create Camera** | **Low** | Backend admin-protected, but frontend shows add to all roles. |
| G-11 | **Update Camera** | **Low** | Backend operator-protected, but frontend shows edit to all roles. |
| G-12 | **Export Recording** | **Low** | Backend operator-protected, but frontend shows export to all roles. |
| G-13 | **Global Retention Settings** | **Low** | Backend admin-protected, but frontend has no role check. |
| G-14 | **Per-Camera Retention** | **Low** | Backend operator-protected, but frontend has no role check. |
| G-15 | **PTZ Control** | **Low** | Backend operator-protected, but frontend does not gate. |
| G-16 | **Discovery Scan/Import** | **Low** | Backend operator-protected, but frontend does not gate. |

---

## Key Observations

1. **No `investigator` or `auditor` role implemented**: These roles appear in documentation/plans but are not found anywhere in the codebase. Only `viewer`, `operator`, and `admin` exist.

2. **Inconsistent backend protection**: Some write operations (create camera, delete camera, retention-policies) use `requireRole`, while equally sensitive operations (evidence CRUD, webhook CRUD, alert/rule management) use only `authMiddleware`. This gap-fill needs prioritization.

3. **Evidence subsystem is the weakest link**: Every operation on evidence (create, delete, export, share) is protected by only `authMiddleware`. Given that evidence is the most legally sensitive data in a VMS, this is the highest-priority gap.

4. **Auth service provides defense-in-depth for user management and SSO**: The auth service's `adminOnly` middleware provides a second layer of protection for user and SSO admin endpoints, even when the gateway only uses `authMiddleware`.

5. **Frontend role gating is sparse**: Of ~35 pages, only 4 pages perform role checks. Most sensitive operations (delete evidence, manage webhooks, manage tours) are visible and accessible to all authenticated users in the UI.

6. **Backend protection exists but frontend does not enforce**: Several operations have backend `requireRole` checks (camera CRUD, PTZ, export, discovery) but the corresponding frontend pages do not conditionally show/hide buttons based on role. This is a defense-in-depth gap, not a direct vulnerability, but it creates confusing UX where users see buttons that will fail with 403 errors.

---

## Files Audited

| File | Purpose |
|------|---------|
| `/home/ubuntu/EVMS/web/src/context/AuthContext.tsx` | Auth context, JWT parsing, role extraction |
| `/home/ubuntu/EVMS/web/src/components/ProtectedRoute.tsx` | Route authentication guard |
| `/home/ubuntu/EVMS/web/src/components/Layout.tsx` | Sidebar navigation, admin-only nav items |
| `/home/ubuntu/EVMS/services/api-gateway/main.go` | Gateway route registration, authMiddleware, requireRole |
| `/home/ubuntu/EVMS/pkg/common/auth.go` | JWT validation, claims struct, auth utilities |
| `/home/ubuntu/EVMS/services/auth/main.go` | Auth service routes, adminOnly middleware |
| `/home/ubuntu/EVMS/services/camera-mgmt/main.go` | Camera service gRPC handlers (DeleteCamera, DeleteSite) |
| `/home/ubuntu/EVMS/web/src/pages/AdminPage.tsx` | User management, API keys, site creation |
| `/home/ubuntu/EVMS/web/src/pages/CamerasPage.tsx` | Camera CRUD UI |
| `/home/ubuntu/EVMS/web/src/pages/EvidencePage.tsx` | Evidence case CRUD, export, share |
| `/home/ubuntu/EVMS/web/src/pages/WebhooksPage.tsx` | Webhook CRUD |
| `/home/ubuntu/EVMS/web/src/pages/ConfigPage.tsx` | System configuration |
| `/home/ubuntu/EVMS/web/src/pages/SsoPage.tsx` | SSO provider management |
| `/home/ubuntu/EVMS/web/src/pages/RetentionPage.tsx` | Retention policy management |
| `/home/ubuntu/EVMS/web/src/pages/ExportPage.tsx` | Recording export |
| `/home/ubuntu/EVMS/web/src/pages/IncidentsPage.tsx` | Incident management |
| `/home/ubuntu/EVMS/web/src/pages/SettingsPage.tsx` | User settings, tours, per-camera retention |
| `/home/ubuntu/EVMS/web/src/pages/MfaPage.tsx` | MFA self-enrollment |
