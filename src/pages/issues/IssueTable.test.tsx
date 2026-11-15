// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { act, fireEvent, render, screen, within } from '@testing-library/react';

import { IssueRow, TriageAction, VolumeSeries } from '../../datasource/types';
import { MemoryRouter } from 'react-router-dom';
import { limiter } from '../../datasource/limiter';
import { SPARKLINE_CONCURRENCY } from '../../datasource/datasource';
import { IssueTable, PAGE_SIZE } from './IssueTable';
import { VolumeSource } from './IssueTableContext';
import { loadTable } from '../../testing/loadTable';

function row(overrides: Partial<IssueRow> = {}): IssueRow {
  return {
    id: 'a1',
    friendlyId: 'SAMPLE-PYTHON-2',
    type: 'LookupError',
    message: 'no such key',
    transaction: '/checkout',
    project: 'sample-python',
    projectId: 1,
    status: 'open',
    events: 22,
    firstSeen: Date.UTC(2026, 10, 13, 23, 6, 27),
    lastSeen: Date.UTC(2026, 10, 15, 5, 45, 33),
    url: 'http://localhost:8800/issues/issue/a1/event/last/',
    ...overrides,
  };
}

const issueHref = (r: IssueRow) => `/a/app/issues/${r.id}?ds=x`;

type Triage = {
  busyIds?: ReadonlySet<string>;
  disabled?: boolean;
  onAction: (row: IssueRow, action: TriageAction) => void;
};

const pendingSource = () => ({
  issueVolume: jest.fn((_id: string, _signal: AbortSignal) => new Promise<VolumeSeries>(() => {})),
});

function tableUi(rows: IssueRow[], triage: Triage | undefined, source: VolumeSource) {
  return (
    <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
      <IssueTable rows={rows} timeZone="utc" issueHref={issueHref} triage={triage} source={source} />
    </MemoryRouter>
  );
}

function renderTable(rows: IssueRow[], triage?: Triage, source: VolumeSource = pendingSource()) {
  const view = render(tableUi(rows, triage, source));
  return { ...view, again: (next: Triage | undefined) => view.rerender(tableUi(rows, next, source)) };
}

const manyRows = (count: number) =>
  Array.from({ length: count }, (_, i) => row({ id: `id${i}`, friendlyId: `SAMPLE-PYTHON-${i + 1}` }));

beforeAll(loadTable);

