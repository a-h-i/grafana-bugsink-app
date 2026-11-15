// SPDX-License-Identifier: GPL-3.0-or-later
import type { Page } from '@playwright/test';
import pluginJson from '../src/plugin.json';
import { expect, test } from './fixtures';
import { BUGSINK_BROWSER_URL, EVENT_MARKER } from './seeded';

const LINK_PATTERN = new RegExp(`^${BUGSINK_BROWSER_URL}/issues/issue/[0-9a-f-]{36}/event/last/$`);
const COLUMN = { message: 3, project: 5, events: 6, status: 9 };

async function choose(page: Page, label: string, option: string) {
  await page.getByRole('combobox', { name: label }).click();
  await page.getByRole('option', { name: option, exact: true }).click();
}

function cells(page: Page, column: number) {
  return page.locator(`tbody tr td:nth-child(${column})`);
}

test.describe('issue list page', () => {
  test('the page shows its title once', async ({ gotoPage, page, selectors }) => {
    const appPage = await gotoPage('');
    await expect(page.getByRole('heading', { name: 'Bugsink', exact: true })).toHaveCount(1);
    await expect(appPage.getByGrafanaSelector(selectors.components.Breadcrumbs.breadcrumb('Bugsink'))).toHaveCount(1);
    await expect(page).toHaveURL(/[?&]ds=bugsink(&|$)/);
    await page.reload();
    await expect(page).toHaveURL(/[?&]ds=bugsink(&|$)/);
    await expect(appPage.getByGrafanaSelector(selectors.components.Breadcrumbs.breadcrumb('Bugsink'))).toHaveCount(1);
  });

  test('the demo: open issues of one project by event count', async ({ gotoPage, page }) => {
    await gotoPage('');
    await choose(page, 'Data source', 'Bugsink');
    await choose(page, 'State', 'Open');
    await choose(page, 'Project', 'sample-python');
    await choose(page, 'Sort by', 'Event count');

    await expect(page).toHaveURL(/[?&]ds=bugsink(&|$)/);
    await expect(page).toHaveURL(/[?&]project=1(&|$)/);
    await expect(page).toHaveURL(/state=open/);
    await expect(page).toHaveURL(/sort=digested_event_count/);

    await expect(async () => {
      const projects = await cells(page, COLUMN.project).allTextContents();
      expect(projects.length).toBeGreaterThan(0);
      expect(new Set(projects)).toEqual(new Set(['sample-python']));
      expect(new Set(await cells(page, COLUMN.status).allTextContents())).toEqual(new Set(['Open']));
      const events = (await cells(page, COLUMN.events).allTextContents()).map(Number);
      expect(events).toEqual([...events].sort((a, b) => b - a));
    }).toPass();

    const link = page.locator('tbody tr').first().getByRole('link', { name: /^Open .* in Bugsink$/ });
    await expect(link).toHaveAttribute('href', LINK_PATTERN);
    await expect(link).toHaveAttribute('target', '_blank');
    const first = page.locator('tbody tr').first();
    await expect(first.getByRole('link', { name: /^SAMPLE-PYTHON-\d+$/ })).toHaveAttribute(
      'href',
      new RegExp(`^/a/${pluginJson.id}/issues/`)
    );
  });

  test("Grafana's own address parameters stay as they are", async ({ page }) => {
    await page.goto(`/a/${pluginJson.id}?orgId=1&kiosk`);
    await expect(page).toHaveURL(/\?orgId=1&kiosk&ds=bugsink&/);
    await choose(page, 'State', 'Open');
    await expect(page).toHaveURL(/\?orgId=1&kiosk&ds=bugsink&state=open&/);
  });

  test('the state survives a reload', async ({ gotoPage, page }) => {
    const params = 'ds=bugsink&project=1&state=open&sort=digested_event_count&order=desc&from=now-24h&to=now';
    await gotoPage('', new URLSearchParams(params));
    await page.reload();
    await expect(page.getByRole('combobox', { name: 'Project' })).toHaveValue('sample-python');
    await expect(page.getByRole('combobox', { name: 'State' })).toHaveValue('Open');
    await expect(page.getByRole('combobox', { name: 'Sort by' })).toHaveValue('Event count');

    await gotoPage('', new URLSearchParams(`${params}&q=${EVENT_MARKER}`));
    await expect(async () => {
      await page.reload();
      await expect(cells(page, COLUMN.message).first()).toContainText(EVENT_MARKER, { timeout: 3000 });
    }).toPass();
  });

  test('a project-bound instance offers its one project', async ({ gotoPage, page }) => {
    await gotoPage('');
    await choose(page, 'Data source', 'Bugsink project-bound');
    await page.getByRole('combobox', { name: 'Project' }).click();
    await expect(page.getByRole('option')).toHaveText(['All projects', 'sample-python']);
  });

  test('no matching issues', async ({ gotoPage, page }) => {
    await gotoPage('', new URLSearchParams('ds=bugsink&q=environment:nosuchenv'));
    await expect(page.getByText('No issues match these filters in this time range.')).toBeVisible();
  });

  test('a refused query explains itself', async ({ gotoPage, page }) => {
    await gotoPage('', new URLSearchParams('ds=bugsink&project=99'));
    await expect(page.getByText('Bugsink refused the request: No Project matches the given query.')).toBeVisible();
  });

  test('an unknown data source', async ({ gotoPage, page }) => {
    await gotoPage('', new URLSearchParams('ds=nosuch'));
    await expect(page.getByText('The data source "nosuch" was not found.')).toBeVisible();
  });
});
