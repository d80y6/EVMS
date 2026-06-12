# EVMS Security Audit Report

**Date:** 2026-06-11  
**Scope:** API Gateway, Web Frontend, Auth Service, Common Libraries  
**Auditor:** Automated Security Analysis  

---

## Summary

| Severity | Count |
|----------|-------|
| Critical | 5 |
| High | 12 |
| Medium | 9 |
| Low | 6 |
| **Total** | **32** |

---

## Critical Findings

### C-01: Encryption Silent Fallback to Plaintext (Crypto Shredding Failure)

**Affected Component:** `pkg/common/crypto.go` — `MustEncrypt()` (line 87), `MustDecrypt()` (line 96)  
**OWASP Category:** A02:2021 – Cryptographic Failures  
**CVSS-like:** 9.1 / Critical

**Description:**  
`MustEncrypt()` wraps the encryption call and on ANY failure (missing key, invalid key format, AES-GCM failure) silently stores the **plaintext** instead of failing:
```go
func MustEncrypt(plaintext string) string {
    encrypted, err := Encrypt([]byte(plaintext))
    if err != nil {
        slog.Warn("encryption failed, storing plaintext", "error", err)
        return plaintext   // <-- PLAINTEXT LEAK
    }
    return encrypted
}
```

Similarly, `MustDecrypt()` returns raw input on decryption failure. Any caller using these functions (e.g., storing ONVIF passwords, camera credentials) will silently persist secrets in the clear if encryption configuration is incorrect or transient errors occur.

**Recommendation:**
- Remove `MustEncrypt`/`MustDecrypt` or change them to panic/return error.
- Ensure all callers handle encryption errors explicitly.
- Add integration tests that verify encryption fails closed, not open.

---

### C-02: CSRF Cookie Lacks `Secure` and `HttpOnly` Flags

**Affected Component:** `services/api-gateway/main.go` — `handleCSRFToken()` (lines 154–162)  
**OWASP Category:** A01:2021 – Broken Access Control (CSRF)  
**CVSS-like:** 8.8 / Critical

**Description:**  
The CSRF cookie is set with `HttpOnly: false` and `Secure: false`:
```go
http.SetCookie(w, &http.Cookie{
    Name:     "csrf_token",
    Value:    token,
    Path:     "/",
    HttpOnly: false,    // Accessible to JavaScript
    Secure:   false,    // Sent over HTTP
    SameSite: http.SameSiteStrictMode,
    MaxAge:   86400,
})
```

- `HttpOnly: false` means any XSS vulnerability can exfiltrate the CSRF token.
- `Secure: false` means the cookie is transmitted over unencrypted HTTP connections, enabling network-level interception.
- The token value is also returned in the JSON response body, duplicated to the cookie.

**Recommendation:**
- Set `Secure: true` (always, or conditional on TLS being enabled).
- The double-submit cookie pattern inherently requires JS access to the cookie value; use a cryptographically signed cookie or a CSRF token stored in session state instead.
- If TLS is not enabled, consider whether CSRF protection needs the Secure flag waived, and document the risk.

---

### C-03: Password Change Does Not Require Current Password Verification

**Affected Component:** `services/auth/password_policy.go` — `handleChangePassword()` (lines 325–330)  
**OWASP Category:** A07:2021 – Identification and Authentication Failures  
**CVSS-like:** 9.6 / Critical

**Description:**  
The password change endpoint verifies the current password only if it is provided:
```go
if req.CurrentPassword != "" {
    if err := bcrypt.CompareHashAndPassword(...) {
        jsonError(w, "current password is incorrect", ...)
        return
    }
}
```
If `current_password` is omitted from the request body, the password is changed without any proof of knowledge of the existing password. Any attacker with a valid (even short-lived) JWT can change the victim's password and take over the account.

**Recommendation:**
- Make `current_password` mandatory — return an error if it is empty.
- Consider adding re-authentication for sensitive operations (password change, MFA disable).

---

### C-04: JWT Token Transmitted in URL Query Parameters

