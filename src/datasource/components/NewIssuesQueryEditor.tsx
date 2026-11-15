// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { QueryEditorProps } from '@grafana/data';
import { BugsinkDataSource } from '../datasource';
import { BugsinkOptions, BugsinkQuery } from '../types';
import { CommitInput } from './CommitInput';
import { ProjectPicker } from './ProjectPicker';

type Props = QueryEditorProps<BugsinkDataSource, BugsinkQuery, BugsinkOptions>;

export function NewIssuesQueryEditor({ query, onChange, onRunQuery, datasource }: Props) {
  const onProjectChange = (project?: number) => {
    const { project: _removed, ...rest } = query;
    onChange(project === undefined ? rest : { ...rest, project });
    onRunQuery();
  };

  return (
    <>
      <ProjectPicker datasource={datasource} value={query.project} onChange={onProjectChange} />
      <CommitInput
        label="Search"
        value={query.q}
        placeholder="environment:production release:1.4.2"
        onCommit={(q) => onChange({ ...query, q })}
        onRunQuery={onRunQuery}
      />
    </>
  );
}
