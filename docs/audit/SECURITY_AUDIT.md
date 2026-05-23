# DAM VMS: Security Audit Report

## 1. Vulnerability Analysis

### 1.1 Path Traversal in Playback Service
- **Location:** `services/playback/main.go`
- **Severity:** Critical
- **Description:** `filepath.Clean` is used but not validated against a base directory boundary, allowing `../../etc/passwd` style attacks.

### 1.2 Broken Authentication
- **Location:** `services/webrtc/main.go`, `services/playback/main.go`
- **Severity:** High
- **Description:** Endpoints serving live streams and recordings do not verify JWT tokens, allowing any network-adjacent actor to view video data.

### 1.3 Insecure Internal Communication
- **Severity:** Medium
- **Description:** NATS communication is unencrypted and lacks per-subject authorization.

## 2. Recommendations
1. Implement strict path validation in the playback service.
2. Integrate JWT middleware into all public-facing and streaming microservices.
3. Enable NATS TLS and implement subject-based access control.
4. Rotate and securely store all secrets (JWT keys, DB passwords).
