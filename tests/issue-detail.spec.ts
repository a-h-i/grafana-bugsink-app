// SPDX-License-Identifier: GPL-3.0-or-later
import pluginJson from '../src/plugin.json';
import { expect, openFirstIssue, section, test } from './fixtures';
import { BUGSINK_BROWSER_URL, EVENT_MARKER } from './seeded';

const LIST_PARAMS = `ds=bugsink&project=1&state=all&q=${EVENT_MARKER}&from=now-24h&to=now`;
const EVENT_LINK = new RegExp(`^${BUGSINK_BROWSER_URL}/issues/issue/[0-9a-f-]{36}/event/[0-9a-f-]{36}/$`);

test.describe('issue detail page', () => {
  test('click an issue to see its detail', async ({ gotoPage, page }) => {
    await gotoPage('', new URLSearchParams(LIST_PARAMS));
    await openFirstIssue(page);

    await expect(page).toHaveURL(new RegExp(`/a/${pluginJson.id}/issues/[0-9a-f-]{36}\\?`));
    const address = new URL(page.url());
    for (const param of LIST_PARAMS.split('&')) {
      const [key, value] = param.split('=');
      expect(address.searchParams.get(key)).toBe(value);
    }
    const summary = section(page, /ValueError/);
    await expect(summary).toContainText(EVENT_MARKER);
    await expect(section(page, 'Tags')).toContainText('environment');
    await expect(section(page, 'Tags')).toContainText('development');
    await expect(section(page, 'History')).toContainText('First seen');
    const eventLink = section(page, 'Events')
      .locator('tbody tr')
      .first()
      .getByRole('link', { name: /^Open event .* in Bugsink$/ });
    await expect(eventLink).toHaveAttribute('href', EVENT_LINK);
  });

  test('back to the list keeps its state', async ({ gotoPage, page }) => {
    await gotoPage('', new URLSearchParams(LIST_PARAMS));
    await expect(page).toHaveURL(/order=desc/);
    const listUrl = page.url();
    await openFirstIssue(page);
    await expect(page).toHaveURL(/\/issues\/[0-9a-f-]{36}/);
    await page.getByRole('button', { name: 'Back to issues' }).click();
    await expect(page).toHaveURL(listUrl);
    await expect(page.getByRole('textbox', { name: 'Search' })).toHaveValue(EVENT_MARKER);
  });

  test("a project-bound token on another project's issue", async ({ gotoPage, page }) => {
    await gotoPage('issues/SAMPLE-JS-1', new URLSearchParams('ds=bugsink-project'));
    const alerts = page.getByRole('alert');
    await expect(alerts).toHaveCount(1);
    await expect(alerts).toContainText(
      'Bugsink refused the request: This token is not allowed to access this project.'
    );
  });

  test('an unknown issue', async ({ gotoPage, page }) => {
    const alerts = page.getByRole('alert');
    for (const id of ['00000000-0000-0000-0000-000000000000', 'NOPE-1']) {
      await gotoPage(`issues/${id}`, new URLSearchParams('ds=bugsink'));
      await expect(alerts).toHaveCount(1);
      await expect(alerts).toContainText('Bugsink refused the request: No Issue matches the given query.');
    }
  });
});