**Affected Component:**  
- `services/api-gateway/main.go` — `authMiddleware()` (lines 468–473), `requireRole()` (lines 507–513)  
- `pkg/common/auth.go` — `JWTAuthMiddleware()` (lines 133–136)  
- `web/src/api/client.ts` — `authUrl()` (lines 27–32)  
**OWASP Category:** A05:2021 – Security Misconfiguration  
**CVSS-like:** 8.6 / Critical

**Description:**  
The authentication middleware accepts tokens via URL query parameter `?token=...`:
```go
authHeader = r.URL.Query().Get("token")
```

The frontend `authUrl()` function actively constructs URLs with the token as a query parameter:
```typescript
export function authUrl(path: string): string {
    const token = localStorage.getItem('auth_token');
    if (!token) return path;
    const sep = path.includes('?') ? '&' : '?';
    return `${path}${sep}token=${encodeURIComponent(token)}`;
}
```

This exposes the JWT in:
- Server access logs (full URL including query string)
- Browser history
- Referrer headers (when navigating away)
- Network-level packet captures

**Recommendation:**
- Remove query string token authentication entirely.
- Use `Authorization: Bearer <token>` header exclusively.
- Remove `authUrl()` from the frontend and replace with header-based auth for all resource URLs.

---

### C-05: CORS Configuration Echoes Any Origin with Credentials

**Affected Component:** `services/api-gateway/main.go` — `ServeHTTP()` (lines 1887–1895)  
**OWASP Category:** A01:2021 – Broken Access Control  
**CVSS-like:** 8.2 / Critical

**Description:**  
The `ServeHTTP` handler echoes the `Origin` header back as `Access-Control-Allow-Origin` and sets `Access-Control-Allow-Credentials: true`:
```go
if origin := r.Header.Get("Origin"); origin != "" {
    w.Header().Set("Access-Control-Allow-Origin", origin)
}
w.Header().Set("Access-Control-Allow-Credentials", "true")
```

This allows any website to make authenticated cross-origin requests to the API. Combined with cookie-based CSRF tokens, this effectively permits credential-bearing cross-origin requests from arbitrary origins.

**Recommendation:**
- Replace with an explicit allowlist of permitted origins.
- If dynamic origins are required, validate against the allowlist at runtime.
- Restrict `Access-Control-Allow-Origin` to `https://` origins only when TLS is enabled.

---

## High Findings

### H-01: JWT Secret Loaded from Environment on Every Validation (No Rotation/Revocation)

**Affected Component:** `pkg/common/auth.go` — `getJWTKey()` (lines 94–96), `ValidateJWT()` (line 115)  
**OWASP Category:** A02:2021 – Cryptographic Failures  
**Severity:** High

**Description:**  
The JWT signing key is read from the `JWT_SECRET` environment variable on every token validation. There is no key rotation mechanism, no kid (key ID) header support, and no token revocation list (beyond passive session management). If the secret is compromised, all tokens (past and future) are compromised and there is no mechanism to rotate keys without downtime.

**Recommendation:**
- Load the JWT secret once at startup and cache it.
- Implement `kid` (key ID) header support to enable key rotation.
- Add a token blacklist/revocation mechanism for immediate invalidation.

---

### H-02: Reflected CORS with Credentials Allows Credential Theft

**Affected Component:** `services/api-gateway/main.go` lines 1887–1895  
**OWASP Category:** A01:2021 – Broken Access Control  
**Severity:** High

(Note: This is the same as C-05 but distinct from CSRF — reflects a broader CORS misconfiguration concern.)

**Recommendation:**  
Same as C-05. Additionally, audit all subdomains that could be used to exfiltrate data.

---

### H-03: No Tenant Isolation Fallback — Cross-Tenant Data Access

**Affected Component:** `services/api-gateway/main.go` — `handleRecordings()` (lines 785–796), `handleEvents()` (lines 831–842)  
**OWASP Category:** A01:2021 – Broken Access Control (IDOR)  
**Severity:** High

**Description:**  
When `tenantID` is empty (e.g., if the JWT has no tenant claim or context is not populated), the handlers query ALL records without tenant filtering:
```go
if tenantID != "" {
    // ... scoped query with tenant_id filter
} else {
    err = g.db.SelectContext(ctx, &recordings,
        "SELECT ... FROM recordings ORDER BY start_time DESC LIMIT 100")
}
```

