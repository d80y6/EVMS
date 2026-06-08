# Discovery Enhancement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Transform the EVMS discovery service from a single-file, in-memory, WS-Discovery-only implementation into a structured, multi-method, persistent, operationally robust pipeline.

**Architecture:** Introduce a `Scanner` interface with 4 implementations (WS-Discovery, IP Range Scan, mDNS, Manual IP), a `ScanOrchestrator` that manages scan lifecycle, a `ResultStore` backed by Postgres (new `discovery_scans` + `discovery_results` tables), and a `Scheduler` for periodic per-site scans. API expands from 3 endpoints to 7. Frontend gains 3 views (scan launcher, scan history, paginated results with per-device credential testing).

**Tech Stack:** Go 1.24, Postgres (sqlx), NATS, ONVIF SOAP, React + TypeScript

---

## File Structure

### New files in `services/discovery/`:
- `scanner.go` — Scanner interface + ScanResult/ScanOptions types
- `wsdiscovery.go` — WSDiscoveryScanner (extracted from existing main.go)
- `iprange.go` — IPRangeScanner (TCP port scan + ONVIF probe)
- `mdns.go` — MDNSScanner (mDNS query)
- `manual.go` — ManualIPScanner (explicit IP:port list)
- `orchestrator.go` — ScanOrchestrator (lifecycle, dedup, aggregation)
- `store.go` — ResultStore (Postgres CRUD)
- `handlers.go` — HTTP handlers (7 endpoints)
- `scheduler.go` — periodic per-site scan scheduler
- `scanner_test.go` — Scanner interface tests
- `wsdiscovery_test.go` — WSDiscoveryScanner tests
- `iprange_test.go` — IPRangeScanner tests
- `orchestrator_test.go` — ScanOrchestrator tests
- `store_test.go` — ResultStore tests
- `handlers_test.go` — HTTP handler tests

### Modified files:
- `services/discovery/main.go` — slim down to entry point + config + wiring
- `migrations/XXX_discovery.up.sql` — new migration
- `services/api-gateway/main.go` — route updates (none needed, existing `/api/discovery/` prefix match handles everything)
- `web/src/api/client.ts` — new API methods
- `web/src/pages/DiscoveryPage.tsx` — full rewrite
- `deploy/docker/docker-compose.yml` — add DB_URL to discovery service
- `deploy/helm/evms/values.yaml` — add DB_URL config
- `deploy/helm/evms/templates/discovery.yaml` — inject DB_URL env var

---

### Task 1: Database Migration

**Files:**
- Create: `migrations/010_discovery.up.sql`
- Create: `migrations/010_discovery.down.sql`

- [ ] **Step 1: Write the up migration**

```sql
-- Migration: 010_discovery.up.sql

CREATE TABLE IF NOT EXISTS discovery_scans (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    site_id     UUID REFERENCES sites(id) ON DELETE CASCADE,
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

CREATE TABLE IF NOT EXISTS discovery_results (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    scan_id       UUID REFERENCES discovery_scans(id) ON DELETE CASCADE,
    site_id       UUID REFERENCES sites(id) ON DELETE CASCADE,
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

CREATE INDEX IF NOT EXISTS idx_discovery_results_scan ON discovery_results(scan_id);
CREATE INDEX IF NOT EXISTS idx_discovery_results_site ON discovery_results(site_id);
CREATE INDEX IF NOT EXISTS idx_discovery_scans_site ON discovery_scans(site_id);
CREATE INDEX IF NOT EXISTS idx_discovery_scans_status ON discovery_scans(status);

ALTER TABLE sites ADD COLUMN IF NOT EXISTS discovery_config JSONB DEFAULT '{}';
```

- [ ] **Step 2: Write the down migration**

```sql
-- Migration: 010_discovery.down.sql

DROP TABLE IF EXISTS discovery_results;
DROP TABLE IF EXISTS discovery_scans;
ALTER TABLE sites DROP COLUMN IF EXISTS discovery_config;
```

- [ ] **Step 3: Apply migration and verify**

Run: `go run ./pkg/common/migrate.go -dir migrations -dsn "postgres://..." up`

Verify tables exist:
```bash
psql $DB_URL -c "\dt discovery_*"
psql $DB_URL -c "\d discovery_scans"
psql $DB_URL -c "\d discovery_results"
```

- [ ] **Step 4: Commit**

```bash
git add migrations/010_discovery.up.sql migrations/010_discovery.down.sql
git commit -m "feat: add discovery_scans and discovery_results tables"
```

---

### Task 2: DB Connection + Config in Discovery Service

**Files:**
- Modify: `services/discovery/main.go`

- [ ] **Step 1: Add DB_URL config and connection**

Add fields to `DiscoveryConfig`:
```go
DBURL    string
DB       *sqlx.DB
```

Update `DefaultDiscoveryConfig`:
```go
DBURL: os.Getenv("DB_URL"),
```

Update `NewDiscoveryService` to connect to DB:
```go
if config.DBURL != "" {
    db, err := common.ConnectDBWithCircuitBreaker(ctx, "postgres", config.DBURL, common.NewDBCircuitBreaker("discovery"))
    if err != nil {
        return nil, fmt.Errorf("failed to connect to database: %w", err)
    }
    config.DB = db
    s.healthHandler.AddDBChecker(db.DB, "postgres")
}
```

Add imports: `"github.com/jmoiron/sqlx"`, `"github.com/dam-vms/dam/pkg/common"`

- [ ] **Step 2: Verify it compiles**

Run: `go build ./services/discovery/`

Expected: binary builds successfully

- [ ] **Step 3: Commit**

```bash
git add services/discovery/main.go
git commit -m "feat: add DB_URL connection to discovery service"
```

---

### Task 3: Scanner Interface + Types

**Files:**
- Create: `services/discovery/scanner.go`

- [ ] **Step 1: Write the Scanner interface and types**

```go
package main

import "context"

type CapabilitySet map[string]bool

type ScanResult struct {
    IP           string         `json:"ip_address"`
    Port         int            `json:"port"`
    XAddr        string         `json:"xaddr"`
    Manufacturer string         `json:"manufacturer"`
    Model        string         `json:"model"`
    Firmware     string         `json:"firmware_version"`
    SerialNumber string         `json:"serial_number"`
    Hostname     string         `json:"hostname"`
    Capabilities CapabilitySet  `json:"capabilities"`
}

type ScanOptions struct {
    Timeout     time.Duration
    Credentials *onvif.Credentials // optional ONVIF auth for probing
}

type Scanner interface {
    Name() string
    Scan(ctx context.Context, subnet string, ports []int, opts ScanOptions) (<-chan ScanResult, error)
}
```

Add import: `"github.com/dam-vms/dam/pkg/onvif"`

- [ ] **Step 2: Verify it compiles**

Run: `go build ./services/discovery/`

- [ ] **Step 3: Commit**

```bash
git add services/discovery/scanner.go
git commit -m "feat: add Scanner interface and ScanResult type"
```

---

### Task 4: WSDiscoveryScanner (extract from existing code)

**Files:**
- Create: `services/discovery/wsdiscovery.go`
- Modify: `services/discovery/main.go` (remove old sendProbe and probe XML types)

- [ ] **Step 1: Write WSDiscoveryScanner**

```go
package main

import (
    "bytes"
    "context"
    "encoding/xml"
    "fmt"
    "net"
    "time"

    "github.com/dam-vms/dam/pkg/onvif"
    "github.com/google/uuid"
)

type WSDiscoveryScanner struct {
    logger *slog.Logger
}

func NewWSDiscoveryScanner(logger *slog.Logger) *WSDiscoveryScanner {
    return &WSDiscoveryScanner{logger: logger}
}

func (s *WSDiscoveryScanner) Name() string { return "ws-discovery" }

func (s *WSDiscoveryScanner) Scan(ctx context.Context, subnet string, ports []int, opts ScanOptions) (<-chan ScanResult, error) {
    ch := make(chan ScanResult)
    go func() {
        defer close(ch)
        // ... send probe via UDP multicast, parse ProbeMatch responses,
        // query each device via ONVIF, send results to ch
        // Use ctx for cancellation and timeout
    }()
    return ch, nil
}
```

Full implementation: Copy the `sendProbe` and `probeXML` logic from `main.go`, adapt to streaming results to channel:

