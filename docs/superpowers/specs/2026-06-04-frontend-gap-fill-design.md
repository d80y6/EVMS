# EVMS Frontend Gap Fill Design

## Goal

Fill all gaps between backend API endpoints and frontend UI, enabling full runtime interaction through the web interface.

## Navigation Structure

Two collapsible sub-menus added to the sidebar Layout:

```
▸ Cameras & Retention         ← collapsible
    Cameras                   ← camera CRUD management
    Legal Holds               ← existing LegalHoldPage (currently unwired)
    Discovery                 ← ONVIF WS-Discovery scan
    ONVIF Events              ← ONVIF event subscription management
    Bookmarks                 ← bookmark management
    Export                    ← export recordings

▸ Monitoring                 ← collapsible
    Alerts & Rules            ← alert list + acknowledge
    Analytics                 ← people counting, facial detection, heatmap
    Audit Chain               ← audit log chain + verify
    POS Transactions          ← POS transaction viewer
```

Existing top-level nav items remain unchanged (Live View, Recordings, Events, Map, Search, Health, Storage, Admin, Settings).

## Gateway Changes (api-gateway)

### New Config Fields

- `DiscoveryURL`: defaults to `http://discovery:8091`
- `OnvifEventsURL`: defaults to `http://onvif-events:8092`

### New Proxy Fields

- `discoveryProxy: *httputil.ReverseProxy`
- `onvifEventsProxy: *httputil.ReverseProxy`

### New Route Patterns

- `/api/discovery/*` → discoveryProxy (authMiddleware)
- `/api/onvif-events/*` → onvifEventsProxy (authMiddleware)

### Camera CRUD Handlers

The gateway currently only proxies GET `/api/cameras` and GET `/api/cameras/{id}`. The camera-mgmt gRPC service supports CreateCamera, UpdateCamera, DeleteCamera. Add gateway handlers:

- `POST /api/cameras` → gRPC CreateCamera (admin role)
- `PUT /api/cameras/{id}` → gRPC UpdateCamera (operator+ role)
- `DELETE /api/cameras/{id}` → gRPC DeleteCamera (admin role)

### Upstream Health

Add `discovery` and `onvif-events` to upstream health checks.

### Docker Compose

Add `DISCOVERY_URL=http://discovery:8091` and `ONVIF_EVENTS_URL=http://onvif-events:8092` env vars to api-gateway service in docker-compose.yml.

## API Client Additions (`web/src/api/client.ts`)

### Camera CRUD

```typescript
createCamera(data: { site_id: string; name: string; connection_url: string; ... })
updateCamera(id: string, data: ...)
deleteCamera(id: string)
```

### Legal Holds (gateway already proxies `/api/legal-holds`)

Functions already exist via `apiClient.fetch` in LegalHoldPage — promote to `api.` namespace.

### Audit

```typescript
getAuditChain()  → GET /api/audit/chain
verifyAudit()    → GET /api/audit/verify
```

### Export (api.exportRecording already exists — unused)

### POS (api.getPOSTransactions already exists — unused)

### Discovery

```typescript
scanDiscovery()          → POST /api/discovery/scan
getDiscoveryResults()    → GET /api/discovery/results
```

### ONVIF Events

```typescript
subscribeOnvifEvents(cameraId, deviceUrl)         → POST /api/onvif-events/subscribe
unsubscribeOnvifEvents(cameraId)                  → DELETE /api/onvif-events/subscribe/{cameraId}
listOnvifSubscriptions()                          → GET /api/onvif-events/subscriptions
```

## New Pages

All pages follow the same visual pattern as SettingsPage:
- Container: `bg-slate-900 border border-slate-800 rounded-xl p-6`
- Inputs: `bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-slate-300`
- Buttons: `px-3 py-1 bg-indigo-600 hover:bg-indigo-500 text-white rounded text-xs`
- Tables: standard table with `border-b border-slate-800 hover:bg-slate-800/50`
- Loading: `<div className="p-4">Loading...</div>`
- Empty state: `<p className="text-sm text-slate-500">No X configured.</p>`

### 1. CamerasPage (`/cameras`)

- Table listing all cameras with columns: Name, Site, Status, PTZ Protocol, Retention, Actions
- "Add Camera" button opens modal dialog with fields: Name, Site (dropdown), Connection URL, Substream URL, PTZ Protocol, Retention Days
- Each row has Edit (inline or modal) and Delete (with confirmation)
- Stateless — fetches fresh data on mount via `api.listCameras()`
- Delete calls `api.deleteCamera(id)` 
- Create calls `api.createCamera(data)`
- Edit calls `api.updateCamera(id, data)`

Note: The gateway currently only proxies GET cameras and GET camera by ID. The gRPC service supports CreateCamera, UpdateCamera, DeleteCamera. Gateway handlers need to be added for POST/PUT/DELETE on `/api/cameras` routes.

### 2. LegalHoldPage (wire existing)

- Add route `/legal-holds` → `LegalHoldPage` in `main.tsx`
- Page already fully implemented with list, create dialog, release

### 3. Bookmarks (embedded in RecordingsPage or standalone page)

- Standalone page at `/bookmarks`
- List bookmarks with camera name, timestamp, label
- Add bookmark dialog

### 4. Export UI (embedded in RecordingsPage or standalone page)

- Could be integrated into RecordingsPage as an "Export" button per recording
- Or standalone page at `/export` with camera/time range/watermark selection
- Standalone page preferred for clarity

### 5. AlertsPage (`/alerts`)

- Table of alerts: Camera, Message, Status, Created At
- "Acknowledge" button per alert
- Create/view alert rules (future enhancement — basic list now)

