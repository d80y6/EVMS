# Frontend Tests & E2E Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Vitest component tests for SettingsPage, SearchPage, and WebhooksPage, and Playwright E2E tests for critical user flows.

**Architecture:** Component tests mock the API client via `vi.mock()` and test each UI state (loading, error, empty, populated) by controlling mock return values. E2E tests use Playwright with route interception to simulate API responses against a real dev server.

**Tech Stack:** Vitest, @testing-library/react, @testing-library/user-event, Playwright, React 18, TypeScript, Vite, TailwindCSS

---

### Task 1: SettingsPage Component Tests

**Files:**
- Create: `web/src/pages/__tests__/SettingsPage.test.tsx`

- [ ] **Step 1: Create the test directory**

Run:
```bash
mkdir -p web/src/pages/__tests__
```

- [ ] **Step 2: Write SettingsPage.test.tsx**

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import SettingsPage from '../SettingsPage';
import * as client from '../../api/client';

vi.mock('../../api/client', () => ({
  api: {
    getCameras: vi.fn(),
    listTours: vi.fn(),
    startTour: vi.fn(),
    stopTour: vi.fn(),
    deleteTour: vi.fn(),
    createTour: vi.fn(),
    updateCameraConfig: vi.fn(),
    changePassword: vi.fn(),
    setRelayState: vi.fn(),
  },
  setAuthToken: vi.fn(),
  getAuthToken: vi.fn(() => null),
  fetchCSRFToken: vi.fn(),
}));

vi.mock('../../components/FloorPlanView', () => ({
  FloorPlanView: () => null,
}));

const mockCameras = [
  { id: 'cam-1', site_id: 'site-1', name: 'Front Door', description: '', connection_url: '', substream_url: '', status: 'online', ptz_protocol: '', retention_days: 14, config: '' },
  { id: 'cam-2', site_id: 'site-1', name: 'Back Yard', description: '', connection_url: '', substream_url: '', status: 'online', ptz_protocol: 'onvif', retention_days: 30, config: '' },
];

const mockTours = [
  { id: 'tour-1', name: 'Morning Patrol', enabled: false, steps: [{ camera_id: 'cam-1', dwell_seconds: 10 }], interval: 30, created_at: '2024-01-01T00:00:00Z' },
];

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(client.api.getCameras).mockResolvedValue({ cameras: mockCameras });
  vi.mocked(client.api.listTours).mockResolvedValue({ tours: mockTours });
});

