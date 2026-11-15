// SPDX-License-Identifier: GPL-3.0-or-later
import { AppPage, expect, test as base } from '@grafana/plugin-e2e';
import type { APIRequestContext, Page } from '@playwright/test';
import pluginJson from '../src/plugin.json';
import { readReported } from './reported';
import { BUGSINK_API, EVENT_MARKER, GLOBAL_READ } from './seeded';

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

export type Issue = { id: string; friendlyId: string };

const bearer = { Authorization: `Bearer ${GLOBAL_READ}` };

export async function thisRunsIssue(bugsink: APIRequestContext): Promise<Issue> {
  const { eventId } = readReported<{ eventId: string }>('trace');
  let issue: Issue | undefined;
  await expect(async () => {
    const issues = await bugsink.get(
      `${BUGSINK_API}/issues/?project=1&state=all&q=${EVENT_MARKER}&sort=last_seen&order=desc`,
      { headers: bearer }
    );
    expect(issues.ok()).toBe(true);
    const found: Issue[] = [];
    for (const candidate of (await issues.json()).results) {
      const events = await bugsink.get(`${BUGSINK_API}/events/?issue=${candidate.id}&order=desc`, { headers: bearer });
      expect(events.ok()).toBe(true);
      if ((await events.json()).results[0]?.event_id.replaceAll('-', '') === eventId) {
        found.push({ id: candidate.id, friendlyId: candidate.friendly_id });
      }
    }
    expect(found, "an issue whose newest event is this run's").toHaveLength(1);
    issue = found[0];
  }).toPass();
  return issue!;
}
