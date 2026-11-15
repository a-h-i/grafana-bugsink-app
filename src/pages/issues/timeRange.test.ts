// SPDX-License-Identifier: GPL-3.0-or-later
import { dateTime } from '@grafana/data';
import { shiftTimeRange, toTimeRange, zoomOutTimeRange } from './timeRange';

const HOUR = 3600 * 1000;

function range(fromMs: number, toMs: number) {
  return { from: dateTime(fromMs), to: dateTime(toMs), raw: { from: dateTime(fromMs), to: dateTime(toMs) } };
}

describe('toTimeRange', () => {
  it('spans 7 days for now-7d to now and keeps raw', () => {
    const result = toTimeRange('now-7d', 'now');

    expect(result.to.valueOf() - result.from.valueOf()).toBe(7 * 24 * HOUR);
    expect(result.raw).toEqual({ from: 'now-7d', to: 'now' });
  });

  it('parses epoch milliseconds', () => {
    expect(toTimeRange('1763164800000', 'now').from.valueOf()).toBe(1763164800000);
  });

  it('gives the default range when a bound does not parse', () => {
    const result = toTimeRange('garbage', 'now');

    expect(result.raw).toEqual({ from: 'now-7d', to: 'now' });
  });
});

describe('shiftTimeRange', () => {
  it('moves a 2-hour range back by 1 hour', () => {
    const shifted = shiftTimeRange(range(10 * HOUR, 12 * HOUR), -1);

    expect(shifted.from.valueOf()).toBe(9 * HOUR);
    expect(shifted.to.valueOf()).toBe(11 * HOUR);
  });

  it('moves it forward by 1 hour', () => {
    const shifted = shiftTimeRange(range(10 * HOUR, 12 * HOUR), 1);

    expect(shifted.from.valueOf()).toBe(11 * HOUR);
    expect(shifted.to.valueOf()).toBe(13 * HOUR);
  });
});

describe('zoomOutTimeRange', () => {
  it('doubles a 2-hour range about its centre', () => {
    const zoomed = zoomOutTimeRange(range(10 * HOUR, 12 * HOUR));

    expect(zoomed.from.valueOf()).toBe(9 * HOUR);
    expect(zoomed.to.valueOf()).toBe(13 * HOUR);
  });
});