```go
func (s *WSDiscoveryScanner) Scan(ctx context.Context, subnet string, ports []int, opts ScanOptions) (<-chan ScanResult, error) {
    ch := make(chan ScanResult)

    go func() {
        defer close(ch)

        probeMsg, err := buildProbeXML()
        if err != nil {
            select {
            case <-ctx.Done():
            case ch <- ScanResult{Error: fmt.Errorf("failed to build probe: %w", err)}:
            }
            return
        }

        conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
        if err != nil {
            select {
            case <-ctx.Done():
            case ch <- ScanResult{Error: fmt.Errorf("failed to create UDP socket: %w", err)}:
            }
            return
        }
        defer conn.Close()

        multicastAddr := &net.UDPAddr{IP: net.ParseIP("239.255.255.250"), Port: 3702}
        if _, err := conn.WriteTo([]byte(probeMsg), multicastAddr); err != nil {
            select {
            case <-ctx.Done():
            case ch <- ScanResult{Error: fmt.Errorf("failed to send probe: %w", err)}:
            }
            return
        }

        deadline, hasDeadline := ctx.Deadline()
        if !hasDeadline {
            deadline = time.Now().Add(5 * time.Second)
        }
        if err := conn.SetReadDeadline(deadline); err != nil {
            return
        }

        seen := make(map[string]bool)
        buf := make([]byte, 65535)

        for {
            select {
            case <-ctx.Done():
                return
            default:
            }

            n, _, err := conn.ReadFromUDP(buf)
            if err != nil {
                if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
                    return
                }
                return
            }

            var env probeMatchEnvelope
            if err := xml.Unmarshal(buf[:n], &env); err != nil {
                continue
            }

            for _, match := range env.Body.ProbeMatches.ProbeMatch {
                if match.XAddrs == "" || seen[match.XAddrs] {
                    continue
                }
                seen[match.XAddrs] = true

                addrList := bytes.Fields([]byte(match.XAddrs))
                if len(addrList) == 0 {
                    continue
                }
                deviceURL := string(addrList[0])

                result := ScanResult{
                    XAddr:  match.XAddrs,
                    IP:     deviceURL,
                    Capabilities: make(CapabilitySet),
                }

                client := onvif.NewSOAPClient(5*time.Second, nil)

                if info, err := onvif.GetDeviceInformation(ctx, client, deviceURL); err == nil {
                    result.Manufacturer = info.Manufacturer
                    result.Model = info.Model
                    result.Firmware = info.FirmwareVersion
                    result.SerialNumber = info.SerialNumber
                }

                if caps, err := onvif.GetCapabilities(ctx, client, deviceURL); err == nil {
                    if caps.Analytics { result.Capabilities["analytics"] = true }
                    if caps.Events    { result.Capabilities["events"] = true }
                    if caps.Imaging   { result.Capabilities["imaging"] = true }
                    if caps.Media     { result.Capabilities["media"] = true }
                    if caps.PTZ       { result.Capabilities["ptz"] = true }
                    if caps.Recording { result.Capabilities["recording"] = true }
                }

                if hostname, err := onvif.GetHostname(ctx, client, deviceURL); err == nil {
                    result.Hostname = hostname
                }

                select {
                case <-ctx.Done():
                    return
                case ch <- result:
                }
            }
        }
    }()

    return ch, nil
}

func buildProbeXML() (string, error) {
    // same as existing probeXML() in main.go
    uid := uuid.New().String()
    env := probeEnvelope{
        Header: probeHeader{
            Action:    "http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe",
            MessageID: "uuid:" + uid,
            To:        "urn:schemas-xmlsoap-org:ws:2005:04:discovery",
        },
        Body: probeBody{
            Probe: probe{Types: "dn:NetworkVideoTransmitter"},
        },
    }
    out, err := xml.MarshalIndent(env, "", "  ")
    if err != nil {
        return "", err
    }
    return xml.Header + string(out), nil
}
```

- [ ] **Step 2: Remove old sendProbe and probeXML from main.go**

Delete from `main.go`:
- `probeEnvelope`, `probeHeader`, `probeBody`, `probe`, `probeMatchEnvelope`, `probeMatchBody`, `probeMatches`, `probeMatchItem` types
- `probeXML()` function
- `sendProbe()` method

- [ ] **Step 3: Keep discoveredCamera type in main.go for backwards compat** (will be replaced later)

- [ ] **Step 4: Verify it compiles**

Run: `go build ./services/discovery/`

- [ ] **Step 5: Commit**

```bash
git add services/discovery/wsdiscovery.go services/discovery/main.go
git commit -m "feat: extract WS-Discovery into WSDiscoveryScanner"
```

---

### Task 5: IPRangeScanner

**Files:**
- Create: `services/discovery/iprange.go`

- [ ] **Step 1: Write IPRangeScanner**

```go
package main

import (
    "context"
    "fmt"
    "net"
    "sync"
    "time"

    "github.com/dam-vms/dam/pkg/onvif"
)

type IPRangeScanner struct {
    logger    *slog.Logger
    dialTimeout time.Duration
    probeTimeout time.Duration
    concurrency  int
}

func NewIPRangeScanner(logger *slog.Logger) *IPRangeScanner {
    return &IPRangeScanner{
        logger:       logger,
        dialTimeout:  2 * time.Second,
        probeTimeout: 3 * time.Second,
        concurrency:  50,
    }
}

func (s *IPRangeScanner) Name() string { return "ip-range" }

func (s *IPRangeScanner) Scan(ctx context.Context, subnet string, ports []int, opts ScanOptions) (<-chan ScanResult, error) {
    ch := make(chan ScanResult)

    go func() {
        defer close(ch)

        ipNet, err := parseCIDR(subnet)
        if err != nil {
            select {
            case <-ctx.Done():
            case ch <- ScanResult{Error: fmt.Errorf("invalid subnet %s: %w", subnet, err)}:
            }
            return
        }

        ips := collectIPs(ipNet)
        sem := make(chan struct{}, s.concurrency)
        var wg sync.WaitGroup

        for _, ip := range ips {
            select {
            case <-ctx.Done():
                wg.Wait()
                return
            case sem <- struct{}{}:
            }

            wg.Add(1)
            go func(ip string) {
                defer wg.Done()
                defer func() { <-sem }()

                for _, port := range ports {
                    select {
                    case <-ctx.Done():
                        return
                    default:
                    }

                    addr := net.JoinHostPort(ip, fmt.Sprint(port))
                    conn, err := net.DialTimeout("tcp", addr, s.dialTimeout)
                    if err != nil {
                        continue
                    }
                    conn.Close()

                    result := probeONVIFDevice(ctx, addr, s.probeTimeout)
                    if result != nil {
                        select {
                        case <-ctx.Done():
                            return
                        case ch <- *result:
                        }
                    }
                    break // found open port, don't scan more ports for this IP
                }
            }(ip)
        }

        wg.Wait()
    }()

    return ch, nil
}

func parseCIDR(cidr string) (*net.IPNet, error) {
    _, ipNet, err := net.ParseCIDR(cidr)
    return ipNet, err
}

func collectIPs(ipNet *net.IPNet) []string {
    var ips []string
    ip := ipNet.IP.Mask(ipNet.Mask)
    for ip := ip.Mask(ipNet.Mask); ipNet.Contains(ip); incIP(ip) {
        ips = append(ips, ip.String())
    }
    // Remove network and broadcast addresses for IPv4
    if ip4 := ipNet.IP.To4(); ip4 != nil && len(ips) > 2 {
        ips = ips[1 : len(ips)-1]
    }
    return ips
}

func incIP(ip net.IP) {
    for j := len(ip) - 1; j >= 0; j-- {
        ip[j]++
        if ip[j] > 0 {
            break
        }
    }
}

func probeONVIFDevice(ctx context.Context, addr string, timeout time.Duration) *ScanResult {
    deviceURL := "http://" + addr + "/onvif/device_service"
    probeCtx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()

    client := onvif.NewSOAPClient(timeout, nil)

    result := &ScanResult{
        IP:           addr,
        XAddr:        deviceURL,
        Capabilities: make(CapabilitySet),
    }

    if info, err := onvif.GetDeviceInformation(probeCtx, client, deviceURL); err == nil {
        result.Manufacturer = info.Manufacturer
        result.Model = info.Model
        result.Firmware = info.FirmwareVersion
        result.SerialNumber = info.SerialNumber
    } else {
        return nil // not an ONVIF device or unreachable
    }

    if caps, err := onvif.GetCapabilities(probeCtx, client, deviceURL); err == nil {
        if caps.Analytics { result.Capabilities["analytics"] = true }
        if caps.Events    { result.Capabilities["events"] = true }
        if caps.Imaging   { result.Capabilities["imaging"] = true }
        if caps.Media     { result.Capabilities["media"] = true }
        if caps.PTZ       { result.Capabilities["ptz"] = true }
        if caps.Recording { result.Capabilities["recording"] = true }
    }

    if hostname, err := onvif.GetHostname(probeCtx, client, deviceURL); err == nil {
        result.Hostname = hostname
    }

    return result
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./services/discovery/`

- [ ] **Step 3: Commit**

```bash
git add services/discovery/iprange.go
git commit -m "feat: add IPRangeScanner for TCP port scan discovery"
```

---

### Task 6: MDNSScanner

**Files:**
- Create: `services/discovery/mdns.go`

- [ ] **Step 1: Write MDNSScanner**

Note: Since there's no mDNS library in the go.mod, we implement a simple mDNS query using `net.ListenUDP` on port 5353. This is a best-effort scanner.

```go
package main

import (
    "context"
    "fmt"
    "net"
    "strings"
    "time"

    "github.com/dam-vms/dam/pkg/onvif"
)

type MDNSScanner struct {
    logger *slog.Logger
}

func NewMDNSScanner(logger *slog.Logger) *MDNSScanner {
    return &MDNSScanner{logger: logger}
}

func (s *MDNSScanner) Name() string { return "mdns" }

func (s *MDNSScanner) Scan(ctx context.Context, subnet string, ports []int, opts ScanOptions) (<-chan ScanResult, error) {
    ch := make(chan ScanResult)

    go func() {
        defer close(ch)
        // mDNS query for _onvif._tcp.local and _rtsp._tcp.local
        // Listen on 5353, send standard mDNS query
        // Parse responses for SRV records, resolve A records
        // Probe each discovered device via ONVIF
        // Since mDNS is limited to local link, subnet param is ignored
    }()

    return ch, nil
}
```

Full implementation:

