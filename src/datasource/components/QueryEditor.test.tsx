// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { BugsinkDataSource } from '../datasource';
import { BugsinkQuery } from '../types';
import { HOURS_MESSAGE, MINUTES_MESSAGE } from '../counts';
import { QueryEditor } from './QueryEditor';

// jsdom has no layout, so the virtualised option list renders nothing without a size.
beforeAll(() => {
  jest.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(500);
  jest.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(300);
});

afterAll(() => jest.restoreAllMocks());

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

describe('QueryEditor alerting queries', () => {
  it('offers the six query types', async () => {
    await renderEditor();

    const labels = screen
      .getAllByRole('radio')
      .map((radio) => radio.getAttribute('aria-label') ?? radio.nextSibling?.textContent);
    expect(labels).toEqual(['Issues', 'Events', 'Event volume', 'New issues', 'Issue count', 'Event count']);
  });

  it('choosing Issue count sets the type and runs', async () => {
    const { onChange, onRunQuery } = await renderEditor({ project: 2 });

    fireEvent.click(screen.getByRole('radio', { name: 'Issue count' }));

    expect(onChange).toHaveBeenCalledWith({ refId: 'A', project: 2, queryType: 'issueCount' });
    expect(onRunQuery).toHaveBeenCalledTimes(1);
  });

  it('renders the issue count controls and not the sort controls', async () => {
    await renderEditor({ queryType: 'issueCount' });

    expect(screen.getByRole('radio', { name: 'Issue count' })).toBeChecked();
    expect(screen.getByRole('combobox', { name: 'Project' })).toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: 'State' })).toHaveValue('Unresolved');
    expect(screen.getByRole('textbox', { name: 'Search' })).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: 'Seen within (minutes)' })).toHaveAttribute('placeholder', 'any time');
    expect(screen.queryByRole('combobox', { name: 'Sort by' })).not.toBeInTheDocument();
    expect(screen.queryByRole('combobox', { name: 'Order' })).not.toBeInTheDocument();
  });

  it('renders the event count controls', async () => {
    await renderEditor({ queryType: 'eventCount' });

    expect(screen.getByRole('combobox', { name: 'Project' })).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: 'Issue' })).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: 'Hours' })).toHaveAttribute('placeholder', '1');
    expect(screen.getByText('The hour in progress plus the full hours before it.')).toBeInTheDocument();
    expect(screen.queryByRole('textbox', { name: 'Search' })).not.toBeInTheDocument();
    expect(screen.queryByRole('combobox', { name: 'State' })).not.toBeInTheDocument();
  });

  it('an event count naming neither project nor issue says what it needs', async () => {
    await renderEditor({ queryType: 'eventCount' });

    expect(screen.getByText('An event count needs a project or an issue.')).toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: 'Project' })).toHaveValue('No project');
  });

  it.each([{ project: 2 }, { issue: 'X-1' }])('an event count with %o does not show the requirement', async (named) => {
    await renderEditor({ queryType: 'eventCount', ...named });

    expect(screen.queryByText('An event count needs a project or an issue.')).not.toBeInTheDocument();
  });

  it.each(['issues', 'events', 'volume', 'newIssues', 'issueCount'] as const)(
    'the %s editor still offers All projects',
    async (queryType) => {
      await renderEditor({ queryType });

      expect(screen.getByRole('combobox', { name: 'Project' })).toHaveValue('All projects');
      expect(screen.queryByText('An event count needs a project or an issue.')).not.toBeInTheDocument();
    }
  );

  it('renders the new issues controls', async () => {
    await renderEditor({ queryType: 'newIssues' });

    expect(screen.getByRole('radio', { name: 'New issues' })).toBeChecked();
    expect(screen.getByRole('combobox', { name: 'Project' })).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: 'Search' })).toBeInTheDocument();
    expect(screen.queryByRole('combobox', { name: 'State' })).not.toBeInTheDocument();
    expect(screen.queryByRole('textbox', { name: 'Issue' })).not.toBeInTheDocument();
  });

  it('applies a typed new issues search on blur', async () => {
    const { onChange, onRunQuery } = await renderEditor({ queryType: 'newIssues' });

    const input = screen.getByRole('textbox', { name: 'Search' });
    fireEvent.change(input, { target: { value: 'release:1' } });
    fireEvent.blur(input);

    expect(onChange).toHaveBeenCalledWith({ refId: 'A', queryType: 'newIssues', q: 'release:1' });
    expect(onRunQuery).not.toHaveBeenCalled();
  });

  describe('minutes', () => {
    const minutes = () => screen.getByRole('textbox', { name: 'Seen within (minutes)' });

    it('typing 60 and Enter sets minutes and runs once', async () => {
      const { onChange, onRunQuery } = await renderEditor({ queryType: 'issueCount' });

      fireEvent.change(minutes(), { target: { value: '60' } });
      fireEvent.keyDown(minutes(), { key: 'Enter' });

      expect(onChange).toHaveBeenCalledWith({ refId: 'A', queryType: 'issueCount', minutes: 60 });
      expect(onRunQuery).toHaveBeenCalledTimes(1);
    });

    it.each(['1', '1440'])('accepts the boundary %s', async (text) => {
      const { onChange } = await renderEditor({ queryType: 'issueCount' });

      fireEvent.change(minutes(), { target: { value: text } });
      fireEvent.blur(minutes());

      expect(onChange).toHaveBeenCalledWith({ refId: 'A', queryType: 'issueCount', minutes: Number(text) });
    });

    it.each(['0', '1441', '61.5', 'abc', '5m'])('shows the message for %s and neither commits nor runs', async (text) => {
      const { onChange, onRunQuery } = await renderEditor({ queryType: 'issueCount' });

      fireEvent.change(minutes(), { target: { value: text } });
      expect(screen.getByText(MINUTES_MESSAGE)).toBeInTheDocument();
      fireEvent.blur(minutes());
      fireEvent.keyDown(minutes(), { key: 'Enter' });

      expect(onChange).not.toHaveBeenCalled();
      expect(onRunQuery).not.toHaveBeenCalled();
    });

    it('still changes the state while minutes is invalid', async () => {
      const { onChange, onRunQuery } = await renderEditor({ queryType: 'issueCount' });

      fireEvent.change(minutes(), { target: { value: '61.5' } });
      fireEvent.click(screen.getByRole('combobox', { name: 'State' }));
      fireEvent.click(await screen.findByRole('option', { name: 'Resolved' }));

      expect(onChange).toHaveBeenCalledWith({ refId: 'A', queryType: 'issueCount', state: 'resolved' });
      expect(onRunQuery).toHaveBeenCalledTimes(1);
    });

    it('clearing the field removes minutes', async () => {
      const { onChange, onRunQuery } = await renderEditor({ queryType: 'issueCount', minutes: 60 });

      fireEvent.change(minutes(), { target: { value: '' } });
      fireEvent.keyDown(minutes(), { key: 'Enter' });

      expect(onChange).toHaveBeenCalledWith({ refId: 'A', queryType: 'issueCount' });
      expect(onRunQuery).toHaveBeenCalledTimes(1);
    });
  });

  describe('hours', () => {
    const hours = () => screen.getByRole('textbox', { name: 'Hours' });

    it('typing 6 and Enter sets hours and runs once', async () => {
      const { onChange, onRunQuery } = await renderEditor({ queryType: 'eventCount' });

      fireEvent.change(hours(), { target: { value: '6' } });
      fireEvent.keyDown(hours(), { key: 'Enter' });

      expect(onChange).toHaveBeenCalledWith({ refId: 'A', queryType: 'eventCount', hours: 6 });
      expect(onRunQuery).toHaveBeenCalledTimes(1);
    });

    it.each(['1', '24'])('accepts the boundary %s', async (text) => {
      const { onChange } = await renderEditor({ queryType: 'eventCount' });

      fireEvent.change(hours(), { target: { value: text } });
      fireEvent.blur(hours());

      expect(onChange).toHaveBeenCalledWith({ refId: 'A', queryType: 'eventCount', hours: Number(text) });
    });

    it.each(['0', '25', '2.5', 'abc'])('shows the message for %s and neither commits nor runs', async (text) => {
      const { onChange, onRunQuery } = await renderEditor({ queryType: 'eventCount' });

      fireEvent.change(hours(), { target: { value: text } });
      expect(screen.getByText(HOURS_MESSAGE)).toBeInTheDocument();
      fireEvent.blur(hours());
      fireEvent.keyDown(hours(), { key: 'Enter' });

      expect(onChange).not.toHaveBeenCalled();
      expect(onRunQuery).not.toHaveBeenCalled();
    });

    it('clearing the field removes hours', async () => {
      const { onChange } = await renderEditor({ queryType: 'eventCount', hours: 6 });

      fireEvent.change(hours(), { target: { value: '' } });
      fireEvent.blur(hours());

      expect(onChange).toHaveBeenCalledWith({ refId: 'A', queryType: 'eventCount' });
    });
  });

  it('keeps the other types fields when switching type', async () => {
    const { onChange } = await renderEditor({ queryType: 'issueCount', project: 2, minutes: 30 });

    fireEvent.click(screen.getByRole('radio', { name: 'Event count' }));

    expect(onChange).toHaveBeenCalledWith({ refId: 'A', queryType: 'eventCount', project: 2, minutes: 30 });
  });

  it('renders without a time range, panel data or app', async () => {
    await renderEditor({ queryType: 'eventCount' });

    expect(screen.getByRole('textbox', { name: 'Hours' })).toBeInTheDocument();
  });
});
