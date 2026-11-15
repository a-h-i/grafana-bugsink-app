// SPDX-License-Identifier: GPL-3.0-or-later
import React, { useState } from 'react';
import { Button, Field, Input, Modal, RadioButtonGroup } from '@grafana/ui';
import { muteTermsError } from '../datasource/muteTerms';
import { MuteAction, MuteTerms, Period } from '../datasource/types';
import { ACTION_LABELS } from './TriageButtons';

const PERIOD_OPTIONS: Array<{ label: string; value: Period }> = [
  { label: 'Minutes', value: 'minute' },
  { label: 'Hours', value: 'hour' },
  { label: 'Days', value: 'day' },
  { label: 'Weeks', value: 'week' },
  { label: 'Months', value: 'month' },
  { label: 'Years', value: 'year' },
];

interface MuteFormProps {
  action: MuteAction;
  onSubmit: (terms: MuteTerms) => void;
  onDismiss: () => void;
}

export function MuteForm({ action, onSubmit, onDismiss }: MuteFormProps): React.JSX.Element {
  const [periods, setPeriods] = useState('1');
  const [period, setPeriod] = useState<Period>('day');
  const [events, setEvents] = useState('10');
  const withThreshold = action === 'mute-until';
  const terms: MuteTerms = withThreshold
    ? { period, periods: Number(periods), threshold: Number(events) }
    : { period, periods: Number(periods) };
  const periodsError = muteTermsError(action, { ...terms, threshold: 1 });
  const thresholdError = withThreshold ? muteTermsError(action, { ...terms, periods: 1 }) : undefined;
  const invalid = muteTermsError(action, terms) !== undefined;

  return (
    <Modal title={ACTION_LABELS[action]} isOpen onDismiss={onDismiss}>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          if (!invalid) {
            onSubmit(terms);
          }
        }}
      >
        {withThreshold && (
          <Field label="Events" invalid={thresholdError !== undefined} error={thresholdError}>
            <Input type="number" value={events} onChange={(event) => setEvents(event.currentTarget.value)} />
          </Field>
        )}
        <Field label="Number of periods" invalid={periodsError !== undefined} error={periodsError}>
          <Input type="number" value={periods} onChange={(event) => setPeriods(event.currentTarget.value)} />
        </Field>
        <Field label="Period" useFieldset>
          <RadioButtonGroup options={PERIOD_OPTIONS} value={period} onChange={setPeriod} />
        </Field>
        <Modal.ButtonRow>
          <Button type="button" variant="secondary" fill="outline" onClick={onDismiss}>
            Cancel
          </Button>
          <Button type="submit" disabled={invalid}>
            Mute
          </Button>
        </Modal.ButtonRow>
      </form>
    </Modal>
  );
}
