// SPDX-License-Identifier: GPL-3.0-or-later
import { expect, test as setup } from '@grafana/plugin-e2e';
import { JS_MARKER, SAMPLE_JS_URL } from './seeded';
import { writeReported } from './reported';

setup.describe.configure({ retries: 0 });

setup('report one handled error from the JavaScript app to Bugsink', async ({ request }) => {
  const response = await request.get(`${SAMPLE_JS_URL}/handled`, { params: { variant: JS_MARKER } });
  expect(response.ok()).toBe(true);
  const { trace_id: traceId, event_id: eventId } = await response.json();
  writeReported('js-event', { traceId, eventId });
});
