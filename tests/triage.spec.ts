// SPDX-License-Identifier: GPL-3.0-or-later
import type { APIRequestContext, Page } from '@playwright/test';
import { basicAuth, expect, section, test } from './fixtures';
import { readReported } from './reported';
import { BUGSINK_API, CHAINED_MARKER, EDITOR, GLOBAL_READ, MESSAGE_MARKER, TRIAGE, VIEWER } from './seeded';

type Issue = { id: string; friendlyId: string };

const REFUSED = 'Only editors and admins can triage issues.';
const SWITCHED_OFF = 'Triage is switched off for this data source.';
const NO_HEADER = 'Triage calls need the header X-Bugsink-Triage: 1.';
const BAD_SHAPE = 'The request body must be a JSON object with period and periods.';
const TRIAGE_HEADER = { 'X-Bugsink-Triage': '1' };
const HOUR = 3600 * 1000;
const BY_EDITOR = ' from Grafana by "m7-editor".';

const listParams = (ds: string) =>
  new URLSearchParams(`ds=${ds}&project=1&state=open&q=${MESSAGE_MARKER}&from=now-24h&to=now`);

const bearer = (token: string) => ({ Authorization: `Bearer ${token}` });

async function findIssue(bugsink: APIRequestContext, marker: string, newestEventId: string): Promise<Issue> {
  let issue: Issue | undefined;
  await expect(async () => {
    const response = await bugsink.get(
      `${BUGSINK_API}/issues/?project=1&state=all&q=${marker}&sort=last_seen&order=desc`,
      { headers: bearer(GLOBAL_READ) }
    );
    expect(response.ok()).toBe(true);
    const first = (await response.json()).results[0];
    expect(first).toBeDefined();
    const events = await bugsink.get(`${BUGSINK_API}/events/?issue=${first.id}&order=desc`, {
      headers: bearer(GLOBAL_READ),
    });
    expect(events.ok()).toBe(true);
    const newest = (await events.json()).results[0];
    expect(newest?.event_id.replaceAll('-', ''), 'the newest event of the issue is this run').toBe(newestEventId);
    issue = { id: first.id, friendlyId: first.friendly_id };
  }).toPass();
  return issue!;
}

async function bugsinkIssue(bugsink: APIRequestContext, id: string) {
  const response = await bugsink.get(`${BUGSINK_API}/issues/${id}/`, { headers: bearer(GLOBAL_READ) });
  return await response.json();
}

async function bugsinkState(bugsink: APIRequestContext, id: string) {
  const issue = await bugsinkIssue(bugsink, id);
  return { resolved: issue.is_resolved as boolean, muted: issue.is_muted as boolean };
}

async function latestComment(bugsink: APIRequestContext, id: string): Promise<string> {
  const response = await bugsink.get(`${BUGSINK_API}/issue-history/?issue=${id}&order=desc`, {
    headers: bearer(GLOBAL_READ),
  });
  const entries: Array<{ kind: string; comment: string }> = (await response.json()).results;
  return entries.find((entry) => entry.kind === 'manual_annotation')?.comment ?? '';
}

async function expectLatestComment(bugsink: APIRequestContext, id: string, comment: string) {
  await expect(async () => {
    expect(await latestComment(bugsink, id)).toBe(comment);
  }).toPass();
}

async function putBack(bugsink: APIRequestContext, id: string) {
  const state = await bugsinkState(bugsink, id);
  const act = (action: string) => bugsink.post(`${BUGSINK_API}/issues/${id}/${action}/`, { headers: bearer(TRIAGE) });
  if (state.resolved) {
    expect((await act('reopen')).ok()).toBe(true);
  } else if (state.muted) {
    expect((await act('unmute')).ok()).toBe(true);
  }
}

async function expectOpenInBugsink(bugsink: APIRequestContext, id: string) {
  await expect(async () => {
    expect(await bugsinkState(bugsink, id)).toEqual({ resolved: false, muted: false });
  }).toPass();
}

async function expectLogin(request: APIRequestContext, login: string) {
  const response = await request.get('/api/user');
  expect((await response.json()).login).toBe(login);
}

function status(page: Page) {
  return page.getByRole('row', { name: /^Status / }).getByRole('cell');
}

function historyEntry(page: Page, index: number) {
  return section(page, 'History').getByRole('listitem').nth(index);
}

function commentWarning(page: Page) {
  return page.getByText(/attribution comment was not written/);
}

let bugsink: APIRequestContext;
let issue: Issue;
let chained: Issue;

