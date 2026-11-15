// SPDX-License-Identifier: GPL-3.0-or-later
import { HOURS_MESSAGE, MINUTES_MESSAGE, wholeNumberError } from './counts';

describe('wholeNumberError', () => {
  it('accepts an empty field and the whole numbers from 1 to the maximum', () => {
    for (const text of ['', '1', '60', '1440']) {
      expect(wholeNumberError(text, 1, 1440, MINUTES_MESSAGE)).toBeUndefined();
    }
  });

  it('refuses both sides of the bounds, fractions, text and padded text', () => {
    for (const text of ['0', '1441', '-1', '2.5', '5m', ' 5', '5 ', 'abc', '1e2', '+5']) {
      expect(wholeNumberError(text, 1, 1440, MINUTES_MESSAGE)).toBe('The minutes must be a whole number from 1 to 1440.');
    }
  });

  it('uses the hours bounds and message', () => {
    expect(wholeNumberError('24', 1, 24, HOURS_MESSAGE)).toBeUndefined();
    expect(wholeNumberError('25', 1, 24, HOURS_MESSAGE)).toBe('The hours must be a whole number from 1 to 24.');
    expect(wholeNumberError('0', 1, 24, HOURS_MESSAGE)).toBe(HOURS_MESSAGE);
  });
});