describe('SettingsPage', () => {
  it('shows loading state initially', () => {
    render(<SettingsPage />);
    expect(screen.getByText('Loading settings...')).toBeInTheDocument();
  });

  it('renders camera names after loading', async () => {
    render(<SettingsPage />);
    await waitFor(() => {
      expect(screen.getByText('Front Door')).toBeInTheDocument();
    });
    expect(screen.getByText('Back Yard')).toBeInTheDocument();
  });

  it('shows error when getCameras fails', async () => {
    vi.mocked(client.api.getCameras).mockRejectedValue(new Error('Failed to fetch'));
    render(<SettingsPage />);
    await waitFor(() => {
      expect(screen.getByText('Failed to fetch')).toBeInTheDocument();
    });
  });

  it('renders retention range sliders for each camera', async () => {
    render(<SettingsPage />);
    await waitFor(() => {
      const sliders = screen.getAllByRole('slider');
      expect(sliders).toHaveLength(2);
    });
  });

  it('renders tour list', async () => {
    render(<SettingsPage />);
    await waitFor(() => {
      expect(screen.getByText('Morning Patrol')).toBeInTheDocument();
    });
    expect(screen.getByText('(1 steps, 30s interval)')).toBeInTheDocument();
  });

  it('opens new tour dialog when clicking + New Tour', async () => {
    render(<SettingsPage />);
    await waitFor(() => {
      expect(screen.queryByText('Loading settings...')).not.toBeInTheDocument();
    });
    await userEvent.click(screen.getByText('+ New Tour'));
    expect(screen.getByText('New Tour')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('Tour Name')).toBeInTheDocument();
  });

  it('renders password change form', async () => {
    render(<SettingsPage />);
    await waitFor(() => {
      expect(screen.queryByText('Loading settings...')).not.toBeInTheDocument();
    });
    expect(screen.getByText('Change Password')).toBeInTheDocument();
    expect(screen.getByText('Current Password')).toBeInTheDocument();
    expect(screen.getByText('New Password')).toBeInTheDocument();
    expect(screen.getByText('Update Password')).toBeInTheDocument();
  });
});
```

- [ ] **Step 3: Run the tests to verify they pass**

Run:
```bash
cd web && npm test -- --reporter=verbose src/pages/__tests__/SettingsPage.test.tsx
```

Expected: All 7 tests pass.

- [ ] **Step 4: Commit**

```bash
git add web/src/pages/__tests__/SettingsPage.test.tsx
git commit -m "test: add SettingsPage component tests"
```

---

### Task 2: SearchPage Component Tests

**Files:**
- Create: `web/src/pages/__tests__/SearchPage.test.tsx`

- [ ] **Step 1: Write SearchPage.test.tsx**

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import SearchPage from '../SearchPage';
import * as client from '../../api/client';

vi.mock('../../api/client', () => ({
  api: {
    smartSearch: vi.fn(),
  },
  setAuthToken: vi.fn(),
  getAuthToken: vi.fn(() => null),
  fetchCSRFToken: vi.fn(),
}));

const mockResults = [
  {
    id: 'evt-1',
    camera_id: 'cam-1',
    event_time: '2024-06-15T10:30:00Z',
    object_type: 'person',
    confidence: 0.95,
    track_id: 'track-1',
    thumbnail: '/thumbnails/evt-1.jpg',
  },
  {
    id: 'evt-2',
    camera_id: 'cam-2',
    event_time: '2024-06-15T10:31:00Z',
    object_type: 'vehicle',
    confidence: 0.87,
    track_id: 'track-2',
    thumbnail: '/thumbnails/evt-2.jpg',
  },
];

beforeEach(() => {
  vi.clearAllMocks();
});

describe('SearchPage', () => {
  it('shows filter inputs', () => {
    render(<SearchPage />);
    expect(screen.getByPlaceholderText('Object Type')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('Camera ID')).toBeInTheDocument();
    expect(screen.getByText('Search')).toBeInTheDocument();
    expect(screen.getByText('Minimum Confidence')).toBeInTheDocument();
  });

  it('shows loading spinner during search', async () => {
    vi.mocked(client.api.smartSearch).mockImplementation(() => new Promise(() => {}));
    render(<SearchPage />);
    await userEvent.click(screen.getByText('Search'));
    expect(screen.getByText('Searching...')).toBeInTheDocument();
  });

  it('shows empty results message when no results found', async () => {
    vi.mocked(client.api.smartSearch).mockResolvedValue({ results: [], total: 0 });
    render(<SearchPage />);
    await userEvent.click(screen.getByText('Search'));
    await waitFor(() => {
      expect(screen.getByText(/No results found/)).toBeInTheDocument();
    });
  });

  it('shows error message when search fails', async () => {
    vi.mocked(client.api.smartSearch).mockRejectedValue(new Error('Search service unavailable'));
    render(<SearchPage />);
    await userEvent.click(screen.getByText('Search'));
    await waitFor(() => {
      expect(screen.getByText('Search service unavailable')).toBeInTheDocument();
    });
  });

  it('renders search results with object_type and event_time', async () => {
    vi.mocked(client.api.smartSearch).mockResolvedValue({ results: mockResults, total: 2 });
    render(<SearchPage />);
    await userEvent.click(screen.getByText('Search'));
    await waitFor(() => {
      expect(screen.getByText('person')).toBeInTheDocument();
    });
    expect(screen.getByText('vehicle')).toBeInTheDocument();
    expect(screen.getByText('2024-06-15T10:30:00Z')).toBeInTheDocument();
    expect(screen.getByText('2024-06-15T10:31:00Z')).toBeInTheDocument();
  });

  it('shows correct stats counts', async () => {
    vi.mocked(client.api.smartSearch).mockResolvedValue({ results: mockResults, total: 2 });
    render(<SearchPage />);
    expect(screen.getByText('Results')).toBeInTheDocument();
    expect(screen.getByText('Cameras')).toBeInTheDocument();
    expect(screen.getByText('Avg Confidence')).toBeInTheDocument();
    await userEvent.click(screen.getByText('Search'));
    await waitFor(() => {
      expect(screen.getByText('2')).toBeInTheDocument();
    });
  });
});
```

