import { expect, test } from '@playwright/test';

test('renders the product landing page without console or page errors', async ({ page }) => {
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  page.on('console', (message) => {
    if (message.type() === 'error') errors.push(message.text());
  });
  await page.goto('/');
  await expect(page).toHaveTitle(/MaskRiver/);
  await expect(page.getByRole('heading', { level: 1 })).toContainText('让数据脱敏');
  await expect(page.getByRole('link', { name: '观看演示' })).toBeVisible();
  await expect(page.getByText('SQLite · MySQL 8.0.46')).toBeVisible();
  expect(errors).toEqual([]);
});

test('has no horizontal overflow at the configured viewport', async ({ page }) => {
  await page.goto('/');
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(overflow).toBeLessThanOrEqual(1);
});

test('navigation and anchor targets work', async ({ page, viewport }) => {
  await page.goto('/');
  if (viewport && viewport.width <= 820) {
    await page.getByRole('button', { name: '打开菜单' }).click();
    const menu = page.getByRole('navigation', { name: '移动端导航' });
    await menu.getByRole('link', { name: '演示视频' }).click();
    await expect(page.locator('#demo')).toBeInViewport();
    await page.getByRole('button', { name: '打开菜单' }).click();
    await page.getByRole('navigation', { name: '移动端导航' }).getByRole('link', { name: '核心功能' }).click();
  } else {
    const nav = page.getByRole('navigation', { name: '主导航' });
    await nav.getByRole('link', { name: '演示视频' }).click();
    await expect(page.locator('#demo')).toBeInViewport();
    await nav.getByRole('link', { name: '核心功能' }).click();
  }
  await expect(page.locator('#features')).toBeInViewport();
});

test('mobile menu opens, closes and navigates', async ({ page, viewport }) => {
  test.skip(!viewport || viewport.width > 820, 'mobile menu only applies below 820px');
  await page.goto('/');
  const toggle = page.getByRole('button', { name: '打开菜单' });
  await expect(toggle).toBeVisible();
  await toggle.click();
  await expect(page.getByRole('navigation', { name: '移动端导航' })).toBeVisible();
  await page.getByRole('navigation', { name: '移动端导航' }).getByRole('link', { name: '技术架构' }).click();
  await expect(page.locator('#architecture')).toBeInViewport();
  await expect(page.getByRole('navigation', { name: '移动端导航' })).toHaveCount(0);
});

test('workflow tabs switch the five steps with ArrowRight', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('tab', { name: 'Scan' }).focus();
  await page.keyboard.press('ArrowRight');
  await expect(page.getByRole('tab', { name: 'Preview' })).toHaveAttribute('aria-selected', 'true');
  for (const [label, text] of [
    ['Scan', '识别敏感字段'],
    ['Preview', '只读预览计划'],
    ['Approve', '签名审批绑定计划'],
    ['Apply', '按批次执行脱敏'],
    ['Validate', '验证结构与结果'],
  ]) {
    await page.getByRole('tab', { name: label }).click();
    await expect(page.getByRole('tabpanel')).toContainText(text);
  }
});

test('recovery demo follows interrupt, recover, validate and reset', async ({ page }) => {
  await page.goto('/');
  await page.locator('#recovery').scrollIntoViewIfNeeded();
  await expect(page.getByRole('button', { name: '恢复执行' })).toBeDisabled();
  await page.getByRole('button', { name: '模拟中断' }).click();
  await expect(page.getByText('TASK INTERRUPTED')).toBeVisible();
  await expect(page.getByRole('button', { name: '恢复执行' })).toBeEnabled();
  await page.getByRole('button', { name: '恢复执行' }).click();
  await expect(page.getByText('VALIDATION PASSED')).toBeVisible({ timeout: 5000 });
  await page.getByRole('button', { name: '重置' }).click();
  await expect(page.getByText('APPLY · 批次事务提交')).toBeVisible();
});

test('architecture scroll region is keyboard focusable and labelled', async ({ page }) => {
  await page.goto('/');
  const region = page.getByRole('region', { name: /架构图/ });
  await expect(region).toHaveAttribute('tabindex', '0');
  await region.focus();
  await expect(region).toBeFocused();
});

test('video poster loads the real MP4 and playback advances', async ({ page }) => {
  await page.goto('/');
  await page.locator('#demo').scrollIntoViewIfNeeded();
  const poster = page.getByRole('button', { name: '播放在线演示视频' });
  await expect(poster).toBeVisible();
  await poster.click();
  const video = page.locator('video');
  await expect(video).toBeVisible();
  await page.waitForFunction(() => {
    const node = document.querySelector('video');
    return Boolean(node && node.readyState >= 1 && Number.isFinite(node.duration));
  });
  const duration = await video.evaluate((node: HTMLVideoElement) => node.duration);
  expect(duration).toBeGreaterThan(255);
  expect(duration).toBeLessThan(270);
  if (await video.evaluate((node: HTMLVideoElement) => node.paused)) {
    const center = page.getByRole('button', { name: '播放视频' });
    if (await center.isVisible()) await center.click();
    else await page.getByRole('button', { name: '播放', exact: true }).click();
  }
  await page.waitForTimeout(1200);
  const currentTime = await video.evaluate((node: HTMLVideoElement) => node.currentTime);
  expect(currentTime).toBeGreaterThan(0.3);
  await page.getByRole('button', { name: '暂停', exact: true }).click();
  const captionButton = page.getByRole('button', { name: /中文字幕/ });
  await expect(captionButton).toBeVisible();
  const before = await captionButton.getAttribute('aria-pressed');
  await captionButton.click();
  await expect(captionButton).toHaveAttribute('aria-pressed', before === 'true' ? 'false' : 'true');
});

test('links to the real release and repository are present', async ({ page }) => {
  await page.goto('/');
  await expect(page.locator('a[href*="releases/download/v1.0.1/maskriver-v1.0.1-linux-amd64.tar.gz"]').first()).toBeVisible();
  await expect(page.locator('footer a[href="https://github.com/haoyuehx/maskriver"]')).toBeVisible();
});
