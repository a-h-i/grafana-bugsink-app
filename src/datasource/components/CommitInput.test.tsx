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
});
