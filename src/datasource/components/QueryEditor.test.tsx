// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { BugsinkDataSource } from '../datasource';
import { BugsinkQuery } from '../types';
import { QueryEditor } from './QueryEditor';

async function renderEditor(query: Partial<BugsinkQuery> = {}) {
  const onChange = jest.fn();
  const onRunQuery = jest.fn();
  const datasource = { getProjects: () => Promise.resolve([]) } as unknown as BugsinkDataSource;
  render(
    <QueryEditor datasource={datasource} query={{ refId: 'A', ...query }} onChange={onChange} onRunQuery={onRunQuery} />
  );
  await screen.findByRole('combobox', { name: 'Project' });
  return { onChange, onRunQuery };
}

describe('QueryEditor', () => {
  it('shows Issues for an empty query and renders the issues controls', async () => {
    await renderEditor();

    expect(screen.getByRole('radio', { name: 'Issues' })).toBeChecked();
    expect(screen.getByRole('radiogroup', { name: 'Query type' })).toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: 'State' })).toBeInTheDocument();
    expect(screen.queryByRole('textbox', { name: 'Event ID' })).not.toBeInTheDocument();
  });

  it('choosing Events keeps the project and search, and runs', async () => {
    const { onChange, onRunQuery } = await renderEditor({ project: 2, q: 'trace:abc' });

    fireEvent.click(screen.getByRole('radio', { name: 'Events' }));

    expect(onChange).toHaveBeenCalledWith({ refId: 'A', project: 2, q: 'trace:abc', queryType: 'events' });
    expect(onRunQuery).toHaveBeenCalledTimes(1);
  });

  it('switching back to Issues without a project replaces Arrival order', async () => {
    const onChange = jest.fn();
    const datasource = { getProjects: () => Promise.resolve([]) } as unknown as BugsinkDataSource;
    const editor = (query: Partial<BugsinkQuery>) => (
      <QueryEditor
        datasource={datasource}
        query={{ refId: 'A', ...query }}
        onChange={onChange}
        onRunQuery={jest.fn()}
      />
    );
    const { rerender } = render(editor({ project: 2, sort: 'digest_order' }));
    await screen.findByRole('combobox', { name: 'Project' });

    fireEvent.click(screen.getByRole('radio', { name: 'Events' }));
    expect(onChange).toHaveBeenLastCalledWith({ refId: 'A', project: 2, sort: 'digest_order', queryType: 'events' });
    rerender(editor({ queryType: 'events', sort: 'digest_order' }));
    fireEvent.click(screen.getByRole('radio', { name: 'Issues' }));

    expect(onChange).toHaveBeenLastCalledWith({ refId: 'A', queryType: 'issues', sort: 'last_seen' });
  });

  it('renders the events controls for an events query', async () => {
    await renderEditor({ queryType: 'events' });

    expect(screen.getByRole('radio', { name: 'Events' })).toBeChecked();
    for (const name of ['Issue', 'Search', 'Event ID']) {
      expect(screen.getByRole('textbox', { name })).toBeInTheDocument();
    }
    expect(screen.queryByRole('combobox', { name: 'State' })).not.toBeInTheDocument();
  });
});