```go
const mdnsAddr = "224.0.0.251:5353"

func (s *MDNSScanner) Scan(ctx context.Context, subnet string, ports []int, opts ScanOptions) (<-chan ScanResult, error) {
    ch := make(chan ScanResult)

    go func() {
        defer close(ch)

        conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
        if err != nil {
            select {
            case <-ctx.Done():
            case ch <- ScanResult{Error: fmt.Errorf("mDNS listen failed: %w", err)}:
            }
            return
        }
        defer conn.Close()

        // Send mDNS query for _onvif._tcp.local
        query := buildMDNSQuery("_onvif._tcp.local")
        dst := &net.UDPAddr{IP: net.ParseIP("224.0.0.251"), Port: 5353}
        if _, err := conn.WriteTo(query, dst); err != nil {
            return
        }

        // Send query for _rtsp._tcp.local as well
        rtspQuery := buildMDNSQuery("_rtsp._tcp.local")
        conn.WriteTo(rtspQuery, dst)

        deadline, hasDeadline := ctx.Deadline()
        if !hasDeadline {
            deadline = time.Now().Add(3 * time.Second)
        }
        conn.SetReadDeadline(deadline)

        buf := make([]byte, 65535)
        seen := make(map[string]bool)

        for {
            select {
            case <-ctx.Done():
                return
            default:
            }

            n, _, err := conn.ReadFromUDP(buf)
            if err != nil {
                return
            }

            hostnames := parseMDNSResponse(buf[:n])
            for _, host := range hostnames {
                if seen[host] {
                    continue
                }
                seen[host] = true

                deviceURL := fmt.Sprintf("http://%s/onvif/device_service", host)
                client := onvif.NewSOAPClient(3*time.Second, nil)
                result := ScanResult{
                    IP:           host,
                    XAddr:        deviceURL,
                    Capabilities: make(CapabilitySet),
                }

                if info, err := onvif.GetDeviceInformation(ctx, client, deviceURL); err == nil {
                    result.Manufacturer = info.Manufacturer
                    result.Model = info.Model
                    result.Firmware = info.FirmwareVersion
                    result.SerialNumber = info.SerialNumber
                } else {
                    continue
                }

                if caps, err := onvif.GetCapabilities(ctx, client, deviceURL); err == nil {
                    if caps.Analytics { result.Capabilities["analytics"] = true }
                    if caps.Events    { result.Capabilities["events"] = true }
                    if caps.Imaging   { result.Capabilities["imaging"] = true }
                    if caps.Media     { result.Capabilities["media"] = true }
                    if caps.PTZ       { result.Capabilities["ptz"] = true }
                    if caps.Recording { result.Capabilities["recording"] = true }
                }

                if hostname, err := onvif.GetHostname(ctx, client, deviceURL); err == nil {
                    result.Hostname = hostname
                }

                select {
                case <-ctx.Done():
                    return
                case ch <- result:
                }
            }
        }
    }()

    return ch, nil
}

// buildMDNSQuery builds a simple mDNS query packet for the given service name
func buildMDNSQuery(service string) []byte {
    // Build a minimal DNS query:
    // Header: ID=0, flags=0x0100 (standard query), QDCOUNT=1
    // Question: service name encoded as labels, QTYPE=PTR(12), QCLASS=IN(1)
    var buf []byte
    // Header
    buf = append(buf, 0x00, 0x00) // ID
    buf = append(buf, 0x00, 0x00) // flags (standard query)
    buf = append(buf, 0x00, 0x01) // QDCOUNT = 1
    buf = append(buf, 0x00, 0x00) // ANCOUNT
    buf = append(buf, 0x00, 0x00) // NSCOUNT
    buf = append(buf, 0x00, 0x00) // ARCOUNT
    // Question
    for _, label := range strings.Split(service, ".") {
        buf = append(buf, byte(len(label)))
        buf = append(buf, []byte(label)...)
    }
    buf = append(buf, 0x00)             // terminating zero length label
    buf = append(buf, 0x00, 0x0C)       // QTYPE = PTR (12)
    buf = append(buf, 0x00, 0x01)       // QCLASS = IN
    return buf
}

func parseMDNSResponse(data []byte) []string {
    var hosts []string
    // Skip DNS header (12 bytes)
    if len(data) < 12 {
        return nil
    }
    // Skip question section (variable length)
    pos := 12
    qdcount := int(data[4])<<8 | int(data[5])
    for i := 0; i < qdcount && pos < len(data); i++ {
        for pos < len(data) {
            if data[pos] == 0 {
                pos++
                break
            }
            if data[pos]&0xC0 == 0xC0 {
                pos += 2
                break
            }
            pos += int(data[pos]) + 1
        }
        pos += 4 // skip QTYPE + QCLASS
    }
    // Parse answer section
    ancount := int(data[6])<<8 | int(data[7])
    for i := 0; i < ancount && pos < len(data); i++ {
        // Name (skip compressed)
        if pos < len(data) && data[pos]&0xC0 == 0xC0 {
            pos += 2
        } else {
            for pos < len(data) {
                if data[pos] == 0 {
                    pos++
                    break
                }
                if data[pos]&0xC0 == 0xC0 {
                    pos += 2
                    break
                }
                pos += int(data[pos]) + 1
            }
        }
        if pos+10 > len(data) {
            break
        }
        rrtype := int(data[pos])<<8 | int(data[pos+1])
        pos += 10 // TYPE(2) + CLASS(2) + TTL(4) + RDLENGTH(2)
        rdlength := int(data[pos-2])<<8 | int(data[pos-1])
        if rrtype == 12 { // PTR record
            // Parse target hostname
            ptrEnd := pos + rdlength
            if ptrEnd > len(data) {
                break
            }
            var nameParts []string
            p := pos
            for p < ptrEnd {
                if data[p] == 0 {
                    break
                }
                if data[p]&0xC0 == 0xC0 {
                    // compressed name - skip
                    break
                }
                length := int(data[p])
                p++
                if p+length > ptrEnd {
                    break
                }
                nameParts = append(nameParts, string(data[p:p+length]))
                p += length
            }
            if len(nameParts) > 0 {
                hosts = append(hosts, strings.Join(nameParts, "."))
            }
        }
        pos += rdlength
    }
    return hosts
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./services/discovery/`

- [ ] **Step 3: Commit**

```bash
git add services/discovery/mdns.go
git commit -m "feat: add MDNSScanner for mDNS-based discovery"
```

---

### Task 7: ManualIPScanner

**Files:**
- Create: `services/discovery/manual.go`

- [ ] **Step 1: Write ManualIPScanner**

```go
package main

import (
    "context"
    "fmt"
    "net"
    "time"

    "github.com/dam-vms/dam/pkg/onvif"
)

type ManualIPScanner struct {
    logger *slog.Logger
}

func NewManualIPScanner(logger *slog.Logger) *ManualIPScanner {
    return &ManualIPScanner{logger: logger}
}

func (s *ManualIPScanner) Name() string { return "manual" }

func (s *ManualIPScanner) Scan(ctx context.Context, subnet string, ports []int, opts ScanOptions) (<-chan ScanResult, error) {
    ch := make(chan ScanResult)

    go func() {
        defer close(ch)
        // subnet param contains comma-separated IP:port list
        // e.g. "10.0.0.1:80,10.0.0.2:554"
        entries := parseManualEntries(subnet)
        if len(entries) == 0 {
            select {
            case <-ctx.Done():
            case ch <- ScanResult{Error: fmt.Errorf("no valid manual entries in: %s", subnet)}:
            }
            return
        }
        for _, entry := range entries {
            select {
            case <-ctx.Done():
                return
            default:
            }

            result := probeONVIFDevice(ctx, entry, 5*time.Second)
            if result != nil {
                select {
                case <-ctx.Done():
                    return
                case ch <- *result:
                }
            }
        }
    }()

    return ch, nil
}

func parseManualEntries(input string) []string {
    var entries []string
    for _, part := range strings.Split(input, ",") {
        part = strings.TrimSpace(part)
        if part == "" {
            continue
        }
        host, port, err := net.SplitHostPort(part)
        if err != nil {
            // assume port 80 if not specified
            entries = append(entries, net.JoinHostPort(part, "80"))
        } else {
            entries = append(entries, net.JoinHostPort(host, port))
        }
    }
    return entries
}
```

Add import: `"strings"`

- [ ] **Step 2: Verify it compiles**

Run: `go build ./services/discovery/`

- [ ] **Step 3: Commit**

```bash
git add services/discovery/manual.go
git commit -m "feat: add ManualIPScanner for explicit IP:port discovery"
```

---

### Task 8: ResultStore (Postgres persistence)

**Files:**
- Create: `services/discovery/store.go`

- [ ] **Step 1: Write ResultStore**

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/dam-vms/dam/pkg/common"
    "github.com/google/uuid"
    "github.com/jmoiron/sqlx"
    "github.com/lib/pq"
)

type ScanRecord struct {
    ID          uuid.UUID  `db:"id" json:"id"`
    SiteID      uuid.UUID  `db:"site_id" json:"site_id"`
    Status      string     `db:"status" json:"status"`
    Methods     []string   `db:"methods" json:"methods"`
    Subnets     []string   `db:"subnets" json:"subnets"`
    Ports       []int      `db:"ports" json:"ports"`
    TotalFound  int        `db:"total_found" json:"total_found"`
    Error       *string    `db:"error" json:"error,omitempty"`
    StartedAt   *time.Time `db:"started_at" json:"started_at"`
    CompletedAt *time.Time `db:"completed_at" json:"completed_at"`
    CreatedBy   *uuid.UUID `db:"created_by" json:"created_by"`
    CreatedAt   time.Time  `db:"created_at" json:"created_at"`
}