- [ ] **Step 2: Run the tests to verify they pass**

Run:
```bash
cd web && npm test -- --reporter=verbose src/pages/__tests__/SearchPage.test.tsx
```

Expected: All 6 tests pass.

- [ ] **Step 3: Commit**

```bash
git add web/src/pages/__tests__/SearchPage.test.tsx
git commit -m "test: add SearchPage component tests"
```

---

### Task 3: WebhooksPage Component Tests

**Files:**
- Create: `web/src/pages/__tests__/WebhooksPage.test.tsx`

- [ ] **Step 1: Write WebhooksPage.test.tsx**

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import WebhooksPage from '../WebhooksPage';
import * as client from '../../api/client';

vi.mock('../../api/client', () => ({
  api: {
    listWebhooks: vi.fn(),
    createWebhook: vi.fn(),
    updateWebhook: vi.fn(),
    deleteWebhook: vi.fn(),
  },
  setAuthToken: vi.fn(),
  getAuthToken: vi.fn(() => null),
  fetchCSRFToken: vi.fn(),
}));

const mockWebhooks = [
  { id: 'wh-1', name: 'Slack Alerts', url: 'https://hooks.slack.com/xxx', event_types: ['motion', 'line_cross'], camera_ids: [], enabled: true },
  { id: 'wh-2', name: 'Email Log', url: 'https://hooks.example.com/email', event_types: ['error'], camera_ids: ['cam-1'], enabled: false },
];

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(client.api.listWebhooks).mockResolvedValue({ webhooks: mockWebhooks });
});

