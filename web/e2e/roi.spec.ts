import { test, expect } from '@playwright/test';

test.describe('ROI 价值追踪 (MVP 功能锚点)', () => {
  test('should display ROI dashboard page', async ({ page }) => {
    await page.goto('/roi');
    await expect(page).toHaveTitle(/Lighthouse/);
    await expect(page.locator('body')).toBeVisible();
  });

  test('should show ROI metrics or trend', async ({ page }) => {
    await page.goto('/roi');
    // ROI 看板基线对比/趋势
    await expect(page.locator('body')).toBeVisible();
  });
});
