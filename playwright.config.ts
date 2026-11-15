// SPDX-License-Identifier: GPL-3.0-or-later
import type { PluginOptions } from '@grafana/plugin-e2e';
import { defineConfig, devices } from '@playwright/test';
import baseConfig from './.config/playwright.config';

/**
 * Read environment variables from file.
 * https://github.com/motdotla/dotenv
 */
// require('dotenv').config();

/**
 * See https://playwright.dev/docs/test-configuration.
 */
export default defineConfig<PluginOptions>(baseConfig, {
  use: {
    baseURL: process.env.GRAFANA_URL || 'http://localhost:3300',
    provisioningRootDir: 'docker/grafana/provisioning',
  },
  projects: [
    { name: 'auth', testMatch: [] },
    { name: 'report-event', testMatch: /.*\.setup\.ts/ },
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'], storageState: undefined },
      dependencies: ['auth', 'report-event'],
    },
  ],
  // See https://grafana.com/developers/plugin-tools/how-to-guides/extend-configurations#extend-the-playwright-config for further info.
});
