// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { QueryEditorProps } from '@grafana/data';
import { BugsinkDataSource } from '../datasource';
import { BugsinkOptions, BugsinkQuery } from '../types';
import { CommitInput } from './CommitInput';
import { ProjectPicker } from './ProjectPicker';

type Props = QueryEditorProps<BugsinkDataSource, BugsinkQuery, BugsinkOptions>;

export function EventsQueryEditor({ query, onChange, onRunQuery, datasource }: Props) {
  const onProjectChange = (project?: number) => {
    const { project: _removed, ...rest } = query;
    onChange(project === undefined ? rest : { ...rest, project });
    onRunQuery();
  };

  return (
    <>
      <ProjectPicker datasource={datasource} value={query.project} onChange={onProjectChange} />
      <CommitInput
        label="Issue"
        value={query.issue}
        placeholder="SAMPLE-PYTHON-2"
        onCommit={(issue) => onChange({ ...query, issue })}
        onRunQuery={onRunQuery}
      />
      <CommitInput
        label="Search"
        value={query.q}
        placeholder="trace:4bf92f3577b34da6a3ce929d0e0e4736"
        onCommit={(q) => onChange({ ...query, q })}
        onRunQuery={onRunQuery}
      />
      <CommitInput
        label="Event ID"
        value={query.eventId}
        placeholder="426ac644fafb475d905046d654730f1b"
        onCommit={(eventId) => onChange({ ...query, eventId })}
        onRunQuery={onRunQuery}
      />
    </>
  );
}
