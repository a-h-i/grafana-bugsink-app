// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { TimeZone } from '@grafana/schema';
import { Alert, Stack } from '@grafana/ui';
import { HistoryDetails, HistoryEntry, IssueHistory as History } from '../../datasource/types';
import { formatTime, periodText } from '../format';

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

function threshold({ threshold, periods, period }: HistoryDetails): string | undefined {
  return threshold !== undefined && periods !== undefined && period
    ? `${threshold} or more events in ${periodText(periods, period)}`
    : undefined;
}

function label(entry: HistoryEntry, timeZone: TimeZone): string {
  const { kind, details } = entry;
  const volume = threshold(details);
  switch (kind) {
    case 'muted':
      if (volume) {
        return `Muted until ${volume}`;
      }
      if (details.periods !== undefined && details.period) {
        const until = details.unmuteAfter ? `, until ${formatTime(details.unmuteAfter, timeZone)}` : '';
        return `Muted for ${periodText(details.periods, details.period)}${until}`;
      }
      break;
    case 'resolved':
      if (details.release) {
        return `Resolved in release ${details.release}`;
      }
      if (details.nextRelease) {
        return 'Resolved in the next release';
      }
      break;
    case 'unmuted':
      if (volume) {
        return `Unmuted after ${volume}`;
      }
      break;
    case 'next_materialized':
      if (details.release) {
        return `Resolved in the next release: ${details.release}`;
      }
      break;
  }
  return KIND_LABELS[kind] ?? kind;
}

export function entryTitle(entry: HistoryEntry, timeZone: TimeZone): string {
  const text = label(entry, timeZone);
  return entry.user ? `${text} by ${entry.user}` : text;
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
                <time>{formatTime(entry.timestamp, timeZone)}</time> <strong>{entryTitle(entry, timeZone)}</strong>
              </div>
              {entry.comment && <div>{entry.comment}</div>}
            </div>
          ))}
        </Stack>
      )}
    </>
  );
}
