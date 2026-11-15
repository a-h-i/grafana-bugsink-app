// SPDX-License-Identifier: GPL-3.0-or-later
import { DataFrame, Field } from '@grafana/data';
import { IssueRow, IssueStatus } from './types';

const ID_PLACEHOLDER = '${__data.fields.id}';

function values<T>(frame: DataFrame, name: string): T[] {
  const field: Field | undefined = frame.fields.find((f) => f.name === name);
  return field ? Array.from(field.values) : [];
}

export function issueRows(frame: DataFrame): IssueRow[] {
  const col = (name: string) => values<any>(frame, name);
  const template = frame.fields.find((f) => f.name === 'friendly_id')?.config.links?.[0]?.url;
  const [friendlyId, type, message, transaction, project, projectId, status, events, firstSeen, lastSeen, id] = [
    'friendly_id',
    'calculated_type',
    'calculated_value',
    'transaction',
    'project',
    'project_id',
    'status',
    'digested_event_count',
    'first_seen',
    'last_seen',
    'id',
  ].map(col);

  return id.map((rowId: string, i: number): IssueRow => {
    const row: IssueRow = {
      id: rowId,
      friendlyId: friendlyId[i],
      type: type[i],
      message: message[i],
      transaction: transaction[i],
      project: project[i],
      projectId: projectId[i],
      status: status[i] as IssueStatus,
      events: events[i],
      firstSeen: firstSeen[i],
      lastSeen: lastSeen[i],
    };
    if (template) {
      row.url = template.replace(ID_PLACEHOLDER, encodeURIComponent(rowId));
    }
    return row;
  });
}
