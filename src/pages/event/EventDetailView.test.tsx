// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { render, screen, within } from '@testing-library/react';
import { EventDetail } from '../../datasource/types';
import { EventDetailView } from './EventDetailView';

function event(overrides: Partial<EventDetail> = {}): EventDetail {
  return {
    id: 'b5f4c9a0-0000-4000-8000-000000000001',
    eventId: '7b5d5499-1cf8-4734-8381-14cfb03da60e',
    issueId: 'issue-1',
    projectId: 1,
    project: 'sample-python',
    timestamp: '2026-11-15T07:52:59.387Z',
    ingestedAt: '2026-11-15T07:53:00Z',
    platform: 'python',
    level: '',
    message: '',
    transaction: '',
    environment: 'development',
    release: '1.2.3',
    traceId: 'abc123',
    tags: [{ key: 'service', value: 'sample-python' }],
    contexts: [{ name: 'trace', fields: [{ key: 'trace_id', value: 'abc123' }] }],
    exceptions: [],
    breadcrumbs: [],
    stacktraceMd: null,
    notices: [],
    bugsinkUrl: 'http://bugsink/x',
    ...overrides,
  };
}

const exception = {
  type: 'RuntimeError',
  value: 'boom',
  module: '',
  mechanism: '',
  handled: true,
  frames: [],
  framesOmitted: 0,
};

const frame = {
  filename: 'a.py',
  function: 'f',
  module: '',
  lineno: 2,
  colno: null,
  inApp: true,
  preContext: [],
  contextLine: 'x = 1',
  postContext: [],
};

const HOSTILE = '<img src=x onerror=alert(1)><script>alert(1)</script>[x](javascript:alert(1)) __call__';

function renderView(e: EventDetail) {
  return render(<EventDetailView event={e} timeZone="utc" />);
}

