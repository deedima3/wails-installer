import { test, expect } from '@playwright/test';

test('choose a directory, enable startup, install and launch', async ({ page }) => {
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Your space. Your setup.' })).toBeVisible();
  const directory = page.getByLabel('Installation folder');
  await expect(directory).toHaveValue('/home/you/.local/share/lumen');
  await page.getByRole('button', { name: 'Browse' }).click();
  await expect(directory).toHaveValue('/home/you/Applications/Lumen');
  await directory.fill('/home/you/My Lumen');
  await expect(page.getByRole('button', { name: 'Install Lumen' })).toBeEnabled();
  await page.getByRole('checkbox', { name: /Start with your day/ }).check();
  await page.getByRole('button', { name: 'Install Lumen' }).click();
  await expect(page.getByRole('progressbar')).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Make yourself at home.' })).toBeVisible();
  await expect(page.getByText('/home/you/My Lumen', { exact: true })).toBeVisible();
  await expect(page.getByText('Launch at login is enabled.')).toBeVisible();
  await page.getByRole('button', { name: 'Open Lumen', exact: true }).click();
  await expect(page.getByText('Application launched. You can close this installer.')).toBeVisible();
  expect(errors).toEqual([]);
});

test('detect an existing version, upgrade, and uninstall with confirmation', async ({ page }) => {
  await page.goto('/');
  await page.getByLabel('Scenario', { exact: true }).selectOption('managed');
  await expect(page.getByText('Installed v1.0.0. This installer contains v2.0.0.')).toBeVisible();
  await page.getByRole('button', { name: 'Upgrade Lumen' }).click();
  await expect(page.getByRole('heading', { name: 'Make yourself at home.' })).toBeVisible();
  await page.getByRole('button', { name: 'Manage installation' }).click();
  await expect(page.getByText('Installed v2.0.0. This installer contains v2.0.0.')).toBeVisible();
  await page.getByRole('radio', { name: 'Uninstall application' }).check();
  const remove = page.getByRole('button', { name: 'Uninstall Lumen' });
  await expect(remove).toBeDisabled();
  await page.getByRole('checkbox', { name: /Remove Lumen/ }).check();
  await remove.click();
  await expect(page.getByRole('heading', { name: 'Uninstall complete.' })).toBeVisible();
  await page.getByText('View kept files').click();
  await expect(page.getByText('notes/private.txt', { exact: true })).toBeVisible();
});

test('cancel cleans up the flow and permits a retry', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: 'Install Lumen' }).click();
  await page.getByRole('button', { name: 'Cancel installation', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Your space. Your setup.' })).toBeVisible();
  await expect(page.getByText('Installation cancelled. Choose your settings to try again.')).toBeVisible();
  await page.getByRole('button', { name: 'Install Lumen' }).click();
  await expect(page.getByRole('heading', { name: 'Make yourself at home.' })).toBeVisible();
  await expect(page.getByText('Launch at login is off.')).toBeVisible();
});

test('cancel uninstall keeps the existing application available for maintenance', async ({ page }) => {
  await page.goto('/');
  await page.getByLabel('Scenario', { exact: true }).selectOption('managed');
  await page.getByRole('radio', { name: 'Uninstall application' }).check();
  await page.getByRole('checkbox', { name: /Remove Lumen/ }).check();
  await page.getByRole('button', { name: 'Uninstall Lumen' }).click();
  await page.getByRole('button', { name: 'Cancel uninstall', exact: true }).click();
  await expect(page.getByText('Uninstall cancelled. The installation was kept.')).toBeVisible();
  await page.getByRole('radio', { name: 'Upgrade application' }).check();
  await expect(page.getByRole('button', { name: 'Upgrade Lumen' })).toBeEnabled();
});

test('mobile layout fits and themes update without resetting choices', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/');
  await expect(page.getByLabel('Installation folder')).toBeVisible();
  await page.getByLabel('Installation folder').fill('/home/you/Custom');
  await page.getByLabel('Theme', { exact: true }).selectOption('clay');
  await expect(page.getByLabel('Installation folder')).toHaveValue('/home/you/Custom');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.getByRole('button', { name: 'Reset' }).click();
  await expect(page.getByLabel('Installation folder')).toHaveValue('/home/you/.local/share/lumen');
});
