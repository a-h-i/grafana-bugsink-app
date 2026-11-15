// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { Alert, Column, InteractiveTable } from '@grafana/ui';
import { IssueTag, IssueTags as Tags } from '../../datasource/types';
import { PAGE_SIZE } from '../issues/IssueTable';

function valuesText(tag: IssueTag): string {
  const shown = tag.values.map(({ value, count }) => `${value} (${count})`).join(', ');
  if (tag.otherValues === 0) {
    return shown;
  }
  return `${shown} and ${tag.otherValues} more value${tag.otherValues === 1 ? '' : 's'}`;
}

const columns: Array<Column<IssueTag>> = [
  { id: 'key', header: 'Tag', cell: ({ row }) => <span>{row.original.key}</span> },
  { id: 'values', header: 'Values', cell: ({ row }) => <span>{valuesText(row.original)}</span> },
];

export function IssueTags({ tags }: { tags: Tags }): React.JSX.Element {
  return (
    <>
      {tags.truncated && <Alert severity="warning" title="Only the first 5000 tag values were read." />}
      {tags.tags.length === 0 ? (
        <p>No tags.</p>
      ) : (
        <InteractiveTable columns={columns} data={tags.tags} getRowId={(tag) => tag.key} pageSize={PAGE_SIZE} />
      )}
    </>
  );
}