describe('EventDetailView', () => {
  it('lists the identifying rows', () => {
    renderView(event());

    expect(screen.getByText('Event ID')).toBeInTheDocument();
    expect(screen.getByText('7b5d5499-1cf8-4734-8381-14cfb03da60e')).toBeInTheDocument();
    expect(screen.getByText('Timestamp')).toBeInTheDocument();
    expect(screen.getByText('2026-11-15 07:52:59')).toBeInTheDocument();
    expect(screen.getByText('Environment')).toBeInTheDocument();
    expect(screen.getByText('development')).toBeInTheDocument();
    expect(screen.getByText('Release')).toBeInTheDocument();
    expect(screen.getByText('Trace ID')).toBeInTheDocument();
    const summary = within(screen.getByText('Event ID').closest('dl')!);
    expect(summary.getByText('1.2.3')).toBeInTheDocument();
    expect(summary.getByText('abc123')).toBeInTheDocument();
  });

  it('leaves out the rows that are empty', () => {
    renderView(event());

    expect(screen.queryByText('Level')).not.toBeInTheDocument();
    expect(screen.queryByText('Transaction')).not.toBeInTheDocument();
  });

  it('shows the notices as warnings in order', () => {
    renderView(event({ notices: ['first notice', 'second notice'] }));

    const notices = screen.getAllByText(/notice$/).map((n) => n.textContent);
    expect(notices).toEqual(['first notice', 'second notice']);
    expect(screen.getAllByTestId('data-testid Alert warning')).toHaveLength(2);
  });

  it('shows two identical notices and two tags with the same key', () => {
    renderView(
      event({
        notices: ['same notice', 'same notice'],
        tags: [
          { key: 'dup', value: 'one' },
          { key: 'dup', value: 'two' },
        ],
      })
    );

    expect(screen.getAllByText('same notice')).toHaveLength(2);
    expect(screen.getByText('one')).toBeInTheDocument();
    expect(screen.getByText('two')).toBeInTheDocument();
  });

  it('shows the exception chain and no stack trace section when an exception has frames', () => {
    renderView(event({ exceptions: [{ ...exception, frames: [frame] }], stacktraceMd: 'ignored' }));

    expect(screen.getByRole('heading', { level: 3, name: 'Exception' })).toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Stack trace' })).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Message' })).not.toBeInTheDocument();
  });

  it('shows the message and Bugsink text in a pre when there are no exceptions', () => {
    const { container } = renderView(event({ message: 'disk almost full', stacktraceMd: '# disk\n  at main' }));

    expect(screen.getByRole('heading', { level: 3, name: 'Message' })).toBeInTheDocument();
    expect(screen.getByText('disk almost full')).toBeInTheDocument();
    expect(screen.getByRole('heading', { level: 3, name: 'Stack trace' })).toBeInTheDocument();
    expect(
      screen.getByText("Bugsink's own rendering of the stack trace. It can include local variables.")
    ).toBeInTheDocument();
    const pre = container.querySelector('pre');
    expect(pre?.textContent).toBe('# disk\n  at main');
  });

  it('shows the exception and Bugsink text when no exception has frames', () => {
    const { container } = renderView(
      event({ exceptions: [exception], message: 'hidden', stacktraceMd: '# boom\n  at main' })
    );

    expect(screen.getByRole('heading', { level: 3, name: 'Exception' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { level: 3, name: 'Stack trace' })).toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Message' })).not.toBeInTheDocument();
    const section = screen.getByRole('heading', { level: 3, name: 'Stack trace' }).parentElement!;
    expect(within(section).getByText('# boom', { exact: false }).tagName).toBe('PRE');
    expect(container.querySelectorAll('pre')).toHaveLength(1);
  });

  it('says there is no stack trace for a frameless exception without text', () => {
    renderView(event({ exceptions: [exception] }));

    expect(screen.getByRole('heading', { level: 3, name: 'Exception' })).toBeInTheDocument();
    expect(screen.getByText('No stack trace.')).toBeInTheDocument();
  });

  it('says there is no stack trace when there is no text', () => {
    renderView(event());

    expect(screen.getByText('No stack trace.')).toBeInTheDocument();
  });

  it('lists the tags and the contexts', () => {
    renderView(event());

    const tags = within(screen.getByRole('heading', { name: 'Event tags' }).parentElement!);
    expect(tags.getByText('service')).toBeInTheDocument();
    expect(tags.getByText('sample-python')).toBeInTheDocument();
    const contexts = within(screen.getByRole('heading', { name: 'Contexts' }).parentElement!);
    expect(contexts.getByText('trace')).toBeInTheDocument();
    expect(contexts.getByText('trace_id')).toBeInTheDocument();
    expect(contexts.getByText('abc123')).toBeInTheDocument();
  });

  it('says when there are no tags and no contexts', () => {
    renderView(event({ tags: [], contexts: [] }));

    expect(screen.getByText('No tags.')).toBeInTheDocument();
    expect(screen.getByText('No contexts.')).toBeInTheDocument();
  });

  it('renders payload text as text', () => {
    const frame = {
      filename: 'a.py',
      function: 'f',
      module: '',
      lineno: 2,
      colno: null,
      inApp: true,
      preContext: [],
      contextLine: HOSTILE,
      postContext: [],
    };
    const withException = renderView(
      event({
        exceptions: [{ ...exception, value: HOSTILE, frames: [frame] }],
        breadcrumbs: [{ timestamp: null, type: 'default', category: 'app', level: 'info', message: HOSTILE, data: [] }],
        tags: [{ key: 'k', value: HOSTILE }],
        contexts: [{ name: 'c', fields: [{ key: 'f', value: HOSTILE }] }],
      })
    );
    expect(screen.getAllByText(HOSTILE, { exact: false }).length).toBeGreaterThanOrEqual(5);
    expect(withException.container.querySelector('img, script, iframe, a')).toBeNull();
    withException.unmount();

    const fallback = renderView(event({ stacktraceMd: HOSTILE }));
    expect(fallback.container.querySelector('pre')?.textContent).toBe(HOSTILE);
    expect(fallback.container.querySelector('img, script, iframe, a')).toBeNull();
  });
});
