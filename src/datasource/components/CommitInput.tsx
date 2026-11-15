// SPDX-License-Identifier: GPL-3.0-or-later
import React, { KeyboardEvent, useState } from 'react';
import { Field, Input } from '@grafana/ui';

interface Props {
  label: string;
  value?: string;
  placeholder?: string;
  description?: string;
  validate?: (text: string) => string | undefined;
  onCommit: (value: string) => void;
  onRunQuery: () => void;
}

export function CommitInput({ label, value, placeholder, description, validate, onCommit, onRunQuery }: Props) {
  const [draft, setDraft] = useState<string>();
  const text = draft ?? value ?? '';
  const error = validate?.(text);

  const commit = () => {
    if (error) {
      return;
    }
    setDraft(undefined);
    if (text !== (value ?? '')) {
      onCommit(text);
    }
  };

  const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter' && !error) {
      commit();
      onRunQuery();
    }
  };

  return (
    <Field label={label} description={description} invalid={Boolean(error)} error={error}>
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
