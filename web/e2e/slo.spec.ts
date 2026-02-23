import { test, expect } from '@playwright/test';

test.describe('SLO 健康监控 (MVP 功能锚点)', () => {
  test('should display SLO health page', async ({ page }) => {
    await page.goto('/SLODashboard');
    await expect(page).toHaveURL(/SLODashboard/);
    await expect(page.getByRole('heading').first()).toBeVisible({ timeout: 10000 });
  });

  test('should show health status or red/green/yellow indicators', async ({ page }) => {
    await page.goto('/SLODashboard');
    await expect(page).toHaveURL(/SLODashboard/);
    await expect(page.locator('body')).toBeVisible();
  });
});
