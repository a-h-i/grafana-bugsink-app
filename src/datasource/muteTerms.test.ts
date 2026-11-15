// SPDX-License-Identifier: GPL-3.0-or-later
import { muteTermsError } from './muteTerms';
import { MuteAction, MuteTerms, PERIODS } from './types';

describe('muteTermsError', () => {
  const SHAPE_FOR = 'The request body must be a JSON object with period and periods.';
  const SHAPE_UNTIL = 'The request body must be a JSON object with period, periods and threshold.';
  const PERIOD = 'The period must be minute, hour, day, week, month or year.';
  const PERIODS_MESSAGE = 'The number of periods must be a whole number from 1 to 100.';
  const THRESHOLD = 'The threshold must be a whole number from 1 to 1000000.';

  it('lists the six periods in order', () => {
    expect(PERIODS).toEqual(['minute', 'hour', 'day', 'week', 'month', 'year']);
  });

  it.each([
    ['mute-for', { period: 'day', periods: 2 }],
    ['mute-for', { period: 'minute', periods: 1 }],
    ['mute-for', { period: 'year', periods: 100 }],
    ['mute-until', { period: 'hour', periods: 1, threshold: 10 }],
    ['mute-until', { period: 'week', periods: 1, threshold: 1 }],
    ['mute-until', { period: 'month', periods: 100, threshold: 1000000 }],
  ] as const)('accepts %s %j', (action, terms) => {
    expect(muteTermsError(action, terms)).toBeUndefined();
  });

  it.each([
    ['mute-for', { period: 'fortnight', periods: 2 }, PERIOD],
    ['mute-for', { period: 'Day', periods: 2 }, PERIOD],
    ['mute-for', { period: '', periods: 2 }, PERIOD],
    ['mute-for', { period: 'day', periods: 0 }, PERIODS_MESSAGE],
    ['mute-for', { period: 'day', periods: -1 }, PERIODS_MESSAGE],
    ['mute-for', { period: 'day', periods: 101 }, PERIODS_MESSAGE],
    ['mute-for', { period: 'day', periods: 2.5 }, PERIODS_MESSAGE],
    ['mute-for', { period: 'day', periods: NaN }, PERIODS_MESSAGE],
    ['mute-for', { period: 'day', periods: Infinity }, PERIODS_MESSAGE],
    ['mute-until', { period: 'day', periods: 101, threshold: 10 }, PERIODS_MESSAGE],
    ['mute-until', { period: 'day', periods: 1, threshold: 0 }, THRESHOLD],
    ['mute-until', { period: 'day', periods: 1, threshold: 1000001 }, THRESHOLD],
    ['mute-until', { period: 'day', periods: 1, threshold: 1.5 }, THRESHOLD],
    ['mute-until', { period: 'day', periods: 1, threshold: NaN }, THRESHOLD],
    ['mute-until', { period: 'fortnight', periods: 0, threshold: 0 }, PERIOD],
    ['mute-until', { period: 'day', periods: 0, threshold: 0 }, PERIODS_MESSAGE],
  ] as const)('refuses %s %p', (action, terms, message) => {
    expect(muteTermsError(action, terms as MuteTerms)).toBe(message);
  });

  it('refuses an emptied number field', () => {
    expect(muteTermsError('mute-for', { period: 'day', periods: Number('') })).toBe(PERIODS_MESSAGE);
    expect(muteTermsError('mute-until', { period: 'day', periods: 1, threshold: Number('') })).toBe(THRESHOLD);
  });

  it.each([
    ['mute-for', { period: 'day', periods: '2' }, SHAPE_FOR],
    ['mute-for', { period: 'day' }, SHAPE_FOR],
    ['mute-for', { period: null, periods: 2 }, SHAPE_FOR],
    ['mute-for', { periods: 2 }, SHAPE_FOR],
    ['mute-until', { period: 'day', periods: 1 }, SHAPE_UNTIL],
    ['mute-until', { period: 'day', periods: 1, threshold: '10' }, SHAPE_UNTIL],
    ['mute-until', { period: 'day', periods: '1', threshold: 10 }, SHAPE_UNTIL],
  ])('refuses %s %j as a malformed body', (action, terms, message) => {
    expect(muteTermsError(action as MuteAction, terms as unknown as MuteTerms)).toBe(message);
  });
});
