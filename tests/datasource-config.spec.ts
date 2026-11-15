// SPDX-License-Identifier: GPL-3.0-or-later
import { expect, test } from '@grafana/plugin-e2e';
import { BUGSINK_URL, GLOBAL_READ, NO_CAPS, PROJECT_READ } from './seeded';

const TYPE = 'ahmedismail-bugsink-datasource';

test.describe('connecting a Bugsink data source', () => {
  async function saveAndTest(
    createDataSourceConfigPage: (args: { type: string }) => Promise<any>,
    page: import('@playwright/test').Page,
    url: string,
    token: string
  ) {
    const configPage = await createDataSourceConfigPage({ type: TYPE });
    await page.getByRole('textbox', { name: 'Bugsink URL' }).fill(url);
    if (token) {
      await page.getByRole('textbox', { name: 'API token' }).fill(token);
    }
    return configPage;
  }

  test('valid global token', async ({ createDataSourceConfigPage, page }) => {
    const configPage = await saveAndTest(createDataSourceConfigPage, page, BUGSINK_URL, GLOBAL_READ);
    await expect(configPage.saveAndTest()).toBeOK();
    await expect(configPage).toHaveAlert('success', { hasText: 'Projects visible to this token:' });
    await expect(configPage).toHaveAlert('success', { hasText: 'sample-python' });
    await expect(configPage).toHaveAlert('success', { hasText: 'sample-js' });
  });

  test('project-bound token', async ({ createDataSourceConfigPage, page }) => {
    const configPage = await saveAndTest(createDataSourceConfigPage, page, BUGSINK_URL, PROJECT_READ);
    await expect(configPage.saveAndTest()).toBeOK();
    await expect(configPage).toHaveAlert('success', { hasText: 'sample-python' });
    await expect(configPage).not.toHaveAlert('success', { hasText: 'sample-js' });
  });

  test('URL with trailing path', async ({ createDataSourceConfigPage, page }) => {
    const configPage = await saveAndTest(
      createDataSourceConfigPage,
      page,
      BUGSINK_URL + '/api/canonical/0/',
      GLOBAL_READ
    );
    await expect(configPage.saveAndTest()).toBeOK();
  });

  test('wrong URL', async ({ createDataSourceConfigPage, page }) => {
    const configPage = await saveAndTest(createDataSourceConfigPage, page, 'http://bugsink:9', GLOBAL_READ);
    await expect(configPage.saveAndTest()).not.toBeOK();
    await expect(configPage).toHaveAlert('error', { hasText: 'Cannot reach Bugsink at http://bugsink:9' });
  });

  test('not a Bugsink', async ({ createDataSourceConfigPage, page }) => {
    const configPage = await saveAndTest(createDataSourceConfigPage, page, 'http://grafana:3000', GLOBAL_READ);
    await expect(configPage.saveAndTest()).not.toBeOK();
    await expect(configPage).toHaveAlert('error', { hasText: 'did not answer like a Bugsink API' });
  });

  test('bad token', async ({ createDataSourceConfigPage, page }) => {
    const configPage = await saveAndTest(createDataSourceConfigPage, page, BUGSINK_URL, '0'.repeat(40));
    await expect(configPage.saveAndTest()).not.toBeOK();
    await expect(configPage).toHaveAlert('error', { hasText: 'Bugsink rejected the API token' });
  });

  test('token without capabilities', async ({ createDataSourceConfigPage, page }) => {
    const configPage = await saveAndTest(createDataSourceConfigPage, page, BUGSINK_URL, NO_CAPS);
    await expect(configPage.saveAndTest()).not.toBeOK();
    await expect(configPage).toHaveAlert('error', {
      hasText: 'The API token lacks: projects:read, issues:read, events:read.',
    });
  });

  test('no token', async ({ createDataSourceConfigPage, page }) => {
    const configPage = await saveAndTest(createDataSourceConfigPage, page, BUGSINK_URL, '');
    await expect(configPage.saveAndTest()).not.toBeOK();
    await expect(configPage).toHaveAlert('error', { hasText: 'Enter an API token.' });
  });

  test('provisioned data source', async ({ gotoDataSourceConfigPage, page }) => {
    await gotoDataSourceConfigPage('bugsink');
    await expect(page.getByRole('textbox', { name: 'API token' })).toHaveValue('configured');
    const health = page.waitForResponse((response) => response.url().includes('/api/datasources/uid/bugsink/health'));
    await page.getByRole('button', { name: 'Test', exact: true }).click();
    expect((await health).ok()).toBe(true);
  });
});
