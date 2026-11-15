// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { locationUtil } from '@grafana/data';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { IssueRow } from '../datasource/types';
import { IssuesPage } from './IssuesPage';

const mockGetList = jest.fn();
const mockGet = jest.fn();
const mockSources: Record<string, { getProjects: jest.Mock; listIssues: jest.Mock }> = {};

jest.mock('@grafana/runtime', () => ({
  config: { bootData: { user: { timezone: 'utc' } } },
  getDataSourceSrv: () => ({
    getList: (filter: unknown) => mockGetList(filter),
    get: (uid: string) => mockGet(uid),
  }),
  PluginPage: ({ actions, children }: { actions?: React.ReactNode; children: React.ReactNode }) => (
    <div>
      <div>{actions}</div>
      {children}
    </div>
  ),
}));

// jsdom has no layout, so the virtualised option list renders nothing without a size.
beforeAll(() => {
  jest.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(500);
  jest.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(300);
});

afterAll(() => jest.restoreAllMocks());

const INSTANCES = [
  { uid: 'bugsink', name: 'Bugsink' },
  { uid: 'eu', name: 'Bugsink EU' },
];

function row(id: string, friendlyId: string): IssueRow {
  return {
    id,
    friendlyId,
    type: 'ValueError',
    message: `message ${id}`,
    transaction: '/x',
    project: 'sample-python',
    projectId: 1,
    status: 'open',
    events: 3,
    firstSeen: 1763164800000,
    lastSeen: 1763164800000,
  };
}

function Location() {
  return <div data-testid="location-search">{useLocation().search}</div>;
}

function renderPage(search = '') {
  return render(
    <MemoryRouter
      initialEntries={[`/a/app${search}`]}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      <Routes>
        <Route path="/a/app" element={<IssuesPage />} />
      </Routes>
      <Location />
    </MemoryRouter>
  );
}

const search = () => screen.getByTestId('location-search').textContent;
const spinning = () =>
  within(screen.getByTestId('data-testid RefreshPicker run button')).queryByTestId('spinner') !== null;
const lastQuery = (uid: string) => {
  const calls = mockSources[uid].listIssues.mock.calls;
  return calls[calls.length - 1];
};

async function pick(name: string, option: string) {
  fireEvent.click(screen.getByRole('combobox', { name }));
  const choice = await screen.findByRole('option', { name: option });
  await act(async () => fireEvent.click(choice));
}

