// SPDX-License-Identifier: GPL-3.0-or-later
import type { Page } from '@playwright/test';
import { expect, openFirstIssue, section, test } from './fixtures';
import { readReported } from './reported';
import { BUGSINK_BROWSER_URL, EVENT_MARKER, JS_MARKER, MESSAGE_MARKER } from './seeded';

type Reported = { traceId: string; eventId: string };

const LOG_LINE = 'handling a request that fails with a handled error';

const listParams = (marker: string, project = 1, ds = 'bugsink') =>
  new URLSearchParams(`ds=${ds}&project=${project}&state=all&q=${marker}&from=now-24h&to=now`);

function dashed(hex: string) {
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

async function showsTraceId(page: Page, traceId: string) {
  await expect(async () => {
    await page.reload();
    await expect(section(page, 'Event')).toContainText(traceId, { timeout: 3000 });
  }).toPass();
}

async function exploreShows(page: Page, text: string | RegExp) {
  await expect(page).toHaveURL(/\/explore/);
  await expect(async () => {
    try {
      await expect(page.getByText(text).first()).toBeVisible({ timeout: 5000 });
    } catch (error) {
      await page.reload();
      throw error;
    }
  }).toPass();
}

async function backToIssue(page: Page, issueUrl: string, traceId: string) {
  await page.goto(issueUrl);
  await showsTraceId(page, traceId);
}

test.describe('trace and log links', () => {
  test("a dashboard row links to the trace's logs", async ({ gotoDashboardPage, readProvisionedDashboard, page }) => {
    const { traceId } = readReported<Reported>('trace');
    const dashboard = await readProvisionedDashboard({ fileName: 'bugsink-trace.json' });
    const dashboardPage = await gotoDashboardPage({
      uid: dashboard.uid,
      queryParams: new URLSearchParams({ 'var-trace': traceId }),
    });
    const events = dashboardPage.getPanelByTitle('Events in this trace');
    const cell = events.locator.getByRole('gridcell').filter({ hasText: traceId });
    await expect(async () => {
      await dashboardPage.refreshDashboard();
      await expect(cell).toBeVisible({ timeout: 3000 });
    }).toPass();

    await cell.click();
    await expect(page.getByRole('link', { name: 'Open trace' })).toBeVisible();
    const logsLink = page.getByRole('link', { name: 'Open logs' });
    const href = (await logsLink.getAttribute('href')) ?? '';
    const range = JSON.parse(new URL(href, 'http://grafana').searchParams.get('left') ?? '{}').range;
    expect(Number(range.to) - Number(range.from)).toBe(30 * 60 * 1000);
    await logsLink.click();
    await exploreShows(page, LOG_LINE);
  });

  test('a traceparent reaches the event, and Open trace and Open logs land on it', async ({ gotoPage, page }) => {
    const { traceId } = readReported<Reported>('trace');
    await gotoPage('', listParams(EVENT_MARKER));
    await openFirstIssue(page);
    await showsTraceId(page, traceId);
    const issueUrl = page.url();

    await section(page, 'Event').getByRole('link', { name: 'Open trace' }).click();
    await exploreShows(page, 'GET /handled');
    await exploreShows(page, 'sample-python');

    await backToIssue(page, issueUrl, traceId);
    await section(page, 'Event').getByRole('link', { name: 'Open logs' }).click();
    await exploreShows(page, LOG_LINE);
  });

  test('a JavaScript event parses and links', async ({ gotoPage, page }) => {
    const { traceId } = readReported<Reported>('js-event');
    await gotoPage('', listParams(JS_MARKER, 2));
    await openFirstIssue(page, /^SAMPLE-JS-/);
    await showsTraceId(page, traceId);
    const issueUrl = page.url();

    const exception = section(page, 'Exception');
    await expect(exception.getByRole('heading', { name: 'Error', exact: true })).toBeVisible();
    await expect(exception.getByRole('heading', { name: 'TypeError', exact: true })).toBeVisible();
    await expect(exception.getByText('app.mjs').first()).toBeVisible();
    await expect(section(page, 'Breadcrumbs')).toContainText('about to fail');

    await section(page, 'Event').getByRole('link', { name: 'Open trace' }).click();
    await exploreShows(page, 'sample-js');

    await backToIssue(page, issueUrl, traceId);
    await section(page, 'Event').getByRole('link', { name: 'Open logs' }).click();
    await exploreShows(page, LOG_LINE);
  });

  test('an event whose trace Tempo lacks', async ({ gotoPage, page }) => {
    const { message } = readReported<{ message: string }>('event-detail');
    await gotoPage('', listParams(MESSAGE_MARKER));
    await openFirstIssue(page);
    await expect(async () => {
      await page.reload();
      await expect(section(page, 'Event')).toContainText(dashed(message), { timeout: 3000 });
    }).toPass();

    await section(page, 'Event').getByRole('link', { name: 'Open trace' }).click();
    await exploreShows(page, /trace with id \w+ not found/i);
  });

  test('no trace buttons without targets', async ({ gotoPage, page }) => {
    const { traceId } = readReported<Reported>('trace');
    await gotoPage('', listParams(EVENT_MARKER, 1, 'bugsink-project'));
    await openFirstIssue(page);
    await showsTraceId(page, traceId);

    const event = section(page, 'Event');
    await expect(event.getByRole('link', { name: 'Open trace' })).toHaveCount(0);
    await expect(event.getByRole('link', { name: 'Open logs' })).toHaveCount(0);
    await expect(event.getByRole('link', { name: 'Open event in Bugsink' })).toHaveAttribute(
      'href',
      new RegExp(`^${BUGSINK_BROWSER_URL}/issues/issue/[0-9a-f-]{36}/event/[0-9a-f-]{36}/$`)
    );
  });
});
