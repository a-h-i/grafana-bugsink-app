// SPDX-License-Identifier: GPL-3.0-or-later
import { withEvent } from './routes';

describe('withEvent', () => {
  it('adds the event after the other segments as written', () => {
    expect(withEvent('?orgId=1&kiosk&ds=eu', 'e-1')).toBe('?orgId=1&kiosk&ds=eu&event=e-1');
  });

  it('starts a query string when there is none', () => {
    expect(withEvent('', 'e-1')).toBe('?event=e-1');
  });

  it('replaces an existing event in place', () => {
    expect(withEvent('?a=1&event=e-1&kiosk', 'e-2')).toBe('?a=1&event=e-2&kiosk');
  });

  it('removes the event and keeps the other segments', () => {
    expect(withEvent('?kiosk&event=e-1')).toBe('?kiosk');
  });

  it('is empty when nothing is left', () => {
    expect(withEvent('?event=e-1')).toBe('');
  });
});
