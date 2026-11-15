// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { dateTimeFormat } from '@grafana/data';
import { TimeZone } from '@grafana/schema';
import { CellProps, Column, EmptyState, InteractiveTable, LinkButton, Stack } from '@grafana/ui';
import { Link } from 'react-router-dom';
import { IssueRow, IssueStatus } from '../../datasource/types';

export const PAGE_SIZE = 25;

const STATUS_LABELS: Record<IssueStatus, string> = { open: 'Open', muted: 'Muted', resolved: 'Resolved' };
const TIME_FORMAT = 'YYYY-MM-DD HH:mm:ss';

interface IssueTableProps {
  rows: IssueRow[];
  timeZone: TimeZone;
  issueHref: (row: IssueRow) => string;
}

export function IssueTable({ rows, timeZone, issueHref }: IssueTableProps): React.JSX.Element {
  if (rows.length === 0) {
    return <EmptyState variant="not-found" message="No issues match these filters in this time range." />;
  }

  const time = (value: number) => dateTimeFormat(value, { timeZone, format: TIME_FORMAT });
  const columns: Array<Column<IssueRow>> = [
    {
      id: 'friendlyId',
      header: 'Issue',
      cell: ({ row }: CellProps<IssueRow>) => (
        <Stack alignItems="center" gap={0.5}>
          <Link to={issueHref(row.original)}>{row.original.friendlyId}</Link>
          {row.original.url && (
            <LinkButton
              href={row.original.url}
              target="_blank"
              rel="noreferrer"
              icon="external-link-alt"
              variant="secondary"
              fill="text"
              size="sm"
              aria-label={`Open ${row.original.friendlyId} in Bugsink`}
            />
          )}
        </Stack>
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