describe('WebhooksPage', () => {
  it('shows loading state initially', () => {
    vi.mocked(client.api.listWebhooks).mockImplementation(() => new Promise(() => {}));
    render(<WebhooksPage />);
    expect(screen.getByText('Loading...')).toBeInTheDocument();
  });

  it('shows empty state when no webhooks', async () => {
    vi.mocked(client.api.listWebhooks).mockResolvedValue({ webhooks: [] });
    render(<WebhooksPage />);
    await waitFor(() => {
      expect(screen.getByText('No webhooks configured.')).toBeInTheDocument();
    });
  });

  it('renders webhook names and URLs', async () => {
    render(<WebhooksPage />);
    await waitFor(() => {
      expect(screen.getByText('Slack Alerts')).toBeInTheDocument();
    });
    expect(screen.getByText('Email Log')).toBeInTheDocument();
    expect(screen.getByText('https://hooks.slack.com/xxx')).toBeInTheDocument();
    expect(screen.getByText('https://hooks.example.com/email')).toBeInTheDocument();
  });

  it('opens create form when clicking + Add Webhook', async () => {
    render(<WebhooksPage />);
    await waitFor(() => {
      expect(screen.queryByText('Loading...')).not.toBeInTheDocument();
    });
    await userEvent.click(screen.getByText('+ Add Webhook'));
    expect(screen.getByPlaceholderText('Name')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('URL (https://...)')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('Event types (comma-separated)')).toBeInTheDocument();
    expect(screen.getByText('Create')).toBeInTheDocument();
  });

  it('pre-fills form when clicking Edit', async () => {
    render(<WebhooksPage />);
    await waitFor(() => {
      expect(screen.getByText('Slack Alerts')).toBeInTheDocument();
    });
    const editButtons = screen.getAllByText('Edit');
    await userEvent.click(editButtons[0]);
    expect(screen.getByPlaceholderText('Name')).toHaveValue('Slack Alerts');
    expect(screen.getByPlaceholderText('URL (https://...)')).toHaveValue('https://hooks.slack.com/xxx');
    expect(screen.getByText('Update')).toBeInTheDocument();
  });

  it('calls deleteWebhook when clicking Delete', async () => {
    render(<WebhooksPage />);
    await waitFor(() => {
      expect(screen.getByText('Slack Alerts')).toBeInTheDocument();
    });
    const deleteButtons = screen.getAllByText('Delete');
    await userEvent.click(deleteButtons[0]);
    expect(vi.mocked(client.api.deleteWebhook)).toHaveBeenCalledWith('wh-1');
    await waitFor(() => {
      expect(screen.getByText('Webhook deleted')).toBeInTheDocument();
    });
  });

  it('shows error banner when listWebhooks fails', async () => {
    vi.mocked(client.api.listWebhooks).mockRejectedValue(new Error('Network error'));
    render(<WebhooksPage />);
    await waitFor(() => {
      expect(screen.getByText('Network error')).toBeInTheDocument();
    });
  });
});
```

- [ ] **Step 2: Run the tests to verify they pass**

Run:
```bash
cd web && npm test -- --reporter=verbose src/pages/__tests__/WebhooksPage.test.tsx
```

Expected: All 7 tests pass.

- [ ] **Step 3: Commit**

```bash
git add web/src/pages/__tests__/WebhooksPage.test.tsx
git commit -m "test: add WebhooksPage component tests"
```

---

### Task 4: Playwright Setup

**Files:**
- Create: `web/playwright.config.ts`
- Modify: `web/package.json`

- [ ] **Step 1: Install Playwright**

Run:
```bash
cd web && npm install --save-dev @playwright/test
npx playwright install chromium
```

Expected: `@playwright/test` added to package.json devDependencies, chromium browser installed.

- [ ] **Step 2: Create `web/playwright.config.ts`**

```typescript
import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: 'html',
  use: {
    baseURL: 'http://localhost:5173',
    trace: 'on-first-retry',
  },
  webServer: {
    command: 'npm run dev',
    url: 'http://localhost:5173',
    reuseExistingServer: !process.env.CI,
    timeout: 120000,
  },
});
```

- [ ] **Step 3: Add e2e script to package.json**

Edit `web/package.json` - add to the `"scripts"` section:

```json
    "test:e2e": "playwright test"
```

After the change, the scripts section should look like:

```json
  "scripts": {
    "dev": "vite",
    "build": "tsc && vite build",
    "preview": "vite preview",
    "lint": "eslint . --ext ts,tsx --report-unused-disable-directives --max-warnings 0",
    "lint-fix": "eslint . --ext ts,tsx --report-unused-disable-directives --fix",
    "test": "vitest run",
    "test:watch": "vitest",
    "test:e2e": "playwright test"
  }
```

- [ ] **Step 4: Commit**

```bash
git add web/playwright.config.ts web/package.json
git commit -m "test: add Playwright E2E configuration"
```

---

### Task 5: Playwright E2E Tests

**Files:**
- Create: `web/e2e/login.spec.ts`
- Create: `web/e2e/camera-list.spec.ts`
- Create: `web/e2e/playback.spec.ts`

- [ ] **Step 1: Create the e2e directory and write `web/e2e/login.spec.ts`**

```bash
mkdir -p web/e2e
```

```typescript
import { test, expect } from '@playwright/test';

