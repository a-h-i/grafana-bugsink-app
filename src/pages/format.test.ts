// SPDX-License-Identifier: GPL-3.0-or-later
import { formatTime, periodText, STATUS_LABELS } from './format';

describe('format', () => {
  it('formats a millisecond time and an ISO string alike', () => {
    expect(formatTime(Date.UTC(2026, 10, 13, 10), 'utc')).toBe('2026-11-13 10:00:00');
    expect(formatTime('2026-11-13T10:00:00Z', 'utc')).toBe('2026-11-13 10:00:00');
  });

  it('names a number of periods', () => {
    expect(periodText(1, 'hour')).toBe('1 hour');
    expect(periodText(2, 'day')).toBe('2 days');
  });

  it('labels each status', () => {
    expect(STATUS_LABELS).toEqual({ open: 'Open', muted: 'Muted', resolved: 'Resolved' });
  });
});
