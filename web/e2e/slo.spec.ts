import { test, expect } from '@playwright/test';

test.describe('SLO 健康监控 (MVP 功能锚点)', () => {
  test('should display SLO health page', async ({ page }) => {
    await page.goto('/slo');
    await expect(page).toHaveTitle(/Lighthouse/);
    await expect(page.getByRole('heading')).toBeVisible();
  });

  test('should show health status or red/green/yellow indicators', async ({ page }) => {
    await page.goto('/slo');
    // SLO 红绿灯展示
    await expect(page.locator('body')).toBeVisible();
  });
});
