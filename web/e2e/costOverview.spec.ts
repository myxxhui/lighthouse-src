import { test, expect } from '@playwright/test';

test('should display cost overview page', async ({ page }) => {
  await page.goto('/');
  
  // Check page title
  await expect(page).toHaveTitle(/Lighthouse/);
  
  // Check main heading
  await expect(page.getByRole('heading', { name: '全域成本透视' })).toBeVisible();
  
  // Check global metrics cards
  await expect(page.getByText('总账单成本')).toBeVisible();
  await expect(page.getByText('可优化空间')).toBeVisible();
  await expect(page.getByText('全局效率分')).toBeVisible();
  
  // Check namespace table
  await expect(page.getByRole('table')).toBeVisible();
  
  // Check mock data switch
  await expect(page.getByText('使用Mock数据')).toBeVisible();
});

test('should navigate to drilldown page', async ({ page }) => {
  await page.goto('/');
  
  // Click on a namespace row
  const firstRow = page.locator('table tbody tr').first();
  await firstRow.click();
  
  // Should navigate to drilldown page
  await expect(page).toHaveURL(/drilldown/);
  
  // Check drilldown page content
  await expect(page.getByRole('heading', { level: 2 })).toBeVisible();
});