// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { dateTimeFormat } from '@grafana/data';
import { TimeZone } from '@grafana/schema';
import { IssueDetail, IssueStatus } from '../../datasource/types';

const STATUS_LABELS: Record<IssueStatus, string> = { open: 'Open', muted: 'Muted', resolved: 'Resolved' };
const TIME_FORMAT = 'YYYY-MM-DD HH:mm:ss';

export function IssueSummary({ issue, timeZone }: { issue: IssueDetail; timeZone: TimeZone }): React.JSX.Element {
  const time = (value: string) => dateTimeFormat(value, { timeZone, format: TIME_FORMAT });
  const rows: Array<[string, string]> = [
    ['Status', STATUS_LABELS[issue.status]],
    ['Project', issue.project],
    ['Transaction', issue.transaction],
    ['Events', String(issue.events)],
    ['Stored events', String(issue.storedEvents)],
    ['First seen', time(issue.firstSeen)],
    ['Last seen', time(issue.lastSeen)],
    ...(issue.unmuteAfter ? [['Unmute after', time(issue.unmuteAfter)] as [string, string]] : []),
    ...(issue.isResolvedByNextRelease ? [['Resolved by next release', 'Yes'] as [string, string]] : []),
  ];

  return (
    <section>
      <h2>{issue.message ? `${issue.type}: ${issue.message}` : issue.type}</h2>
      <table>
        <tbody>
          {rows.map(([label, value]) => (
            <tr key={label}>
              <th scope="row" style={{ textAlign: 'left', paddingRight: 24 }}>
                {label}
              </th>
              <td>{value}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}
