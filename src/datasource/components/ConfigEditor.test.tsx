// SPDX-License-Identifier: GPL-3.0-or-later
import React, { useState } from 'react';
import { DataSourcePluginOptionsEditorProps } from '@grafana/data';
import { fireEvent, render, screen } from '@testing-library/react';

import { BugsinkOptions, BugsinkSecureJsonData } from '../types';
import { ConfigEditor } from './ConfigEditor';

jest.mock('@grafana/runtime', () => ({
  ...jest.requireActual('@grafana/runtime'),
  DataSourcePicker: ({
    type,
    current,
    onChange,
    onClear,
    inputId,
  }: {
    type: string;
    current: string | null;
    onChange: (ds: { uid: string }) => void;
    onClear: () => void;
    inputId: string;
  }) => (
    <div>
      <span data-testid={inputId}>{current ?? 'none'}</span>
      <button onClick={() => onChange({ uid: `${type}-uid` })}>{`pick ${type}`}</button>
      <button onClick={onClear}>{`clear ${type}`}</button>
    </div>
  ),
}));

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

  it('shows the browser URL from jsonData', () => {
    renderEditor({ jsonData: { browserUrl: 'https://bugsink.example.com' } });

    expect(screen.getByLabelText('Browser URL')).toHaveValue('https://bugsink.example.com');
  });

  it('typing a browser URL sets jsonData.browserUrl and keeps the other options', () => {
    const onOptionsChange = renderEditor({ url: 'http://bugsink:8000', jsonData: {} });

    fireEvent.change(screen.getByLabelText('Browser URL'), { target: { value: 'https://b.example.com' } });

    const next = lastCall(onOptionsChange);
    expect(next.jsonData).toEqual({ browserUrl: 'https://b.example.com' });
    expect(next.url).toBe('http://bugsink:8000');
  });

  it('explains the browser URL', () => {
    renderEditor();

    expect(screen.getByText(/Leave empty when it is the same as the Bugsink URL/)).toBeInTheDocument();
  });

  it('explains the token', () => {
    renderEditor();

    expect(screen.getByText(/projects:read, issues:read and events:read/)).toBeInTheDocument();
  });

  describe('trace and log links', () => {
    it('has a heading and the five fields with their descriptions and defaults', () => {
      renderEditor();

      expect(screen.getByRole('heading', { level: 3, name: 'Trace and log links' })).toBeInTheDocument();
      expect(screen.getByText('Tempo data source')).toBeInTheDocument();
      expect(screen.getByText('Open trace runs a trace ID query here.')).toBeInTheDocument();
      expect(screen.getByText('Loki data source')).toBeInTheDocument();
      expect(screen.getByText('Open logs runs the logs query here.')).toBeInTheDocument();
      expect(screen.getByText('Links open this many minutes before and after the event.')).toBeInTheDocument();
      expect(screen.getByText(/Variables: \$\{trace_id\}, \$\{service\}, \$\{project\}/)).toBeInTheDocument();
      expect(
        screen.getByText(/Put every variable inside double quotes\. Values are not escaped for regular expressions\./)
      ).toBeInTheDocument();
      expect(screen.getByText(/The event tag that holds the service name/)).toBeInTheDocument();
      expect(screen.getByText('minutes')).toBeInTheDocument();
      expect(screen.getByLabelText('Time window')).toHaveValue(null);
      expect(screen.getByLabelText('Time window')).toHaveAttribute('placeholder', '15');
      expect(screen.getByLabelText('Service tag')).toHaveValue('service');
      expect(screen.getByLabelText('Logs query')).toHaveValue('');
      expect(screen.getByLabelText('Logs query')).toHaveAttribute(
        'placeholder',
        '{service_name="${service}"} | trace_id="${trace_id}"'
      );
    });

    it('shows the stored settings', () => {
      renderEditor({
        jsonData: {
          tempoDatasourceUid: 't1',
          lokiDatasourceUid: 'l1',
          linkWindowMinutes: 30,
          serviceTag: 'svc',
          logsQuery: '{a="b"}',
        },
      });

      expect(screen.getByTestId('bugsink-tempo')).toHaveTextContent('t1');
      expect(screen.getByTestId('bugsink-loki')).toHaveTextContent('l1');
      expect(screen.getByLabelText('Time window')).toHaveValue(30);
      expect(screen.getByLabelText('Service tag')).toHaveValue('svc');
      expect(screen.getByLabelText('Logs query')).toHaveValue('{a="b"}');
    });

    it('stores the picked Tempo and Loki uids and keeps the other options', () => {
      const onOptionsChange = renderEditor({ url: 'http://b', jsonData: { browserUrl: 'x' } });

      fireEvent.click(screen.getByRole('button', { name: 'pick tempo' }));
      fireEvent.click(screen.getByRole('button', { name: 'pick loki' }));

      const next = lastCall(onOptionsChange);
      expect(next.jsonData).toEqual({
        browserUrl: 'x',
        tempoDatasourceUid: 'tempo-uid',
        lokiDatasourceUid: 'loki-uid',
      });
      expect(next.url).toBe('http://b');
    });

    it('clearing a picker removes its uid', () => {
      const onOptionsChange = renderEditor({ jsonData: { tempoDatasourceUid: 't1', lokiDatasourceUid: 'l1' } });

      fireEvent.click(screen.getByRole('button', { name: 'clear tempo' }));
      expect(lastCall(onOptionsChange).jsonData).toEqual({ lokiDatasourceUid: 'l1' });

      fireEvent.click(screen.getByRole('button', { name: 'clear loki' }));
      expect(lastCall(onOptionsChange).jsonData).toEqual({});
    });

    it('stores the time window as a number and removes the key when cleared', () => {
      const onOptionsChange = renderEditor({ jsonData: { browserUrl: 'x' } });

      fireEvent.change(screen.getByLabelText('Time window'), { target: { value: '30' } });
      expect(lastCall(onOptionsChange).jsonData).toEqual({ browserUrl: 'x', linkWindowMinutes: 30 });

      fireEvent.change(screen.getByLabelText('Time window'), { target: { value: '' } });
      expect(lastCall(onOptionsChange).jsonData).toEqual({ browserUrl: 'x' });
      expect(screen.getByLabelText('Time window')).toHaveValue(null);
    });

    it('stores the logs query and an emptied service tag', () => {
      const onOptionsChange = renderEditor();

      fireEvent.change(screen.getByLabelText('Logs query'), { target: { value: '{a="b"}' } });
      expect(lastCall(onOptionsChange).jsonData).toEqual({ logsQuery: '{a="b"}' });

      fireEvent.change(screen.getByLabelText('Service tag'), { target: { value: '' } });
      expect(lastCall(onOptionsChange).jsonData).toEqual({ logsQuery: '{a="b"}', serviceTag: '' });
    });
  });

  describe('switches', () => {
    it.each([
      [
        'Triage',
        'triageEnabled',
        "Let editors and admins resolve, reopen, mute and unmute issues from Grafana. The API token needs issues:triage. Every action runs as the token's Bugsink user. If anonymous users have the Editor or Admin role, anyone who can open Grafana can triage.",
      ],
      [
        'Hide stack trace text',
        'hideStacktraceText',
        "Do not show Bugsink's own stack trace text, which can include local variables, for events without structured frames.",
      ],
    ] as const)('%s renders off, explains itself and stores a boolean', (label, key, description) => {
      const onOptionsChange = renderEditor({ jsonData: { browserUrl: 'x' } });
      const toggle = screen.getByLabelText(label);

      expect(toggle).not.toBeChecked();
      expect(screen.getByText(description)).toBeInTheDocument();

      fireEvent.click(toggle);

      expect(lastCall(onOptionsChange).jsonData).toEqual({ browserUrl: 'x', [key]: true });
    });

    it('shows a stored switch as on', () => {
      renderEditor({ jsonData: { triageEnabled: true } });

      expect(screen.getByLabelText('Triage')).toBeChecked();
      expect(screen.getByLabelText('Hide stack trace text')).not.toBeChecked();
    });

    it.each(['Triage', 'Hide stack trace text'])(
      'switching %s off stores false and keeps the other options',
      (label) => {
        const onOptionsChange = renderEditor({
          jsonData: { browserUrl: 'x', triageEnabled: true, hideStacktraceText: true },
        });

        fireEvent.click(screen.getByLabelText(label));

        const key = label === 'Triage' ? 'triageEnabled' : 'hideStacktraceText';
        expect(lastCall(onOptionsChange).jsonData).toEqual({
          browserUrl: 'x',
          triageEnabled: true,
          hideStacktraceText: true,
          [key]: false,
        });
      }
    );

    it('puts each switch under its own heading', () => {
      renderEditor();

      const triage = screen.getByRole('heading', { level: 3, name: 'Triage' });
      const privacy = screen.getByRole('heading', { level: 3, name: 'Privacy' });
      const follows = (a: Node, b: Node) => (a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0;
      expect(follows(triage, screen.getByLabelText('Triage'))).toBe(true);
      expect(follows(screen.getByLabelText('Triage'), privacy)).toBe(true);
      expect(follows(privacy, screen.getByLabelText('Hide stack trace text'))).toBe(true);
    });
  });
});
