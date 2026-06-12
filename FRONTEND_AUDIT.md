# Frontend Architecture Audit

## Executive Summary

- **Total pages audited:** 40
- **Placeholder pages:** 0
- **Pages with missing loading states:** 3 (SettingsPage, SearchPage)
- **Pages with missing error handling:** 2 (WebhooksPage, ImagingPage)
- **Pages with missing empty states:** 4 (CamerasPage, VideoWallPage, EventsPage, SearchPage)
- **Dead routes (sidebar but no route):** 2 (/gis, /maps)
- **Duplicate pages:** 1 (MapPage ≈ MapsPage)
- **Overlapping functionality:** SearchPage, EventsPage, ForensicsPage all show AI events

---

## Page-by-Page Audit

### Live View & Cameras
| Page | Status | Loading | Error | Empty | Issues |
|------|--------|:-------:|:-----:|:-----:|--------|
| CamerasPage | Complete | ✓ | ✓ | ✗ | No empty state for zero results |
| CameraHealthPage | Complete | ✓ | ✓ | ✓ | Shows site UUID instead of site name |
| DiscoveryPage | Complete | Partial | ✓ | ✓ | Sites fetch errors silently swallowed |
| VideoWallPage | Complete | ✓ | ✓ | ✗ | No empty wall message |

### Playback & Recordings
| Page | Status | Loading | Error | Empty | Issues |
|------|--------|:-------:|:-----:|:-----:|--------|
| PlaybackPage | Partial | ✓ | ✓ | ✓ | Date filters are dead code; no pagination |
| RecordingsPage | Complete | ✓ | ✓ | ✓ | Uses alert(); camera checkbox doesn't filter |
| ExportPage | Complete | ✓ | ✓ | ✓ | Clean |
| TimelinePage | Complete | ✓ | ✓ | ✓ | Silent camera fetch failures |

### Events & Search
| Page | Status | Loading | Error | Empty | Issues |
|------|--------|:-------:|:-----:|:-----:|--------|
| EventsPage | Partial | ✓ | ✓ | ✗ | No empty state; overlaps SearchPage |
| SearchPage | Partial | ✗ | ✓ | ✗ | No loading indicator; no empty state; overlaps EventsPage |
| ForensicsPage | Complete | ✓ | ✓ | ✓ | Overlaps SearchPage/EventsPage; heavy `any` |

### Evidence & Incidents
| Page | Status | Loading | Error | Empty | Issues |
|------|--------|:-------:|:-----:|:-----:|--------|
| EvidencePage | Complete | ✓ | ✓ | ✓ | No UI for adding evidence items |
| IncidentsPage | Complete | Partial | ✓ | ✓ | Filter loading not visible |
| BookmarksPage | Complete | ✓ | ✓ | ✓ | Clean |
| LegalHoldPage | Complete | ✓ | ✓ | ✓ | No success feedback |

### Admin & Settings
| Page | Status | Loading | Error | Empty | Issues |
|------|--------|:-------:|:-----:|:-----:|--------|
| AdminPage | Complete | ✓ | ✓ | ✓ | Site creation uses empty string for ID |
| SettingsPage | Partial | ✗ | ✗ | ✓ | Archive sliders do nothing; all errors swallowed |
| ConfigPage | Complete | ✓ | ✓ | ✓ | Clean |
| AuditPage | Complete | ✓ | ✓ | ✓ | Clean |
| SessionsPage | Complete | ✓ | ✓ | ✓ | Clean |
| SsoPage | Complete | ✓ | ✓ | ✓ | Success message never auto-clears |
| MfaPage | Partial | ✓ | ✓ | N/A | Disable MFA uses wrong API endpoint |
| WebhooksPage | Partial | ✓ | ✗ | ✓ | No error handling at all |
| ChannelsPage | Complete | ✓ | ✓ | ✓ | Fragile JSON parsing |

### Monitoring & Analytics
| Page | Status | Loading | Error | Empty | Issues |
|------|--------|:-------:|:-----:|:-----:|--------|
| HealthPage | Complete | ✓ | ✓ | ✓ | Uses raw fetch instead of api client |
| AnalyticsPage | Complete | Partial | ✓ | ✓ | Race condition in loading |
| StoragePage | Complete | ✓ | ✓ | ✓ | Clean |
| AlertsPage | Complete | ✓ | ✓ | ✓ | Hardcoded admin username |

### Maps & GIS
| Page | Status | Loading | Error | Empty | Issues |
|------|--------|:-------:|:-----:|:-----:|--------|
| MapPage | Complete | ✓ | ✓ | ✗ | No empty state for no markers |
| MapsPage | Partial | ✓ | ✓ | ✗ | DEAD ROUTE (no /maps route); duplicates MapPage |
| GISPage | Partial | ✗ | ✓ | ✓ | DEAD ROUTE (no /gis route); KMZ support broken |

### ONVIF & Device Management
| Page | Status | Loading | Error | Empty | Issues |
|------|--------|:-------:|:-----:|:-----:|--------|
| OnvifEventsPage | Complete | ✓ | ✓ | ✓ | Error persistence in dialog |
| OnvifRecordingsPage | Complete | ✓ | Partial | ✓ | Error messages styled as green |
| DevicePage | Complete | ✓ | Weak | N/A | Success/error ambiguity |
| ImagingPage | Complete | ✓ | ✗ | N/A | Errors shown as green text |
| POSPage | Complete | ✓ | ✓ | ✓ | No auto-refresh |
| ZonesPage | Complete | ✓ | ✓ | ✓ | Heavy `any` types |

---

## Cross-Cutting Issues

1. **Page Duplication:** MapPage and MapsPage are nearly identical; one should be removed.
2. **Dead Routes:** `/gis` and `/maps` links exist in sidebar but have no route → always redirect to `/`.
3. **Overlapping Functionality:** SearchPage, EventsPage, and ForensicsPage all display AI detection events. Consider consolidation.
4. **Inconsistent Error Display:** Some pages use red error banners, others use green message text for errors.
5. **Heavy `any` Usage:** At least 8 pages type API responses as `any[]`, losing schema safety.
6. **Silent Error Swallowing:** At least 5 pages have `.catch(() => {})` that silently drop errors.
7. **No Error Clearing:** Most pages never dismiss error messages (no timeout/dismiss button).

---

## Required Fixes

1. Add routes for `/gis` and `/maps` or remove sidebar links
2. Add empty states to CamerasPage, VideoWallPage, EventsPage, SearchPage
3. Fix SearchPage loading indicator
4. Fix SettingsPage error handling (add catch handlers with user feedback)
5. Fix MfaPage disable endpoint
6. Add error handling to WebhooksPage
7. Fix HealthPage to use `api` client instead of raw `fetch`
8. Fix AlertsPage hardcoded admin username
9. Merge MapsPage into MapPage
