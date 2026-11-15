// SPDX-License-Identifier: GPL-3.0-or-later
export const MINUTES_MESSAGE = 'The minutes must be a whole number from 1 to 1440.';
export const HOURS_MESSAGE = 'The hours must be a whole number from 1 to 24.';

export function wholeNumberError(text: string, min: number, max: number, message: string): string | undefined {
  if (text === '') {
    return undefined;
  }
  const n = /^[0-9]+$/.test(text) ? Number(text) : NaN;
  return n >= min && n <= max ? undefined : message;
}