### 6. AnalyticsPage (`/analytics`)

- Tabbed layout: People Counting | Facial Detection | Heatmap
- People Counting: table with camera, zone, count
- Facial Detection: search form + results table
- Heatmap: camera selector + visualization placeholder

### 7. AuditPage (`/audit`)

- Two sections: Chain Integrity status + Audit Log table
- Verify button shows chain validity, count, first/last hash
- Table of audit log entries (preliminary — just GET chain info)

### 8. POSPage (`/pos`)

- Transaction list: Transaction ID, Store, Register, Camera, Total, Timestamp
- Expand row to show line items

### 9. DiscoveryPage (`/discovery`)

- "Scan Network" button triggers `POST /api/discovery/scan`
- Results table: Device URL, Manufacturer, Model, Firmware Version
- "Add as Camera" button per result — pre-fills CamerasPage create form
- CameraSelectorService integration for ONVIF WS-Discovery

### 10. OnvifEventsPage (`/onvif-events`)

- List current ONVIF event subscriptions per camera
- "Subscribe" button with camera ID + ONVIF device URL dialog
- "Unsubscribe" button per subscription
- Auto-refresh subscription list

## Route Registration

All new pages registered in `main.tsx` with `ProtectedRoute` + `Layout` wrapper:

```tsx
<Route path="/cameras" element={<ProtectedRoute><Layout><CamerasPage /></Layout></ProtectedRoute>} />
<Route path="/legal-holds" element={<ProtectedRoute><Layout><LegalHoldPage /></Layout></ProtectedRoute>} />
<Route path="/bookmarks" element={<ProtectedRoute><Layout><BookmarksPage /></Layout></ProtectedRoute>} />
<Route path="/export" element={<ProtectedRoute><Layout><ExportPage /></Layout></ProtectedRoute>} />
<Route path="/alerts" element={<ProtectedRoute><Layout><AlertsPage /></Layout></ProtectedRoute>} />
<Route path="/analytics" element={<ProtectedRoute><Layout><AnalyticsPage /></Layout></ProtectedRoute>} />
<Route path="/audit" element={<ProtectedRoute><Layout><AuditPage /></Layout></ProtectedRoute>} />
<Route path="/pos" element={<ProtectedRoute><Layout><POSPage /></Layout></ProtectedRoute>} />
<Route path="/discovery" element={<ProtectedRoute><Layout><DiscoveryPage /></Layout></ProtectedRoute>} />
<Route path="/onvif-events" element={<ProtectedRoute><Layout><OnvifEventsPage /></Layout></ProtectedRoute>} />
```

## Existing Page Fixes

### Dashboard
- Add empty state when no cameras exist
- Fix filter fallback: show empty state instead of silently showing all cameras when filter yields 0 results
- Add error state for failed API calls
- Add retry/timeout feedback to "Connecting..." loading state

### RecordingsPage
- Fetch events and pass to TimelineScrubber (currently hardcoded `events={[]}`)
- Add close button to POS overlay

### EventsPage
- Fix rule toggle checkboxes: add `onChange` handler and `api.toggleRule()` method
- Add `api.getRules()` method to API client
- Add loading and error states for events, alerts, and rules fetches

### AdminPage
- Add site management UI: list sites, create site dialog (using existing `api.createSite()` and `api.getSites()`)
- Fix LegalHoldPage styling: convert from light theme to dark theme (bg-slate-900, etc.)

### SearchPage
- Remove `as any` type cast on smartSearch params
- Add error state for failed search

### MapPage
- Add loading state while cameras load
- Add error state for failures
- Fix `<h1>` to `<h2>` for consistency

### HealthPage
- Add loading state ("Loading system health...")

### StoragePage
- Add error state for failed API calls
- Add empty state for table
- Switch from `apiClient.fetch()` to `api.` namespace

### SettingsPage
- **Replace ONVIF relay placeholder text with operable relay controls**: toggle buttons calling `api.setRelayState()`
- **Add state binding + save button to Archive Tiering inputs**
- **Add create/edit/delete UI for Privacy Masking**
- Remove placeholder "Notifications" and "Streaming" sections (or make functional)
- Remove `console.log()` debug statement
- Add error states across all sections

### LegalHoldPage
- Convert from light theme to dark theme (consistent with rest of app)
- Replace `console.error()` with user-facing error messages
- Switch from `apiClient.fetch()` to `api.` namespace

### Cross-cutting
- Add 404 catch-all route in main.tsx
- Add `api.toggleRule()`, `api.getRules()` to API client
- Add `api.getAuditChain()`, `api.verifyAudit()`, `api.createCamera()`, `api.updateCamera()`, `api.deleteCamera()`, `api.scanDiscovery()`, `api.getDiscoveryResults()`, `api.subscribeOnvifEvents()`, `api.unsubscribeOnvifEvents()`, `api.listOnvifSubscriptions()`, `api.getLegalHolds()`, `api.createLegalHold()`, `api.releaseLegalHold()` to API client

## Implementation Order

1. Gateway: add camera CRUD handlers + discovery + onvif-events proxies and routes
2. Docker compose: add new env vars to api-gateway service
3. API client: add all missing functions (camera CRUD, audit, legal holds, rules, discovery, onvif-events)
4. Fix existing pages (in order: SettingsPage, EventsPage, AdminPage, LegalHoldPage, RecordingsPage, Dashboard, SearchPage, MapPage, HealthPage, StoragePage)
5. Navigation: update Layout.tsx with collapsible sub-menus
6. Routes: add all new routes to main.tsx
7. Implement new pages (in dependency order)
8. Rebuild and restart api-gateway with new proxy config
9. Verify: check all routes render and API calls succeed
