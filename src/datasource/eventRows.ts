// SPDX-License-Identifier: GPL-3.0-or-later
import { DataFrame } from '@grafana/data';
import { EventRow } from './types';

const ISSUE_ID_PLACEHOLDER = '${__data.fields.issue_id}';
const ID_PLACEHOLDER = '${__data.fields.id}';

function values<T>(frame: DataFrame, name: string): T[] {
  const field = frame.fields.find((f) => f.name === name);
  return field ? Array.from<T>(field.values) : [];
}

export function eventRows(frame: DataFrame): EventRow[] {
  const text = (name: string) => values<string>(frame, name);
  const numbers = (name: string) => values<number>(frame, name);
  const template = frame.fields.find((f) => f.name === 'event_id')?.config.links?.[0]?.url;
  const eventId = text('event_id');
  const issue = text('issue');
  const issueId = text('issue_id');
  const project = text('project');
  const projectId = numbers('project_id');
  const timestamp = numbers('timestamp');
  const ingestedAt = numbers('ingested_at');

  return text('id').map((id, i): EventRow => {
    const row: EventRow = {
      id,
      eventId: eventId[i],
      issue: issue[i],
      issueId: issueId[i],
      project: project[i],
      projectId: projectId[i],
      timestamp: timestamp[i],
      ingestedAt: ingestedAt[i],
    };
    if (template) {
      row.url = template
        .replace(ISSUE_ID_PLACEHOLDER, encodeURIComponent(issueId[i]))
        .replace(ID_PLACEHOLDER, encodeURIComponent(id));
    }
    return row;
  });
}