describe('IssuesPage', () => {
  beforeEach(() => {
    mockGetList.mockReset().mockReturnValue(INSTANCES);
    mockGet.mockReset().mockImplementation((uid: string) => Promise.resolve(mockSources[uid]));
    for (const uid of ['bugsink', 'eu']) {
      mockSources[uid] = {
        getProjects: jest.fn().mockResolvedValue([{ id: 2, name: 'sample-js' }]),
        listIssues: jest.fn().mockResolvedValue({ rows: [row(`${uid}-1`, `${uid.toUpperCase()}-1`)] }),
      };
    }
  });

  it('asks for the instances of the Bugsink data source type', async () => {
    renderPage();

    await screen.findByText('BUGSINK-1');
    expect(mockGetList).toHaveBeenCalledWith({ pluginId: 'ahmedismail-bugsink-datasource' });
  });

  it('shows the empty state with a link when there is no instance', () => {
    mockGetList.mockReturnValue([]);

    renderPage();

    expect(screen.getByText('No Bugsink data source yet.')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Add a Bugsink data source' })).toHaveAttribute(
      'href',
      '/connections/datasources/new'
    );
  });

  it('prefixes the add link with the Grafana sub-path', () => {
    mockGetList.mockReturnValue([]);
    const init = (appSubUrl: string) =>
      locationUtil.initialize({
        config: { appSubUrl } as never,
        getVariablesUrlParams: jest.fn(),
        getTimeRangeForUrl: jest.fn(),
      });
    init('/grafana');

    try {
      render(
        <MemoryRouter
          basename="/grafana"
          initialEntries={['/grafana']}
          future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
        >
          <IssuesPage />
        </MemoryRouter>
      );

      expect(screen.getByRole('link', { name: 'Add a Bugsink data source' })).toHaveAttribute(
        'href',
        '/grafana/connections/datasources/new'
      );
    } finally {
      init('');
    }
  });

  it('chooses the first instance and writes the state to the URL', async () => {
    renderPage();

    await screen.findByText('BUGSINK-1');
    expect(search()).toBe('?ds=bugsink&state=unresolved&sort=last_seen&order=desc&from=now-7d&to=now');
    expect(screen.getByRole('combobox', { name: 'Data source' })).toHaveValue('Bugsink');
  });

  it('chooses the default instance', async () => {
    mockGetList.mockReturnValue([INSTANCES[0], { ...INSTANCES[1], isDefault: true }]);

    renderPage();

    await screen.findByText('EU-1');
    expect(search()).toContain('ds=eu');
  });

  it('chooses the first instance by name when none is the default', async () => {
    mockGetList.mockReturnValue([INSTANCES[1], INSTANCES[0]]);

    renderPage();

    await screen.findByText('BUGSINK-1');
    expect(search()).toContain('ds=bugsink');
  });

  it('keeps the address parameters it does not own, verbatim', async () => {
    renderPage('?orgId=1&kiosk&var-x=a%20b&ds=bugsink&state=open');
    await screen.findByText('BUGSINK-1');

    expect(search()).toBe(
      '?orgId=1&kiosk&var-x=a%20b&ds=bugsink&state=open&sort=last_seen&order=desc&from=now-7d&to=now'
    );

    await pick('State', 'Muted');

    await waitFor(() =>
      expect(search()).toBe(
        '?orgId=1&kiosk&var-x=a%20b&ds=bugsink&state=muted&sort=last_seen&order=desc&from=now-7d&to=now'
      )
    );
  });

  it('queries with the URL state and shows it in the pickers', async () => {
    renderPage('?ds=eu&project=2&state=open&sort=digested_event_count&order=desc&from=now-24h&to=now');

    await screen.findByText('EU-1');
    const [query, range] = lastQuery('eu');
    expect(query).toMatchObject({ project: 2, state: 'open', sort: 'digested_event_count', order: 'desc' });
    expect(range.raw).toEqual({ from: 'now-24h', to: 'now' });
    expect(mockSources.bugsink.listIssues).not.toHaveBeenCalled();
    expect(screen.getByRole('combobox', { name: 'State' })).toHaveValue('Open');
    expect(screen.getByRole('combobox', { name: 'Sort by' })).toHaveValue('Event count');
  });

  it('a filter change writes the URL and queries again; refresh repeats the query', async () => {
    renderPage('?ds=bugsink');
    await screen.findByText('BUGSINK-1');
    await screen.findByRole('combobox', { name: 'Project' });

    await pick('State', 'Muted');

    await waitFor(() => expect(search()).toContain('state=muted'));
    await waitFor(() => expect(lastQuery('bugsink')[0].state).toBe('muted'));
    const calls = mockSources.bugsink.listIssues.mock.calls.length;
    const [query, range] = lastQuery('bugsink');

    fireEvent.click(screen.getByTestId('data-testid RefreshPicker run button'));

    await waitFor(() => expect(mockSources.bugsink.listIssues).toHaveBeenCalledTimes(calls + 1));
    expect(lastQuery('bugsink')[0]).toEqual(query);
    expect(lastQuery('bugsink')[1].raw).toEqual(range.raw);
  });

  it('switching instance drops the project and arrival order and loads the new projects', async () => {
    renderPage('?ds=bugsink&project=2&sort=digest_order');
    await screen.findByText('BUGSINK-1');

    await pick('Data source', 'Bugsink EU');

    await screen.findByText('EU-1');
    expect(search()).toContain('ds=eu');
    expect(search()).not.toContain('project');
    expect(search()).toContain('sort=last_seen');
    await waitFor(() => expect(mockSources.eu.getProjects).toHaveBeenCalled());
    expect(lastQuery('eu')[0].project).toBeUndefined();
  });

  it('explains an unknown data source and runs no query', async () => {
    renderPage('?ds=nosuch');

    expect(await screen.findByText('The data source "nosuch" was not found.')).toBeInTheDocument();
    expect(mockSources.bugsink.listIssues).not.toHaveBeenCalled();
    expect(mockSources.eu.listIssues).not.toHaveBeenCalled();
    expect(spinning()).toBe(false);
  });

  it('explains a data source that fails to load', async () => {
    mockGet.mockRejectedValue(new Error('plugin failed'));

    renderPage('?ds=bugsink');

    expect(await screen.findByText('The data source "Bugsink" could not be loaded.')).toBeInTheDocument();
    expect(screen.queryByText('Loading issues...')).not.toBeInTheDocument();
    expect(spinning()).toBe(false);
  });

  it('refresh loads a data source that failed to load', async () => {
    mockGet.mockRejectedValueOnce(new Error('plugin failed'));
    renderPage('?ds=bugsink');
    await screen.findByText('The data source "Bugsink" could not be loaded.');

    fireEvent.click(screen.getByTestId('data-testid RefreshPicker run button'));

    expect(await screen.findByText('BUGSINK-1')).toBeInTheDocument();
    expect(mockGet).toHaveBeenCalledTimes(2);
    expect(screen.queryByText('The data source "Bugsink" could not be loaded.')).not.toBeInTheDocument();
  });

  it('links each issue to its detail page with the address parameters', async () => {
    renderPage('?orgId=1&kiosk&ds=bugsink&state=open');

    const link = await screen.findByRole('link', { name: 'BUGSINK-1' });
    expect(link).toHaveAttribute(
      'href',
      '/a/ahmedismail-bugsink-app/issues/bugsink-1?orgId=1&kiosk&ds=bugsink&state=open&sort=last_seen&order=desc&from=now-7d&to=now'
    );
  });

  it('falls back to defaults for an invalid project and range', async () => {
    renderPage('?project=abc&from=garbage');

    await screen.findByText('BUGSINK-1');
    const [query, range] = lastQuery('bugsink');
    expect(query.project).toBeUndefined();
    expect(range.raw).toEqual({ from: 'now-7d', to: 'now' });
    await waitFor(() =>
      expect(search()).toBe('?ds=bugsink&state=unresolved&sort=last_seen&order=desc&from=now-7d&to=now')
    );
  });

  it('shows a placeholder while the first load is pending', async () => {
    mockSources.bugsink.listIssues.mockReturnValue(new Promise(() => {}));

    renderPage('?ds=bugsink');

    await waitFor(() => expect(mockSources.bugsink.listIssues).toHaveBeenCalled());
    expect(screen.getByText('Loading issues...')).toBeInTheDocument();
  });

  it('keeps the old rows with a loading bar while a later load is pending', async () => {
    renderPage('?ds=bugsink');
    await screen.findByText('BUGSINK-1');
    await screen.findByRole('combobox', { name: 'State' });
    let resolve: (value: unknown) => void = () => {};
    mockSources.bugsink.listIssues.mockReturnValueOnce(new Promise((r) => (resolve = r)));

    await pick('State', 'Muted');

    await waitFor(() => expect(lastQuery('bugsink')[0].state).toBe('muted'));
    expect(screen.getByText('BUGSINK-1')).toBeInTheDocument();
    expect(screen.getByRole('status', { name: 'Loading bar' })).toBeInTheDocument();

    await act(async () => resolve({ rows: [row('m', 'MUTED-1')] }));

    expect(await screen.findByText('MUTED-1')).toBeInTheDocument();
    expect(screen.queryByRole('status', { name: 'Loading bar' })).not.toBeInTheDocument();
  });

  it('shows the backend message and neither table nor empty state when the query fails', async () => {
    mockSources.bugsink.listIssues.mockRejectedValue(
      new Error('Bugsink refused the request: No Project matches the given query.')
    );

    renderPage('?ds=bugsink');

    expect(
      await screen.findByText('Bugsink refused the request: No Project matches the given query.')
    ).toBeInTheDocument();
    expect(screen.queryByRole('table')).not.toBeInTheDocument();
    expect(screen.queryByText('No issues match these filters in this time range.')).not.toBeInTheDocument();
  });

  it('shows the notice above the rows', async () => {
    mockSources.bugsink.listIssues.mockResolvedValue({
      rows: [row('1', 'SAMPLE-1')],
      notice: 'Showing the first 5000 issues.',
    });

    renderPage('?ds=bugsink');

    const notice = await screen.findByText('Showing the first 5000 issues.');
    const table = await screen.findByRole('table');
    expect(notice.compareDocumentPosition(table) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it('shows only the answer to the latest state when answers arrive out of order', async () => {
    let resolveFirst: (value: unknown) => void = () => {};
    mockSources.bugsink.listIssues
      .mockReturnValueOnce(new Promise((resolve) => (resolveFirst = resolve)))
      .mockResolvedValueOnce({ rows: [row('2', 'SECOND-1')] });
    renderPage('?ds=bugsink');
    await screen.findByText('Loading issues...');
    await screen.findByRole('combobox', { name: 'State' });

    await pick('State', 'Open');
    await screen.findByText('SECOND-1');
    await act(async () => resolveFirst({ rows: [row('1', 'FIRST-1')] }));

    expect(screen.getByText('SECOND-1')).toBeInTheDocument();
    expect(screen.queryByText('FIRST-1')).not.toBeInTheDocument();
  });
});