test.describe('triage', () => {
  test.describe.configure({ mode: 'serial' });

  test.beforeAll(async ({ playwright }) => {
    bugsink = await playwright.request.newContext();
    const reported = readReported<{ chained: [string, string]; message: string }>('event-detail');
    issue = await findIssue(bugsink, MESSAGE_MARKER, reported.message);
    chained = await findIssue(bugsink, CHAINED_MARKER, reported.chained[1]);
    await putBack(bugsink, issue.id);
    await putBack(bugsink, chained.id);
  });

  test.beforeEach(async () => {
    await putBack(bugsink, issue.id);
    await putBack(bugsink, chained.id);
  });

  test.afterAll(async () => {
    for (const put of [issue, chained]) {
      if (put) {
        await putBack(bugsink, put.id);
      }
    }
    await bugsink.dispose();
  });

  test.describe('as an editor', () => {
    test.use({ extraHTTPHeaders: basicAuth(EDITOR) });

    test('the demo: mute for two days, resolve in the next release, attributed', async ({
      gotoPage,
      page,
      request,
    }) => {
      await expectLogin(request, EDITOR.user);
      await gotoPage(`issues/${issue.id}`, new URLSearchParams('ds=bugsink-triage'));
      await page.getByRole('button', { name: 'Mute for a period' }).click();
      const dialog = page.getByRole('dialog');
      await dialog.getByRole('spinbutton', { name: 'Number of periods' }).fill('2');
      await dialog.getByRole('button', { name: 'Mute', exact: true }).click();
      await expect(status(page)).toHaveText('Muted');
      await expect(page.getByRole('row', { name: /^Unmute after / })).toBeVisible();
      await expect(historyEntry(page, 0)).toContainText('Muted for 2 days' + BY_EDITOR);
      await expect(historyEntry(page, 1)).toContainText('Muted for 2 days, until');
      await expect(commentWarning(page)).toHaveCount(0);

      const now = Date.now();
      const muted = await bugsinkIssue(bugsink, issue.id);
      expect(muted.is_muted).toBe(true);
      const until = Date.parse(muted.unmute_after);
      expect(until).toBeGreaterThan(now + 47 * HOUR);
      expect(until).toBeLessThan(now + 49 * HOUR);
      await expectLatestComment(bugsink, issue.id, 'Muted for 2 days' + BY_EDITOR);

      await gotoPage(`issues/${chained.id}`, new URLSearchParams('ds=bugsink-triage'));
      await page.getByRole('button', { name: 'Resolve in next release' }).click();
      await expect(status(page)).toHaveText('Resolved');
      await expect(page.getByRole('row', { name: /^Resolved by next release / }).getByRole('cell')).toHaveText('Yes');
      await expect(commentWarning(page)).toHaveCount(0);
      expect((await bugsinkIssue(bugsink, chained.id)).is_resolved_by_next_release).toBe(true);
      await expectLatestComment(bugsink, chained.id, 'Resolved in the next release' + BY_EDITOR);
    });

    test('mute until a threshold', async ({ gotoPage, page, request }) => {
      await expectLogin(request, EDITOR.user);
      await gotoPage(`issues/${issue.id}`, new URLSearchParams('ds=bugsink-triage'));
      await page.getByRole('button', { name: 'Mute until a threshold' }).click();
      const dialog = page.getByRole('dialog');
      await dialog.getByRole('spinbutton', { name: 'Events' }).fill('10');
      await dialog.getByRole('spinbutton', { name: 'Number of periods' }).fill('1');
      await dialog.getByRole('radio', { name: 'Hours' }).check({ force: true });
      await dialog.getByRole('button', { name: 'Mute', exact: true }).click();
      await expect(status(page)).toHaveText('Muted');
      await expect(historyEntry(page, 0)).toContainText('Muted until 10 or more events in 1 hour' + BY_EDITOR);
      await expect(historyEntry(page, 1)).toContainText('Muted until 10 or more events in 1 hour');
      await expect(commentWarning(page)).toHaveCount(0);
      const muted = await bugsinkIssue(bugsink, issue.id);
      expect(muted.is_muted).toBe(true);
      expect(muted.unmute_on_volume_based_conditions).toEqual([{ period: 'hour', nr_of_periods: 1, volume: 10 }]);
      await expectLatestComment(bugsink, issue.id, 'Muted until 10 or more events in 1 hour' + BY_EDITOR);
      await page.getByRole('button', { name: 'Unmute', exact: true }).click();
      await expect(status(page)).toHaveText('Open');
      await expectOpenInBugsink(bugsink, issue.id);
    });

    test('resolve in the latest release', async ({ gotoPage, page, request }) => {
      await expectLogin(request, EDITOR.user);
      await gotoPage(`issues/${chained.id}`, new URLSearchParams('ds=bugsink-triage'));
      await page.getByRole('button', { name: 'Resolve in latest release' }).click();
      await expect(status(page)).toHaveText('Resolved');
      const resolved = await bugsinkIssue(bugsink, chained.id);
      expect(resolved.is_resolved).toBe(true);
      expect(resolved.is_resolved_by_next_release).toBe(false);
      await expect(historyEntry(page, 0)).toContainText('Resolved in the latest release' + BY_EDITOR);
      await expect(historyEntry(page, 1)).toContainText(/Resolved in release \S+/);
      await expect(commentWarning(page)).toHaveCount(0);
      await expectLatestComment(bugsink, chained.id, 'Resolved in the latest release' + BY_EDITOR);
      await page.getByRole('button', { name: 'Reopen', exact: true }).click();
      await expect(status(page)).toHaveText('Open');
      await expectOpenInBugsink(bugsink, chained.id);
    });

    test('a bad body is refused', async ({ request }) => {
      await expectLogin(request, EDITOR.user);
      const response = await request.post(`/api/datasources/uid/bugsink-triage/resources/issues/${issue.id}/mute-for`, {
        headers: TRIAGE_HEADER,
        data: { period: 'day', periods: '2' },
      });
      expect(response.status()).toBe(400);
      expect((await response.json()).message).toBe(BAD_SHAPE);
      await expectOpenInBugsink(bugsink, issue.id);
    });

    test('the demo: resolve from the list, reopen from the issue page', async ({ gotoPage, page, request }) => {
      await expectLogin(request, EDITOR.user);
      await gotoPage('', listParams('bugsink-triage'));
      const row = page
        .getByRole('row')
        .filter({ has: page.getByRole('link', { name: issue.friendlyId, exact: true }) });
      await row.getByRole('button', { name: `Resolve ${issue.friendlyId}` }).click();
      await expect(row.getByRole('cell', { name: 'Resolved', exact: true })).toBeVisible();

      await row.getByRole('link', { name: issue.friendlyId, exact: true }).click();
      await expect(status(page)).toHaveText('Resolved');
      await page.getByRole('button', { name: 'Reopen', exact: true }).click();
      await expect(status(page)).toHaveText('Open');
      await expectOpenInBugsink(bugsink, issue.id);
    });

    test('mute and unmute on the issue page', async ({ gotoPage, page, request }) => {
      await expectLogin(request, EDITOR.user);
      await gotoPage(`issues/${issue.id}`, new URLSearchParams('ds=bugsink-triage'));
      await expect(status(page)).toHaveText('Open');
      await page.getByRole('button', { name: 'Mute', exact: true }).click();
      await expect(status(page)).toHaveText('Muted');
      await expect(page.getByRole('button', { name: 'Resolve', exact: true })).toBeVisible();
      await page.getByRole('button', { name: 'Unmute', exact: true }).click();
      await expect(status(page)).toHaveText('Open');
      await expectOpenInBugsink(bugsink, issue.id);
    });

    test('triage switched off', async ({ gotoPage, page, request }) => {
      await expectLogin(request, EDITOR.user);
      await gotoPage('', listParams('bugsink'));
      await expect(page.getByRole('link', { name: issue.friendlyId, exact: true })).toBeVisible();
      await expect(page.getByRole('columnheader', { name: 'Actions' })).toHaveCount(0);
      await expect(page.getByRole('button', { name: /^(Resolve|Mute) / })).toHaveCount(0);

      await gotoPage(`issues/${issue.id}`, new URLSearchParams('ds=bugsink'));
      await expect(status(page)).toHaveText('Open');
      await expect(page.getByRole('button', { name: /^(Resolve|Reopen|Mute|Unmute)$/ })).toHaveCount(0);

      const response = await request.post(`/api/datasources/uid/bugsink/resources/issues/${issue.id}/resolve`);
      expect(response.status()).toBe(403);
      expect((await response.json()).message).toBe(SWITCHED_OFF);
      await expectOpenInBugsink(bugsink, issue.id);
    });

    test('a call without the triage header is refused', async ({ request }) => {
      await expectLogin(request, EDITOR.user);
      const response = await request.post(`/api/datasources/uid/bugsink-triage/resources/issues/${issue.id}/mute`, {
        form: { a: 'b' },
      });
      expect(response.status()).toBe(403);
      expect((await response.json()).message).toBe(NO_HEADER);
      await expectOpenInBugsink(bugsink, issue.id);
    });
  });

  test.describe('as a viewer', () => {
    test.use({ extraHTTPHeaders: basicAuth(VIEWER) });

    test('no controls and a refusal', async ({ gotoPage, page, request }) => {
      await expectLogin(request, VIEWER.user);
      await gotoPage('', listParams('bugsink-triage'));
      await expect(page.getByRole('link', { name: issue.friendlyId, exact: true })).toBeVisible();
      await expect(page.getByRole('columnheader', { name: 'Actions' })).toHaveCount(0);

      await gotoPage(`issues/${issue.id}`, new URLSearchParams('ds=bugsink-triage'));
      await expect(status(page)).toHaveText('Open');
      await expect(page.getByRole('button', { name: /^(Resolve|Reopen|Mute|Unmute)$/ })).toHaveCount(0);

      const response = await request.post(`/api/datasources/uid/bugsink-triage/resources/issues/${issue.id}/resolve`);
      expect(response.status()).toBe(403);
      expect((await response.json()).message).toBe(REFUSED);

      const mute = await request.post(`/api/datasources/uid/bugsink-triage/resources/issues/${issue.id}/mute-for`, {
        headers: TRIAGE_HEADER,
        data: { period: 'day', periods: 2 },
      });
      expect(mute.status()).toBe(403);
      expect((await mute.json()).message).toBe(REFUSED);
      await expectOpenInBugsink(bugsink, issue.id);
    });
  });
});
