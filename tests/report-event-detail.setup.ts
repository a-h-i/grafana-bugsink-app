// SPDX-License-Identifier: GPL-3.0-or-later
import { expect, test as setup } from '@grafana/plugin-e2e';
import { writeReported } from './reported';
import { CHAINED_MARKER, MESSAGE_MARKER, SAMPLE_APP_URL } from './seeded';

setup.describe.configure({ retries: 0 });

setup('report two chained errors and one message to Bugsink', async ({ request }) => {
  const chained: string[] = [];
  for (let i = 0; i < 2; i++) {
    const response = await request.get(`${SAMPLE_APP_URL}/chained`, { params: { variant: CHAINED_MARKER } });
    expect(response.ok()).toBe(true);
    chained.push((await response.json()).event_id);
  }
  const response = await request.get(`${SAMPLE_APP_URL}/message`, { params: { variant: MESSAGE_MARKER } });
  expect(response.ok()).toBe(true);
  writeReported('event-detail', { chained, message: (await response.json()).event_id });
});
