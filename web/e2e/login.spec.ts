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
