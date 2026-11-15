// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { dateTimeFormat } from '@grafana/data';
import { TimeZone } from '@grafana/schema';
import { CellProps, Column, EmptyState, InteractiveTable, TextLink } from '@grafana/ui';
import { IssueRow, IssueStatus } from '../../datasource/types';

export const PAGE_SIZE = 25;

const STATUS_LABELS: Record<IssueStatus, string> = { open: 'Open', muted: 'Muted', resolved: 'Resolved' };
const TIME_FORMAT = 'YYYY-MM-DD HH:mm:ss';

export function IssueTable({ rows, timeZone }: { rows: IssueRow[]; timeZone: TimeZone }): React.JSX.Element {
  if (rows.length === 0) {
    return <EmptyState variant="not-found" message="No issues match these filters in this time range." />;
  }

  const time = (value: number) => dateTimeFormat(value, { timeZone, format: TIME_FORMAT });
  const columns: Array<Column<IssueRow>> = [
    {
      id: 'friendlyId',
      header: 'Issue',
      cell: ({ row }: CellProps<IssueRow>) =>
        row.original.url ? (
          <TextLink href={row.original.url} external>
            {row.original.friendlyId}
          </TextLink>
        ) : (
          <span>{row.original.friendlyId}</span>
        ),
    },
    { id: 'type', header: 'Type', cell: ({ row }) => <span>{row.original.type}</span> },
    { id: 'message', header: 'Message', cell: ({ row }) => <span>{row.original.message}</span> },
    { id: 'transaction', header: 'Transaction', cell: ({ row }) => <span>{row.original.transaction}</span> },
    { id: 'project', header: 'Project', cell: ({ row }) => <span>{row.original.project}</span> },
    { id: 'events', header: 'Events', cell: ({ row }) => <span>{row.original.events}</span> },
    { id: 'firstSeen', header: 'First seen', cell: ({ row }) => <span>{time(row.original.firstSeen)}</span> },
    { id: 'lastSeen', header: 'Last seen', cell: ({ row }) => <span>{time(row.original.lastSeen)}</span> },
    { id: 'status', header: 'Status', cell: ({ row }) => <span>{STATUS_LABELS[row.original.status]}</span> },
  ];

  return <InteractiveTable columns={columns} data={rows} getRowId={(row) => row.id} pageSize={PAGE_SIZE} />;
}
