// SPDX-License-Identifier: GPL-3.0-or-later
import { randomBytes } from 'node:crypto';
import { expect, test as setup } from '@grafana/plugin-e2e';
import { EVENT_MARKER, SAMPLE_APP_URL } from './seeded';
import { writeReported } from './reported';

setup.describe.configure({ retries: 0 });

setup('report one handled error with an incoming traceparent to Bugsink', async ({ request }) => {
  const sent = randomBytes(16).toString('hex');
  const traceparent = `00-${sent}-${randomBytes(8).toString('hex')}-01`;
  const response = await request.get(`${SAMPLE_APP_URL}/handled`, {
    params: { variant: EVENT_MARKER },
    headers: { traceparent },
  });
  expect(response.ok()).toBe(true);
  const { trace_id: traceId, event_id: eventId } = await response.json();
  expect(traceId).toBe(sent);
  writeReported('trace', { traceId, eventId });
});
