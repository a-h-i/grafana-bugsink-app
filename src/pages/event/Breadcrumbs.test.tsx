// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { render, screen } from '@testing-library/react';
import { Breadcrumb } from '../../datasource/types';
import { Breadcrumbs } from './Breadcrumbs';
import { loadTable } from '../../testing/loadTable';

function crumb(overrides: Partial<Breadcrumb> = {}): Breadcrumb {
  return {
    timestamp: '2026-11-15T07:52:59.387Z',
    type: 'default',
    category: 'app',
    level: 'info',
    message: 'hello',
    data: [],
    ...overrides,
  };
}

beforeAll(loadTable);

describe('Breadcrumbs', () => {
  it('has the five columns', () => {
    render(<Breadcrumbs breadcrumbs={[crumb()]} timeZone="utc" />);

    expect(screen.getAllByRole('columnheader').map((h) => h.textContent)).toEqual([
      'Time',
      'Type',
      'Category',
      'Level',
      'Message',
    ]);
  });

  it('lists the rows in the given order with milliseconds in the time zone', () => {
    render(
      <Breadcrumbs breadcrumbs={[crumb({ message: 'first' }), crumb({ message: 'second', category: 'db' })]} timeZone="utc" />
    );

    const rows = screen.getAllByRole('row').slice(1);
    expect(rows[0]).toHaveTextContent('first');
    expect(rows[1]).toHaveTextContent('second');
    expect(rows[0]).toHaveTextContent('2026-11-15 07:52:59.387');
  });

  it('leaves the time empty when there is none', () => {
    render(<Breadcrumbs breadcrumbs={[crumb({ timestamp: null })]} timeZone="utc" />);

    const cells = screen.getAllByRole('row')[1].querySelectorAll('[role="cell"]');
    expect(cells[0].textContent).toBe('');
  });

  it('shows the data pairs when there is no message', () => {
    render(
      <Breadcrumbs
        breadcrumbs={[
          crumb({
            message: '',
            data: [
              { key: 'order', value: '42' },
              { key: 'user', value: 'ada' },
            ],
          }),
        ]}
        timeZone="utc"
      />
    );

    expect(screen.getByText('order=42, user=ada')).toBeInTheDocument();
  });

  it('says when there are none', () => {
    render(<Breadcrumbs breadcrumbs={[]} timeZone="utc" />);

    expect(screen.getByText('No breadcrumbs.')).toBeInTheDocument();
  });
});
