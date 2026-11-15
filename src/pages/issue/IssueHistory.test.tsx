// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { render, screen } from '@testing-library/react';
import { HistoryEntry } from '../../datasource/types';
import { IssueHistory } from './IssueHistory';

function entry(overrides: Partial<HistoryEntry> = {}): HistoryEntry {
  return { id: 1, kind: 'muted', timestamp: '2026-11-15T05:45:33Z', user: null, comment: '', ...overrides };
}

function renderHistory(history: HistoryEntry[], truncated = false) {
  return render(<IssueHistory history={{ history, truncated }} timeZone="utc" />);
}

describe('IssueHistory', () => {
  it('labels each kind', () => {
    const labels: Record<string, string> = {
      first_seen: 'First seen',
      resolved: 'Resolved',
      muted: 'Muted',
      regressed: 'Regressed',
      unmuted: 'Unmuted',
      reopened: 'Reopened',
      next_materialized: 'Resolved in the next release',
      manual_annotation: 'Comment',
    };
    renderHistory(Object.keys(labels).map((kind, id) => entry({ id, kind })));

    for (const label of Object.values(labels)) {
      expect(screen.getByText(label)).toBeInTheDocument();
    }
  });

  it('shows an unknown kind as written', () => {
    renderHistory([entry({ kind: 'something_new' })]);

    expect(screen.getByText('something_new')).toBeInTheDocument();
  });

  it('shows the time in the time zone', () => {
    renderHistory([entry()]);

    expect(screen.getByText('2026-11-15 05:45:33')).toBeInTheDocument();
  });

  it('names the author only when there is one', () => {
    renderHistory([entry({ id: 1, user: 'Ada' }), entry({ id: 2, kind: 'resolved' })]);

    expect(screen.getByText('Muted by Ada')).toBeInTheDocument();
    expect(screen.getByText('Resolved')).toBeInTheDocument();
    expect(screen.queryByText(/Resolved by/)).not.toBeInTheDocument();
  });

  it('shows a comment under its entry', () => {
    renderHistory([entry({ kind: 'manual_annotation', comment: 'm4 plan probe' })]);

    expect(screen.getByText('Comment')).toBeInTheDocument();
    expect(screen.getByText('m4 plan probe')).toBeInTheDocument();
  });

  it('keeps the order it is given', () => {
    renderHistory([entry({ id: 1, kind: 'resolved' }), entry({ id: 2, kind: 'muted' })]);

    const items = screen.getAllByRole('listitem').map((item) => item.textContent);
    expect(items[0]).toContain('Resolved');
    expect(items[1]).toContain('Muted');
  });

  it('warns when the read was cut short', () => {
    renderHistory([entry()], true);

    expect(screen.getByText('Only the newest 5000 history entries were read.')).toBeInTheDocument();
  });

  it('explains an issue without history', () => {
    renderHistory([]);

    expect(screen.getByText('No history.')).toBeInTheDocument();
  });
});
