import { test, expect } from '@playwright/test';

// 应用路由：/ 重定向到 /CostOverviewPage，下钻为 /DrilldownPage?type=namespace&id=xxx
const COST_OVERVIEW_URL = '/CostOverviewPage';
const DRILLDOWN_PATH = '/DrilldownPage';

test('should display cost overview page', async ({ page }) => {
  await page.goto('/');
  await expect(page).toHaveURL(/\/(CostOverviewPage)?(\?|$)/);

  await expect(page.getByRole('heading', { name: '全域成本透视' })).toBeVisible({ timeout: 15000 });
  await expect(page.getByText('总账单成本')).toBeVisible();
  await expect(page.getByText('可优化空间')).toBeVisible();
  await expect(page.getByText('全局效率分')).toBeVisible();
  await expect(page.getByRole('table')).toBeVisible();
  await expect(page.getByText('使用Mock数据')).toBeVisible();
});

test('should navigate to drilldown page', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByRole('table')).toBeVisible({ timeout: 15000 });

  const firstRow = page.locator('table tbody tr').first();
  await firstRow.click();

  await expect(page).toHaveURL(new RegExp(DRILLDOWN_PATH));
  await expect(page.getByRole('heading', { level: 2 }).or(page.getByText('命名空间', { exact: false }))).toBeVisible({ timeout: 10000 });
});

test('drilldown entry shows resource dimension selector and namespaces', async ({ page }) => {
  await page.goto(DRILLDOWN_PATH);
  await expect(page.getByText('选择命名空间开始下钻').or(page.getByText('资源维度'))).toBeVisible({ timeout: 15000 });
  await expect(page.getByText('资源维度').or(page.getByText('算力'))).toBeVisible();
  await expect(page.getByText('算力').first()).toBeVisible();
  await expect(page.getByText('存储').first()).toBeVisible();
  await expect(page.getByText('网络').first()).toBeVisible();

  const nsCards = page.getByText('production').or(page.getByText('staging')).or(page.getByText('development'));
  await expect(nsCards.first()).toBeVisible({ timeout: 10000 });
});

test('storage dimension drilldown shows storage_class or pvc', async ({ page }) => {
  await page.goto(DRILLDOWN_PATH);
  await expect(page.getByText('选择命名空间开始下钻').or(page.getByText('资源维度'))).toBeVisible({ timeout: 15000 });

  await page.getByText('存储').first().click();
  await page.getByText('production').first().click();

  await expect(page).toHaveURL(/dimension=storage/);
  await expect(page).toHaveURL(/type=namespace/);
  await expect(page.getByText('命名空间', { exact: false })).toBeVisible({ timeout: 10000 });
  await expect(
    page.getByText('存储类').or(page.getByText('PVC')).or(page.getByText('子资源')).or(page.getByText('成本'))
  ).toBeVisible({ timeout: 5000 });
});

test('drilldown detail shows cost breakdown when present', async ({ page }) => {
  await page.goto(`${DRILLDOWN_PATH}?dimension=compute&type=namespace&id=production`);

  await expect(page).toHaveURL(/dimension=compute/);
  await expect(page.getByText('成本构成')).toBeVisible({ timeout: 15000 });
  await expect(page.getByText('算力(CPU)', { exact: false })).toBeVisible();
  await expect(page.getByText('存储', { exact: false })).toBeVisible();
  await expect(page.getByText('网络', { exact: false })).toBeVisible();
});