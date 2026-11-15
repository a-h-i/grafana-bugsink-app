// SPDX-License-Identifier: GPL-3.0-or-later
import type { APIRequestContext, Locator, Page, Request, Route } from '@playwright/test';
import { expect, test } from './fixtures';
import { readReported } from './reported';
import { BUGSINK_API, EVENT_MARKER, GLOBAL_READ } from './seeded';

type Issue = { id: string; friendlyId: string };

const VOLUME_COLUMN = 7;
const SPARKLINE_LIMIT = 4;
const SPARKLINE_NAME = /^[1-9]\d* events in the last 24 hours$/;
const LOADING = 'Loading the last 24 hours';

const bearer = { Authorization: `Bearer ${GLOBAL_READ}` };
const listParams = (extra = '') => new URLSearchParams(`ds=bugsink&project=1&state=all${extra}&from=now-24h&to=now`);

async function thisRunsIssue(bugsink: APIRequestContext): Promise<Issue> {
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

function volumeCounts(body: {
  results: Record<string, { error?: string; frames?: Array<{ data: { values: number[][] } }> }>;
}) {
  const result = body.results.A;
  expect(result.error).toBeUndefined();
  return result.frames![0].data.values[1];
}

function isVolumeQuery(request: Request) {
  return request.url().includes('/api/ds/query') && (request.postData() ?? '').includes('"queryType":"volume"');
}

function rowOf(page: Page, issue: Issue) {
  return page.getByRole('row').filter({ has: page.getByRole('link', { name: issue.friendlyId, exact: true }) });
}

function volumeCells(page: Page) {
  return page.locator(`tbody tr td:nth-child(${VOLUME_COLUMN})`);
}

function cellLabel(cell: Locator) {
  return cell.locator('[aria-label]').first();
}

test.describe('event volume', () => {
  let issue: Issue;

  test.beforeEach(async ({ request }) => {
    issue = await thisRunsIssue(request);
  });

  test("the volume dashboard draws this run's events", async ({
    gotoDashboardPage,
    page,
    readProvisionedDashboard,
    selectors,
  }) => {
    const dashboard = await readProvisionedDashboard({ fileName: 'bugsink-volume.json' });
    const dashboardPage = await gotoDashboardPage({ uid: dashboard.uid });
    const panel = dashboardPage.getPanelByTitle('Events per hour in sample-python');
    await expect(panel.locator).toBeVisible();
    await expect(panel.getErrorIcon()).toHaveCount(0);

    await expect(async () => {
      const answered = page.waitForResponse(
        (response) =>
          response.request().url().includes('/api/ds/query') &&
          (response.request().postData() ?? '').includes('"refId":"A"')
      );
      await page.reload();
      const body = await (await answered).json();
      const counts = volumeCounts(body);
      expect(body.results.A.frames[0].schema.meta?.notices).toBeUndefined();
      expect([168, 169]).toContain(counts.length);
      expect(Math.max(...counts.slice(-2))).toBeGreaterThan(0);
    }).toPass();
    await expect(panel.getErrorIcon()).toHaveCount(0);
    await expect(panel.getByGrafanaSelector(selectors.components.Panels.Panel.status('warning'))).toHaveCount(0);
  });

  test('issue volume in Explore', async ({ explorePage, page, selectors }) => {
    await explorePage.getByGrafanaSelector(selectors.components.DataSourcePicker.inputV2).fill('Bugsink');
    await page.getByRole('option', { name: 'Bugsink', exact: true }).click();
    await explorePage.timeRange.set({ from: 'now-24h', to: 'now' });
    await page.getByRole('radio', { name: 'Event volume' }).click();
    const input = page.getByRole('textbox', { name: 'Issue' });
    await input.fill(issue.friendlyId);
    await expect(async () => {
      const answered = page.waitForResponse(
        (response) =>
          isVolumeQuery(response.request()) && (response.request().postData() ?? '').includes(issue.friendlyId)
      );
      await input.press('Enter');
      const counts = volumeCounts(await (await answered).json());
      expect(Math.max(...counts.slice(-2))).toBeGreaterThan(0);
    }).toPass();
  });

  test('sparklines show for the rows', async ({ gotoPage, page }) => {
    await gotoPage('', listParams(`&q=${EVENT_MARKER}`));
    await expect(async () => {
      await page.reload();
      const line = rowOf(page, issue).getByRole('img', { name: SPARKLINE_NAME });
      await expect(line).toBeVisible({ timeout: 5000 });
      await expect(line.locator('canvas')).toBeVisible();
    }).toPass();

    await gotoPage('', listParams());
    await expect(rowOf(page, issue)).toBeVisible();
    const cells = volumeCells(page);
    const count = await cells.count();
    expect(count).toBeGreaterThan(0);
    for (let index = 0; index < count; index++) {
      await expect(cellLabel(cells.nth(index))).toHaveAttribute('aria-label', /events in the last 24 hours$/);
    }
    await expect(page.getByRole('img', { name: /^Events per hour unavailable/ })).toHaveCount(0);
  });

  test('the list stays usable while sparklines load', async ({ gotoPage, page }) => {
    const held = new Map<Request, Route>();
    let maxHeld = 0;
    let holding = true;
    page.on('requestfailed', (request) => held.delete(request));
    await page.route(/\/api\/ds\/query/, async (route) => {
      if (holding && isVolumeQuery(route.request())) {
        held.set(route.request(), route);
        maxHeld = Math.max(maxHeld, held.size);
      } else {
        await route.continue();
      }
    });

    await gotoPage('', new URLSearchParams('ds=bugsink&state=all&from=now-7d&to=now'));
    const rows = page.locator('tbody tr');
    await expect(async () => {
      await page.reload();
      await expect(rows.first()).toBeVisible();
      expect(await rows.count(), 'needs more than 4 issues; run scripts/verify_stack.py first').toBeGreaterThan(
        SPARKLINE_LIMIT
      );
    }).toPass();
    await expect(async () => expect(held.size).toBe(SPARKLINE_LIMIT)).toPass();
    expect(maxHeld).toBeLessThanOrEqual(SPARKLINE_LIMIT);
    const count = await rows.count();
    await expect(page.getByRole('img', { name: LOADING })).toHaveCount(count);
    await expect(page.getByRole('img', { name: SPARKLINE_NAME })).toHaveCount(0);

    const search = page.getByRole('textbox', { name: 'Search' });
    await search.fill(EVENT_MARKER);
    await search.press('Enter');
    await expect(rowOf(page, issue)).toBeVisible();
    await expect(rows).not.toHaveCount(count);
    await expect(rowOf(page, issue).getByRole('img', { name: LOADING })).toBeVisible();
    await expect(async () => expect(held.size).toBeGreaterThan(0)).toPass();
    expect(maxHeld).toBeLessThanOrEqual(SPARKLINE_LIMIT);

    holding = false;
    for (const route of held.values()) {
      await route.continue().catch(() => {});
    }
    held.clear();
    await expect(cellLabel(volumeCells(page).first())).toHaveAttribute('aria-label', /events in the last 24 hours$/);
    const line = rowOf(page, issue).getByRole('img', { name: SPARKLINE_NAME });
    await expect(line).toBeVisible();
    await expect(line.locator('canvas')).toBeVisible();
    expect(maxHeld).toBeLessThanOrEqual(SPARKLINE_LIMIT);
  });
});
