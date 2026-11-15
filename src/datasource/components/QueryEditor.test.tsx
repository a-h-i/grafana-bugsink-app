// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { BugsinkDataSource } from '../datasource';
import { BugsinkQuery } from '../types';
import { QueryEditor } from './QueryEditor';

// jsdom has no layout, so the virtualised option list renders nothing without a size.
beforeAll(() => {
  jest.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(500);
  jest.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(300);
});

afterAll(() => jest.restoreAllMocks());

const PROJECTS = [
  { id: 1, name: 'sample-python' },
  { id: 2, name: 'sample-js' },
];

async function renderEditor(query: Partial<BugsinkQuery> = {}, getProjects = () => Promise.resolve(PROJECTS)) {
  const onChange = jest.fn();
  const onRunQuery = jest.fn();
  const datasource = { getProjects } as unknown as BugsinkDataSource;

  render(
    <QueryEditor datasource={datasource} query={{ refId: 'A', ...query }} onChange={onChange} onRunQuery={onRunQuery} />
  );
  await screen.findByRole('combobox', { name: 'Project' });
  return { onChange, onRunQuery };
}

async function pick(name: string, option: string) {
  fireEvent.click(screen.getByRole('combobox', { name }));
  fireEvent.click(await screen.findByRole('option', { name: option }));
}

const lastCall = (fn: jest.Mock) => fn.mock.calls[fn.mock.calls.length - 1][0];