type ResultRecord struct {
    ID           uuid.UUID              `db:"id" json:"id"`
    ScanID       uuid.UUID              `db:"scan_id" json:"scan_id"`
    SiteID       uuid.UUID              `db:"site_id" json:"site_id"`
    IPAddress    string                 `db:"ip_address" json:"ip_address"`
    Port         *int                   `db:"port" json:"port"`
    XAddr        *string                `db:"xaddr" json:"xaddr"`
    Manufacturer *string                `db:"manufacturer" json:"manufacturer"`
    Model        *string                `db:"model" json:"model"`
    Firmware     *string                `db:"firmware" json:"firmware"`
    SerialNumber *string                `db:"serial_number" json:"serial_number"`
    Hostname     *string                `db:"hostname" json:"hostname"`
    Capabilities map[string]interface{} `db:"capabilities" json:"capabilities"`
    OnvifData    map[string]interface{} `db:"onvif_data" json:"onvif_data,omitempty"`
    IsNew        bool                   `db:"is_new" json:"is_new"`
    AlreadyInDB  bool                   `db:"already_in_db" json:"already_in_db"`
    Imported     bool                   `db:"imported" json:"imported"`
    ImportedAt   *time.Time             `db:"imported_at" json:"imported_at"`
    CreatedAt    time.Time              `db:"created_at" json:"created_at"`
}

type ResultStore struct {
    db     *common.CircuitBreakerDB
    rawDB  *sqlx.DB
    logger *slog.Logger
}

func NewResultStore(db *sqlx.DB, logger *slog.Logger) *ResultStore {
    cb := common.NewDBCircuitBreaker("discovery-store")
    return &ResultStore{
        db:     common.WrapDB(db, cb),
        rawDB:  db,
        logger: logger,
    }
}

func (s *ResultStore) CreateScan(ctx context.Context, scan *ScanRecord) error {
    query := `INSERT INTO discovery_scans (id, site_id, status, methods, subnets, ports, created_by)
              VALUES ($1, $2, $3, $4, $5, $6, $7)`
    _, err := s.db.ExecContext(ctx, query,
        scan.ID, scan.SiteID, scan.Status,
        pq.Array(scan.Methods), pq.Array(scan.Subnets), pq.Array(scan.Ports),
        scan.CreatedBy)
    return err
}

func (s *ResultStore) UpdateScanStatus(ctx context.Context, id uuid.UUID, status string, totalFound int, errMsg *string) error {
    now := time.Now()
    query := `UPDATE discovery_scans SET status=$1, total_found=$2, error=$3, completed_at=$4 WHERE id=$5`
    _, err := s.db.ExecContext(ctx, query, status, totalFound, errMsg, now, id)
    return err
}

func (s *ResultStore) GetScans(ctx context.Context, siteID *uuid.UUID, page, perPage int) ([]ScanRecord, int, error) {
    where := ""
    args := []interface{}{}
    if siteID != nil {
        where = "WHERE site_id = $1"
        args = append(args, *siteID)
    }
    var total int
    countQuery := fmt.Sprintf("SELECT COUNT(*) FROM discovery_scans %s", where)
    if err := s.db.GetContext(ctx, &total, countQuery, args...); err != nil {
        return nil, 0, err
    }
    offset := (page - 1) * perPage
    query := fmt.Sprintf("SELECT * FROM discovery_scans %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
        where, len(args)+1, len(args)+2)
    args = append(args, perPage, offset)
    var scans []ScanRecord
    if err := s.db.SelectContext(ctx, &scans, query, args...); err != nil {
        return nil, 0, err
    }
    return scans, total, nil
}

func (s *ResultStore) GetScan(ctx context.Context, id uuid.UUID) (*ScanRecord, error) {
    var scan ScanRecord
    err := s.db.GetContext(ctx, &scan, "SELECT * FROM discovery_scans WHERE id=$1", id)
    if err != nil {
        return nil, err
    }
    return &scan, nil
}

func (s *ResultStore) InsertResult(ctx context.Context, result *ResultRecord) error {
    query := `INSERT INTO discovery_results
        (id, scan_id, site_id, ip_address, port, xaddr, manufacturer, model, firmware,
         serial_number, hostname, capabilities, onvif_data, is_new, already_in_db)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`
    capsJSON := mapToJSON(result.Capabilities)
    onvifJSON := mapToJSON(result.OnvifData)
    _, err := s.db.ExecContext(ctx, query,
        result.ID, result.ScanID, result.SiteID, result.IPAddress, result.Port,
        result.XAddr, result.Manufacturer, result.Model, result.Firmware,
        result.SerialNumber, result.Hostname, capsJSON, onvifJSON,
        result.IsNew, result.AlreadyInDB)
    return err
}

func (s *ResultStore) GetResults(ctx context.Context, scanID uuid.UUID, page, perPage int, queryFilter string) ([]ResultRecord, int, error) {
    where := "WHERE scan_id = $1"
    args := []interface{}{scanID}
    argIdx := 2
    if queryFilter != "" {
        where += fmt.Sprintf(" AND (ip_address ILIKE $%d OR manufacturer ILIKE $%d OR model ILIKE $%d OR hostname ILIKE $%d)",
            argIdx, argIdx, argIdx, argIdx)
        args = append(args, "%"+queryFilter+"%")
        argIdx++
    }
    var total int
    countQuery := fmt.Sprintf("SELECT COUNT(*) FROM discovery_results %s", where)
    if err := s.db.GetContext(ctx, &total, countQuery, args...); err != nil {
        return nil, 0, err
    }
    offset := (page - 1) * perPage
    query := fmt.Sprintf("SELECT * FROM discovery_results %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
        where, argIdx, argIdx+1)
    args = append(args, perPage, offset)
    var results []ResultRecord
    if err := s.db.SelectContext(ctx, &results, query, args...); err != nil {
        return nil, 0, err
    }
    return results, total, nil
}

func (s *ResultStore) MarkImported(ctx context.Context, resultIDs []uuid.UUID) error {
    query := `UPDATE discovery_results SET imported=true, imported_at=NOW() WHERE id = ANY($1)`
    _, err := s.db.ExecContext(ctx, query, pq.Array(resultIDs))
    return err
}

func (s *ResultStore) CheckAlreadyInDB(ctx context.Context, xaddr string) (bool, error) {
    var count int
    err := s.db.GetContext(ctx, &count,
        `SELECT COUNT(*) FROM cameras WHERE connection_url = $1`, xaddr)
    return count > 0, err
}

