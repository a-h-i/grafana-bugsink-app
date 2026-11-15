// SPDX-License-Identifier: GPL-3.0-or-later
import { expect, test } from '@grafana/plugin-e2e';
import type { Page } from '@playwright/test';
import type { ExplorePage, PanelEditPage } from '@grafana/plugin-e2e';
import { BUGSINK_BROWSER_URL, EVENT_MARKER } from './seeded';

const DATA_SOURCE_NAME = 'Bugsink';
const LINK_PATTERN = new RegExp(`^${BUGSINK_BROWSER_URL}/issues/issue/[0-9a-f-]{36}/event/last/$`);

// Picks the provisioned data source by its exact name; data sources made by the config tests share the prefix.
async function pickBugsink(
  model: PanelEditPage | ExplorePage,
  page: Page,
  selectors: { components: { DataSourcePicker: { inputV2: string } } }
) {
  await model.getByGrafanaSelector(selectors.components.DataSourcePicker.inputV2).fill(DATA_SOURCE_NAME);
  await page.getByRole('option', { name: DATA_SOURCE_NAME, exact: true }).click();
}

test.describe('issues in dashboards', () => {
  test('sample dashboard lists the issue', async ({ gotoDashboardPage, readProvisionedDashboard }) => {
    const dashboard = await readProvisionedDashboard({ fileName: 'bugsink-issues.json' });
    const dashboardPage = await gotoDashboardPage({ uid: dashboard.uid });
    const panel = dashboardPage.getPanelByTitle('Unresolved issues in sample-python');
    await expect(async () => {
      await dashboardPage.refreshDashboard();
      await expect(panel.data).toContainText([EVENT_MARKER]);
    }).toPass();
    const row = panel.locator.getByRole('row').filter({ hasText: EVENT_MARKER }).first();
    await expect(row.getByRole('link', { name: /SAMPLE-PYTHON-\d+/ })).toHaveAttribute('href', LINK_PATTERN);
  });

  test('query editor in a new panel', async ({ panelEditPage, page, selectors }) => {
    await pickBugsink(panelEditPage, page, selectors);
    await panelEditPage.setVisualization('Table');
    await panelEditPage.timeRange.set({ from: 'now-24h', to: 'now' });
    await page.getByRole('combobox', { name: 'Project' }).click();
    await page.getByRole('option', { name: 'sample-python' }).click();
    const search = page.getByRole('textbox', { name: 'Search' });
    await search.fill(EVENT_MARKER);
    await search.press('Enter');
    await expect(async () => {
      await panelEditPage.refreshPanel();
      await expect(panelEditPage.panel.data).toContainText([EVENT_MARKER]);
    }).toPass();

    await search.fill('environment:nosuchenv');
    await search.press('Enter');
    await expect(panelEditPage.panel.locator).toContainText('No rows');
    await expect(panelEditPage.panel.getErrorIcon()).toHaveCount(0);
  });

  test("project picker lists the token's projects", async ({ panelEditPage, page, selectors }) => {
    await pickBugsink(panelEditPage, page, selectors);
    await page.getByRole('combobox', { name: 'Project' }).click();
    await expect(page.getByRole('option')).toHaveText(['All projects', 'sample-js', 'sample-python']);
  });

  test('the same query in Explore', async ({ explorePage, page, selectors }) => {
    await pickBugsink(explorePage, page, selectors);
    await explorePage.timeRange.set({ from: 'now-24h', to: 'now' });
    await page.getByRole('combobox', { name: 'Project' }).click();
    await page.getByRole('option', { name: 'sample-python' }).click();
    const search = page.getByRole('textbox', { name: 'Search' });
    await search.fill(EVENT_MARKER);
    const answered = page.waitForResponse(
      (response) =>
        response.url().includes('/api/ds/query') && (response.request().postData() ?? '').includes(EVENT_MARKER)
    );
    await search.press('Enter');
    await answered;
    await expect(async () => {
      await explorePage.runQuery({ timeout: 10_000 });
      await expect(explorePage.tablePanel.data).toContainText([EVENT_MARKER]);
    }).toPass();
  });

  test('a refused query explains itself', async ({ gotoDashboardPage, readProvisionedDashboard, page }) => {
    const dashboard = await readProvisionedDashboard({ fileName: 'bugsink-e2e-refused.json' });
    const dashboardPage = await gotoDashboardPage({ uid: dashboard.uid });
    const panel = dashboardPage.getPanelByTitle('Project that does not exist');
    await panel.getErrorIcon().click();
    await expect(page.getByText('Bugsink refused the request: No Project matches the given query.')).toBeVisible();
  });
});
