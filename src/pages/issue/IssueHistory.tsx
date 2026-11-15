// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { dateTimeFormat } from '@grafana/data';
import { TimeZone } from '@grafana/schema';
import { Alert } from '@grafana/ui';
import { HistoryEntry, IssueHistory as History } from '../../datasource/types';

const KIND_LABELS: Record<string, string> = {
  first_seen: 'First seen',
  resolved: 'Resolved',
  muted: 'Muted',
  regressed: 'Regressed',
  unmuted: 'Unmuted',
  reopened: 'Reopened',
  next_materialized: 'Resolved in the next release',
  manual_annotation: 'Comment',
};
const TIME_FORMAT = 'YYYY-MM-DD HH:mm:ss';

function describe(entry: HistoryEntry): string {
  const label = KIND_LABELS[entry.kind] ?? entry.kind;
  return entry.user ? `${label} by ${entry.user}` : label;
}

export function IssueHistory({ history, timeZone }: { history: History; timeZone: TimeZone }): React.JSX.Element {
  return (
    <>
      {history.truncated && <Alert severity="warning" title="Only the newest 5000 history entries were read." />}
      {history.history.length === 0 ? (
        <p>No history.</p>
      ) : (
        <ul style={{ listStyle: 'none', padding: 0 }}>
          {history.history.map((entry) => (
            <li key={entry.id} style={{ marginBottom: 8 }}>
              <div>
                <time>{dateTimeFormat(entry.timestamp, { timeZone, format: TIME_FORMAT })}</time>{' '}
                <strong>{describe(entry)}</strong>
              </div>
              {entry.comment && <div>{entry.comment}</div>}
            </li>
          ))}
        </ul>
      )}
    </>
  );
}
