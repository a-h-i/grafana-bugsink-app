// SPDX-License-Identifier: GPL-3.0-or-later
import { expect, test as setup } from '@grafana/plugin-e2e';
import { SAMPLE_APP_URL, TRACE_MARKER } from './seeded';
import { writeReported } from './reported';

setup.describe.configure({ retries: 0 });

setup('report one handled error with a trace to Bugsink', async ({ request }) => {
  const response = await request.get(`${SAMPLE_APP_URL}/handled`, { params: { variant: TRACE_MARKER } });
  expect(response.ok()).toBe(true);
  const { trace_id: traceId, event_id: eventId } = await response.json();
  writeReported('trace', { traceId, eventId });
});
