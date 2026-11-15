// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { CommitInput } from './CommitInput';

function setup(value?: string) {
  const onCommit = jest.fn();
  const onRunQuery = jest.fn();
  render(
    <CommitInput
      label="Issue"
      value={value}
      placeholder="SAMPLE-PYTHON-2"
      onCommit={onCommit}
      onRunQuery={onRunQuery}
    />
  );
  return { onCommit, onRunQuery, input: screen.getByRole('textbox', { name: 'Issue' }) };
}

describe('CommitInput', () => {
  it('shows the value and the placeholder', () => {
    const { input } = setup('X-1');

    expect(input).toHaveValue('X-1');
    expect(input).toHaveAttribute('placeholder', 'SAMPLE-PYTHON-2');
  });

  it('commits nothing while typing and applies on blur without running', () => {
    const { input, onCommit, onRunQuery } = setup();

    fireEvent.change(input, { target: { value: 'X-1' } });
    expect(onCommit).not.toHaveBeenCalled();

    fireEvent.blur(input);
    expect(onCommit).toHaveBeenCalledWith('X-1');
    expect(onRunQuery).not.toHaveBeenCalled();
  });

  it('blur without a change calls nothing', () => {
    const { input, onCommit, onRunQuery } = setup('X-1');

    fireEvent.blur(input);

    expect(onCommit).not.toHaveBeenCalled();
    expect(onRunQuery).not.toHaveBeenCalled();
  });

  it('Enter applies the typed text and runs once', () => {
    const { input, onCommit, onRunQuery } = setup();

    fireEvent.change(input, { target: { value: 'X-2' } });
    fireEvent.keyDown(input, { key: 'Enter' });

    expect(onCommit).toHaveBeenCalledWith('X-2');
    expect(onRunQuery).toHaveBeenCalledTimes(1);
  });

  it('Enter without a change runs without committing', () => {
    const { input, onCommit, onRunQuery } = setup('X-1');

    fireEvent.keyDown(input, { key: 'Enter' });

    expect(onCommit).not.toHaveBeenCalled();
    expect(onRunQuery).toHaveBeenCalledTimes(1);
  });

  describe('validate', () => {
    function setupValidated() {
      const onCommit = jest.fn();
      const onRunQuery = jest.fn();
      const validate = (text: string) => (text === 'bad' ? 'Not allowed.' : undefined);
      render(
        <CommitInput
          label="Issue"
          description="Helps."
          validate={validate}
          onCommit={onCommit}
          onRunQuery={onRunQuery}
        />
      );
      return { onCommit, onRunQuery, input: screen.getByRole('textbox', { name: 'Issue' }) };
    }

    it('shows the description', () => {
      setupValidated();

      expect(screen.getByText('Helps.')).toBeInTheDocument();
    });

    it('shows the message as the error and neither commits on blur nor runs on Enter', () => {
      const { input, onCommit, onRunQuery } = setupValidated();

      fireEvent.change(input, { target: { value: 'bad' } });
      expect(screen.getByText('Not allowed.')).toBeInTheDocument();
      fireEvent.blur(input);
      fireEvent.keyDown(input, { key: 'Enter' });

      expect(onCommit).not.toHaveBeenCalled();
      expect(onRunQuery).not.toHaveBeenCalled();
      expect(input).toHaveValue('bad');
      expect(screen.getByText('Not allowed.')).toBeInTheDocument();
    });

    it('commits and runs again once the text is valid', () => {
      const { input, onCommit, onRunQuery } = setupValidated();

      fireEvent.change(input, { target: { value: 'bad' } });
      fireEvent.change(input, { target: { value: 'good' } });
      expect(screen.queryByText('Not allowed.')).not.toBeInTheDocument();
      fireEvent.keyDown(input, { key: 'Enter' });

      expect(onCommit).toHaveBeenCalledWith('good');
      expect(onRunQuery).toHaveBeenCalledTimes(1);
    });
  });
});
