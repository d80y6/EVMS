import { test, expect } from '@playwright/test';

test.describe('Playback (Recordings) page', () => {
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
    await page.route('**/api/sites', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ sites: [] }),
      });
    });
  });

  test('displays recordings list', async ({ page }) => {
    await page.goto('/login');
    await page.getByLabel('Username').fill('admin');
    await page.getByLabel('Password').fill('admin123');
    await page.getByRole('button', { name: 'Sign In' }).click();
    await page.waitForURL('/');

    await page.goto('/recordings');

    await expect(page.getByText(/cam-1|cam-2|Recording/)).toBeVisible({ timeout: 10000 });
  });

  test('shows recording time ranges', async ({ page }) => {
    await page.goto('/login');
    await page.getByLabel('Username').fill('admin');
    await page.getByLabel('Password').fill('admin123');
    await page.getByRole('button', { name: 'Sign In' }).click();
    await page.waitForURL('/');

    await page.goto('/recordings');

    await expect(page.getByText(/2024-06-15|10:00|11:00/).first()).toBeVisible({ timeout: 10000 });
  });
});