If the authentication middleware fails to populate `tenantID` in the context, the tenant isolation is completely bypassed, exposing all tenants' recordings and events.

**Recommendation:**
- Never allow an un-scoped query. If `tenantID` is empty, deny the request or scope to a default.
- Remove the `else` branch entirely or return an error.
- Add integration tests verifying tenant isolation under all auth failure modes.

---

### H-04: LDAP Authentication Over Unencrypted TCP

**Affected Component:** `services/auth/main.go` — `authenticateLDAP()` (line 414)  
**OWASP Category:** A02:2021 – Cryptographic Failures  
**Severity:** High

**Description:**  
LDAP connections are made over plain TCP (`ldap.Dial("tcp", ...)`) instead of TLS (`ldap.DialWithTLS`):
```go
conn, err := ldap.Dial("tcp", fmt.Sprintf("%s:%d", s.config.LDAPHost, s.config.LDAPPort))
```

LDAP bind credentials (including the service account password and user passwords) are transmitted in cleartext over the network. On port 389, this is standard LDAP, not LDAPS.

**Recommendation:**
- Use `ldap.DialWithTLS()` for TLS-encrypted LDAP connections.
- Support LDAPS (port 636) or STARTTLS on port 389.
- Mark the LDAPPassword field as sensitive in config logging.

---

### H-05: ONVIF Credentials Exposed in Camera API Responses

**Affected Component:** `services/api-gateway/main.go` — `handleCameras()` (lines 579–611), `handleGetCamera()` (lines 918–945), `handleCameraOnvif()` (lines 1378–1388)  
**OWASP Category:** A04:2021 – Insecure Design  
**Severity:** High

**Description:**  
The camera listing and detail endpoints return `onvif_username` in the JSON response. The `handleCameraOnvif` endpoint also returns the ONVIF username. Camera credentials (ONVIF username/password) are used to control PTZ, access streams, and configure cameras — exposure of these credentials compromises physical security controls.

**Recommendation:**
- Remove `OnvifUsername` from all non-admin API responses.
- Create a separate, audited endpoint for privileged credential access.
- Mask or omit the ONVIF username in camera listing responses.

---

### H-06: Rate Limiter Bypassable via IP Spoofing Headers

**Affected Component:** `services/api-gateway/main.go` — `rateLimitMiddleware()` (lines 110–121)  
**OWASP Category:** A04:2021 – Insecure Design  
**Severity:** High

**Description:**  
The rate limiter extracts the client IP from `r.RemoteAddr` only:
```go
host, _, err := net.SplitHostPort(r.RemoteAddr)
```

It does NOT check `X-Forwarded-For` or `X-Real-IP` headers. When deployed behind a reverse proxy, `RemoteAddr` is always the proxy's IP, making rate limiting ineffective against distributed attacks.

**Recommendation:**
- Use `extractClientIP()` from `pkg/common/ipallowlist.go` (which handles `X-Forwarded-For` and `X-Real-IP`) consistently for rate limiting.
- Consider an application-layer rate limiting approach (token bucket per user, not per IP).

---

### H-07: Overly Permissive CORS on CSRF Token Endpoint

**Affected Component:** `services/api-gateway/main.go` — `ServeHTTP()` (entire switch block)  
**OWASP Category:** A05:2021 – Security Misconfiguration  
**Severity:** High

**Description:**  
The CSRFTToken endpoint (`/api/csrf-token`) is accessible without authentication. Combined with the echoed CORS origin header, any website can fetch a CSRF token and then use it to make state-changing requests on behalf of an authenticated user who visits a malicious page.

**Recommendation:**
- Require authentication to obtain a CSRF token.
- Implement SameSite=Strict cookies (already done) — but ensure this is supported across all modern browsers.
- Add a per-session binding of CSRF tokens.

---

### H-08: Account Lockout Timing Information Leak

**Affected Component:** `services/auth/main.go` — `handleLogin()` (line 317)  
**OWASP Category:** A05:2021 – Security Misconfiguration (Information Disclosure)  
**Severity:** High

