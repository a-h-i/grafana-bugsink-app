// SPDX-License-Identifier: GPL-3.0-or-later
import { createDataFrame, DataFrame, FieldType } from '@grafana/data';
import { issueRows } from './issueRows';

const LINK = 'http://localhost:8800/issues/issue/${__data.fields.id}/event/last/';

function frame(ids: string[], withLink = true): DataFrame {
  const n = ids.length;
  const pick = <T>(values: T[]) => values.slice(0, n);
  return createDataFrame({
    fields: [
      {
        name: 'friendly_id',
        type: FieldType.string,
        values: pick(['SAMPLE-1', 'SAMPLE-2', 'SAMPLE-3']),
        config: withLink ? { links: [{ title: 'Open in Bugsink', targetBlank: true, url: LINK }] } : {},
      },
      { name: 'calculated_type', type: FieldType.string, values: pick(['ValueError', 'KeyError', 'TypeError']) },
      { name: 'calculated_value', type: FieldType.string, values: pick(['bad value', 'missing key', 'bad type']) },
      { name: 'transaction', type: FieldType.string, values: pick(['/a', '/b', '/c']) },
      { name: 'project', type: FieldType.string, values: pick(['sample-python', 'sample-js', '7']) },
      { name: 'status', type: FieldType.string, values: pick(['open', 'muted', 'resolved']) },
      { name: 'digested_event_count', type: FieldType.number, values: pick([106, 22, 3]) },
      { name: 'first_seen', type: FieldType.time, values: pick([1000, 2000, 3000]) },
      { name: 'last_seen', type: FieldType.time, values: pick([4000, 5000, 6000]) },
      { name: 'project_id', type: FieldType.number, values: pick([1, 2, 7]) },
      { name: 'id', type: FieldType.string, values: ids },
    ],
  });
}

describe('issueRows', () => {
  it('maps an open, a muted and a resolved issue with every field', () => {
    const rows = issueRows(frame(['id-1', 'id-2', 'id-3']));

    expect(rows).toEqual([
      {
        id: 'id-1',
        friendlyId: 'SAMPLE-1',
        type: 'ValueError',
        message: 'bad value',
        transaction: '/a',
        project: 'sample-python',
        projectId: 1,
        status: 'open',
        events: 106,
        firstSeen: 1000,
        lastSeen: 4000,
        url: 'http://localhost:8800/issues/issue/id-1/event/last/',
      },
      expect.objectContaining({ id: 'id-2', status: 'muted', project: 'sample-js', projectId: 2, events: 22 }),
      expect.objectContaining({ id: 'id-3', status: 'resolved', project: '7', projectId: 7, lastSeen: 6000 }),
    ]);
  });

  it('encodes the id in the link', () => {
    expect(issueRows(frame(['a/b']))[0].url).toBe('http://localhost:8800/issues/issue/a%2Fb/event/last/');
  });

  it('gives no url when the issue field has no link', () => {
    expect(issueRows(frame(['id-1'], false))[0].url).toBeUndefined();
  });

  it('gives no rows for a frame without values', () => {
    expect(issueRows(frame([]))).toEqual([]);
  });
});
