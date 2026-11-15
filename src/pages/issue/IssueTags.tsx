// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { Alert } from '@grafana/ui';
import { IssueTag, IssueTags as Tags } from '../../datasource/types';

function valuesText(tag: IssueTag): string {
  const shown = tag.values.map(({ value, count }) => `${value} (${count})`).join(', ');
  if (tag.otherValues === 0) {
    return shown;
  }
  return `${shown} and ${tag.otherValues} more value${tag.otherValues === 1 ? '' : 's'}`;
}

export function IssueTags({ tags }: { tags: Tags }): React.JSX.Element {
  return (
    <>
      {tags.truncated && <Alert severity="warning" title="Only the first 5000 tag values were read." />}
      {tags.tags.length === 0 ? (
        <p>No tags.</p>
      ) : (
        <table>
          <thead>
            <tr>
              <th style={{ textAlign: 'left', paddingRight: 24 }}>Tag</th>
              <th style={{ textAlign: 'left' }}>Values</th>
            </tr>
          </thead>
          <tbody>
            {tags.tags.map((tag) => (
              <tr key={tag.key}>
                <td style={{ paddingRight: 24 }}>{tag.key}</td>
                <td>{valuesText(tag)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  );
}