func (s *ResultStore) Close() error {
    return s.rawDB.Close()
}
```

Helper function (add to same file or a utils file):
```go
func mapToJSON(m map[string]interface{}) []byte {
    if m == nil {
        return nil
    }
    b, _ := json.Marshal(m)
    return b
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./services/discovery/`

- [ ] **Step 3: Commit**

```bash
git add services/discovery/store.go
git commit -m "feat: add ResultStore for Postgres-backed discovery persistence"
```

---

### Task 9: ScanOrchestrator

**Files:**
- Create: `services/discovery/orchestrator.go`

- [ ] **Step 1: Write ScanOrchestrator**

```go
package main

import (
    "context"
    "fmt"
    "sync"
    "time"

    "github.com/dam-vms/dam/pkg/onvif"
    "github.com/google/uuid"
)

type ScanRequest struct {
    SiteID    uuid.UUID
    Methods   []string
    Subnets   []string
    Ports     []int
    CreatedBy *uuid.UUID
}

type ScanOrchestrator struct {
    store      *ResultStore
    scanners   map[string]Scanner
    logger     *slog.Logger
    activeMu   sync.Mutex
    activeScans map[uuid.UUID]context.CancelFunc
}

func NewScanOrchestrator(store *ResultStore, scanners map[string]Scanner, logger *slog.Logger) *ScanOrchestrator {
    return &ScanOrchestrator{
        store:       store,
        scanners:    scanners,
        logger:      logger,
        activeScans: make(map[uuid.UUID]context.CancelFunc),
    }
}

func (o *ScanOrchestrator) StartScan(ctx context.Context, req ScanRequest) (*ScanRecord, error) {
    scan := &ScanRecord{
        ID:        uuid.New(),
        SiteID:    req.SiteID,
        Status:    "pending",
        Methods:   req.Methods,
        Subnets:   req.Subnets,
        Ports:     req.Ports,
        CreatedBy: req.CreatedBy,
    }

    if err := o.store.CreateScan(ctx, scan); err != nil {
        return nil, fmt.Errorf("failed to create scan record: %w", err)
    }

    scanCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
    o.activeMu.Lock()
    o.activeScans[scan.ID] = cancel
    o.activeMu.Unlock()

    scan.Status = "running"
    now := time.Now()
    scan.StartedAt = &now
    if err := o.store.UpdateScanStatus(ctx, scan.ID, "running", 0, nil); err != nil {
        cancel()
        return nil, err
    }

    go o.executeScan(scanCtx, scan, req)

    return scan, nil
}

func (o *ScanOrchestrator) CancelScan(ctx context.Context, scanID uuid.UUID) error {
    o.activeMu.Lock()
    defer o.activeMu.Unlock()

    cancel, ok := o.activeScans[scanID]
    if !ok {
        return fmt.Errorf("scan %s is not active", scanID)
    }
    cancel()
    return o.store.UpdateScanStatus(ctx, scanID, "cancelled", 0, nil)
}

func (o *ScanOrchestrator) executeScan(ctx context.Context, scan *ScanRecord, req ScanRequest) {
    defer func() {
        o.activeMu.Lock()
        delete(o.activeScans, scan.ID)
        o.activeMu.Unlock()
    }()

    var found int
    resultCh := make(chan ScanResult, 100)
    var scannerWg sync.WaitGroup

    for _, method := range req.Methods {
        scanner, ok := o.scanners[method]
        if !ok {
            o.logger.Warn("unknown scanner method", "method", method)
            continue
        }
        for _, subnet := range req.Subnets {
            scannerWg.Add(1)
            go func(sc Scanner, subnet string) {
                defer scannerWg.Done()
                ch, err := sc.Scan(ctx, subnet, req.Ports, ScanOptions{
                    Timeout: 5 * time.Second,
                })
                if err != nil {
                    o.logger.Error("scanner failed", "method", sc.Name(), "subnet", subnet, "error", err)
                    return
                }
                for res := range ch {
                    select {
                    case <-ctx.Done():
                        return
                    case resultCh <- res:
                    }
                }
            }(scanner, subnet)
        }
    }

    go func() {
        scannerWg.Wait()
        close(resultCh)
    }()

    seen := make(map[string]bool)
    for res := range resultCh {
        select {
        case <-ctx.Done():
            return
        default:
        }

        if res.Error != nil {
            o.logger.Error("scan result error", "error", res.Error)
            continue
        }

        if seen[res.XAddr] {
            continue
        }
        seen[res.XAddr] = true

        alreadyInDB, _ := o.store.CheckAlreadyInDB(ctx, res.XAddr)

        record := &ResultRecord{
            ID:        uuid.New(),
            ScanID:    scan.ID,
            SiteID:    scan.SiteID,
            IPAddress: res.IP,
            XAddr:     &res.XAddr,
            Manufacturer: &res.Manufacturer,
            Model:        &res.Model,
            Firmware:     &res.Firmware,
            SerialNumber: &res.SerialNumber,
            Hostname:     &res.Hostname,
            IsNew:        true,
            AlreadyInDB:  alreadyInDB,
        }
        if res.Port > 0 {
            record.Port = &res.Port
        }
        if res.Capabilities != nil {
            caps := make(map[string]interface{})
            for k, v := range res.Capabilities {
                caps[k] = v
            }
            record.Capabilities = caps
        }

        if err := o.store.InsertResult(ctx, record); err != nil {
            o.logger.Error("failed to store result", "error", err)
            continue
        }
        found++
    }

    errStr := ""
    if ctx.Err() != nil && ctx.Err() != context.Canceled {
        errStr = ctx.Err().Error()
    }
    status := "completed"
    if errStr != "" {
        status = "failed"
    }
    o.store.UpdateScanStatus(ctx, scan.ID, status, found, &errStr)
    o.logger.Info("scan completed", "scan_id", scan.ID, "found", found, "status", status)
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./services/discovery/`

- [ ] **Step 3: Commit**

```bash
git add services/discovery/orchestrator.go
git commit -m "feat: add ScanOrchestrator for scan lifecycle management"
```

---

### Task 10: HTTP Handlers (7 endpoints)

**Files:**
- Create: `services/discovery/handlers.go`
- Modify: `services/discovery/main.go` (replace old handlers, wire up new ones)

- [ ] **Step 1: Write the 7 handlers**

```go
package main

import (
    "encoding/json"
    "net/http"
    "strconv"

    "github.com/google/uuid"
    "github.com/dam-vms/dam/pkg/common"
)

type ScanHandler struct {
    orchestrator *ScanOrchestrator
    store        *ResultStore
    logger       *slog.Logger
}

func NewScanHandler(orchestrator *ScanOrchestrator, store *ResultStore, logger *slog.Logger) *ScanHandler {
    return &ScanHandler{
        orchestrator: orchestrator,
        store:        store,
        logger:       logger,
    }
}

// POST /discovery/scan
func (h *ScanHandler) handleCreateScan(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
        return
    }
    var req struct {
        SiteID  string   `json:"site_id"`
        Methods []string `json:"methods"`
        Subnets []string `json:"subnets"`
        Ports   []int    `json:"ports"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        jsonError(w, "invalid request body", http.StatusBadRequest)
        return
    }
    siteID, err := uuid.Parse(req.SiteID)
    if err != nil {
        jsonError(w, "invalid site_id", http.StatusBadRequest)
        return
    }
    if len(req.Methods) == 0 {
        req.Methods = []string{"ws-discovery"}
    }
    if len(req.Ports) == 0 {
        req.Ports = []int{80, 554, 8080}
    }
    userID, _ := common.GetUserIDFromContext(r.Context())

    scan, err := h.orchestrator.StartScan(r.Context(), ScanRequest{
        SiteID:    siteID,
        Methods:   req.Methods,
        Subnets:   req.Subnets,
        Ports:     req.Ports,
        CreatedBy: userID,
    })
    if err != nil {
        jsonError(w, err.Error(), http.StatusInternalServerError)
        return
    }
    jsonResponse(w, http.StatusOK, scan)
}

// GET /discovery/scans
func (h *ScanHandler) handleListScans(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
        return
    }
    page, _ := strconv.Atoi(r.URL.Query().Get("page"))
    if page < 1 { page = 1 }
    perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
    if perPage < 1 || perPage > 100 { perPage = 20 }

    var siteID *uuid.UUID
    if sid := r.URL.Query().Get("site_id"); sid != "" {
        parsed, err := uuid.Parse(sid)
        if err == nil { siteID = &parsed }
    }

    scans, total, err := h.store.GetScans(r.Context(), siteID, page, perPage)
    if err != nil {
        jsonError(w, err.Error(), http.StatusInternalServerError)
        return
    }
    jsonResponse(w, http.StatusOK, map[string]interface{}{
        "scans": scans,
        "total": total,
        "page":  page,
        "per_page": perPage,
    })
}

// GET /discovery/scans/{id}
func (h *ScanHandler) handleGetScan(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
        return
    }
    scanID, err := uuid.Parse(r.PathValue("id"))
    if err != nil {
        jsonError(w, "invalid scan id", http.StatusBadRequest)
        return
    }
    scan, err := h.store.GetScan(r.Context(), scanID)
    if err != nil {
        jsonError(w, "scan not found", http.StatusNotFound)
        return
    }
    jsonResponse(w, http.StatusOK, scan)
}

// POST /discovery/scans/{id}/cancel
func (h *ScanHandler) handleCancelScan(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
        return
    }
    scanID, err := uuid.Parse(r.PathValue("id"))
    if err != nil {
        jsonError(w, "invalid scan id", http.StatusBadRequest)
        return
    }
    if err := h.orchestrator.CancelScan(r.Context(), scanID); err != nil {
        jsonError(w, err.Error(), http.StatusBadRequest)
        return
    }
    jsonResponse(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

// GET /discovery/scans/{id}/results
func (h *ScanHandler) handleGetResults(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
        return
    }
    scanID, err := uuid.Parse(r.PathValue("id"))
    if err != nil {
        jsonError(w, "invalid scan id", http.StatusBadRequest)
        return
    }
    page, _ := strconv.Atoi(r.URL.Query().Get("page"))
    if page < 1 { page = 1 }
    perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
    if perPage < 1 || perPage > 100 { perPage = 20 }
    query := r.URL.Query().Get("query")

    results, total, err := h.store.GetResults(r.Context(), scanID, page, perPage, query)
    if err != nil {
        jsonError(w, err.Error(), http.StatusInternalServerError)
        return
    }
    jsonResponse(w, http.StatusOK, map[string]interface{}{
        "results":  results,
        "total":    total,
        "page":     page,
        "per_page": perPage,
    })
}

// POST /discovery/scans/{id}/import
func (h *ScanHandler) handleImport(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
        return
    }
    scanID, err := uuid.Parse(r.PathValue("id"))
    if err != nil {
        jsonError(w, "invalid scan id", http.StatusBadRequest)
        return
    }
    var req struct {
        ResultIDs   []string `json:"result_ids"`
        Credentials []struct {
            ResultID string `json:"result_id"`
            Username string `json:"username"`
            Password string `json:"password"`
        } `json:"credentials"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        jsonError(w, "invalid request body", http.StatusBadRequest)
        return
    }
    // Convert result IDs to UUIDs
    var resultUUIDs []uuid.UUID
    for _, id := range req.ResultIDs {
        uid, err := uuid.Parse(id)
        if err != nil {
            jsonError(w, "invalid result_id: "+id, http.StatusBadRequest)
            return
        }
        resultUUIDs = append(resultUUIDs, uid)
    }
    // Mark as imported (actual camera creation is done via existing /api/cameras endpoint)
    if err := h.store.MarkImported(r.Context(), resultUUIDs); err != nil {
        jsonError(w, err.Error(), http.StatusInternalServerError)
        return
    }
    jsonResponse(w, http.StatusOK, map[string]interface{}{
        "imported": len(resultUUIDs),
        "failed":   []interface{}{},
    })
}

// POST /discovery/credentials/test
func (h *ScanHandler) handleTestCredentials(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
        return
    }
    var req struct {
        IP       string `json:"ip"`
        Port     int    `json:"port"`
        Username string `json:"username"`
        Password string `json:"password"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        jsonError(w, "invalid request body", http.StatusBadRequest)
        return
    }
    deviceURL := "http://" + req.IP + ":" + strconv.Itoa(req.Port) + "/onvif/device_service"
    client := onvif.NewSOAPClient(5*time.Second, &onvif.Credentials{
        Username: req.Username,
        Password: req.Password,
    })
    _, err := onvif.GetDeviceInformation(r.Context(), client, deviceURL)
    if err != nil {
        jsonResponse(w, http.StatusOK, map[string]interface{}{
            "success": false,
            "error":   err.Error(),
        })
        return
    }
    jsonResponse(w, http.StatusOK, map[string]interface{}{
        "success": true,
    })
}

func jsonError(w http.ResponseWriter, msg string, code int) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func jsonResponse(w http.ResponseWriter, code int, data interface{}) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    json.NewEncoder(w).Encode(data)
}
```

Add imports: `"time"`, `"github.com/dam-vms/dam/pkg/onvif"`

- [ ] **Step 2: Update main.go routes**

Replace the old 3 routes in `Start()`:
```go
mux.Handle("/discovery/scan", common.JWTAuthMiddleware(s.handleScan))
mux.Handle("/discovery/results", common.JWTAuthMiddleware(s.handleResults))
mux.Handle("/discovery/status", common.JWTAuthMiddleware(s.handleStatus))
```

With new routes:
```go
scanHandler := NewScanHandler(s.orchestrator, s.store, s.logger)

mux.HandleFunc("/discovery/scans", common.JWTAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodPost:
        scanHandler.handleCreateScan(w, r)
    case http.MethodGet:
        scanHandler.handleListScans(w, r)
    default:
        http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
    }
}))
mux.HandleFunc("/discovery/scans/{id}", common.JWTAuthMiddleware(scanHandler.handleGetScan))
mux.HandleFunc("/discovery/scans/{id}/cancel", common.JWTAuthMiddleware(scanHandler.handleCancelScan))
mux.HandleFunc("/discovery/scans/{id}/results", common.JWTAuthMiddleware(scanHandler.handleGetResults))
mux.HandleFunc("/discovery/scans/{id}/import", common.JWTAuthMiddleware(scanHandler.handleImport))
mux.HandleFunc("/discovery/credentials/test", common.JWTAuthMiddleware(scanHandler.handleTestCredentials))
```

- [ ] **Step 3: Wire dependencies in main.go NewDiscoveryService**

```go
type DiscoveryService struct {
    store        *ResultStore
    orchestrator *ScanOrchestrator
    scanner      map[string]Scanner
    // ... existing fields
}
```

In `NewDiscoveryService`:
```go
s.store = NewResultStore(db, logger)
s.scanners = map[string]Scanner{
    "ws-discovery": NewWSDiscoveryScanner(logger),
    "ip-range":     NewIPRangeScanner(logger),
    "mdns":         NewMDNSScanner(logger),
    "manual":       NewManualIPScanner(logger),
}
s.orchestrator = NewScanOrchestrator(s.store, s.scanners, logger)
```

- [ ] **Step 4: Verify it compiles**

Run: `go build ./services/discovery/`

- [ ] **Step 5: Commit**

```bash
git add services/discovery/handlers.go services/discovery/main.go
git commit -m "feat: add 7 discovery API endpoints with ScanHandler"
```

---

### Task 11: Scheduler (Periodic Per-Site Scanning)

**Files:**
- Create: `services/discovery/scheduler.go`
- Modify: `services/discovery/main.go` (start scheduler)

- [ ] **Step 1: Write Scheduler**

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/google/uuid"
    "github.com/jmoiron/sqlx"
)

type SiteDiscoveryConfig struct {
    Enabled bool     `json:"enabled"`
    Cron    string   `json:"cron"`
    Methods []string `json:"methods"`
    Ports   []int    `json:"ports"`
    Subnets []string `json:"subnets"`
}

type Scheduler struct {
    db           *sqlx.DB
    orchestrator *ScanOrchestrator
    logger       *slog.Logger
    tickInterval time.Duration
}

func NewScheduler(db *sqlx.DB, orchestrator *ScanOrchestrator, logger *slog.Logger) *Scheduler {
    return &Scheduler{
        db:           db,
        orchestrator: orchestrator,
        logger:       logger,
        tickInterval: 60 * time.Second,
    }
}

func (s *Scheduler) Start(ctx context.Context) {
    s.logger.Info("starting discovery scheduler", "interval", s.tickInterval)
    ticker := time.NewTicker(s.tickInterval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            s.logger.Info("scheduler stopped")
            return
        case <-ticker.C:
            s.checkDueScans(ctx)
        }
    }
}

