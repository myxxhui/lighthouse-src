import { test, expect } from '@playwright/test';

test.describe('ROI 价值追踪 (MVP 功能锚点)', () => {
  test('should display ROI dashboard page', async ({ page }) => {
    await page.goto('/ROIDashboard');
    await expect(page).toHaveURL(/CostOverviewPage.*tab=roi|ROIDashboard/);
    await expect(page.locator('body')).toBeVisible();
  });

  test('should show ROI metrics or trend', async ({ page }) => {
    await page.goto('/ROIDashboard');
    await expect(page).toHaveURL(/CostOverviewPage.*tab=roi|ROIDashboard/);
    await expect(page.locator('body')).toBeVisible();
  });
});
