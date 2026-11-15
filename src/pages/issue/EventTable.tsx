// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { dateTimeFormat } from '@grafana/data';
import { TimeZone } from '@grafana/schema';
import { CellProps, Column, EmptyState, InteractiveTable, TextLink } from '@grafana/ui';
import { EventRow } from '../../datasource/types';
import { PAGE_SIZE } from '../issues/IssueTable';

const TIME_FORMAT = 'YYYY-MM-DD HH:mm:ss';

export function EventTable({ rows, timeZone }: { rows: EventRow[]; timeZone: TimeZone }): React.JSX.Element {
  if (rows.length === 0) {
    return <EmptyState variant="not-found" message="No stored events." />;
  }

  const time = (value: number) => dateTimeFormat(value, { timeZone, format: TIME_FORMAT });
  const columns: Array<Column<EventRow>> = [
    {
      id: 'eventId',
      header: 'Event ID',
      cell: ({ row }: CellProps<EventRow>) =>
        row.original.url ? (
          <TextLink href={row.original.url} external>
            {row.original.eventId}
          </TextLink>
        ) : (
          <span>{row.original.eventId}</span>
        ),
    },
    { id: 'timestamp', header: 'Timestamp', cell: ({ row }) => <span>{time(row.original.timestamp)}</span> },
    { id: 'ingestedAt', header: 'Ingested at', cell: ({ row }) => <span>{time(row.original.ingestedAt)}</span> },
  ];

  return <InteractiveTable columns={columns} data={rows} getRowId={(row) => row.id} pageSize={PAGE_SIZE} />;
}
