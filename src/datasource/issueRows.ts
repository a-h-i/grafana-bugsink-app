// SPDX-License-Identifier: GPL-3.0-or-later
import { DataFrame } from '@grafana/data';
import { IssueRow, IssueStatus } from './types';

const ID_PLACEHOLDER = '${__data.fields.id}';

function values<T>(frame: DataFrame, name: string): T[] {
  const field = frame.fields.find((f) => f.name === name);
  return field ? Array.from<T>(field.values) : [];
}

export function issueRows(frame: DataFrame): IssueRow[] {
  const text = (name: string) => values<string>(frame, name);
  const numbers = (name: string) => values<number>(frame, name);
  const template = frame.fields.find((f) => f.name === 'friendly_id')?.config.links?.[0]?.url;
  const friendlyId = text('friendly_id');
  const type = text('calculated_type');
  const message = text('calculated_value');
  const transaction = text('transaction');
  const project = text('project');
  const status = text('status');
  const projectId = numbers('project_id');
  const events = numbers('digested_event_count');
  const firstSeen = numbers('first_seen');
  const lastSeen = numbers('last_seen');

  return text('id').map((id, i): IssueRow => {
    const row: IssueRow = {
      id,
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
      row.url = template.replace(ID_PLACEHOLDER, encodeURIComponent(id));
    }
    return row;
  });
}
