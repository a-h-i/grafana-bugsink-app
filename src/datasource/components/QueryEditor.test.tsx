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

  it('offers Issues, Events and Event volume', async () => {
    await renderEditor();

    const labels = screen
      .getAllByRole('radio')
      .map((radio) => radio.getAttribute('aria-label') ?? radio.nextSibling?.textContent);
    expect(labels).toEqual(['Issues', 'Events', 'Event volume']);
  });

  it('choosing Event volume sets the type and runs', async () => {
    const { onChange, onRunQuery } = await renderEditor({ project: 2 });

    fireEvent.click(screen.getByRole('radio', { name: 'Event volume' }));

    expect(onChange).toHaveBeenCalledWith({ refId: 'A', project: 2, queryType: 'volume' });
    expect(onRunQuery).toHaveBeenCalledTimes(1);
  });

  it('renders Project and Issue for a volume query and nothing else', async () => {
    await renderEditor({ queryType: 'volume' });

    expect(screen.getByRole('radio', { name: 'Event volume' })).toBeChecked();
    expect(screen.getByRole('combobox', { name: 'Project' })).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: 'Issue' })).toBeInTheDocument();
    for (const name of ['Search', 'Event ID']) {
      expect(screen.queryByRole('textbox', { name })).not.toBeInTheDocument();
    }
    expect(screen.queryByRole('combobox', { name: 'State' })).not.toBeInTheDocument();
    expect(screen.queryByRole('combobox', { name: 'Sort by' })).not.toBeInTheDocument();
  });

  it('applies a typed issue on blur and runs on Enter', async () => {
    const { onChange, onRunQuery } = await renderEditor({ queryType: 'volume' });
    const input = screen.getByRole('textbox', { name: 'Issue' });

    fireEvent.change(input, { target: { value: 'SAMPLE-PYTHON-2' } });
    expect(onChange).not.toHaveBeenCalled();
    fireEvent.blur(input);
    expect(onChange).toHaveBeenLastCalledWith({ refId: 'A', queryType: 'volume', issue: 'SAMPLE-PYTHON-2' });
    expect(onRunQuery).not.toHaveBeenCalled();

    fireEvent.change(input, { target: { value: 'SAMPLE-PYTHON-3' } });
    fireEvent.keyDown(input, { key: 'Enter' });
    expect(onChange).toHaveBeenLastCalledWith({ refId: 'A', queryType: 'volume', issue: 'SAMPLE-PYTHON-3' });
    expect(onRunQuery).toHaveBeenCalledTimes(1);
  });
});