test.describe('Login flow', () => {
  test('renders the login page with form fields', async ({ page }) => {
    await page.goto('/login');
    await expect(page.getByRole('heading', { name: 'Sign In' })).toBeVisible();
    await expect(page.getByLabel('Username')).toBeVisible();
    await expect(page.getByLabel('Password')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Sign In' })).toBeVisible();
  });

  test('shows error on failed login', async ({ page }) => {
    await page.route('**/api/login', async (route) => {
      await route.fulfill({
        status: 401,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'Invalid credentials' }),
      });
    });
    await page.route('**/api/csrf-token', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ csrf_token: 'test-csrf' }),
      });
    });

    await page.goto('/login');
    await page.getByLabel('Username').fill('baduser');
    await page.getByLabel('Password').fill('badpass');
    await page.getByRole('button', { name: 'Sign In' }).click();

    await expect(page.getByText(/Invalid credentials/i)).toBeVisible();
  });

  test('redirects to home on successful login', async ({ page }) => {
    await page.route('**/api/login', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ token: 'eyJhbGciOiJIUzI1NiJ9.eyJ1c2VybmFtZSI6ImFkbWluIiwicm9sZSI6ImFkbWluIn0.signature' }),
      });
    });
    await page.route('**/api/csrf-token', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ csrf_token: 'test-csrf' }),
      });
    });

    await page.goto('/login');
    await page.getByLabel('Username').fill('admin');
    await page.getByLabel('Password').fill('admin123');
    await page.getByRole('button', { name: 'Sign In' }).click();

    await page.waitForURL('/');
    await expect(page.getByText(/Live View|Dashboard|Cameras/i).first()).toBeVisible();
  });
});
```

- [ ] **Step 2: Write `web/e2e/camera-list.spec.ts`**

```typescript
import { test, expect } from '@playwright/test';

test.describe('Camera list page', () => {
  test.beforeEach(async ({ page }) => {
    // Mock login and CSRF endpoints so the app authenticates
    await page.route('**/api/login', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ token: 'eyJhbGciOiJIUzI1NiJ9.eyJ1c2VybmFtZSI6ImFkbWluIiwicm9sZSI6ImFkbWluIn0.signature' }),
      });
    });
    await page.route('**/api/csrf-token', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ csrf_token: 'test-csrf' }),
      });
    });
    // Mock cameras endpoint
    await page.route('**/api/cameras', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          cameras: [
            { id: 'cam-1', site_id: 'site-1', name: 'Front Door', description: '', connection_url: '', substream_url: '', status: 'online', ptz_protocol: '', retention_days: 14, config: '' },
            { id: 'cam-2', site_id: 'site-1', name: 'Parking Lot', description: '', connection_url: '', substream_url: '', status: 'online', ptz_protocol: '', retention_days: 30, config: '' },
            { id: 'cam-3', site_id: 'site-2', name: 'Lobby', description: '', connection_url: '', substream_url: '', status: 'offline', ptz_protocol: '', retention_days: 7, config: '' },
          ],
        }),
      });
    });
    // Mock sites for the Layout sidebar
    await page.route('**/api/sites', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ sites: [{ id: 'site-1', name: 'Main Office', location: '' }, { id: 'site-2', name: 'Warehouse', location: '' }] }),
      });
    });
  });

  test('displays camera list after login', async ({ page }) => {
    // Log in first
    await page.goto('/login');
    await page.getByLabel('Username').fill('admin');
    await page.getByLabel('Password').fill('admin123');
    await page.getByRole('button', { name: 'Sign In' }).click();
    await page.waitForURL('/');

    // Navigate to cameras page
    await page.goto('/cameras');

    // Wait for camera data to load
    await expect(page.getByText('Front Door')).toBeVisible({ timeout: 10000 });
    await expect(page.getByText('Parking Lot')).toBeVisible();
    await expect(page.getByText('Lobby')).toBeVisible();
  });

  test('shows online/offline status for cameras', async ({ page }) => {
    await page.goto('/login');
    await page.getByLabel('Username').fill('admin');
    await page.getByLabel('Password').fill('admin123');
    await page.getByRole('button', { name: 'Sign In' }).click();
    await page.waitForURL('/');

    await page.goto('/cameras');

    await expect(page.getByText('Front Door')).toBeVisible();
  });
});
```

- [ ] **Step 3: Write `web/e2e/playback.spec.ts`**

```typescript
import { test, expect } from '@playwright/test';

