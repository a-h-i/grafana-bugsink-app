// SPDX-License-Identifier: GPL-3.0-or-later
import { expect, test } from './fixtures';

test('the home page links to the provisioned data source', async ({ gotoPage, page }) => {
  await gotoPage('');
  const content = page.getByRole('main');
  await expect(page.getByRole('heading', { name: 'Bugsink' })).toHaveCount(1);
  await expect(content.getByRole('link', { name: 'Bugsink', exact: true })).toHaveAttribute(
    'href',
    /\/connections\/datasources\/edit\/bugsink$/
  );
});
