// SPDX-License-Identifier: GPL-3.0-or-later
import { AppPage, expect, test as base } from '@grafana/plugin-e2e';
import type { Page } from '@playwright/test';
import pluginJson from '../src/plugin.json';

type AppTestFixture = {
  gotoPage: (path?: string, queryParams?: URLSearchParams) => Promise<AppPage>;
};

export const test = base.extend<AppTestFixture>({
  gotoPage: async ({ page, selectors, grafanaVersion, request }, use, testInfo) => {
    await use(async (path, queryParams) => {
      const appPage = new AppPage({ page, selectors, grafanaVersion, request, testInfo }, { pluginId: pluginJson.id });
      await appPage.goto({ path, queryParams });
      return appPage;
    });
  },
});

export { expect };

export function section(page: Page, heading: string | RegExp) {
  const title =
    typeof heading === 'string' ? new RegExp(`^${heading.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}$`) : heading;
  return page.locator('section').filter({ has: page.locator(':scope > :is(h2, h3)').filter({ hasText: title }) });
}

export async function openFirstIssue(page: Page, name: string | RegExp = /^SAMPLE-PYTHON-/) {
  const firstIssue = page
    .locator('tbody tr')
    .first()
    .getByRole('link', { name });
  await expect(async () => {
    await page.reload();
    await expect(firstIssue).toBeVisible({ timeout: 3000 });
  }).toPass();
  await firstIssue.click();
}

export function basicAuth(user: { user: string; password: string }): Record<string, string> {
  return { Authorization: `Basic ${Buffer.from(`${user.user}:${user.password}`).toString('base64')}` };
}
