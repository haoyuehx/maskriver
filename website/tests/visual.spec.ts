import { mkdir } from 'node:fs/promises';
import { test } from '@playwright/test';

test('capture visual evidence for review', async ({ page }, testInfo) => {
  const dir = `test-results/screens/${testInfo.project.name}`;
  await mkdir(dir, { recursive: true });
  await page.goto('/');
  await page.waitForLoadState('networkidle');
  await page.screenshot({ path: `${dir}/home.png`, fullPage: true });
  await page.locator('#demo').scrollIntoViewIfNeeded();
  await page.waitForTimeout(400);
  await page.screenshot({ path: `${dir}/demo.png` });
  await page.locator('#recovery').scrollIntoViewIfNeeded();
  await page.waitForTimeout(300);
  await page.getByRole('button', { name: '模拟中断' }).click();
  await page.screenshot({ path: `${dir}/recovery.png` });
});
