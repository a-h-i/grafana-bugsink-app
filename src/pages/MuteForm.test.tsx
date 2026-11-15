// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { MuteAction } from '../datasource/types';
import { MuteForm } from './MuteForm';

const PERIODS_MESSAGE = 'The number of periods must be a whole number from 1 to 100.';
const THRESHOLD_MESSAGE = 'The threshold must be a whole number from 1 to 1000000.';

function renderForm(action: MuteAction) {
  const onSubmit = jest.fn();
  const onDismiss = jest.fn();
  render(<MuteForm action={action} onSubmit={onSubmit} onDismiss={onDismiss} />);
  return { onSubmit, onDismiss };
}

const muteButton = () => screen.getByRole('button', { name: 'Mute' });
const periods = (value: string) => fireEvent.change(screen.getByLabelText('Number of periods'), { target: { value } });

describe('MuteForm', () => {
  it('asks for a number of periods and a period when muting for a period', () => {
    renderForm('mute-for');

    expect(screen.getByRole('dialog', { name: 'Mute for a period' })).toBeInTheDocument();
    expect(screen.getByLabelText('Number of periods')).toHaveValue(1);
    expect(screen.getByLabelText('Days')).toBeChecked();
    expect(screen.queryByLabelText('Events')).not.toBeInTheDocument();
  });

  it('submits the terms it shows', () => {
    const { onSubmit } = renderForm('mute-for');

    periods('2');
    fireEvent.click(screen.getByLabelText('Hours'));
    fireEvent.click(muteButton());

    expect(onSubmit).toHaveBeenCalledWith({ period: 'hour', periods: 2 });
  });

  it.each(['0', '101', '2.5'])('refuses %s periods and disables Mute', (value) => {
    const { onSubmit } = renderForm('mute-for');

    periods(value);

    expect(screen.getByText(PERIODS_MESSAGE)).toBeInTheDocument();
    expect(muteButton()).toBeDisabled();
    fireEvent.click(muteButton());
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it('asks for events as well when muting until a threshold', () => {
    const { onSubmit } = renderForm('mute-until');

    expect(screen.getByLabelText('Events')).toHaveValue(10);
    fireEvent.click(muteButton());
    expect(onSubmit).toHaveBeenCalledWith({ period: 'day', periods: 1, threshold: 10 });
  });

  it('refuses a threshold that is too large', () => {
    renderForm('mute-until');

    fireEvent.change(screen.getByLabelText('Events'), { target: { value: '1000001' } });

    expect(screen.getByText(THRESHOLD_MESSAGE)).toBeInTheDocument();
    expect(muteButton()).toBeDisabled();
  });

  it('dismisses on Cancel', () => {
    const { onDismiss } = renderForm('mute-for');

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(onDismiss).toHaveBeenCalledTimes(1);
  });
});
