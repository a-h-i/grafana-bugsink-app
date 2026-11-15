// SPDX-License-Identifier: GPL-3.0-or-later
import { dateTimeFormat } from '@grafana/data';
import { TimeZone } from '@grafana/schema';
import { IssueStatus } from '../datasource/types';

const TIME_FORMAT = 'YYYY-MM-DD HH:mm:ss';

export const STATUS_LABELS: Record<IssueStatus, string> = { open: 'Open', muted: 'Muted', resolved: 'Resolved' };

export function formatTime(value: number | string, timeZone: TimeZone): string {
  return dateTimeFormat(value, { timeZone, format: TIME_FORMAT });
}
