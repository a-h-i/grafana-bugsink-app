// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { QueryEditorProps } from '@grafana/data';
import { BugsinkDataSource } from '../datasource';
import { HOURS_MESSAGE, wholeNumberError } from '../counts';
import { BugsinkOptions, BugsinkQuery } from '../types';
import { CommitInput } from './CommitInput';
import { ProjectPicker } from './ProjectPicker';

type Props = QueryEditorProps<BugsinkDataSource, BugsinkQuery, BugsinkOptions>;

export function EventCountQueryEditor({ query, onChange, onRunQuery, datasource }: Props) {
  const onProjectChange = (project?: number) => {
    const { project: _removed, ...rest } = query;
    onChange(project === undefined ? rest : { ...rest, project });
    onRunQuery();
  };

  const onHoursCommit = (text: string) => {
    const { hours: _removed, ...rest } = query;
    onChange(text === '' ? rest : { ...rest, hours: Number(text) });
  };

  return (
    <>
      <ProjectPicker
        datasource={datasource}
        value={query.project}
        onChange={onProjectChange}
        emptyLabel="No project"
        description={!query.project && !query.issue ? 'An event count needs a project or an issue.' : undefined}
      />
      <CommitInput
        label="Issue"
        value={query.issue}
        placeholder="SAMPLE-PYTHON-2"
        onCommit={(issue) => onChange({ ...query, issue })}
        onRunQuery={onRunQuery}
      />
      <CommitInput
        label="Hours"
        value={query.hours === undefined ? undefined : String(query.hours)}
        placeholder="1"
        description="The hour in progress plus the full hours before it."
        validate={(text) => wholeNumberError(text, 1, 24, HOURS_MESSAGE)}
        onCommit={onHoursCommit}
        onRunQuery={onRunQuery}
      />
    </>
  );
}
