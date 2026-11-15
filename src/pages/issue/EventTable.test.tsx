// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { EventRow } from '../../datasource/types';
import { EventTable } from './EventTable';

function row(overrides: Partial<EventRow> = {}): EventRow {
  return {
    id: 'e1',
    eventId: 'aaaaaaaa-0000-0000-0000-000000000001',
    issue: 'X-1',
    issueId: 'i1',
    project: 'sample-python',
    projectId: 1,
    timestamp: Date.UTC(2026, 10, 15, 5, 45, 33),
    ingestedAt: Date.UTC(2026, 10, 15, 5, 45, 40),
    url: 'http://localhost:8800/issues/issue/i1/event/e1/',
    ...overrides,
  };
}

function renderTable(rows: EventRow[]) {
  return render(<EventTable rows={rows} timeZone="utc" />);
}

describe('EventTable', () => {
  it('shows the columns in order', () => {
    renderTable([row()]);

    expect(screen.getAllByRole('columnheader').map((h) => h.textContent)).toEqual([
      'Event ID',
      'Timestamp',
      'Ingested at',
    ]);
  });

  it('links the event id to the event in a new tab and formats the times', () => {
    renderTable([row()]);

    const link = screen.getByRole('link', { name: 'aaaaaaaa-0000-0000-0000-000000000001' });
    expect(link).toHaveAttribute('href', 'http://localhost:8800/issues/issue/i1/event/e1/');
    expect(link).toHaveAttribute('target', '_blank');
    expect(link.getAttribute('rel')).toContain('noreferrer');
    expect(screen.getByText('2026-11-15 05:45:33')).toBeInTheDocument();
    expect(screen.getByText('2026-11-15 05:45:40')).toBeInTheDocument();
  });

  it('shows the event id as text when the row has no url', () => {
    renderTable([row({ url: undefined })]);

    expect(screen.queryByRole('link')).not.toBeInTheDocument();
    expect(screen.getByText('aaaaaaaa-0000-0000-0000-000000000001')).toBeInTheDocument();
  });

  it('keeps the order it is given', () => {
    renderTable([row({ id: 'b', eventId: 'ev-b' }), row({ id: 'a', eventId: 'ev-a' })]);

    const cells = screen.getAllByRole('cell').filter((cell) => /^ev-/.test(cell.textContent ?? ''));
    expect(cells.map((cell) => cell.textContent)).toEqual(['ev-b', 'ev-a']);
  });

  it('pages 30 rows into 25 and 5', () => {
    renderTable(Array.from({ length: 30 }, (_, i) => row({ id: `id${i}`, eventId: `ev-${i + 1}` })));

    expect(screen.getAllByRole('row')).toHaveLength(26);

    fireEvent.click(screen.getByRole('button', { name: '2' }));

    expect(screen.getAllByRole('row')).toHaveLength(6);
    expect(screen.getByText('ev-26')).toBeInTheDocument();
    expect(screen.queryByText('ev-25')).not.toBeInTheDocument();
  });

  it('explains an issue without stored events', () => {
    renderTable([]);

    expect(screen.getByText('No stored events.')).toBeInTheDocument();
  });
});