func (s *Scheduler) checkDueScans(ctx context.Context) {
    var sites []struct {
        ID              uuid.UUID `db:"id"`
        DiscoveryConfig *string   `db:"discovery_config"`
        Subnets         []string  `db:"subnets"`
    }

    // Query sites with discovery_config->>enabled = 'true'
    err := s.db.SelectContext(ctx, &sites, `
        SELECT id, discovery_config->>'subnets' as subnets
        FROM sites
        WHERE discovery_config->>'enabled' = 'true'
    `)
    if err != nil {
        s.logger.Error("scheduler: failed to query sites", "error", err)
        return
    }

    for _, site := range sites {
        if len(site.Subnets) == 0 {
            continue
        }
        cfg := SiteDiscoveryConfig{
            Enabled: true,
            Methods: []string{"ws-discovery", "ip-range"},
            Ports:   []int{80, 554, 8080},
            Subnets: site.Subnets,
        }

        _, err := s.orchestrator.StartScan(ctx, ScanRequest{
            SiteID:  site.ID,
            Methods: cfg.Methods,
            Subnets: cfg.Subnets,
            Ports:   cfg.Ports,
        })
        if err != nil {
            s.logger.Error("scheduler: failed to start scan", "site_id", site.ID, "error", err)
        } else {
            s.logger.Info("scheduler: started periodic scan", "site_id", site.ID)
        }
    }
}
```

- [ ] **Step 2: Wire scheduler into main.go**

In `Start()`:
```go
if s.store != nil {
    s.scheduler = NewScheduler(s.store.rawDB, s.orchestrator, s.logger)
    go s.scheduler.Start(ctx)
}
```

Add `scheduler *Scheduler` field to DiscoveryService. Pass the main context from `main()`.

- [ ] **Step 3: Verify it compiles**

Run: `go build ./services/discovery/`

- [ ] **Step 4: Commit**

```bash
git add services/discovery/scheduler.go services/discovery/main.go
git commit -m "feat: add periodic per-site discovery scheduler"
```

---

### Task 12: Frontend API Client Methods

**Files:**
- Modify: `web/src/api/client.ts`

- [ ] **Step 1: Add new API client methods**

Replace existing discovery methods at lines 513-521 with:

```typescript
  // Discovery
  startDiscoveryScan: (data: { site_id: string; methods?: string[]; subnets?: string[]; ports?: number[] }) =>
    request<ScanRecord>('/discovery/scans', { method: 'POST', body: JSON.stringify(data) }),

  getDiscoveryScans: (params?: { site_id?: string; page?: number; per_page?: number }) =>
    request<{ scans: ScanRecord[]; total: number; page: number; per_page: number }>('/discovery/scans?' + new URLSearchParams(params as any)),

  getDiscoveryScan: (id: string) =>
    request<ScanRecord>(`/discovery/scans/${id}`),

  cancelDiscoveryScan: (id: string) =>
    request<{ status: string }>(`/discovery/scans/${id}/cancel`, { method: 'POST' }),

  getDiscoveryResults: (id: string, params?: { page?: number; per_page?: number; query?: string }) =>
    request<{ results: ResultRecord[]; total: number; page: number; per_page: number }>(
      `/discovery/scans/${id}/results?` + new URLSearchParams(params as any)),

  importDiscoveryResults: (scanId: string, data: { result_ids: string[]; credentials?: { result_id: string; username: string; password: string }[] }) =>
    request<{ imported: number; failed: { result_id: string; error: string }[] }>(
      `/discovery/scans/${scanId}/import`, { method: 'POST', body: JSON.stringify(data) }),

  testOnvifCredentials: (data: { ip: string; port: number; username: string; password: string }) =>
    request<{ success: boolean; error?: string }>('/discovery/credentials/test', { method: 'POST', body: JSON.stringify(data) }),
```

- [ ] **Step 2: Add TypeScript interfaces**

At top of client.ts or in a shared types file:
```typescript
interface ScanRecord {
  id: string;
  site_id: string;
  status: 'pending' | 'running' | 'completed' | 'cancelled' | 'failed';
  methods: string[];
  subnets: string[];
  ports: number[];
  total_found: number;
  error?: string;
  started_at?: string;
  completed_at?: string;
  created_at: string;
}

