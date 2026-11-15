// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { BugsinkDataSource } from '../datasource';
import { ProjectPicker } from './ProjectPicker';

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

function source(getProjects: () => Promise<unknown>) {
  return { getProjects } as unknown as BugsinkDataSource;
}

async function renderPicker(value?: number, getProjects: () => Promise<unknown> = () => Promise.resolve(PROJECTS)) {
  const onChange = jest.fn();
  render(<ProjectPicker datasource={source(getProjects)} value={value} onChange={onChange} />);
  await screen.findByRole('combobox', { name: 'Project' });
  return { onChange };
}

async function optionNames() {
  return (await screen.findAllByRole('option')).map((option) => option.textContent);
}

describe('ProjectPicker', () => {
  it('lists All projects first, then the projects by name', async () => {
    await renderPicker();

    fireEvent.click(screen.getByRole('combobox', { name: 'Project' }));

    expect(await optionNames()).toEqual(['All projects', 'sample-js', 'sample-python']);
  });

  it('reports the picked project', async () => {
    const { onChange } = await renderPicker();

    fireEvent.click(screen.getByRole('combobox', { name: 'Project' }));
    fireEvent.click(await screen.findByRole('option', { name: 'sample-js' }));

    expect(onChange).toHaveBeenCalledWith(2);
  });

  it('reports no project when All projects is picked', async () => {
    const { onChange } = await renderPicker(2);

    fireEvent.click(screen.getByRole('combobox', { name: 'Project' }));
    fireEvent.click(await screen.findByRole('option', { name: 'All projects' }));

    expect(onChange).toHaveBeenCalledWith(undefined);
  });

  it.each([0, null])('treats a saved project of %s as all projects', async (project) => {
    await renderPicker(project as unknown as number);

    const box = screen.getByRole('combobox', { name: 'Project' });
    expect(box).toHaveValue('All projects');
    fireEvent.click(box);
    expect(await optionNames()).toEqual(['All projects', 'sample-js', 'sample-python']);
  });

  it('shows a saved project that is not in the list as Project <id>', async () => {
    await renderPicker(99);

    expect(screen.getByRole('combobox', { name: 'Project' })).toHaveValue('Project 99');
  });

  it('shows the message when the projects fail to load', async () => {
    await renderPicker(undefined, () => Promise.reject({ data: { message: 'Bugsink rejected the API token.' } }));

    expect(await screen.findByText('Bugsink rejected the API token.')).toBeInTheDocument();
  });

  it('falls back to a generic message when the failure carries none', async () => {
    await renderPicker(undefined, () => Promise.reject(new Error('')));

    expect(await screen.findByText('Could not load the projects.')).toBeInTheDocument();
  });

  it("forgets the previous data source's projects", async () => {
    const failing = source(() => Promise.reject({ data: { message: 'Bugsink rejected the API token.' } }));
    const other = source(() => Promise.resolve([{ id: 3, name: 'other' }]));

    const { rerender } = render(<ProjectPicker datasource={failing} onChange={jest.fn()} />);
    expect(await screen.findByText('Bugsink rejected the API token.')).toBeInTheDocument();

    rerender(<ProjectPicker datasource={other} onChange={jest.fn()} />);
    await waitFor(() => expect(screen.queryByText('Bugsink rejected the API token.')).not.toBeInTheDocument());

    fireEvent.click(screen.getByRole('combobox', { name: 'Project' }));
    expect(await optionNames()).toEqual(['All projects', 'other']);
  });

  it('ignores a late answer from the previous data source', async () => {
    let resolveFirst: (projects: unknown) => void = () => {};
    const first = source(() => new Promise((resolve) => (resolveFirst = resolve)));
    const other = source(() => Promise.resolve([{ id: 3, name: 'other' }]));

    const { rerender } = render(<ProjectPicker datasource={first} onChange={jest.fn()} />);
    rerender(<ProjectPicker datasource={other} onChange={jest.fn()} />);
    fireEvent.click(screen.getByRole('combobox', { name: 'Project' }));
    await screen.findByRole('option', { name: 'other' });
    await act(async () => resolveFirst([{ id: 1, name: 'stale' }]));

    expect(await optionNames()).toEqual(['All projects', 'other']);
  });
});
