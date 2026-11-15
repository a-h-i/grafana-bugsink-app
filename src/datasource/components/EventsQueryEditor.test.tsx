// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { BugsinkDataSource } from '../datasource';
import { BugsinkQuery } from '../types';
import { EventsQueryEditor } from './EventsQueryEditor';

// jsdom has no layout, so the virtualised option list renders nothing without a size.
beforeAll(() => {
  jest.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(500);
  jest.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(300);
});

afterAll(() => jest.restoreAllMocks());

async function renderEditor(query: Partial<BugsinkQuery> = {}) {
  const onChange = jest.fn();
  const onRunQuery = jest.fn();
  const datasource = {
    getProjects: () =>
      Promise.resolve([
        { id: 1, name: 'sample-python' },
        { id: 2, name: 'sample-js' },
      ]),
  } as unknown as BugsinkDataSource;
  render(
    <EventsQueryEditor
      datasource={datasource}
      query={{ refId: 'A', queryType: 'events', ...query }}
      onChange={onChange}
      onRunQuery={onRunQuery}
    />
  );
  await screen.findByRole('combobox', { name: 'Project' });
  return { onChange, onRunQuery };
}

describe('EventsQueryEditor', () => {
  it('shows Project, Issue, Search and Event ID with their placeholders', async () => {
    await renderEditor({ issue: 'X-1', q: 'trace:abc', eventId: 'e1' });

    expect(screen.getByRole('textbox', { name: 'Issue' })).toHaveValue('X-1');
    expect(screen.getByRole('textbox', { name: 'Issue' })).toHaveAttribute('placeholder', 'SAMPLE-PYTHON-2');
    expect(screen.getByRole('textbox', { name: 'Search' })).toHaveValue('trace:abc');
    expect(screen.getByRole('textbox', { name: 'Search' })).toHaveAttribute(
      'placeholder',
      'trace:4bf92f3577b34da6a3ce929d0e0e4736'
    );
    expect(screen.getByRole('textbox', { name: 'Event ID' })).toHaveValue('e1');
    expect(screen.getByRole('textbox', { name: 'Event ID' })).toHaveAttribute(
      'placeholder',
      '426ac644fafb475d905046d654730f1b'
    );
  });

  it('typing in Issue calls nothing until blur, then changes issue without running', async () => {
    const { onChange, onRunQuery } = await renderEditor();
    const issue = screen.getByRole('textbox', { name: 'Issue' });

    fireEvent.change(issue, { target: { value: 'SAMPLE-PYTHON-2' } });
    expect(onChange).not.toHaveBeenCalled();

    fireEvent.blur(issue);
    expect(onChange).toHaveBeenCalledWith({ refId: 'A', queryType: 'events', issue: 'SAMPLE-PYTHON-2' });
    expect(onRunQuery).not.toHaveBeenCalled();
  });

  it('Enter in Event ID applies and runs once', async () => {
    const { onChange, onRunQuery } = await renderEditor();
    const eventId = screen.getByRole('textbox', { name: 'Event ID' });

    fireEvent.change(eventId, { target: { value: '426ac644fafb475d905046d654730f1b' } });
    fireEvent.keyDown(eventId, { key: 'Enter' });

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ eventId: '426ac644fafb475d905046d654730f1b' }));
    expect(onRunQuery).toHaveBeenCalledTimes(1);
  });

  it('blur in Search applies without running', async () => {
    const { onChange, onRunQuery } = await renderEditor();
    const search = screen.getByRole('textbox', { name: 'Search' });

    fireEvent.change(search, { target: { value: 'trace:abc' } });
    fireEvent.blur(search);

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ q: 'trace:abc' }));
    expect(onRunQuery).not.toHaveBeenCalled();
  });

  it('picking sample-js sets project 2 and runs', async () => {
    const { onChange, onRunQuery } = await renderEditor();

    fireEvent.click(screen.getByRole('combobox', { name: 'Project' }));
    fireEvent.click(await screen.findByRole('option', { name: 'sample-js' }));

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ project: 2 }));
    expect(onRunQuery).toHaveBeenCalledTimes(1);
  });

  it('picking All projects removes the project and runs', async () => {
    const { onChange, onRunQuery } = await renderEditor({ project: 2 });

    fireEvent.click(screen.getByRole('combobox', { name: 'Project' }));
    fireEvent.click(await screen.findByRole('option', { name: 'All projects' }));

    expect(onChange.mock.calls[0][0]).not.toHaveProperty('project');
    expect(onRunQuery).toHaveBeenCalledTimes(1);
  });
});