interface ResultRecord {
  id: string;
  scan_id: string;
  ip_address: string;
  port?: number;
  xaddr?: string;
  manufacturer?: string;
  model?: string;
  firmware?: string;
  serial_number?: string;
  hostname?: string;
  capabilities: Record<string, boolean>;
  is_new: boolean;
  already_in_db: boolean;
  imported: boolean;
  created_at: string;
}
```

- [ ] **Step 3: Verify TypeScript compiles**

Run: `npx tsc --noEmit` (or equivalent)

- [ ] **Step 4: Commit**

```bash
git add web/src/api/client.ts
git commit -m "feat: add new discovery API client methods"
```

---

### Task 13: Frontend DiscoveryPage Rewrite

**Files:**
- Modify: `web/src/pages/DiscoveryPage.tsx`

- [ ] **Step 1: Rewrite DiscoveryPage with 3 views**

The page splits into 3 views managed by a `mode` state:

1. **Scan Launcher** (`mode: 'launcher'`)
   - Site selector (fetched from API)
   - Method checkboxes (ws-discovery, ip-range, mdns, manual)
   - Subnet text input (comma-separated CIDRs, auto-populated from site config)
   - Ports text input (comma-separated, default `80,554,8080`)
   - "Start Scan" button

2. **Scan List** (`mode: 'history'`)
   - Paginated table: status badge, methods, subnets, found count, duration, timestamp
   - Click row to view results

3. **Results View** (`mode: 'results'`)
   - Paginated table with search filter
   - Columns: checkbox, IP:Port, Manufacturer, Model, Firmware, Serial, Hostname, Capabilities badges, "In DB" badge
   - Inline credential fields (username/password per device) + "Test" button
   - Import Selected / Import All buttons
   - Import progress panel (per-device success/fail)

```tsx
import { useState, useEffect, useCallback } from 'react';
import { api } from '../api/client';

type ViewMode = 'launcher' | 'history' | 'results';

interface Site { id: string; name: string; }

export default function DiscoveryPage() {
  const [mode, setMode] = useState<ViewMode>('launcher');
  const [sites, setSites] = useState<Site[]>([]);
  const [selectedSite, setSelectedSite] = useState('');
  const [methods, setMethods] = useState<string[]>(['ws-discovery', 'ip-range']);
  const [subnets, setSubnets] = useState('');
  const [ports, setPorts] = useState('80,554,8080');
  const [scanning, setScanning] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Scan history
  const [scans, setScans] = useState<ScanRecord[]>([]);
  const [scanTotal, setScanTotal] = useState(0);
  const [scanPage, setScanPage] = useState(1);

  // Current scan results
  const [currentScanId, setCurrentScanId] = useState<string | null>(null);
  const [results, setResults] = useState<ResultRecord[]>([]);
  const [resultTotal, setResultTotal] = useState(0);
  const [resultPage, setResultPage] = useState(1);
  const [resultQuery, setResultQuery] = useState('');
  const [selectedResults, setSelectedResults] = useState<Set<string>>(new Set());
  const [credentials, setCredentials] = useState<Record<string, { username: string; password: string }>>({});
  const [importing, setImporting] = useState(false);
  const [importResults, setImportResults] = useState<{ result_id: string; status: string; error?: string }[]>([]);
  const [testingCreds, setTestingCreds] = useState<Record<string, 'idle' | 'testing' | 'success' | 'fail'>>({});

  useEffect(() => {
    api.getSites().then(d => setSites(d.sites || [])).catch(() => {});
  }, []);

  const handleStartScan = async () => {
    if (!selectedSite) { setError('Select a site'); return; }
    setScanning(true);
    setError(null);
    try {
      const scan = await api.startDiscoveryScan({
        site_id: selectedSite,
        methods,
        subnets: subnets ? subnets.split(',').map(s => s.trim()) : undefined,
        ports: ports.split(',').map(p => parseInt(p.trim())).filter(p => !isNaN(p)),
      });
      setCurrentScanId(scan.id);
      // Poll for completion
      const poll = setInterval(async () => {
        const s = await api.getDiscoveryScan(scan.id);
        if (s.status !== 'running' && s.status !== 'pending') {
          clearInterval(poll);
          setScanning(false);
          setMode('results');
          loadResults(scan.id, 1, '');
        }
      }, 1000);
    } catch (e: any) {
      setError(e.message || 'Failed to start scan');
      setScanning(false);
    }
  };

  const loadScans = useCallback(async (page: number) => {
    const data = await api.getDiscoveryScans({ site_id: selectedSite || undefined, page, per_page: 20 });
    setScans(data.scans);
    setScanTotal(data.total);
    setScanPage(page);
  }, [selectedSite]);

  const loadResults = useCallback(async (scanId: string, page: number, query: string) => {
    const data = await api.getDiscoveryResults(scanId, { page, per_page: 20, query: query || undefined });
    setResults(data.results);
    setResultTotal(data.total);
    setResultPage(page);
  }, []);

  const handleTestCreds = async (resultId: string, ip: string, port: number, username: string, password: string) => {
    setTestingCreds(prev => ({ ...prev, [resultId]: 'testing' }));
    try {
      const res = await api.testOnvifCredentials({ ip, port, username, password });
      setTestingCreds(prev => ({ ...prev, [resultId]: res.success ? 'success' : 'fail' }));
    } catch {
      setTestingCreds(prev => ({ ...prev, [resultId]: 'fail' }));
    }
  };

  const handleImport = async () => {
    if (!currentScanId || selectedResults.size === 0) return;
    setImporting(true);
    setImportResults([]);
    try {
      const credsList = Array.from(selectedResults)
        .filter(id => credentials[id])
        .map(id => ({
          result_id: id,
          username: credentials[id].username,
          password: credentials[id].password,
        }));
      const res = await api.importDiscoveryResults(currentScanId, {
        result_ids: Array.from(selectedResults),
        credentials: credsList.length > 0 ? credsList : undefined,
      });
      setImportResults([
        ...res.imported.map(id => ({ result_id: id, status: 'imported' })),
        ...res.failed.map(f => ({ result_id: f.result_id, status: 'failed', error: f.error })),
      ]);
      loadResults(currentScanId, 1, '');
    } catch (e: any) {
      setError(e.message || 'Import failed');
    }
    setImporting(false);
  };

  // Render view based on mode
  // ... (full TSX with 3 views)
}
```

The full TSX for 3 views is ~350 lines. Key sections:

**Scan Launcher view:**
- Site dropdown, method checkboxes, subnet input, port input
- Start button with scanning indicator

**Scan History view:**
- Table with columns: Date, Methods, Subnets, Found, Duration, Status
- Click navigates to results
- Pagination

**Results view:**
- Search input + pagination
- Table with checkboxes, IP:Port, manufacturer, model, firmware, serial, hostname, capabilities badges, "In DB" badge
- Inline credential inputs per selected device + Test button
- Import button with progress feedback

- [ ] **Step 2: Verify TypeScript compiles**

Run: `npx tsc --noEmit`

- [ ] **Step 3: Commit**

```bash
git add web/src/pages/DiscoveryPage.tsx
git commit -m "feat: rewrite DiscoveryPage with scan launcher, history, and results views"
```

---

### Task 14: Tests for Scanner Interface

**Files:**
- Create: `services/discovery/scanner_test.go`

- [ ] **Step 1: Write scanner tests**

```go
package main

import (
    "context"
    "testing"
    "time"
)

func TestScannerInterface(t *testing.T) {
    // Verify all scanners implement the interface
    var _ Scanner = (*WSDiscoveryScanner)(nil)
    var _ Scanner = (*IPRangeScanner)(nil)
    var _ Scanner = (*MDNSScanner)(nil)
    var _ Scanner = (*ManualIPScanner)(nil)
}

func TestManualIPScanner_ParseEntries(t *testing.T) {
    entries := parseManualEntries("10.0.0.1:80, 10.0.0.2, 10.0.0.3:554")
    if len(entries) != 3 {
        t.Fatalf("expected 3 entries, got %d", len(entries))
    }
    if entries[0] != "10.0.0.1:80" { t.Errorf("expected 10.0.0.1:80, got %s", entries[0]) }
    if entries[1] != "10.0.0.2:80" { t.Errorf("expected 10.0.0.2:80 (default port), got %s", entries[1]) }
    if entries[2] != "10.0.0.3:554" { t.Errorf("expected 10.0.0.3:554, got %s", entries[2]) }
}

func TestManualIPScanner_EmptyInput(t *testing.T) {
    entries := parseManualEntries("")
    if len(entries) != 0 { t.Errorf("expected 0 entries, got %d", len(entries)) }
}

func TestParseCIDR(t *testing.T) {
    ipNet, err := parseCIDR("10.0.0.0/24")
    if err != nil { t.Fatal(err) }
    if ipNet == nil { t.Fatal("expected non-nil IPNet") }
}

