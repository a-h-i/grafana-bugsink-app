// SPDX-License-Identifier: GPL-3.0-or-later
import { dateMath, DateTime, dateTime, TimeRange } from '@grafana/data';
import { DEFAULT_FROM, DEFAULT_TO } from './listState';

function parseBound(value: string, roundUp: boolean, now: DateTime): DateTime | undefined {
  const parsed = /^\d+$/.test(value) ? dateTime(Number(value)) : dateMath.toDateTime(value, { roundUp, now });
  return parsed?.isValid() ? parsed : undefined;
}

export function toTimeRange(from: string, to: string): TimeRange {
  const now = dateTime();
  const start = parseBound(from, false, now);
  const end = parseBound(to, true, now);
  if (!start || !end) {
    return toTimeRange(DEFAULT_FROM, DEFAULT_TO);
  }
  const raw = {
    from: /^\d+$/.test(from) ? start : from,
    to: /^\d+$/.test(to) ? end : to,
  };
  return { from: start, to: end, raw };
}

function absolute(fromMs: number, toMs: number): TimeRange {
  const from = dateTime(fromMs);
  const to = dateTime(toMs);
  return { from, to, raw: { from, to } };
}

export function shiftTimeRange(range: TimeRange, direction: -1 | 1): TimeRange {
  const offset = ((range.to.valueOf() - range.from.valueOf()) / 2) * direction;
  return absolute(range.from.valueOf() + offset, range.to.valueOf() + offset);
}

export function zoomOutTimeRange(range: TimeRange): TimeRange {
  const half = (range.to.valueOf() - range.from.valueOf()) / 2;
  return absolute(range.from.valueOf() - half, range.to.valueOf() + half);
}
