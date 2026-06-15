import { test, expect } from '@playwright/test';

test.describe('Camera list page', () => {
  test.beforeEach(async ({ page }) => {
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
    await page.route('**/api/sites', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ sites: [{ id: 'site-1', name: 'Main Office', location: '' }, { id: 'site-2', name: 'Warehouse', location: '' }] }),
      });
    });
  });

  test('displays camera list after login', async ({ page }) => {
    await page.goto('/login');
    await page.getByLabel('Username').fill('admin');
    await page.getByLabel('Password').fill('admin123');
    await page.getByRole('button', { name: 'Sign In' }).click();
    await page.waitForURL('/');

    await page.goto('/cameras');

    await expect(page.getByText('Front Door')).toBeVisible({ timeout: 10000 });
    await expect(page.getByText('Parking Lot')).toBeVisible();
    await expect(page.getByText('Lobby')).toBeVisible();
  });

  test('shows online/offline status for cameras', async ({ page }) => {
    await page.goto('/login');
    await page.getByLabel('Username').fill('admin');
    await page.getByLabel('Password').fill('admin123');
    await page.waitForURL('/');

    await page.goto('/cameras');

    await expect(page.getByText('Front Door')).toBeVisible();
  });
});