describe('IssueTable', () => {
  it('shows the columns in order without sort buttons', () => {
    renderTable([row()]);

    const headers = screen.getAllByRole('columnheader').map((h) => h.textContent);
    expect(headers).toEqual([
      'Issue',
      'Type',
      'Message',
      'Transaction',
      'Project',
      'Events',
      'Last 24 hours',
      'First seen',
      'Last seen',
      'Status',
    ]);
    expect(within(screen.getAllByRole('row')[0]).queryAllByRole('button')).toHaveLength(0);
  });

  it('renders the cells of a row', () => {
    renderTable([row()]);

    const link = screen.getByRole('link', { name: 'SAMPLE-PYTHON-2' });
    expect(link).toHaveAttribute('href', '/a/app/issues/a1?ds=x');
    expect(link).not.toHaveAttribute('target');
    const external = screen.getByRole('link', { name: 'Open SAMPLE-PYTHON-2 in Bugsink' });
    expect(external).toHaveAttribute('href', 'http://localhost:8800/issues/issue/a1/event/last/');
    expect(external).toHaveAttribute('target', '_blank');
    expect(external.getAttribute('rel')).toContain('noreferrer');
    for (const text of ['LookupError', 'no such key', '/checkout', 'sample-python', '22']) {
      expect(screen.getByText(text)).toBeInTheDocument();
    }
    expect(screen.getByText('2026-11-13 23:06:27')).toBeInTheDocument();
    expect(screen.getByText('2026-11-15 05:45:33')).toBeInTheDocument();
  });

  it('labels each status', () => {
    renderTable([
      row({ id: '1', status: 'open' }),
      row({ id: '2', status: 'muted' }),
      row({ id: '3', status: 'resolved' }),
    ]);

    for (const label of ['Open', 'Muted', 'Resolved']) {
      expect(screen.getByText(label)).toBeInTheDocument();
    }
  });

  it('shows only the detail link when the row has no url', () => {
    renderTable([row({ url: undefined, message: '', transaction: '' })]);

    expect(screen.getAllByRole('link')).toHaveLength(1);
    expect(screen.getByRole('link', { name: 'SAMPLE-PYTHON-2' })).toHaveAttribute('href', '/a/app/issues/a1?ds=x');
    const cells = screen.getAllByRole('cell');
    expect(cells[2]).toHaveTextContent('');
    expect(cells[3]).toHaveTextContent('');
  });

  it('pages 30 rows into 25 and 5', () => {
    const rows = Array.from({ length: 30 }, (_, i) => row({ id: `id${i}`, friendlyId: `SAMPLE-PYTHON-${i + 1}` }));
    renderTable(rows);

    expect(PAGE_SIZE).toBe(25);
    expect(screen.getAllByRole('row')).toHaveLength(PAGE_SIZE + 1);
    expect(screen.getByRole('navigation')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: '2' }));

    expect(screen.getAllByRole('row')).toHaveLength(5 + 1);
    expect(screen.getByText('SAMPLE-PYTHON-26')).toBeInTheDocument();
    expect(screen.queryByText('SAMPLE-PYTHON-25')).not.toBeInTheDocument();
  });

  it('explains an empty result', () => {
    renderTable([]);

    expect(screen.getByText('No issues match these filters in this time range.')).toBeInTheDocument();
  });

  it('has no Actions column without triage', () => {
    renderTable([row()]);

    expect(screen.queryByRole('columnheader', { name: 'Actions' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /^Resolve/ })).not.toBeInTheDocument();
  });

  it('offers Resolve and Mute on an open row, Resolve on a muted row and nothing on a resolved row', () => {
    renderTable(
      [
        row({ id: '1', friendlyId: 'SAMPLE-PYTHON-2', status: 'open' }),
        row({ id: '2', friendlyId: 'SAMPLE-PYTHON-3', status: 'muted' }),
        row({ id: '3', friendlyId: 'SAMPLE-PYTHON-4', status: 'resolved' }),
      ],
      { onAction: jest.fn() }
    );

    expect(screen.getByRole('columnheader', { name: 'Actions' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-2' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Mute SAMPLE-PYTHON-2' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-3' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Mute SAMPLE-PYTHON-3' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Unmute SAMPLE-PYTHON-3' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /SAMPLE-PYTHON-4$/ })).not.toBeInTheDocument();
  });

  it('calls onAction with the row and the action', () => {
    const onAction = jest.fn();
    const open = row();
    renderTable([open], { onAction });

    fireEvent.click(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-2' }));

    expect(onAction).toHaveBeenCalledWith(open, 'resolve');
  });

  it('disables only the busy rows', () => {
    renderTable(
      [
        row({ id: '1', friendlyId: 'SAMPLE-PYTHON-2' }),
        row({ id: '2', friendlyId: 'SAMPLE-PYTHON-3' }),
        row({ id: '3', friendlyId: 'SAMPLE-PYTHON-4' }),
      ],
      { busyIds: new Set(['1', '3']), onAction: jest.fn() }
    );

    expect(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-2' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-3' })).toBeEnabled();
    expect(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-4' })).toBeDisabled();
  });

  it('disables every button when the table is disabled', () => {
    renderTable([row({ id: '1', friendlyId: 'SAMPLE-PYTHON-2' }), row({ id: '2', friendlyId: 'SAMPLE-PYTHON-3' })], {
      disabled: true,
      onAction: jest.fn(),
    });

    for (const button of screen.getAllByRole('button', { name: /^(Resolve|Mute) / })) {
      expect(button).toBeDisabled();
    }
  });

  describe('sparkline column', () => {
    it('asks for the volume of each first-page row once', () => {
      const source = pendingSource();
      renderTable(manyRows(30), undefined, source);

      expect(source.issueVolume).toHaveBeenCalledTimes(25);
      const ids = source.issueVolume.mock.calls.map((call) => call[0]);
      expect(ids).toEqual(Array.from({ length: 25 }, (_, i) => `id${i}`));
    });

    it('paging aborts the previous page', () => {
      const source = pendingSource();
      renderTable(manyRows(30), undefined, source);
      const signals = source.issueVolume.mock.calls.map((call) => call[1]);

      fireEvent.click(screen.getByRole('button', { name: '2' }));

      expect(signals.every((signal) => signal.aborted)).toBe(true);
      expect(source.issueVolume).toHaveBeenCalledTimes(30);
      expect(source.issueVolume.mock.calls.slice(25).map((call) => call[0])).toEqual([
        'id25',
        'id26',
        'id27',
        'id28',
        'id29',
      ]);
    });

    it('one failing row stays in its cell', async () => {
      const source = {
        issueVolume: jest.fn(async (id: string) => {
          if (id === 'id1') {
            throw new Error('no such issue');
          }
          return { times: [1, 2], counts: [1, 2], total: 3 };
        }),
      };
      const onAction = jest.fn();
      renderTable(manyRows(3), { onAction }, source);

      expect(await screen.findByLabelText('Events per hour unavailable: no such issue')).toBeInTheDocument();
      expect(await screen.findAllByLabelText('3 events in the last 24 hours')).toHaveLength(2);
      fireEvent.click(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-1' }));
      expect(onAction).toHaveBeenCalledTimes(1);
    });

    it('triage does not restart sparklines', () => {
      const source = pendingSource();
      const rows = manyRows(3);
      const { again } = renderTable(rows, { onAction: jest.fn() }, source);
      const signals = source.issueVolume.mock.calls.map((call) => call[1]);

      again({ busyIds: new Set(['id1']), disabled: false, onAction: jest.fn() });
      again({ busyIds: new Set(), disabled: true, onAction: jest.fn() });

      expect(source.issueVolume).toHaveBeenCalledTimes(3);
      expect(signals.some((signal) => signal.aborted)).toBe(false);
      again({ busyIds: new Set(['id1']), disabled: false, onAction: jest.fn() });
      expect(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-2' })).toBeDisabled();
      expect(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-1' })).toBeEnabled();
    });

    it('unmount aborts and logs nothing', async () => {
      const spy = jest.spyOn(console, 'error').mockImplementation(() => {});
      const rejects: Array<(reason: unknown) => void> = [];
      const source = {
        issueVolume: jest.fn(
          (_id: string, _signal: AbortSignal) => new Promise<VolumeSeries>((_, reject) => rejects.push(reject))
        ),
      };
      const { unmount } = renderTable(manyRows(3), undefined, source);
      const signals = source.issueVolume.mock.calls.map((call) => call[1]);

      unmount();
      await act(async () =>
        rejects.forEach((reject) => reject(Object.assign(new Error('aborted'), { name: 'AbortError' })))
      );

      expect(signals).toHaveLength(3);
      expect(signals.every((signal) => signal.aborted)).toBe(true);
      expect(spy).not.toHaveBeenCalled();
      spy.mockRestore();
    });

    it('paging with the real limiter never starts the queued rows of the old page', async () => {
      const limit = limiter(SPARKLINE_CONCURRENCY);
      const started: string[] = [];
      const settle = new Map<string, () => void>();
      const source = {
        issueVolume: (id: string, signal: AbortSignal) =>
          limit(
            () =>
              new Promise<VolumeSeries>((resolve) => {
                started.push(id);
                settle.set(id, () => resolve({ times: [1], counts: [1], total: 1 }));
              }),
            signal
          ).catch(() => ({ times: [1], counts: [0], total: 0 })),
      };
      renderTable(manyRows(30), undefined, source);
      expect(started).toEqual(['id0', 'id1', 'id2', 'id3']);

      fireEvent.click(screen.getByRole('button', { name: '2' }));
      await act(async () => {});

      expect(started).toEqual(['id0', 'id1', 'id2', 'id3', 'id25', 'id26', 'id27', 'id28']);

      await act(async () => settle.get('id25')?.());

      expect(started).toContain('id29');
      expect(started).toHaveLength(9);
    });
  });
});
