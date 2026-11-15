// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { Button, ComponentSize, Stack } from '@grafana/ui';
import { IssueStatus, TriageAction } from '../datasource/types';

export const ACTION_LABELS: Record<TriageAction, string> = {
  resolve: 'Resolve',
  reopen: 'Reopen',
  mute: 'Mute',
  unmute: 'Unmute',
};

export function triageActions(status: IssueStatus): TriageAction[] {
  switch (status) {
    case 'open':
      return ['resolve', 'mute'];
    case 'muted':
      return ['resolve', 'unmute'];
    case 'resolved':
      return ['reopen'];
  }
}

interface TriageButtonsProps {
  actions: TriageAction[];
  busy: boolean;
  onAction: (action: TriageAction) => void;
  name?: (action: TriageAction) => string;
  size?: ComponentSize;
}

export function TriageButtons({ actions, busy, onAction, name, size = 'sm' }: TriageButtonsProps): React.JSX.Element {
  return (
    <Stack gap={0.5}>
      {actions.map((action) => (
        <Button
          key={action}
          variant="secondary"
          size={size}
          disabled={busy}
          aria-label={name?.(action)}
          onClick={() => onAction(action)}
        >
          {ACTION_LABELS[action]}
        </Button>
      ))}
    </Stack>
  );
}
