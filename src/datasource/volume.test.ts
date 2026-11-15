// SPDX-License-Identifier: GPL-3.0-or-later
import { createDataFrame, FieldType } from '@grafana/data';
import { volumeSeries } from './volume';

describe('volumeSeries', () => {
  it('reads the times, the counts and their total', () => {
    const frame = createDataFrame({
      fields: [
        { name: 'time', type: FieldType.time, values: [1000, 2000] },
        { name: 'count', type: FieldType.number, values: [2, 5] },
      ],
    });

    expect(volumeSeries(frame)).toEqual({ times: [1000, 2000], counts: [2, 5], total: 7 });
  });

  it('is empty for a frame without those fields', () => {
    const frame = createDataFrame({ fields: [{ name: 'other', type: FieldType.number, values: [1] }] });

    expect(volumeSeries(frame)).toEqual({ times: [], counts: [], total: 0 });
  });
});
