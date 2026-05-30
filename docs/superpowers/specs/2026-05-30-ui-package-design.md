# UI Package — Production Frontend for DAM VMS

## Status

Approved. Implementation follows.

## Architecture

### New backend services (2)

**`camera-control` service** — Go gRPC+HTTP service that translates REST commands to ONVIF PTZ, Axis VAPIX, or Hikvision CGI.

API surface:
- `POST /api/cameras/{id}/ptz/move` — `{direction, speed}` (8-way: up/down/left/right/upleft/upright/downleft/downright)
- `POST /api/cameras/{id}/ptz/stop`
- `GET /api/cameras/{id}/ptz/presets` — `[{name, token}]`
- `POST /api/cameras/{id}/ptz/presets/{token}/goto`
- `POST /api/cameras/{id}/ptz/zoom` — `{level: 0.0-1.0}`

Protocol mapping stored per-camera in DB (`ptz_protocol` column: `onvif`, `vapix`, `hikvision`, `none`). Service discovers ONVIF device URI from camera connection URL or uses vendor overrides.

**`thumbnails` service** — Go HTTP service that generates timeline thumbnails.

API surface:
- `GET /api/thumbnails/timeline?camera_id=X&start=ISO&end=ISO&interval=60` — returns `{thumbnails: [{timestamp, url}]}`
- `GET /api/thumbnails/image/{camera_id}/{timestamp}.jpg` — serves cached thumbnail

Generates thumbnails via FFmpeg seeking. Caches to disk with TTL = retention period. Interval minimum: 10s, default: 60s.

### Gateway changes

Add routes in `services/api-gateway/main.go`:
- `/api/cameras/{id}/ptz/*` → proxy to camera-control service
- `/api/thumbnails/*` → proxy to thumbnails service

Add `requireRole(role string)` middleware for server-side RBAC enforcement.

### Frontend component tree

```
Layout (sidebar expanded)
├── Dashboard
│   └── CameraCard (virtual-scrolled grid)
│       ├── CameraView (WebRTC, refactored)
│       ├── PtzOverlay (directional pad + zoom + presets)
│       └── CameraInfo (name, status badge)
├── RecordingsPage
│   ├── Video player
│   ├── TimelineScrubber (thumbnail timeline, zoom levels)
│   └── Recordings table
├── EventsPage (filterable table)
├── SettingsPage (retention + camera CRUD)
└── AdminPage (user CRUD + role assignment)
```

### RBAC model

| Resource | admin | operator | viewer |
|---|---|---|---|
| View cameras | ✓ | ✓ | ✓ |
| View recordings | ✓ | ✓ | ✓ |
| View events | ✓ | ✓ | ✓ |
| PTZ control | ✓ | ✓ | — |
| User management | ✓ | — | — |
| Settings | ✓ | — | — |

Enforced server-side via `common.Claims.Role` in JWT + gateway `requireRole` middleware.

## Multi-Camera Grid

- Grid of `CameraCard` components using `react-virtuoso` for virtualized rendering
- 3 layout modes: 1×1 (single), 2×2 (4), 3×3 (9)
- Camera count > 9: scrollable grid with layout selector
- Left sidebar: collapsible camera tree grouped by site, search filter
- WebRTC connections only open for cameras in viewport
- Responsive: <1024px single-view swipe, 1024-1920px auto-fit, >1920px user-selected

## PTZ Controls

- `PtzOverlay` component: translucent on hover, disappears after 3s inactivity
- 8-way directional pad + center stop button
- Zoom slider (±), preset buttons, speed selector
- Only shown when role ≥ operator and camera `ptz_protocol ≠ none`
- PTZ endpoints require role ≥ operator (gateway enforcement)

## Timeline

- `TimelineScrubber` component: horizontal thumbnail strip for selected camera
- Zoom levels: 1h, 6h, 24h — adjusts thumbnail interval
- Drag playhead to seek, click thumbnail to play
- Time ruler with labeled markers
- Event overlays: colored dots at AI event timestamps (person=yellow, vehicle=red)
- Current time indicator (red line)
- Retention per camera configurable 7-90 days via settings

## User Admin

- Admin-only page with user list (username, role, status, created date)
- Create user: modal form (username, password, role selector)
- Edit user: inline role change, password reset
- Deactivate: soft-delete toggle
- New auth service endpoints: `GET/POST /api/admin/users`, `PUT/DELETE /api/admin/users/{id}`
- Sidebar shows role badge, Admin nav item conditional on role

## Implementation Order

1. **camera-control service** — PTZ backend, gateway routes, DB migration for `ptz_protocol`
2. **Multi-camera grid** — virtual-scrolled Dashboard, CameraCard, responsive layouts
3. **PTZ frontend** — PtzOverlay, gateway requireRole middleware
4. **thumbnails service** — thumbnail generation, caching, cleanup
5. **Timeline frontend** — TimelineScrubber, updated RecordingsPage
6. **User admin backend** — auth service endpoints, DB migration for soft-delete
7. **User admin frontend** — AdminPage, RBAC conditional rendering in Layout
8. **Settings rewrite** — retention per camera, camera CRUD from UI
