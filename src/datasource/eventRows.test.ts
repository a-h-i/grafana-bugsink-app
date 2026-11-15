// SPDX-License-Identifier: GPL-3.0-or-later
import { createDataFrame, DataFrame, FieldType } from '@grafana/data';
import { eventRows } from './eventRows';

const LINK = 'http://localhost:8800/issues/issue/${__data.fields.issue_id}/event/${__data.fields.id}/';

function frame(ids: string[], options: { link?: boolean; issueId?: string } = {}): DataFrame {
  const { link = true, issueId = 'issue-uuid' } = options;
  const n = ids.length;
  const pick = <T>(values: T[]) => values.slice(0, n);
  return createDataFrame({
    fields: [
      { name: 'timestamp', type: FieldType.time, values: pick([1000, 2000]) },
      {
        name: 'event_id',
        type: FieldType.string,
        values: pick(['sdk-1', 'sdk-2']),
        config: link ? { links: [{ title: 'Open in Bugsink', targetBlank: true, url: LINK }] } : {},
      },
      { name: 'issue', type: FieldType.string, values: pick(['SAMPLE-PYTHON-2', 'SAMPLE-PYTHON-2']) },
      { name: 'project', type: FieldType.string, values: pick(['sample-python', 'sample-python']) },
      { name: 'ingested_at', type: FieldType.time, values: pick([1500, 2500]) },
      { name: 'id', type: FieldType.string, values: ids },
      { name: 'issue_id', type: FieldType.string, values: pick([issueId, issueId]) },
      { name: 'project_id', type: FieldType.number, values: pick([1, 1]) },
    ],
  });
}

describe('eventRows', () => {
  it('maps the events frame to rows with the Bugsink link', () => {
    const rows = eventRows(frame(['event-uuid-1', 'event-uuid-2']));

    expect(rows).toEqual([
      {
        id: 'event-uuid-1',
        eventId: 'sdk-1',
        issue: 'SAMPLE-PYTHON-2',
        issueId: 'issue-uuid',
        project: 'sample-python',
        projectId: 1,
        timestamp: 1000,
        ingestedAt: 1500,
        url: 'http://localhost:8800/issues/issue/issue-uuid/event/event-uuid-1/',
      },
      expect.objectContaining({ id: 'event-uuid-2', eventId: 'sdk-2', timestamp: 2000, ingestedAt: 2500 }),
    ]);
  });

  it('encodes the ids in the link', () => {
    const [row] = eventRows(frame(['e/1'], { issueId: 'i/1' }));

    expect(row.url).toBe('http://localhost:8800/issues/issue/i%2F1/event/e%2F1/');
  });

  it('gives no url when the event field has no link', () => {
    expect(eventRows(frame(['e1'], { link: false }))[0].url).toBeUndefined();
  });

  it('gives no rows for a frame without values', () => {
    expect(eventRows(frame([]))).toEqual([]);
  });
});
