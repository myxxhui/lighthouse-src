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

test('drilldown entry shows resource dimension selector and namespaces', async ({ page }) => {
  await page.goto('/DrilldownPage');
  
  await expect(page.getByText('选择命名空间开始下钻')).toBeVisible();
  await expect(page.getByText('资源维度')).toBeVisible();
  await expect(page.getByText('算力').first()).toBeVisible();
  await expect(page.getByText('存储').first()).toBeVisible();
  await expect(page.getByText('网络').first()).toBeVisible();
  
  const nsCards = page.getByText('production').or(page.getByText('staging')).or(page.getByText('development'));
  await expect(nsCards.first()).toBeVisible();
});

test('storage dimension drilldown shows storage_class or pvc', async ({ page }) => {
  await page.goto('/DrilldownPage');
  await expect(page.getByText('选择命名空间开始下钻')).toBeVisible();
  
  await page.getByText('存储').first().click();
  await page.getByText('production').first().click();
  
  await expect(page).toHaveURL(/dimension=storage/);
  await expect(page).toHaveURL(/type=namespace/);
  await expect(page.getByText('命名空间', { exact: false })).toBeVisible();
  await expect(page.getByText('存储类').or(page.getByText('PVC')).or(page.getByText('子资源'))).toBeVisible();
});

test('drilldown detail shows cost breakdown when present', async ({ page }) => {
  await page.goto('/DrilldownPage?dimension=compute&type=namespace&id=production');
  
  await expect(page).toHaveURL(/dimension=compute/);
  await expect(page.getByText('成本构成')).toBeVisible();
  await expect(page.getByText('算力(CPU)', { exact: false })).toBeVisible();
  await expect(page.getByText('存储', { exact: false })).toBeVisible();
  await expect(page.getByText('网络', { exact: false })).toBeVisible();
});