# Discovery Enhancement Design

## Summary

Overhaul the EVMS discovery service from a single-file, in-memory, WS-Discovery-only implementation into a structured, multi-method, persistent, operationally robust pipeline suitable for large multi-site deployments.

## Architecture

```
Browser (SPA)
  │ POST/GET /api/discovery/*
  ▼
API Gateway (reverse proxy + JWT auth)
  │
  ▼
Discovery Service
  ┌──────────────────────────────────────────┐
  │ ScanOrchestrator                         │
  │  - starts/cancels scans                  │
  │  - tracks scan state in DB               │
  │  - aggregates results from scanner ch    │
  │  - deduplicates by XAddr                 │
  │                                          │
  │ ScannerPipeline                          │
  │  ┌──────────────┐ ┌──────────────┐      │
  │  │WSDiscovery   │ │IPRangeScan   │      │
  │  └──────────────┘ └──────────────┘      │
  │  ┌──────────────┐ ┌──────────────┐      │
  │  │MDNSScanner   │ │ManualIPScan  │      │
  │  └──────────────┘ └──────────────┘      │
  │                                          │
  │ ResultStore (Postgres)                   │
  │ Scheduler (per-site cron)                │
  └──────────────────────────────────────────┘
  │
  ▼
Postgres
  discovery_scans
  discovery_results
  sites.discovery_config (JSONB)
```

## Scanner Interface

```go
type Scanner interface {
    Name() string
    Scan(ctx context.Context, subnet string, ports []int, opts ScanOptions) (<-chan ScanResult, error)
}

type ScanResult struct {
    IP           string
    Port         int
    XAddr        string
    Manufacturer string
    Model        string
    Firmware     string
    SerialNumber string
    Hostname     string
    Capabilities CapabilitySet
    Error        error
}
```

### Implementations

| Scanner | Mechanism | Notes |
|---|---|---|
| WSDiscovery | UDP multicast 239.255.255.250:3702, probe for dn:NetworkVideoTransmitter | Local subnet only; queries DeviceInformation, Capabilities, Hostname per response |
| IPRangeScan | TCP connect scan on configured ports, ONVIF probe on open ports | Configurable ports (default 80,554,8080), configurable concurrency |
| MDNSScanner | mDNS query _onvif._tcp.local + _rtsp._tcp.local | Optional, disabled by default |
| ManualIP | Accepts explicit IP:port list, ONVIF probe each | For known devices, cross-subnet |

All scanners run concurrently per scan. Results deduplicated by XAddr (first wins).

## Persistence

### discovery_scans table

```sql
CREATE TABLE discovery_scans (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    site_id     UUID REFERENCES sites(id),
    status      TEXT NOT NULL DEFAULT 'pending',
    methods     TEXT[] NOT NULL,
    subnets     TEXT[],
    ports       INT[] DEFAULT '{80,554,8080}',
    total_found INT DEFAULT 0,
    error       TEXT,
    started_at  TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_by  UUID REFERENCES users(id),
    created_at  TIMESTAMPTZ DEFAULT NOW()
);
```

### discovery_results table

```sql
CREATE TABLE discovery_results (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    scan_id       UUID REFERENCES discovery_scans(id) ON DELETE CASCADE,
    site_id       UUID REFERENCES sites(id),
    ip_address    TEXT NOT NULL,
    port          INT,
    xaddr         TEXT,
    manufacturer  TEXT,
    model         TEXT,
    firmware      TEXT,
    serial_number TEXT,
    hostname      TEXT,
    capabilities  JSONB DEFAULT '{}',
    onvif_data    JSONB,
    is_new        BOOLEAN DEFAULT TRUE,
    already_in_db BOOLEAN DEFAULT FALSE,
    imported      BOOLEAN DEFAULT FALSE,
    imported_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ DEFAULT NOW()
);
```

Indexes: `(scan_id)`, `(site_id)`. Auto-purge results > 30 days (configurable).

### sites column addition

```sql
ALTER TABLE sites ADD COLUMN discovery_config JSONB DEFAULT '{}';
-- { "subnets": ["10.0.0.0/24"], "gateway_ips": ["10.0.0.1"] }
```

## API Endpoints

| Method | Path | Body / Params | Description |
|---|---|---|---|
| POST | /api/discovery/scans | {site_id, methods, subnets, ports} | Start scan, returns {id} |
| GET | /api/discovery/scans/{id} | - | Scan status + summary |
| GET | /api/discovery/scans | ?site_id=&page=&per_page= | List scans |
| POST | /api/discovery/scans/{id}/cancel | - | Cancel running scan |
| GET | /api/discovery/scans/{id}/results | ?page=&per_page=&query= | Paginated results |
| POST | /api/discovery/scans/{id}/import | {result_ids, credentials[]} | Import selected devices |
| POST | /api/discovery/credentials/test | {ip, port, username, password} | Test ONVIF creds |

## Frontend

DiscoveryPage split into 3 views:
1. **Scan Launcher** — site picker, method checkboxes, port config, subnet list, "Start Scan"
2. **Scan List** — paginated history table with status/duration/count, click to drill in
3. **Results View** — paginated device table with inline credential fields, credential test button, import with per-device progress feedback

## Scheduled Scanning

- `discovery_config` on sites table: `{ enabled, cron: "0 */6 * * *", methods: ["ws-discovery","ip-range"], ports: [80,554,8080] }`
- Scheduler checks every 60s for due scans
- Runs via ScanOrchestrator — same code path as manual scans
- Results accumulate across scans (no wipe)

## Import Flow Improvements

- Per-device credentials: `POST /api/discovery/scans/{id}/import` accepts array of `{result_id, username, password}`
- Shared credential fallback: if credentials empty for a device, use site-level ONVIF credentials
- Pre-import credential test endpoint
- Import returns per-device status: `{imported: [{result_id, camera_id}], failed: [{result_id, error}]}`
- If no credentials provided for a device and site has no ONVIF credentials, that device import fails with `no_credentials`
- `already_in_db` flag computed by matching `discovery_results.xaddr` against `cameras.connection_url` (or IP match)
- Frontend shows progressive results, not just "done"

## Operational

- Scan cancellation via context cancellation + DB status update
- Horizontal scaling: DB-backed state means multiple replicas can serve reads; writes use leader election or advisory lock
- JWT_SECRET configurable via Helm
- Full test coverage for scanners, orchestrator, result store

## Future Considerations (Out of Scope)

- Auto-provisioning via NATS `cameras.discovered` subscriber
- Active device health polling
- Trend analysis (device online/offline patterns over time)
