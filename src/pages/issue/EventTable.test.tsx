// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
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

function renderTable(rows: EventRow[], selectedId?: string) {
  return render(
    <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
      <EventTable rows={rows} timeZone="utc" selectedId={selectedId} eventHref={(r) => `/issues/i1?event=${r.id}`} />
    </MemoryRouter>
  );
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

  it('links the event id to the event in the page and formats the times', () => {
    renderTable([row()]);

    const link = screen.getByRole('link', { name: 'aaaaaaaa-0000-0000-0000-000000000001' });
    expect(link).toHaveAttribute('href', '/issues/i1?event=e1');
    expect(link).not.toHaveAttribute('target');
    expect(screen.getByText('2026-11-15 05:45:33')).toBeInTheDocument();
    expect(screen.getByText('2026-11-15 05:45:40')).toBeInTheDocument();
  });

  it('opens the event in Bugsink in a new tab from an icon link', () => {
    renderTable([row()]);

    const link = screen.getByRole('link', { name: 'Open event aaaaaaaa-0000-0000-0000-000000000001 in Bugsink' });
    expect(link).toHaveAttribute('href', 'http://localhost:8800/issues/issue/i1/event/e1/');
    expect(link).toHaveAttribute('target', '_blank');
    expect(link.getAttribute('rel')).toContain('noreferrer');
  });

  it('has no Bugsink link when the row has no url', () => {
    renderTable([row({ url: undefined })]);

    expect(screen.getAllByRole('link')).toHaveLength(1);
    expect(screen.queryByRole('link', { name: /in Bugsink/ })).not.toBeInTheDocument();
  });

  it('shows the selected event id as text and links the others', () => {
    renderTable([row({ id: 'a', eventId: 'ev-a' }), row({ id: 'b', eventId: 'ev-b' })], 'a');

    expect(screen.getByText('ev-a')).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'ev-a' })).not.toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'ev-b' })).toHaveAttribute('href', '/issues/i1?event=b');
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

    expect(screen.getByText('No stored events.').tagName).toBe('P');
  });
});
