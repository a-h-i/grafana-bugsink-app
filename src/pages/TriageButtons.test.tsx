// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { TriageButtons, triageActions } from './TriageButtons';

describe('triageActions', () => {
  it('offers the actions the status allows', () => {
    expect(triageActions('open')).toEqual([
      'resolve',
      'resolve-latest',
      'resolve-next',
      'mute',
      'mute-for',
      'mute-until',
    ]);
    expect(triageActions('muted')).toEqual(['resolve', 'resolve-latest', 'resolve-next', 'unmute']);
    expect(triageActions('resolved')).toEqual(['reopen']);
  });
});

describe('TriageButtons', () => {
  it('calls onAction with the clicked action', () => {
    const onAction = jest.fn();
    render(<TriageButtons actions={['resolve', 'mute']} busy={false} onAction={onAction} />);

    fireEvent.click(screen.getByRole('button', { name: 'Mute' }));

    expect(onAction).toHaveBeenCalledTimes(1);
    expect(onAction).toHaveBeenCalledWith('mute');
  });

  it('disables every button while busy', () => {
    const onAction = jest.fn();
    render(<TriageButtons actions={['resolve', 'mute']} busy onAction={onAction} />);

    for (const button of screen.getAllByRole('button')) {
      expect(button).toBeDisabled();
    }
    fireEvent.click(screen.getByRole('button', { name: 'Resolve' }));
    expect(onAction).not.toHaveBeenCalled();
  });

  it('names the buttons with name', () => {
    render(
      <TriageButtons actions={['reopen']} busy={false} onAction={jest.fn()} name={(a) => `${a} SAMPLE-PYTHON-2`} />
    );

    expect(screen.getByRole('button', { name: 'reopen SAMPLE-PYTHON-2' })).toBeInTheDocument();
  });
});
