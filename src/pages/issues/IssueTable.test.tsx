// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { fireEvent, render, screen, within } from '@testing-library/react';

import { IssueRow } from '../../datasource/types';
import { MemoryRouter } from 'react-router-dom';
import { IssueTable, PAGE_SIZE } from './IssueTable';

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

function renderTable(rows: IssueRow[]) {
  return render(
    <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
      <IssueTable rows={rows} timeZone="utc" issueHref={issueHref} />
    </MemoryRouter>
  );
}

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
});
