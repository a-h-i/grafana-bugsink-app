// SPDX-License-Identifier: GPL-3.0-or-later
import { formatTime, STATUS_LABELS } from './format';

describe('format', () => {
  it('formats a millisecond time and an ISO string alike', () => {
    expect(formatTime(Date.UTC(2026, 10, 13, 10), 'utc')).toBe('2026-11-13 10:00:00');
    expect(formatTime('2026-11-13T10:00:00Z', 'utc')).toBe('2026-11-13 10:00:00');
  });

  it('labels each status', () => {
    expect(STATUS_LABELS).toEqual({ open: 'Open', muted: 'Muted', resolved: 'Resolved' });
  });
});