func TestParseCIDR_Invalid(t *testing.T) {
    _, err := parseCIDR("invalid")
    if err == nil { t.Fatal("expected error for invalid CIDR") }
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./services/discovery/ -run TestManual -v`

Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add services/discovery/scanner_test.go
git commit -m "test: add Scanner interface and ManualIPScanner tests"
```

---

### Task 15: Tests for WSDiscoveryScanner

**Files:**
- Create: `services/discovery/wsdiscovery_test.go`

- [ ] **Step 1: Write WSDiscoveryScanner test**

```go
package main

import (
    "context"
    "net"
    "testing"
    "time"
)

func TestBuildProbeXML(t *testing.T) {
    xml, err := buildProbeXML()
    if err != nil { t.Fatal(err) }
    if xml == "" { t.Fatal("expected non-empty XML") }
    if !contains(xml, "Probe") { t.Error("expected Probe element") }
    if !contains(xml, "NetworkVideoTransmitter") { t.Error("expected NetworkVideoTransmitter type") }
}

func TestWSDiscoveryScanner_Name(t *testing.T) {
    s := NewWSDiscoveryScanner(nil)
    if s.Name() != "ws-discovery" {
        t.Errorf("expected ws-discovery, got %s", s.Name())
    }
}

// Test with a mock UDP server
func TestWSDiscoveryScanner_ContextCancellation(t *testing.T) {
    scanner := NewWSDiscoveryScanner(nil)
    ctx, cancel := context.WithCancel(context.Background())
    cancel() // immediately cancel

    ch, err := scanner.Scan(ctx, "local", []int{80}, ScanOptions{Timeout: 1 * time.Second})
    if err != nil { t.Fatal(err) }

    timeout := time.After(2 * time.Second)
    select {
    case res, ok := <-ch:
        if ok {
            t.Logf("got result: %+v", res)
        }
    case <-timeout:
        t.Fatal("channel not closed after context cancellation")
    }
}

func contains(s, substr string) bool {
    return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
    for i := 0; i <= len(s)-len(substr); i++ {
        if s[i:i+len(substr)] == substr {
            return true
        }
    }
    return false
}
```

(Note: `contains` is a simple helper since we may not want to import strings)

- [ ] **Step 2: Run tests**

Run: `go test ./services/discovery/ -run TestWSDiscovery -v`

Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add services/discovery/wsdiscovery_test.go
git commit -m "test: add WSDiscoveryScanner tests"
```

---

### Task 16: Tests for IPRangeScanner

**Files:**
- Create: `services/discovery/iprange_test.go`

- [ ] **Step 1: Write IPRangeScanner test**

```go
package main

import (
    "net"
    "testing"
)

func TestCollectIPs_SmallSubnet(t *testing.T) {
    _, ipNet, _ := net.ParseCIDR("10.0.0.0/30")
    ips := collectIPs(ipNet)
    // /30 = 4 IPs, minus network and broadcast = 2 usable
    if len(ips) != 2 {
        t.Logf("got ips: %v", ips)
    }
}

func TestIPRangeScanner_Name(t *testing.T) {
    s := NewIPRangeScanner(nil)
    if s.Name() != "ip-range" {
        t.Errorf("expected ip-range, got %s", s.Name())
    }
}

func TestProbeONVIFDevice_Timeout(t *testing.T) {
    // Should return nil for unreachable IP
    result := probeONVIFDevice(nil, "10.255.255.255:80", time.Millisecond)
    if result != nil {
        t.Log("expected nil for unreachable device, got result")
    }
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./services/discovery/ -run TestIPRange -v`

Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add services/discovery/iprange_test.go
git commit -m "test: add IPRangeScanner tests"
```

---

### Task 17: Tests for ResultStore

**Files:**
- Create: `services/discovery/store_test.go`

- [ ] **Step 1: Write ResultStore tests**

Since these need a DB, use a test container or skip if not available:

```go
package main

import (
    "context"
    "os"
    "testing"
    "github.com/google/uuid"
    "github.com/jmoiron/sqlx"
    _ "github.com/lib/pq"
)

func newTestStore(t *testing.T) *ResultStore {
    dbURL := os.Getenv("TEST_DB_URL")
    if dbURL == "" {
        t.Skip("TEST_DB_URL not set, skipping")
    }
    db, err := sqlx.Connect("postgres", dbURL)
    if err != nil {
        t.Fatalf("failed to connect: %v", err)
    }
    // Ensure tables exist
    db.MustExec(`CREATE TABLE IF NOT EXISTS discovery_scans (...same as migration...)`)
    db.MustExec(`CREATE TABLE IF NOT EXISTS discovery_results (...same as migration...)`)
    t.Cleanup(func() { db.Close() })
    return NewResultStore(db, nil)
}

func TestCreateAndGetScan(t *testing.T) {
    store := newTestStore(t)
    ctx := context.Background()
    id := uuid.New()
    siteID := uuid.New()
    scan := &ScanRecord{
        ID: id, SiteID: siteID, Status: "pending",
        Methods: []string{"ws-discovery"}, Ports: []int{80, 554},
    }
    if err := store.CreateScan(ctx, scan); err != nil {
        t.Fatal(err)
    }
    got, err := store.GetScan(ctx, id)
    if err != nil { t.Fatal(err) }
    if got.Status != "pending" { t.Errorf("expected pending, got %s", got.Status) }
    if got.SiteID != siteID { t.Errorf("expected %s, got %s", siteID, got.SiteID) }
}
```

- [ ] **Step 2: Run tests**

Run: `TEST_DB_URL=postgres://... go test ./services/discovery/ -run TestCreateAndGetScan -v`

Expected: PASS (or SKIP if no TEST_DB_URL)

- [ ] **Step 3: Commit**

```bash
git add services/discovery/store_test.go
git commit -m "test: add ResultStore tests"
```

---

### Task 18: Tests for ScanOrchestrator

**Files:**
- Create: `services/discovery/orchestrator_test.go`

- [ ] **Step 1: Write orchestrator tests**

```go
package main

import (
    "context"
    "testing"
    "github.com/google/uuid"
)

type mockScanner struct {
    name    string
    results []ScanResult
}

func (m *mockScanner) Name() string { return m.name }
func (m *mockScanner) Scan(ctx context.Context, subnet string, ports []int, opts ScanOptions) (<-chan ScanResult, error) {
    ch := make(chan ScanResult)
    go func() {
        defer close(ch)
        for _, r := range m.results {
            select {
            case <-ctx.Done():
                return
            case ch <- r:
            }
        }
    }()
    return ch, nil
}

func TestOrchestrator_Deduplication(t *testing.T) {
    store := &mockStore{}
    scanners := map[string]Scanner{
        "mock": &mockScanner{
            name: "mock",
            results: []ScanResult{
                {XAddr: "http://10.0.0.1/onvif", IP: "10.0.0.1"},
                {XAddr: "http://10.0.0.1/onvif", IP: "10.0.0.1"}, // duplicate
                {XAddr: "http://10.0.0.2/onvif", IP: "10.0.0.2"},
            },
        },
    }
    o := NewScanOrchestrator(store, scanners, nil)
    scan, _ := o.StartScan(context.Background(), ScanRequest{
        SiteID: uuid.New(), Methods: []string{"mock"}, Subnets: []string{"local"},
    })
    if scan == nil { t.Fatal("expected scan record") }
    // Wait for scan to complete
    // Verify store got 2 unique results
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./services/discovery/ -run TestOrchestrator -v`

- [ ] **Step 3: Commit**

```bash
git add services/discovery/orchestrator_test.go
git commit -m "test: add ScanOrchestrator tests"
```

---

### Task 19: Docker Compose + Helm Updates

**Files:**
- Modify: `deploy/docker/docker-compose.yml`
- Modify: `deploy/helm/evms/values.yaml`
- Modify: `deploy/helm/evms/templates/discovery.yaml`

- [ ] **Step 1: Add DB_URL to docker-compose discovery service**

```yaml
  discovery:
    build:
      context: ../../
      dockerfile: services/discovery/Dockerfile
    environment:
      - METRICS_ADDR=:2112
      - DISCOVERY_NATS_URL=nats://nats:4222
      - JWT_SECRET=${JWT_SECRET:?JWT_SECRET must be set}
      - DB_URL=postgres://${DB_USER:-dam_admin}:${DB_PASSWORD:?DB_PASSWORD must be set}@db:5432/${DB_NAME:-dam_vms}?sslmode=disable
    depends_on:
      nats:
        condition: service_healthy
      db:
        condition: service_healthy
```

- [ ] **Step 2: Add DB_URL to Helm values.yaml**

```yaml
  discovery:
    enabled: true
    replicas: 1
    image: damvms/discovery
    port: 8091
    resources:
      requests:
        cpu: 100m
        memory: 128Mi
      limits:
        cpu: 300m
        memory: 256Mi
    env:
      DB_URL: ""  # set via --set or external secret
```

- [ ] **Step 3: Add DB_URL to Helm discovery template**

In `deploy/helm/evms/templates/discovery.yaml`, add to env section:
```yaml
env:
- name: DB_URL
  value: {{ .Values.discovery.env.DB_URL | default "" | quote }}
```

- [ ] **Step 4: Commit**

```bash
git add deploy/docker/docker-compose.yml deploy/helm/evms/values.yaml deploy/helm/evms/templates/discovery.yaml
git commit -m "chore: add DB_URL to discovery service deployment configs"
```

---

### Task 20: Clean Up main.go (remove dead code)

**Files:**
- Modify: `services/discovery/main.go`

- [ ] **Step 1: Remove old discoveredCamera type, scanStatus, old handlers**

Delete from `main.go`:
- `discoveredCamera` struct (replaced by `ScanResult`)
- `scanStatus` struct (replaced by `ScanRecord`)
- `handleScan`, `handleStatus`, `handleResults` methods
- `DiscoveryService.results`, `.scanning`, `.scanError` fields
- `sync.RWMutex` and `sync.Mutex` fields (replaced by orchestrator+store)

Keep:
- `DiscoveryConfig`
- `NewDiscoveryService` / `Start` / `Shutdown`
- NATS connection logic
- main() function

- [ ] **Step 2: Verify it compiles**

Run: `go build ./services/discovery/`

- [ ] **Step 3: Run all tests**

Run: `go test ./services/discovery/ -v`

- [ ] **Step 4: Commit**

```bash
git add services/discovery/main.go
git commit -m "refactor: remove old discovery types and handlers, use new architecture"
```

---

## Self-Review Checklist

After writing this plan, verify against the spec:

1. **Scanner Interface + 4 implementations** → Tasks 3-7
2. **Persistence (discovery_scans + discovery_results tables)** → Tasks 1, 8
3. **7 API endpoints** → Task 10
4. **Frontend 3 views** → Tasks 12-13
5. **Scheduled scanning** → Task 11
6. **Import improvements + credential testing** → Tasks 10, 13
7. **Deploy config** → Task 19
8. **Tests** → Tasks 14-18
9. **Clean up old code** → Task 20
