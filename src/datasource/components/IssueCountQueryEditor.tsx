// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { QueryEditorProps } from '@grafana/data';
import { Combobox, ComboboxOption, Field, InlineFieldRow } from '@grafana/ui';
import { BugsinkDataSource } from '../datasource';
import { MINUTES_MESSAGE, wholeNumberError } from '../counts';
import { BugsinkOptions, BugsinkQuery, DEFAULT_QUERY, IssueState } from '../types';
import { CommitInput } from './CommitInput';
import { ProjectPicker } from './ProjectPicker';

type Props = QueryEditorProps<BugsinkDataSource, BugsinkQuery, BugsinkOptions>;

const STATE_OPTIONS: Array<ComboboxOption<IssueState>> = [
  { value: 'all', label: 'All' },
  { value: 'open', label: 'Open' },
  { value: 'unresolved', label: 'Unresolved' },
  { value: 'resolved', label: 'Resolved' },
  { value: 'muted', label: 'Muted' },
];

export function IssueCountQueryEditor({ query, onChange, onRunQuery, datasource }: Props) {
  const onProjectChange = (project?: number) => {
    const { project: _removed, ...rest } = query;
    onChange(project === undefined ? rest : { ...rest, project });
    onRunQuery();
  };

  const onMinutesCommit = (text: string) => {
    const { minutes: _removed, ...rest } = query;
    onChange(text === '' ? rest : { ...rest, minutes: Number(text) });
  };

  return (
    <>
      <InlineFieldRow>
        <ProjectPicker datasource={datasource} value={query.project} onChange={onProjectChange} />
        <Field label="State">
          <Combobox
            aria-label="State"
            width={20}
            options={STATE_OPTIONS}
            value={query.state ?? DEFAULT_QUERY.state}
            onChange={({ value }) => {
              onChange({ ...query, state: value });
              onRunQuery();
            }}
          />
        </Field>
      </InlineFieldRow>
      <CommitInput
        label="Search"
        value={query.q}
        placeholder="environment:production release:1.4.2"
        onCommit={(q) => onChange({ ...query, q })}
        onRunQuery={onRunQuery}
      />
      <CommitInput
        label="Seen within (minutes)"
        value={query.minutes === undefined ? undefined : String(query.minutes)}
        placeholder="any time"
        validate={(text) => wholeNumberError(text, 1, 1440, MINUTES_MESSAGE)}
        onCommit={onMinutesCommit}
        onRunQuery={onRunQuery}
      />
    </>
  );
}
