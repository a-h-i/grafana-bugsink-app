// SPDX-License-Identifier: GPL-3.0-or-later
import { MuteAction, MuteTerms, PERIODS } from './types';

const MAX_PERIODS = 100;
const MAX_THRESHOLD = 1000000;
export const SHAPE_MESSAGE: Record<MuteAction, string> = {
  'mute-for': 'The request body must be a JSON object with period and periods.',
  'mute-until': 'The request body must be a JSON object with period, periods and threshold.',
};

function wholeNumberIn(n: number, max: number): boolean {
  return Number.isInteger(n) && n >= 1 && n <= max;
}

// muteTermsError is the refusal message for the terms of a mute action, or undefined when they are valid.
export function muteTermsError(action: MuteAction, terms: MuteTerms): string | undefined {
  const { period, periods, threshold } = terms;
  const withThreshold = action === 'mute-until';
  if (typeof period !== 'string' || typeof periods !== 'number' || (withThreshold && typeof threshold !== 'number')) {
    return SHAPE_MESSAGE[action];
  }
  if (!PERIODS.includes(period)) {
    return 'The period must be minute, hour, day, week, month or year.';
  }
  if (!wholeNumberIn(periods, MAX_PERIODS)) {
    return `The number of periods must be a whole number from 1 to ${MAX_PERIODS}.`;
  }
  if (withThreshold && !wholeNumberIn(threshold ?? 0, MAX_THRESHOLD)) {
    return `The threshold must be a whole number from 1 to ${MAX_THRESHOLD}.`;
  }
  return undefined;
}
