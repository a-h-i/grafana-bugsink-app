// SPDX-License-Identifier: GPL-3.0-or-later
import type { APIRequestContext, Page } from '@playwright/test';
import { basicAuth, expect, test } from './fixtures';
import { readReported } from './reported';
import { BUGSINK_API, EDITOR, GLOBAL_READ, MESSAGE_MARKER, TRIAGE, VIEWER } from './seeded';

type Issue = { id: string; friendlyId: string };

const REFUSED = 'Only editors and admins can triage issues.';
const SWITCHED_OFF = 'Triage is switched off for this data source.';
const NO_HEADER = 'Triage calls need the header X-Bugsink-Triage: 1.';

const listParams = (ds: string) =>
  new URLSearchParams(`ds=${ds}&project=1&state=open&q=${MESSAGE_MARKER}&from=now-24h&to=now`);

const bearer = (token: string) => ({ Authorization: `Bearer ${token}` });

async function findIssue(bugsink: APIRequestContext): Promise<Issue> {
  const { message } = readReported<{ message: string }>('event-detail');
  let issue: Issue | undefined;
  await expect(async () => {
    const response = await bugsink.get(
      `${BUGSINK_API}/issues/?project=1&state=all&q=${MESSAGE_MARKER}&sort=last_seen&order=desc`,
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
    expect(newest?.event_id.replaceAll('-', ''), 'the newest event of the issue is this run').toBe(message);
    issue = { id: first.id, friendlyId: first.friendly_id };
  }).toPass();
  return issue!;
}

async function bugsinkState(bugsink: APIRequestContext, id: string) {
  const response = await bugsink.get(`${BUGSINK_API}/issues/${id}/`, { headers: bearer(GLOBAL_READ) });
  const issue = await response.json();
  return { resolved: issue.is_resolved as boolean, muted: issue.is_muted as boolean };
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

let bugsink: APIRequestContext;
let issue: Issue;

test.describe('triage', () => {
  test.describe.configure({ mode: 'serial' });

  test.beforeAll(async ({ playwright }) => {
    bugsink = await playwright.request.newContext();
    issue = await findIssue(bugsink);
    await putBack(bugsink, issue.id);
  });

  test.afterAll(async () => {
    if (issue) {
      await putBack(bugsink, issue.id);
    }
    await bugsink.dispose();
  });

  test.describe('as an editor', () => {
    test.use({ extraHTTPHeaders: basicAuth(EDITOR) });

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
      await expectOpenInBugsink(bugsink, issue.id);
    });
  });
});