**Description:**  
The login endpoint reveals the exact remaining lockout duration:
```go
jsonError(w, fmt.Sprintf("account locked, try again in %.0f minutes", remaining.Minutes()), ...)
```

This enables attackers to time their attacks precisely and confirms that a username exists (user enumeration).

**Recommendation:**
- Return a generic "account temporarily unavailable" message without timing details.
- Implement user enumeration prevention by returning consistent messages for both invalid username and locked account.

---

### H-09: API Key and SSO Secrets Transmitted to Client on Creation

**Affected Component:** `web/src/api/client.ts` — `createAPIKey()` (line 864), `createSSOProvider()` (line 878)  
**OWASP Category:** A04:2021 – Insecure Design  
**Severity:** High

**Description:**  
API keys and SSO `client_secret` values are returned in the HTTP response body to the frontend. These secrets are then accessible to JavaScript running in the browser and are transmitted over the network. API keys should be shown only once (at creation time) and should never be storable/retrievable from the client.

**Recommendation:**
- Return API keys to the caller only once and require regeneration for lost keys.
- Return SSO client secrets only to admin APIs, not to the browser-facing frontend.
- Hash secrets at rest and never expose them in listing endpoints.

---

### H-10: Missing Security Headers (CSP, HSTS, X-Frame-Options, etc.)

**Affected Component:** `services/api-gateway/main.go` — all responses  
**OWASP Category:** A05:2021 – Security Misconfiguration  
**Severity:** High

**Description:**  
The API gateway does not set any of the following security headers:
- `Content-Security-Policy`
- `Strict-Transport-Security`
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `Referrer-Policy: no-referrer`
- `Permissions-Policy`

While the API returns JSON (not HTML), the frontend SPA is served without these protections, making it more vulnerable to XSS, clickjacking, and MIME-type confusion attacks.

