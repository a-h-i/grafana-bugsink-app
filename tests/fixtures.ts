// SPDX-License-Identifier: GPL-3.0-or-later
import { AppPage, test as base } from '@grafana/plugin-e2e';
import pluginJson from '../src/plugin.json';

type AppTestFixture = {
  gotoPage: (path?: string, queryParams?: URLSearchParams) => Promise<AppPage>;
};

export const test = base.extend<AppTestFixture>({
  gotoPage: async ({ page, selectors, grafanaVersion, request }, use, testInfo) => {
    await use(async (path, queryParams) => {
      const appPage = new AppPage({ page, selectors, grafanaVersion, request, testInfo }, { pluginId: pluginJson.id });
      await appPage.goto({ path, queryParams });
      return appPage;
    });
  },
});

export { expect } from '@grafana/plugin-e2e';
