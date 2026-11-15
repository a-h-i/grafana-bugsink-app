// SPDX-License-Identifier: GPL-3.0-or-later
import React, { useState } from 'react';
import { DataSourcePluginOptionsEditorProps } from '@grafana/data';
import { fireEvent, render, screen } from '@testing-library/react';
import { BugsinkOptions, BugsinkSecureJsonData } from '../types';
import { ConfigEditor } from './ConfigEditor';

type Options = DataSourcePluginOptionsEditorProps<BugsinkOptions, BugsinkSecureJsonData>['options'];

function renderEditor(overrides: Partial<Options> = {}) {
  const onOptionsChange = jest.fn();
  const initial = { url: '', jsonData: {}, secureJsonFields: {}, ...overrides } as Options;

  function Harness() {
    const [options, setOptions] = useState(initial);
    return (
      <ConfigEditor
        options={options}
        onOptionsChange={(next) => {
          onOptionsChange(next);
          setOptions(next);
        }}
      />
    );
  }

  render(<Harness />);
  return onOptionsChange;
}

const lastCall = (fn: jest.Mock) => fn.mock.calls[fn.mock.calls.length - 1][0];

describe('ConfigEditor', () => {
  it('shows the URL from options', () => {
    renderEditor({ url: 'http://bugsink:8000' });

    expect(screen.getByLabelText('Bugsink URL')).toHaveValue('http://bugsink:8000');
  });

  it('typing a URL calls onOptionsChange with url', () => {
    const onOptionsChange = renderEditor({ secureJsonData: { apiToken: 't' }, secureJsonFields: { apiToken: false } });

    fireEvent.change(screen.getByLabelText('Bugsink URL'), { target: { value: 'http://x' } });

    const next = lastCall(onOptionsChange);
    expect(next.url).toBe('http://x');
    expect(next.secureJsonData).toEqual({ apiToken: 't' });
    expect(next.secureJsonFields).toEqual({ apiToken: false });
  });

  it('typing a token sets secureJsonData.apiToken', () => {
    const onOptionsChange = renderEditor();

    fireEvent.change(screen.getByLabelText('API token'), { target: { value: 'abc' } });

    expect(lastCall(onOptionsChange).secureJsonData).toEqual({ apiToken: 'abc' });
  });

  it('a stored token shows as configured and is not sent again', () => {
    const onOptionsChange = renderEditor({ secureJsonFields: { apiToken: true } });

    expect(screen.getByLabelText('API token')).toBeDisabled();
    expect(screen.getByDisplayValue('configured')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Reset' })).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText('Bugsink URL'), { target: { value: 'h' } });

    expect(lastCall(onOptionsChange).secureJsonData?.apiToken).toBeUndefined();
  });

  it('Reset clears the stored token', () => {
    const onOptionsChange = renderEditor({ secureJsonFields: { apiToken: true } });

    fireEvent.click(screen.getByRole('button', { name: 'Reset' }));

    const next = lastCall(onOptionsChange);
    expect(next.secureJsonFields.apiToken).toBe(false);
    expect(next.secureJsonData.apiToken).toBe('');
  });

  it('explains the token', () => {
    renderEditor();

    expect(screen.getByText(/projects:read, issues:read and events:read/)).toBeInTheDocument();
  });
});