describe('QueryEditor', () => {
  it('shows the five controls with the default labels for an empty query', async () => {
    const { onChange } = await renderEditor();

    expect(screen.getByRole('combobox', { name: 'Project' })).toHaveValue('All projects');
    expect(screen.getByRole('combobox', { name: 'State' })).toHaveValue('Unresolved');
    expect(screen.getByRole('textbox', { name: 'Search' })).toHaveValue('');
    expect(screen.getByRole('textbox', { name: 'Search' })).toHaveAttribute(
      'placeholder',
      'environment:production release:1.4.2'
    );
    expect(screen.getByRole('combobox', { name: 'Sort by' })).toHaveValue('Last seen');
    expect(screen.getByRole('combobox', { name: 'Order' })).toHaveValue('Descending');
    expect(onChange).not.toHaveBeenCalled();
  });

  it('lists All projects first, then the projects by name', async () => {
    await renderEditor();

    fireEvent.click(screen.getByRole('combobox', { name: 'Project' }));

    const names = (await screen.findAllByRole('option')).map((option) => option.textContent);
    expect(names).toEqual(['All projects', 'sample-js', 'sample-python']);
  });

  it('picking a project sets it and runs the query', async () => {
    const { onChange, onRunQuery } = await renderEditor();

    await pick('Project', 'sample-js');

    expect(lastCall(onChange).project).toBe(2);
    expect(onRunQuery).toHaveBeenCalledTimes(1);
  });

  it('picking All projects removes the project', async () => {
    const { onChange, onRunQuery } = await renderEditor({ project: 2 });

    await pick('Project', 'All projects');

    expect(lastCall(onChange)).not.toHaveProperty('project');
    expect(onRunQuery).toHaveBeenCalledTimes(1);
  });

  it('picking a state, sort or order changes the query and runs it', async () => {
    const { onChange, onRunQuery } = await renderEditor();

    await pick('State', 'Muted');
    expect(lastCall(onChange).state).toBe('muted');

    await pick('Sort by', 'Event count');
    expect(lastCall(onChange).sort).toBe('digested_event_count');

    await pick('Order', 'Ascending');
    expect(lastCall(onChange).order).toBe('asc');
    expect(onRunQuery).toHaveBeenCalledTimes(3);
  });

  it('typing in Search changes nothing until blur, and blur applies the text without running', async () => {
    const { onChange, onRunQuery } = await renderEditor();
    const search = screen.getByRole('textbox', { name: 'Search' });

    fireEvent.change(search, { target: { value: 'environment:production' } });
    expect(onChange).not.toHaveBeenCalled();
    expect(onRunQuery).not.toHaveBeenCalled();

    fireEvent.blur(search);
    expect(lastCall(onChange).q).toBe('environment:production');
    expect(onRunQuery).not.toHaveBeenCalled();
  });

  it('Enter after blur runs the applied text', async () => {
    const onChange = jest.fn();
    const onRunQuery = jest.fn();
    const datasource = { getProjects: () => Promise.resolve(PROJECTS) } as unknown as BugsinkDataSource;
    const { rerender } = render(
      <QueryEditor datasource={datasource} query={{ refId: 'A' }} onChange={onChange} onRunQuery={onRunQuery} />
    );
    const search = await screen.findByRole('textbox', { name: 'Search' });
    fireEvent.change(search, { target: { value: 'release:1.4.2' } });
    fireEvent.blur(search);
    rerender(
      <QueryEditor
        datasource={datasource}
        query={{ refId: 'A', q: 'release:1.4.2' }}
        onChange={onChange}
        onRunQuery={onRunQuery}
      />
    );

    fireEvent.keyDown(search, { key: 'Enter' });

    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onRunQuery).toHaveBeenCalledTimes(1);
  });

  it('Enter in Search applies the text and runs once', async () => {
    const { onChange, onRunQuery } = await renderEditor();
    const search = screen.getByRole('textbox', { name: 'Search' });

    fireEvent.change(search, { target: { value: 'release:1.4.2' } });
    fireEvent.keyDown(search, { key: 'Enter' });

    expect(lastCall(onChange).q).toBe('release:1.4.2');
    expect(onRunQuery).toHaveBeenCalledTimes(1);
  });

  it('offers Arrival order only while a project is chosen', async () => {
    await renderEditor();
    fireEvent.click(screen.getByRole('combobox', { name: 'Sort by' }));
    await screen.findByRole('option', { name: 'Event count' });
    expect(screen.queryByRole('option', { name: 'Arrival order' })).not.toBeInTheDocument();
  });

  it('offers Arrival order when a project is chosen', async () => {
    await renderEditor({ project: 2 });
    fireEvent.click(screen.getByRole('combobox', { name: 'Sort by' }));
    expect(await screen.findByRole('option', { name: 'Arrival order' })).toBeInTheDocument();
  });

  it('choosing All projects while sorted by arrival falls back to Last seen', async () => {
    const { onChange } = await renderEditor({ project: 2, sort: 'digest_order' });

    await pick('Project', 'All projects');

    const next = lastCall(onChange);
    expect(next).not.toHaveProperty('project');
    expect(next.sort).toBe('last_seen');
  });

  it.each([0, null])('treats a saved project of %s as all projects', async (project) => {
    await renderEditor({ project: project as unknown as number });

    const projectBox = screen.getByRole('combobox', { name: 'Project' });
    expect(projectBox).toHaveValue('All projects');
    fireEvent.click(projectBox);
    const names = (await screen.findAllByRole('option')).map((option) => option.textContent);
    expect(names).toEqual(['All projects', 'sample-js', 'sample-python']);
    fireEvent.click(projectBox);

    fireEvent.click(screen.getByRole('combobox', { name: 'Sort by' }));
    await screen.findByRole('option', { name: 'Event count' });
    expect(screen.queryByRole('option', { name: 'Arrival order' })).not.toBeInTheDocument();
  });

  it('shows a saved project that is not in the list as Project <id>', async () => {
    await renderEditor({ project: 99 });

    expect(screen.getByRole('combobox', { name: 'Project' })).toHaveValue('Project 99');
  });

  it('shows the message when the projects fail to load and keeps the other controls working', async () => {
    const getProjects = () => Promise.reject({ data: { message: 'Bugsink rejected the API token.' } });
    const { onChange } = await renderEditor({}, getProjects);

    expect(await screen.findByText('Bugsink rejected the API token.')).toBeInTheDocument();

    await pick('State', 'Open');
    expect(lastCall(onChange).state).toBe('open');
  });

  it('falls back to a generic message when the failure carries none', async () => {
    await renderEditor({}, () => Promise.reject(new Error('')));

    expect(await screen.findByText('Could not load the projects.')).toBeInTheDocument();
  });

  it("forgets the previous data source's projects", async () => {
    const failing = {
      getProjects: () => Promise.reject({ data: { message: 'Bugsink rejected the API token.' } }),
    } as unknown as BugsinkDataSource;
    const other = { getProjects: () => Promise.resolve([{ id: 3, name: 'other' }]) } as unknown as BugsinkDataSource;
    const props = { query: { refId: 'A' }, onChange: jest.fn(), onRunQuery: jest.fn() };

    const { rerender } = render(<QueryEditor datasource={failing} {...props} />);
    expect(await screen.findByText('Bugsink rejected the API token.')).toBeInTheDocument();

    rerender(<QueryEditor datasource={other} {...props} />);
    await waitFor(() => expect(screen.queryByText('Bugsink rejected the API token.')).not.toBeInTheDocument());

    fireEvent.click(screen.getByRole('combobox', { name: 'Project' }));
    const names = (await screen.findAllByRole('option')).map((option) => option.textContent);
    expect(names).toEqual(['All projects', 'other']);
  });

  it('ignores a late answer from the previous data source', async () => {
    let resolveFirst: (projects: unknown) => void = () => {};
    const first = {
      getProjects: () => new Promise((resolve) => (resolveFirst = resolve)),
    } as unknown as BugsinkDataSource;
    const other = { getProjects: () => Promise.resolve([{ id: 3, name: 'other' }]) } as unknown as BugsinkDataSource;
    const props = { query: { refId: 'A' }, onChange: jest.fn(), onRunQuery: jest.fn() };

    const { rerender } = render(<QueryEditor datasource={first} {...props} />);
    rerender(<QueryEditor datasource={other} {...props} />);
    fireEvent.click(screen.getByRole('combobox', { name: 'Project' }));
    await screen.findByRole('option', { name: 'other' });
    await act(async () => resolveFirst([{ id: 1, name: 'stale' }]));

    const names = (await screen.findAllByRole('option')).map((option) => option.textContent);
    expect(names).toEqual(['All projects', 'other']);
  });
});
