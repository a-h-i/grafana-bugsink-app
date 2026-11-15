// SPDX-License-Identifier: GPL-3.0-or-later
import { expect, openFirstIssue, section, test } from './fixtures';
import { readReported } from './reported';
import { CHAINED_MARKER, MESSAGE_MARKER } from './seeded';

type Reported = { chained: [string, string]; message: string };

const listParams = (marker: string) =>
  new URLSearchParams(`ds=bugsink&project=1&state=all&q=${marker}&from=now-24h&to=now`);

function dashed(hex: string) {
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

test.describe('event detail', () => {
  test("the latest event's stack trace and breadcrumbs", async ({ gotoPage, page }) => {
    const { chained } = readReported<Reported>('event-detail');
    await gotoPage('', listParams(CHAINED_MARKER));
    await openFirstIssue(page);

    const events = section(page, 'Events');
    await expect(async () => {
      await page.reload();
      await expect(events.locator('tbody tr').first()).toContainText(dashed(chained[1]), { timeout: 3000 });
    }).toPass();
    await expect(section(page, 'Event')).toContainText(dashed(chained[1]));

    const exception = section(page, 'Exception');
    await expect(exception.getByRole('heading', { name: 'JSONDecodeError', exact: true })).toBeVisible();
    await expect(exception.getByRole('heading', { name: 'RuntimeError', exact: true })).toBeVisible();
    await expect(exception.getByText('app.py').first()).toBeVisible();
    await expect(exception.getByText(/app\.py:\d+ in chained/).first()).toBeVisible();
    await expect(exception.getByText('json/decoder.py')).toBeHidden();
    await exception.getByRole('button', { name: /other frames?$/ }).click();
    await expect(exception.getByText('json/decoder.py').first()).toBeVisible();

    await expect(section(page, 'Breadcrumbs')).toContainText('parsing the order');
    await expect(section(page, 'Event tags')).toContainText('sample-python');
    await expect(section(page, 'Contexts')).toContainText('trace_id');
  });

  test('switch to an older event', async ({ gotoPage, page }) => {
    const { chained } = readReported<Reported>('event-detail');
    await gotoPage('', listParams(CHAINED_MARKER));
    await openFirstIssue(page);

    const secondRow = section(page, 'Events').locator('tbody tr').nth(1);
    await expect(async () => {
      await page.reload();
      await expect(secondRow).toContainText(dashed(chained[0]), { timeout: 3000 });
    }).toPass();
    await secondRow.getByRole('link', { name: /^[0-9a-f-]{36}$/ }).click();

    await expect(page).toHaveURL(/event=/);
    await expect(section(page, 'Event')).toContainText(dashed(chained[0]));
  });

  test("an event without structured frames shows Bugsink's text", async ({ gotoPage, page }) => {
    const { message } = readReported<Reported>('event-detail');
    await gotoPage('', listParams(MESSAGE_MARKER));
    await openFirstIssue(page);

    await expect(async () => {
      await page.reload();
      await expect(section(page, 'Event')).toContainText(dashed(message), { timeout: 3000 });
    }).toPass();

    await expect(section(page, 'Message')).toContainText(`message without exception (variant v${MESSAGE_MARKER})`);
    const trace = section(page, 'Stack trace');
    await expect(
      trace.getByText("Bugsink's own rendering of the stack trace. It can include local variables.")
    ).toBeVisible();
    await expect(trace.locator('pre')).toContainText(`# message without exception (variant v${MESSAGE_MARKER})`);
    await expect(trace.locator('pre')).toContainText('app.py');
  });
});
