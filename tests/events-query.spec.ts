// SPDX-License-Identifier: GPL-3.0-or-later
import { expect, test } from '@grafana/plugin-e2e';
import { TRACE_MARKER } from './seeded';
import { readTrace } from './trace-event';

function dashed(hex: string) {
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

test.describe('events', () => {
  test('issues and events of one trace', async ({ gotoDashboardPage, readProvisionedDashboard }) => {
    const { traceId, eventId } = readTrace();
    const dashboard = await readProvisionedDashboard({ fileName: 'bugsink-trace.json' });
    const dashboardPage = await gotoDashboardPage({
      uid: dashboard.uid,
      queryParams: new URLSearchParams({ 'var-trace': traceId }),
    });
    const issues = dashboardPage.getPanelByTitle('Issues in this trace');
    const events = dashboardPage.getPanelByTitle('Events in this trace');
    await expect(async () => {
      await dashboardPage.refreshDashboard();
      await expect(issues.data).toContainText([TRACE_MARKER]);
      await expect(events.data).toContainText([dashed(eventId)]);
    }).toPass();
    await expect(events.data).toContainText([/SAMPLE-PYTHON-\d+/]);
  });

  test('look up an event by its ID in Explore', async ({ explorePage, page, selectors }) => {
    const { eventId } = readTrace();
    await explorePage.getByGrafanaSelector(selectors.components.DataSourcePicker.inputV2).fill('Bugsink');
    await page.getByRole('option', { name: 'Bugsink', exact: true }).click();
    await explorePage.timeRange.set({ from: 'now-24h', to: 'now' });
    await page.getByRole('radio', { name: 'Events' }).click();
    await page.getByRole('combobox', { name: 'Project' }).click();
    await page.getByRole('option', { name: 'sample-python' }).click();
    const input = page.getByRole('textbox', { name: 'Event ID' });
    await input.fill(eventId);
    await expect(async () => {
      const answered = page.waitForResponse(
        (response) =>
          response.url().includes('/api/ds/query') && (response.request().postData() ?? '').includes(eventId)
      );
      await input.press('Enter');
      await answered;
      await expect(explorePage.tablePanel.data).toContainText([dashed(eventId)], { timeout: 3000 });
    }).toPass();
  });
});
