// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { TimeZone } from '@grafana/schema';
import { Alert, Stack } from '@grafana/ui';
import { HistoryEntry, IssueHistory as History } from '../../datasource/types';
import { formatTime } from '../format';

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
        <Stack direction="column" gap={1} role="list">
          {history.history.map((entry) => (
            <div key={entry.id} role="listitem">
              <div>
                <time>{formatTime(entry.timestamp, timeZone)}</time>{' '}
                <strong>{describe(entry)}</strong>
              </div>
              {entry.comment && <div>{entry.comment}</div>}
            </div>
          ))}
        </Stack>
      )}
    </>
  );
}
