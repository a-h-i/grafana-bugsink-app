// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { dateTimeFormat } from '@grafana/data';
import { TimeZone } from '@grafana/schema';
import { Column, InteractiveTable } from '@grafana/ui';
import { Breadcrumb } from '../../datasource/types';
import { PAGE_SIZE } from '../issues/IssueTable';

const TIME_FORMAT = 'YYYY-MM-DD HH:mm:ss.SSS';

function messageText(breadcrumb: Breadcrumb): string {
  return breadcrumb.message || breadcrumb.data.map(({ key, value }) => `${key}=${value}`).join(', ');
}

export function Breadcrumbs({
  breadcrumbs,
  timeZone,
}: {
  breadcrumbs: Breadcrumb[];
  timeZone: TimeZone;
}): React.JSX.Element {
  if (breadcrumbs.length === 0) {
    return <p>No breadcrumbs.</p>;
  }

  const columns: Array<Column<Breadcrumb>> = [
    {
      id: 'time',
      header: 'Time',
      cell: ({ row }) => (
        <span>{row.original.timestamp ? dateTimeFormat(row.original.timestamp, { timeZone, format: TIME_FORMAT }) : ''}</span>
      ),
    },
    { id: 'type', header: 'Type', cell: ({ row }) => <span>{row.original.type}</span> },
    { id: 'category', header: 'Category', cell: ({ row }) => <span>{row.original.category}</span> },
    { id: 'level', header: 'Level', cell: ({ row }) => <span>{row.original.level}</span> },
    { id: 'message', header: 'Message', cell: ({ row }) => <span>{messageText(row.original)}</span> },
  ];

  return (
    <InteractiveTable
      columns={columns}
      data={breadcrumbs}
      getRowId={(_, index) => String(index)}
      pageSize={PAGE_SIZE}
    />
  );
}
