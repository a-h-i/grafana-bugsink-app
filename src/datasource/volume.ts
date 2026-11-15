// SPDX-License-Identifier: GPL-3.0-or-later
import { DataFrame } from '@grafana/data';
import { VolumeSeries } from './types';

export function volumeSeries(frame: DataFrame): VolumeSeries {
  const times = frame.fields.find((field) => field.name === 'time')?.values ?? [];
  const counts = frame.fields.find((field) => field.name === 'count')?.values ?? [];
  return { times: [...times], counts: [...counts], total: counts.reduce((sum, count) => sum + count, 0) };
}
