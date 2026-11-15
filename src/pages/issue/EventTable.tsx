// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { TimeZone } from '@grafana/schema';
import { CellProps, Column, InteractiveTable, LinkButton, Stack, Text } from '@grafana/ui';
import { Link } from 'react-router-dom';
import { EventRow } from '../../datasource/types';
import { formatTime } from '../format';
import { PAGE_SIZE } from '../issues/IssueTable';

interface EventTableProps {
  rows: EventRow[];
  timeZone: TimeZone;
  selectedId?: string;
  eventHref: (row: EventRow) => string;
}

export function EventTable({ rows, timeZone, selectedId, eventHref }: EventTableProps): React.JSX.Element {
  if (rows.length === 0) {
    return <p>No stored events.</p>;
  }

  const time = (value: number) => formatTime(value, timeZone);
  const columns: Array<Column<EventRow>> = [
    {
      id: 'eventId',
      header: 'Event ID',
      cell: ({ row }: CellProps<EventRow>) => (
        <Stack alignItems="center" gap={0.5}>
          {row.original.id === selectedId ? (
            <Text weight="bold">{row.original.eventId}</Text>
          ) : (
            <Link to={eventHref(row.original)}>{row.original.eventId}</Link>
          )}
          {row.original.url && (
            <LinkButton
              href={row.original.url}
              target="_blank"
              rel="noreferrer"
              icon="external-link-alt"
              variant="secondary"
              fill="text"
              size="sm"
              aria-label={`Open event ${row.original.eventId} in Bugsink`}
            />
          )}
        </Stack>
      ),
    },
    { id: 'timestamp', header: 'Timestamp', cell: ({ row }) => <span>{time(row.original.timestamp)}</span> },
    { id: 'ingestedAt', header: 'Ingested at', cell: ({ row }) => <span>{time(row.original.ingestedAt)}</span> },
  ];

  return <InteractiveTable columns={columns} data={rows} getRowId={(row) => row.id} pageSize={PAGE_SIZE} />;
}