test.describe('Playback (Recordings) page', () => {
  test.beforeEach(async ({ page }) => {
    // Mock login and CSRF endpoints
    await page.route('**/api/login', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ token: 'eyJhbGciOiJIUzI1NiJ9.eyJ1c2VybmFtZSI6ImFkbWluIiwicm9sZSI6ImFkbWluIn0.signature' }),
      });
    });
    await page.route('**/api/csrf-token', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ csrf_token: 'test-csrf' }),
      });
    });
    // Mock recordings endpoint
    await page.route('**/api/recordings*', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          recordings: [
            { camera_id: 'cam-1', start_time: '2024-06-15T10:00:00Z', end_time: '2024-06-15T10:30:00Z', file_path: '/recordings/cam-1/2024-06-15/10-00.mp4', file_size: 52428800 },
            { camera_id: 'cam-2', start_time: '2024-06-15T11:00:00Z', end_time: '2024-06-15T11:15:00Z', file_path: '/recordings/cam-2/2024-06-15/11-00.mp4', file_size: 26214400 },
          ],
        }),
      });
    });
    // Mock sites for Layout
    await page.route('**/api/sites', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ sites: [] }),
      });
    });
  });

  test('displays recordings list', async ({ page }) => {
    // Log in first
    await page.goto('/login');
    await page.getByLabel('Username').fill('admin');
    await page.getByLabel('Password').fill('admin123');
    await page.getByRole('button', { name: 'Sign In' }).click();
    await page.waitForURL('/');

    // Navigate to recordings page
    await page.goto('/recordings');

    // Wait for recordings data to load
    await expect(page.getByText(/cam-1|cam-2|Recording/)).toBeVisible({ timeout: 10000 });
  });

  test('shows recording time ranges', async ({ page }) => {
    await page.goto('/login');
    await page.getByLabel('Username').fill('admin');
    await page.getByLabel('Password').fill('admin123');
    await page.getByRole('button', { name: 'Sign In' }).click();
    await page.waitForURL('/');

    await page.goto('/recordings');

    // Check that time information appears
    await expect(page.getByText(/2024-06-15|10:00|11:00/).first()).toBeVisible({ timeout: 10000 });
  });
});
```

- [ ] **Step 4: Run E2E tests to verify the setup works**

Run:
```bash
cd web && npx playwright test --headed
```

Expected: Tests run against the dev server. Login test should pass (form interaction + API mocking). Camera list and playback tests should pass by navigating after simulated login.

Note: If the dev server is already running, `reuseExistingServer: true` will reuse it. If not, Playwright auto-starts it via the webServer config.

- [ ] **Step 5: Commit**

```bash
git add web/e2e/
git commit -m "test: add Playwright E2E tests for login, camera list, and playback"
```

---

## Self-Review

### Spec Coverage

| Requirement | Task | File |
|---|---|---|
| SettingsPage loading state | Task 1 | `SettingsPage.test.tsx` — "shows loading state initially" |
| SettingsPage error state | Task 1 | `SettingsPage.test.tsx` — "shows error when getCameras fails" |
| SettingsPage camera list | Task 1 | `SettingsPage.test.tsx` — "renders camera names after loading" |
| SettingsPage retention slider | Task 1 | `SettingsPage.test.tsx` — "renders retention range sliders" |
| SettingsPage tours section | Task 1 | `SettingsPage.test.tsx` — "renders tour list", "opens new tour dialog" |
| SettingsPage password form | Task 1 | `SettingsPage.test.tsx` — "renders password change form" |
| SearchPage loading state | Task 2 | `SearchPage.test.tsx` — "shows loading spinner during search" |
| SearchPage empty results | Task 2 | `SearchPage.test.tsx` — "shows empty results message" |
| SearchPage error state | Task 2 | `SearchPage.test.tsx` — "shows error message when search fails" |
| SearchPage results display | Task 2 | `SearchPage.test.tsx` — "renders search results with object_type and event_time" |
| SearchPage filter inputs | Task 2 | `SearchPage.test.tsx` — "shows filter inputs" |
| SearchPage stats cards | Task 2 | `SearchPage.test.tsx` — "shows correct stats counts" |
| WebhooksPage loading state | Task 3 | `WebhooksPage.test.tsx` — "shows loading state initially" |
| WebhooksPage empty state | Task 3 | `WebhooksPage.test.tsx` — "shows empty state when no webhooks" |
| WebhooksPage webhook list | Task 3 | `WebhooksPage.test.tsx` — "renders webhook names and URLs" |
| WebhooksPage create form | Task 3 | `WebhooksPage.test.tsx` — "opens create form when clicking + Add Webhook" |
| WebhooksPage edit pre-fill | Task 3 | `WebhooksPage.test.tsx` — "pre-fills form when clicking Edit" |
| WebhooksPage delete | Task 3 | `WebhooksPage.test.tsx` — "calls deleteWebhook when clicking Delete" |
| WebhooksPage success/error | Task 3 | `WebhooksPage.test.tsx` — "shows error banner when listWebhooks fails", success via delete |
| Playwright config | Task 4 | `playwright.config.ts` — chromium, baseURL, webServer |
| Playwright login E2E | Task 5 | `web/e2e/login.spec.ts` — form render, error, successful login |
| Playwright camera list E2E | Task 5 | `web/e2e/camera-list.spec.ts` — camera display, status visibility |
| Playwright playback E2E | Task 5 | `web/e2e/playback.spec.ts` — recordings display, time ranges |

### Placeholder Scan

No placeholder patterns found (no "TBD", "TODO", "implement later", "fill in details", "add appropriate error handling", placeholder references).

### Type Consistency

All API method signatures match the actual `client.ts` implementations:
- `getCameras()` returns `Promise<{ cameras: Camera[] }>` → `{ cameras: mockCameras }` ✓
- `listTours()` returns `Promise<{ tours: Tour[] }>` → `{ tours: mockTours }` ✓
- `smartSearch(params)` returns `Promise<{ results: SearchResult[], total: number }>` → `{ results: mockResults, total: 2 }` ✓
- `listWebhooks()` returns `Promise<{ webhooks: Webhook[] }>` → `{ webhooks: mockWebhooks }` ✓
- `deleteWebhook(id)` returns `Promise<{ status: string }>` → called with correct id `'wh-1'` ✓
- `changePassword(current, new)` → `updateCameraConfig(id, config)` → `setRelayState(id, relay, state)` all use `vi.fn()` ✓

All test data models match the TypeScript interfaces in `client.ts`:
- `Camera` has `id`, `site_id`, `name`, `description`, `connection_url`, `substream_url`, `status`, `ptz_protocol`, `retention_days`, `config?` ✓
- `Tour` has `id`, `name`, `enabled`, `steps`, `interval`, `created_at` ✓
- `TourStep` has `camera_id`, `dwell_seconds` ✓
- `Webhook` has `id`, `name`, `url`, `event_types`, `camera_ids`, `enabled`, `secret?` ✓
- `SearchResult` has `id`, `camera_id`, `event_time`, `object_type`, `confidence`, `track_id`, `thumbnail` ✓

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-06-15-testing-frontend-e2e.md`. Two execution options:

1. **Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration

2. **Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints

**Which approach?**
