// SPDX-License-Identifier: GPL-3.0-or-later
import { expect, test as setup } from '@grafana/plugin-e2e';
import { EVENT_MARKER, SAMPLE_APP_URL } from './seeded';

setup('report one handled error to Bugsink', async ({ request }) => {
  const response = await request.get(`${SAMPLE_APP_URL}/handled`, { params: { variant: EVENT_MARKER } });
  expect(response.ok()).toBe(true);
});