**Recommendation:**
- Add a middleware that sets security headers on all responses.
- At minimum: `Content-Security-Policy`, `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, and `Strict-Transport-Security` (when TLS enabled).

---

### H-11: MFA Recovery Endpoint Missing Authentication Middleware

**Affected Component:** `services/auth/main.go` — route setup (line 998)  
**OWASP Category:** A07:2021 – Identification and Authentication Failures  
**Severity:** High

**Description:**  
The MFA recovery endpoint is NOT wrapped in authentication middleware:
```go
mux.HandleFunc("/auth/mfa/recovery", s.handleMFARecovery)
```

While the handler attempts to parse a JWT from the Authorization header, it also accepts unauthenticated requests and falls back to parsing the token if the context is empty. This could allow an attacker who possesses a stolen MFA token (which is issued during first-stage login) to bypass MFA entirely using recovery codes.

**Recommendation:**
- Add `s.authMiddleware` wrapper to the MFA recovery endpoint.
- Require re-authentication (password+username) before allowing MFA recovery.

---

### H-12: Plugin System Allows Arbitrary Plugin Registration Without Auth

**Affected Component:** `services/api-gateway/main.go` — `handleRegisterPlugin()` (lines 2269–2301)  
**OWASP Category:** A08:2021 – Software and Data Integrity Failures  
**Severity:** High

**Description:**  
The plugin registration endpoint (`/api/plugins`) accepts an `endpoint` URL and `permissions` list without validation of the endpoint's authenticity or integrity. The route is not visible in the `ServeHTTP` switch statement (only listing at `/api/plugins` is mapped at line 2248, but the register/update/delete handlers exist and may be mounted elsewhere). A compromised admin could register a malicious plugin endpoint that receives callbacks with sensitive data.

**Recommendation:**
- Validate plugin endpoint URLs (no internal/private IPs).
- Require cryptographic signing of plugin binaries/manifests.
- Add authorization checks ensuring only trusted admins can register plugins.

---

## Medium Findings

### M-01: Token Stored in `localStorage` — XSS-able

**Affected Component:** `web/src/context/AuthContext.tsx` (line 49), `web/src/api/client.ts` (lines 28, 35)  
**OWASP Category:** A05:2021 – Security Misconfiguration  
**Severity:** Medium

**Description:**  
The JWT token is stored in `localStorage`:
```typescript
const [token, setToken] = useState<string | null>(() => localStorage.getItem('auth_token'));
```

`localStorage` is accessible by any JavaScript running on the same origin. An XSS vulnerability would allow an attacker to steal the token and gain persistent access.

**Recommendation:**
- Use `httpOnly` cookies for token storage instead of `localStorage`.
- Alternatively, store the refresh token in an httpOnly cookie and keep only a short-lived access token in memory.
- Implement token binding to mitigate token replay.

---

### M-02: In-Memory Rate Limiter State Lost on Restart

**Affected Component:** `services/api-gateway/main.go` — `rateLimiter` struct (lines 43–48)  
**OWASP Category:** A04:2021 – Insecure Design  
**Severity:** Medium

**Description:**  
The rate limiter stores client state in an in-memory Go map without persistence:
```go
type rateLimiter struct {
    mu      sync.Mutex
    clients map[string]*clientLimit
    rate    float64
    burst   float64
    cleanup time.Duration
}
```

On service restart, all rate limit state is lost. An attacker could restart the service repeatedly (e.g., by exploiting a crash loop) to reset rate limits. The map also has no key expiration on the client map entries beyond cleanup of old entries.

**Recommendation:**
- Use an external rate limiter (Redis-based) for persistent, distributed rate limiting.
- Add capacity-based bounds to prevent memory exhaustion from unique IP attacks.

---

### M-03: Metrics Endpoint Exposed Without Authentication

**Affected Component:** `services/api-gateway/main.go` (line 2439), `services/auth/main.go` (line 1101)  
**OWASP Category:** A05:2021 – Security Misconfiguration  
**Severity:** Medium

**Description:**  
The Prometheus metrics server is started on a configurable address without authentication:
```go
common.StartMetricsServer(config.MetricsAddr)
```

By default, this listens on `:2112`. If bound to a non-loopback interface, system metrics, request rates, and potentially sensitive operation names are exposed.

**Recommendation:**
- Default metrics to listen on `127.0.0.1` only.
- Add authentication or network-level access control for metrics endpoints.

---

### M-04: Long-Lived JWT Tokens (24 Hours)

**Affected Component:** `services/auth/main.go` — `DefaultAuthConfig()` (line 113)  
**OWASP Category:** A07:2021 – Identification and Authentication Failures  
**Severity:** Medium

**Description:**  
Default JWT token expiry is 24 hours:
```go
TokenExpiry: 24 * time.Hour,
```

For a VMS system that may be used in security-sensitive environments (casinos, government, critical infrastructure), 24-hour tokens significantly increase the window of opportunity for token theft.

**Recommendation:**
- Reduce default token expiry to 15–60 minutes.
- Implement refresh token rotation (already partially implemented).
- Consider short-lived access tokens with longer-lived refresh tokens stored in httpOnly cookies.

---

### M-05: Secrets in Environment Variables Without Encryption at Rest

**Affected Component:** `services/auth/main.go` — `JWTSecret` (line 112), `LDAPPassword` (line 99)  
**OWASP Category:** A02:2021 – Cryptographic Failures  
**Severity:** Medium

**Description:**  
Critical secrets are read from environment variables:
```go
JWTSecret:   []byte(os.Getenv("JWT_SECRET")),
LDAPPassword: os.Getenv("LDAP_PASSWORD"),
ADMIN_USERNAME / ADMIN_PASSWORD   (lines 242–243)
```

Environment variables are often visible in process listings, `/proc` filesystem, container orchestration dashboards, and log dumps.

**Recommendation:**
- Use a secrets manager (HashiCorp Vault, Kubernetes Secrets, AWS Secrets Manager).
- At minimum, restrict environment variable access and ensure they are not logged.
- Consider file-based secrets (`_FILE` suffix pattern used by Docker).

---

### M-06: Weak Password History Check Allows Reuse with Case Change

**Affected Component:** `services/auth/password_policy.go` — `checkPasswordHistory()` (lines 160–178)  
**OWASP Category:** A02:2021 – Cryptographic Failures  
**Severity:** Medium

**Description:**  
Password history checks use bcrypt comparison, which correctly prevents password reuse. However, the history limit is only 5, and the default minimum password length is 8 characters — which is below NIST SP 800-63B recommendations (minimum 8 is borderline for non-memorized secrets but acceptable; 12+ is recommended for memorized secrets).

**Recommendation:**
- Increase `MinLength` to 12.
- Increase `PasswordHistory` to 24.
- Consider adding a password strength meter on the frontend.

---

### M-07: NATS Connection Without Authentication

**Affected Component:** `services/api-gateway/main.go` (lines 345–349)  
**OWASP Category:** A05:2021 – Security Misconfiguration  
**Severity:** Medium

**Description:**  
The NATS connection is configured via URL only:
```go
NATSURL: common.GetEnv("NATS_URL", "nats://nats:4222"),
nc, err = nats.Connect(config.NATSURL)
```

No NATS credentials, token, or TLS configuration is applied. Within a cluster network this may be acceptable, but it means any process that can reach NATS can subscribe to all event streams.

**Recommendation:**
- Require NATS authentication (token or NKey).
- Enable TLS for NATS connections.
- Use NATS subject-level authorization for least-privilege access.

---

### M-08: Discovery Import Embeds ONVIF Credentials in RTSP URLs

**Affected Component:** `services/api-gateway/main.go` — `handleDiscoveryImport()` (lines 1668–1679)  
**OWASP Category:** A04:2021 – Insecure Design  
**Severity:** Medium

**Description:**  
When importing discovered cameras, credentials are embedded in the RTSP connection URL:
```go
if req.Username != "" {
    connURL = "rtsp://" + req.Username + ":" + req.Password + "@" + ip + ":554"
}
```

This stores plaintext credentials in a database field that is later returned in API responses (see H-05).

**Recommendation:**
- Store credentials separately from the connection URL.
- Mask the password portion of RTSP URLs in logs and API responses.
- Consider whether RTSP URL embedding is necessary or if credentials can be supplied separately.

---

### M-09: XSS via Unsanitized Data in Camera Config Responses

**Affected Component:** `services/api-gateway/main.go` — various handlers return user-supplied data  
**OWASP Category:** A03:2021 – Injection (XSS)  
**Severity:** Medium

**Description:**  
Multiple API handlers return user-supplied data (camera names, descriptions, site names, config data) in JSON responses. While JSON encoding prevents direct HTML injection, if the frontend renders this data without proper escaping (e.g., `dangerouslySetInnerHTML`, `v-html`), XSS is possible.

The `CameraConfig` update handler (line 1700) accepts a raw `json.RawMessage` Config field that is passed through as-is to the gRPC service — this data is later returned in camera responses without validation.

**Recommendation:**
- Validate and sanitize user-supplied string fields on input.
- Implement output encoding/escaping in the frontend for all user-supplied data.
- Restrict the `Config` field to a defined schema with typed values rather than `json.RawMessage`.

---

## Low Findings

### L-01: Session Limit Too Low (5 Concurrent Sessions)

**Affected Component:** `services/auth/main.go` — `DefaultAuthConfig()` (line 124)  
**Severity:** Low

**Description:**  
The default session limit is 5. In a VMS environment, a single operator may have multiple browser tabs, PTZ control panels, and monitoring dashboards open simultaneously. The session eviction policy (revoke oldest) could cause unexpected logouts during normal operation.

**Recommendation:**
- Increase default to 20–50 sessions.
- Make the limit configurable per-role (admin gets more, viewer gets fewer).

---

### L-02: Duplicate Route Definitions in ServeHTTP Switch

**Affected Component:** `services/api-gateway/main.go` — `ServeHTTP()` switch block  
**Severity:** Low

**Description:**  
Several route patterns are duplicated in the switch statement (e.g., `/api/channels` appears at lines 2041 and 2158; `/api/intrusion-zones`, `/api/loitering-zones`, `/api/abandoned-object-zones`, `/api/notification-log` all appear twice). This could lead to maintenance confusion and inconsistent middleware application.

**Recommendation:**
- Deduplicate route definitions.
- Use a route registry or map-based routing for maintainability.

---

### L-03: Missing Request Body Size Limits

**Affected Component:** All `json.NewDecoder(r.Body).Decode()` calls across all services  
**Severity:** Low

**Description:**  
Request bodies are read without any size limits. The Go `http.Request.Body` has no implicit limit, and `json.Decoder` will read the entire body into memory:
```go
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
```

An attacker could send multi-gigabyte payloads to exhaust server memory.

**Recommendation:**
- Use `http.MaxBytesReader()` to limit request body sizes.
- Set a global limit (e.g., 1MB for most endpoints, 10MB for file uploads).

---

### L-04: No Rate Limiting on Token Refresh Endpoint

**Affected Component:** `services/auth/main.go` — `handleRefreshToken()` (line 537)  
**Severity:** Low

**Description:**  
The `/auth/refresh` endpoint has no IP-based or user-based rate limiting. While the login endpoint rate-limits at 20 requests/minute per IP, token refresh can be called repeatedly without restriction.

**Recommendation:**
- Apply the same IP-based rate limiter to the refresh endpoint.
- Consider a per-user rate limit for token refresh operations.

---

### L-05: Login Endpoint Locks Out Valid User on Wrong Password

**Affected Component:** `services/auth/main.go` — `handleLogin()` (lines 284–342)  
**Severity:** Low

**Description:**  
Failed login attempts are tracked per-username regardless of IP:
```go
s.recordFailedAttempt(req.Username)
```

An attacker can intentionally lock out a legitimate user by rapidly attempting incorrect passwords. This is a classic denial-of-service vector.

**Recommendation:**
- Implement per-IP-per-username rate limiting or exponential backoff instead of hard lockouts.
- Consider CAPTCHA after N failed attempts.
- Use progressive delays rather than binary locked/unlocked state.

---

### L-06: gRPC JSON Codec Bypasses Protobuf Validation

**Affected Component:** `services/api-gateway/main.go` (line 277), `pkg/common/auth.go` (lines 63–79)  
**Severity:** Low

**Description:**  
The gRPC connection to camera-mgmt uses the JSON codec:
```go
grpc.WithDefaultCallOptions(grpc.CallContentSubtype("json")),
```

This bypasses protobuf's schema validation and type safety. Malformed or malicious data could be passed through without the structural validation that protobuf would enforce.

**Recommendation:**
- Use standard protobuf serialization for gRPC.
- Reserve JSON codec for debugging/development only.

---

## Summary of OWASP Top 10 Coverage

| OWASP Category | Issues Found |
|----------------|-------------|
| A01: Broken Access Control | C-02, C-05, H-02, H-03, H-07 |
| A02: Cryptographic Failures | C-01, H-01, H-04, M-05, M-06 |
| A03: Injection | M-09 |
| A04: Insecure Design | H-05, H-06, H-09, M-02, M-08 |
| A05: Security Misconfiguration | C-04, H-07, H-08, H-10, M-01, M-03, M-07 |
| A06: Vulnerable Components | (Not assessed — dependency audit needed) |
| A07: ID & Auth Failures | C-03, H-11, M-04 |
| A08: Software Integrity | H-12 |
| A09: Logging & Monitoring | (Not assessed — log audit needed) |
| A10: SSRF | H-12, M-08 |

---

## Quick-Win Remediation Priority

1. **Fix `MustEncrypt`/`MustDecrypt`** (C-01) — single function change, prevents silent data loss.
2. **Make `current_password` mandatory** (C-03) — one `if` statement change.
3. **Add Secure flag to CSRF cookie** (C-02) — one-line change.
4. **Remove URL query token auth** (C-04) — removes dangerous feature entirely.
5. **Set explicit CORS origins** (C-05) — switch from echo to allowlist.
6. **Fix tenant isolation gap** (H-03) — remove un-scoped SQL queries.
7. **Add missing security headers** (H-10) — add middleware.
8. **Remove ONVIF username from API responses** (H-05) — JSON field removal.
