import { test, expect } from '@playwright/test';

/**
 * Phase3 性能验证：L0 聚合 <10ms、页面加载 <3s
 * 本用例带 performance 标记，用于 npm run test:performance
 */
test('performance: cost overview page loads within 3s', async ({ page }) => {
  const start = Date.now();
  await page.goto('/');
  const elapsed = Date.now() - start;
  expect(elapsed).toBeLessThan(3000);
});
