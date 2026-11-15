// SPDX-License-Identifier: GPL-3.0-or-later
import React, { KeyboardEvent, useState } from 'react';
import { Field, Input } from '@grafana/ui';

interface Props {
  label: string;
  value?: string;
  placeholder?: string;
  onCommit: (value: string) => void;
  onRunQuery: () => void;
}

export function CommitInput({ label, value, placeholder, onCommit, onRunQuery }: Props) {
  const [draft, setDraft] = useState<string>();
  const text = draft ?? value ?? '';

  const commit = () => {
    setDraft(undefined);
    if (text !== (value ?? '')) {
      onCommit(text);
    }
  };

  const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter') {
      commit();
      onRunQuery();
    }
  };

  return (
    <Field label={label}>
      <Input
        aria-label={label}
        placeholder={placeholder}
        value={text}
        onChange={(event) => setDraft(event.currentTarget.value)}
        onBlur={commit}
        onKeyDown={onKeyDown}
      />
    </Field>
  );
}
