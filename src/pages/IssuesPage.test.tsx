// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { locationUtil } from '@grafana/data';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { IssueRow } from '../datasource/types';
import { IssuesPage } from './IssuesPage';

const mockGetList = jest.fn();
const mockGet = jest.fn();
const mockSources: Record<
  string,
  { getProjects: jest.Mock; listIssues: jest.Mock; canTriage: jest.Mock; triage: jest.Mock; issueVolume: jest.Mock }
> = {};

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
        canTriage: jest.fn().mockReturnValue(false),
        triage: jest.fn(),
        issueVolume: jest.fn().mockResolvedValue({ times: [1, 2], counts: [0, 2], total: 2 }),
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

  it('aborts the previous list request on a new search and the current one on unmount', async () => {
    mockSources.bugsink.listIssues.mockReturnValue(new Promise(() => {}));
    const { unmount } = renderPage('?ds=bugsink');
    await waitFor(() => expect(mockSources.bugsink.listIssues).toHaveBeenCalledTimes(1));
    const first: AbortSignal = mockSources.bugsink.listIssues.mock.calls[0][2];
    expect(first.aborted).toBe(false);
    await screen.findByRole('combobox', { name: 'State' });

    await pick('State', 'Muted');

    await waitFor(() => expect(mockSources.bugsink.listIssues).toHaveBeenCalledTimes(2));
    expect(first.aborted).toBe(true);
    const second: AbortSignal = mockSources.bugsink.listIssues.mock.calls[1][2];
    expect(second.aborted).toBe(false);

    unmount();

    expect(second.aborted).toBe(true);
  });

  it('shows no message when the list read it cancelled rejects', async () => {
    mockSources.bugsink.listIssues.mockImplementation(
      (_query: unknown, _range: unknown, signal: AbortSignal) =>
        new Promise((resolve, reject) => {
          if (mockSources.bugsink.listIssues.mock.calls.length === 1) {
            signal.addEventListener('abort', () => reject(Object.assign(new Error('aborted'), { name: 'AbortError' })));
          } else {
            resolve({ rows: [row('u2', 'MUTED-1')] });
          }
        })
    );
    renderPage('?ds=bugsink');
    await waitFor(() => expect(mockSources.bugsink.listIssues).toHaveBeenCalledTimes(1));
    await screen.findByRole('combobox', { name: 'State' });

    await pick('State', 'Muted');

    expect(await screen.findByText('MUTED-1')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('shows no alert above the table when a sparkline request fails', async () => {
    mockSources.bugsink.issueVolume.mockRejectedValue(new Error('volume down'));

    renderPage('?ds=bugsink');

    expect(await screen.findByLabelText('Events per hour unavailable: volume down')).toBeInTheDocument();
    expect(screen.getByText('BUGSINK-1')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
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

  describe('triage', () => {
    const statusOf = (friendlyId: string) =>
      within(screen.getByRole('row', { name: new RegExp(friendlyId) })).getByText(/^(Open|Muted|Resolved)$/)
        .textContent;

    beforeEach(() => {
      mockSources.bugsink.canTriage.mockReturnValue(true);
      mockSources.bugsink.listIssues.mockResolvedValue({ rows: [row('u1', 'SAMPLE-PYTHON-2')] });
    });

    it('shows no Actions column when the caller cannot triage', async () => {
      mockSources.bugsink.canTriage.mockReturnValue(false);
      renderPage('?ds=bugsink');

      await screen.findByText('SAMPLE-PYTHON-2');

      expect(screen.queryByRole('columnheader', { name: 'Actions' })).not.toBeInTheDocument();
    });

    it('resolves a row from the answer without loading the list again', async () => {
      mockSources.bugsink.triage.mockResolvedValue({ id: 'u1', status: 'resolved' });
      renderPage('?ds=bugsink');
      await screen.findByText('SAMPLE-PYTHON-2');
      const loads = mockSources.bugsink.listIssues.mock.calls.length;

      await act(async () => fireEvent.click(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-2' })));

      expect(mockSources.bugsink.triage).toHaveBeenCalledWith('u1', 'resolve');
      await waitFor(() => expect(statusOf('SAMPLE-PYTHON-2')).toBe('Resolved'));
      expect(screen.queryByRole('button', { name: /^Resolve SAMPLE-PYTHON-2/ })).not.toBeInTheDocument();
      expect(mockSources.bugsink.listIssues).toHaveBeenCalledTimes(loads);
    });

    it('a succeeded action does not restart any sparkline', async () => {
      mockSources.bugsink.triage.mockResolvedValue({ id: 'u1', status: 'resolved' });
      renderPage('?ds=bugsink');
      await screen.findByText('SAMPLE-PYTHON-2');
      const volumeCalls = mockSources.bugsink.issueVolume.mock.calls.length;
      const signals: AbortSignal[] = mockSources.bugsink.issueVolume.mock.calls.map((call) => call[1]);

      await act(async () => fireEvent.click(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-2' })));

      await waitFor(() => expect(statusOf('SAMPLE-PYTHON-2')).toBe('Resolved'));
      expect(mockSources.bugsink.issueVolume).toHaveBeenCalledTimes(volumeCalls);
      expect(signals.some((signal) => signal.aborted)).toBe(false);
    });

    it('a failed action does not restart any sparkline', async () => {
      mockSources.bugsink.triage.mockRejectedValue(new Error('Bugsink refused the request: nope.'));
      renderPage('?ds=bugsink');
      await screen.findByText('SAMPLE-PYTHON-2');
      const volumeCalls = mockSources.bugsink.issueVolume.mock.calls.length;
      const loads = mockSources.bugsink.listIssues.mock.calls.length;
      const signals: AbortSignal[] = mockSources.bugsink.issueVolume.mock.calls.map((call) => call[1]);

      await act(async () => fireEvent.click(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-2' })));

      await waitFor(() => expect(mockSources.bugsink.listIssues).toHaveBeenCalledTimes(loads + 1));
      expect(await screen.findByText('SAMPLE-PYTHON-2: Bugsink refused the request: nope.')).toBeInTheDocument();
      expect(mockSources.bugsink.issueVolume).toHaveBeenCalledTimes(volumeCalls);
      expect(signals.some((signal) => signal.aborted)).toBe(false);
    });

    it('shows the failure inline and loads the list again', async () => {
      mockSources.bugsink.triage.mockRejectedValue(
        new Error('Bugsink refused the request: Issue is already resolved.')
      );
      renderPage('?ds=bugsink');
      await screen.findByText('SAMPLE-PYTHON-2');
      const loads = mockSources.bugsink.listIssues.mock.calls.length;

      await act(async () => fireEvent.click(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-2' })));

      expect(
        await screen.findByText('SAMPLE-PYTHON-2: Bugsink refused the request: Issue is already resolved.')
      ).toBeInTheDocument();
      await waitFor(() => expect(mockSources.bugsink.listIssues).toHaveBeenCalledTimes(loads + 1));
      expect(
        screen.getByText('SAMPLE-PYTHON-2: Bugsink refused the request: Issue is already resolved.')
      ).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-2' })).toBeEnabled();
    });

    it('sends one request for a double click', async () => {
      mockSources.bugsink.triage.mockReturnValue(new Promise(() => {}));
      renderPage('?ds=bugsink');
      await screen.findByText('SAMPLE-PYTHON-2');
      const button = screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-2' });

      fireEvent.click(button);
      fireEvent.click(button);

      expect(mockSources.bugsink.triage).toHaveBeenCalledTimes(1);
      expect(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-2' })).toBeDisabled();
    });

    it('keeps the buttons of each issue disabled until its own answer arrives', async () => {
      mockSources.bugsink.listIssues.mockResolvedValue({
        rows: [row('u1', 'SAMPLE-PYTHON-2'), row('u2', 'SAMPLE-PYTHON-3')],
      });
      const answers: Record<string, (value: unknown) => void> = {};
      mockSources.bugsink.triage.mockImplementation((id: string) => new Promise((resolve) => (answers[id] = resolve)));
      renderPage('?ds=bugsink');
      await screen.findByText('SAMPLE-PYTHON-3');

      fireEvent.click(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-2' }));
      fireEvent.click(screen.getByRole('button', { name: 'Mute SAMPLE-PYTHON-3' }));

      expect(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-2' })).toBeDisabled();
      expect(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-3' })).toBeDisabled();

      await act(async () => answers.u2({ id: 'u2', status: 'muted' }));

      expect(statusOf('SAMPLE-PYTHON-3')).toBe('Muted');
      expect(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-3' })).toBeEnabled();
      expect(statusOf('SAMPLE-PYTHON-2')).toBe('Open');
      expect(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-2' })).toBeDisabled();

      await act(async () => answers.u1({ id: 'u1', status: 'resolved' }));

      expect(statusOf('SAMPLE-PYTHON-2')).toBe('Resolved');
      expect(statusOf('SAMPLE-PYTHON-3')).toBe('Muted');
    });

    it('drops an answer that arrives after the list has changed', async () => {
      let answer: (value: unknown) => void = () => {};
      mockSources.bugsink.triage.mockReturnValue(new Promise((resolve) => (answer = resolve)));
      renderPage('?ds=bugsink');
      await screen.findByText('SAMPLE-PYTHON-2');
      await screen.findByRole('combobox', { name: 'State' });
      fireEvent.click(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-2' }));
      let land: (value: unknown) => void = () => {};
      mockSources.bugsink.listIssues.mockReturnValueOnce(new Promise((resolve) => (land = resolve)));

      await pick('State', 'Muted');
      await waitFor(() => expect(lastQuery('bugsink')[0].state).toBe('muted'));
      await act(async () => answer({ id: 'u1', status: 'resolved' }));

      expect(statusOf('SAMPLE-PYTHON-2')).toBe('Open');
      await act(async () => land({ rows: [row('u1', 'SAMPLE-PYTHON-2')] }));
      expect(statusOf('SAMPLE-PYTHON-2')).toBe('Open');
      expect(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-2' })).toBeEnabled();
    });

    it('actions wait for the reload', async () => {
      renderPage('?ds=bugsink');
      await screen.findByText('SAMPLE-PYTHON-2');
      let land: (value: unknown) => void = () => {};
      mockSources.bugsink.listIssues.mockReturnValueOnce(new Promise((resolve) => (land = resolve)));

      fireEvent.click(screen.getByTestId('data-testid RefreshPicker run button'));

      expect(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-2' })).toBeDisabled();
      await act(async () => land({ rows: [row('u1', 'SAMPLE-PYTHON-2')] }));
      expect(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-2' })).toBeEnabled();
    });

    it('actions come back when the reload fails', async () => {
      renderPage('?ds=bugsink');
      await screen.findByText('SAMPLE-PYTHON-2');
      mockSources.bugsink.listIssues.mockRejectedValueOnce(new Error('Bugsink is down.'));

      fireEvent.click(screen.getByTestId('data-testid RefreshPicker run button'));

      expect(await screen.findByText('Bugsink is down.')).toBeInTheDocument();
      fireEvent.click(screen.getByTestId('data-testid RefreshPicker run button'));

      expect(await screen.findByRole('button', { name: 'Resolve SAMPLE-PYTHON-2' })).toBeEnabled();
    });

    it('a failure after a new search is shown', async () => {
      let fail: (reason: unknown) => void = () => {};
      mockSources.bugsink.triage.mockReturnValue(new Promise((_, reject) => (fail = reject)));
      renderPage('?ds=bugsink');
      await screen.findByText('SAMPLE-PYTHON-2');
      await screen.findByRole('combobox', { name: 'State' });
      fireEvent.click(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-2' }));

      await pick('State', 'Muted');
      await waitFor(() => expect(lastQuery('bugsink')[0].state).toBe('muted'));
      await screen.findByText('SAMPLE-PYTHON-2');
      const loads = mockSources.bugsink.listIssues.mock.calls.length;
      await act(async () => fail(new Error('Bugsink refused the request: Issue is already resolved.')));

      expect(
        screen.getByText('SAMPLE-PYTHON-2: Bugsink refused the request: Issue is already resolved.')
      ).toBeInTheDocument();
      expect(mockSources.bugsink.listIssues).toHaveBeenCalledTimes(loads);
      expect(statusOf('SAMPLE-PYTHON-2')).toBe('Open');
    });

    it('the failure clears on the next action', async () => {
      mockSources.bugsink.triage.mockRejectedValueOnce(new Error('Issue is already resolved.'));
      renderPage('?ds=bugsink');
      await screen.findByText('SAMPLE-PYTHON-2');
      await act(async () => fireEvent.click(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-2' })));
      await screen.findByText('SAMPLE-PYTHON-2: Issue is already resolved.');
      await waitFor(() => expect(screen.getByRole('button', { name: 'Mute SAMPLE-PYTHON-2' })).toBeEnabled());
      mockSources.bugsink.triage.mockReturnValue(new Promise(() => {}));

      fireEvent.click(screen.getByRole('button', { name: 'Mute SAMPLE-PYTHON-2' }));

      expect(screen.queryByText(/Issue is already resolved/)).not.toBeInTheDocument();
    });

    it('the failure clears on the next load', async () => {
      mockSources.bugsink.triage.mockRejectedValueOnce(new Error('Issue is already resolved.'));
      renderPage('?ds=bugsink');
      await screen.findByText('SAMPLE-PYTHON-2');
      await act(async () => fireEvent.click(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-2' })));
      await screen.findByText('SAMPLE-PYTHON-2: Issue is already resolved.');
      await waitFor(() => expect(screen.getByRole('button', { name: 'Mute SAMPLE-PYTHON-2' })).toBeEnabled());
      let land: (value: unknown) => void = () => {};
      mockSources.bugsink.listIssues.mockReturnValueOnce(new Promise((resolve) => (land = resolve)));

      fireEvent.click(screen.getByTestId('data-testid RefreshPicker run button'));
      await act(async () => land({ rows: [row('u1', 'SAMPLE-PYTHON-2')] }));

      expect(screen.queryByText(/Issue is already resolved/)).not.toBeInTheDocument();
    });

    it('a warning shows with the short id', async () => {
      mockSources.bugsink.triage.mockResolvedValue({
        id: 'u1',
        status: 'resolved',
        warning: 'The comment could not be added.',
      });
      renderPage('?ds=bugsink');
      await screen.findByText('SAMPLE-PYTHON-2');
      const loads = mockSources.bugsink.listIssues.mock.calls.length;

      await act(async () => fireEvent.click(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-2' })));

      expect(await screen.findByTestId('data-testid Alert warning')).toHaveTextContent(
        'SAMPLE-PYTHON-2: The comment could not be added.'
      );
      expect(statusOf('SAMPLE-PYTHON-2')).toBe('Resolved');
      expect(mockSources.bugsink.listIssues).toHaveBeenCalledTimes(loads);
    });

    it('a warning after a new search is shown', async () => {
      let answer: (value: unknown) => void = () => {};
      mockSources.bugsink.triage.mockReturnValue(new Promise((resolve) => (answer = resolve)));
      renderPage('?ds=bugsink');
      await screen.findByText('SAMPLE-PYTHON-2');
      await screen.findByRole('combobox', { name: 'State' });
      fireEvent.click(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-2' }));

      await pick('State', 'Muted');
      await waitFor(() => expect(lastQuery('bugsink')[0].state).toBe('muted'));
      await screen.findByText('SAMPLE-PYTHON-2');
      const loads = mockSources.bugsink.listIssues.mock.calls.length;
      await act(async () => answer({ id: 'u1', status: 'resolved', warning: 'W' }));

      expect(screen.getByTestId('data-testid Alert warning')).toHaveTextContent('SAMPLE-PYTHON-2: W');
      expect(mockSources.bugsink.listIssues).toHaveBeenCalledTimes(loads);
      expect(statusOf('SAMPLE-PYTHON-2')).toBe('Open');
    });

    it('a warning does not come back when its search comes back', async () => {
      mockSources.bugsink.triage.mockResolvedValue({ id: 'u1', status: 'resolved', warning: 'W' });
      renderPage('?ds=bugsink');
      await screen.findByText('SAMPLE-PYTHON-2');
      await screen.findByRole('combobox', { name: 'State' });
      await act(async () => fireEvent.click(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-2' })));
      expect(await screen.findByText('SAMPLE-PYTHON-2: W')).toBeInTheDocument();

      await pick('State', 'Muted');
      await waitFor(() => expect(lastQuery('bugsink')[0].state).toBe('muted'));
      await screen.findByText('SAMPLE-PYTHON-2');
      await pick('State', 'Unresolved');
      await waitFor(() => expect(lastQuery('bugsink')[0].state).toBe('unresolved'));
      await waitFor(() => expect(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-2' })).toBeEnabled());

      expect(screen.queryByText('SAMPLE-PYTHON-2: W')).not.toBeInTheDocument();
    });

    it('a warning does not come back when its search comes back before the other search loads', async () => {
      mockSources.bugsink.triage.mockResolvedValue({ id: 'u1', status: 'resolved', warning: 'W' });
      renderPage('?ds=bugsink');
      await screen.findByText('SAMPLE-PYTHON-2');
      await screen.findByRole('combobox', { name: 'State' });
      await act(async () => fireEvent.click(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-2' })));
      expect(await screen.findByText('SAMPLE-PYTHON-2: W')).toBeInTheDocument();
      mockSources.bugsink.listIssues.mockReturnValueOnce(new Promise(() => {}));

      await pick('State', 'Muted');
      await waitFor(() => expect(lastQuery('bugsink')[0].state).toBe('muted'));
      await pick('State', 'Unresolved');
      await waitFor(() => expect(lastQuery('bugsink')[0].state).toBe('unresolved'));
      await waitFor(() => expect(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-2' })).toBeEnabled());

      expect(screen.queryByText('SAMPLE-PYTHON-2: W')).not.toBeInTheDocument();
    });

    it('two failures at once load the list again once and show the last', async () => {
      mockSources.bugsink.listIssues.mockResolvedValue({
        rows: [row('u1', 'SAMPLE-PYTHON-2'), row('u2', 'SAMPLE-PYTHON-3')],
      });
      const fails: Record<string, (reason: unknown) => void> = {};
      mockSources.bugsink.triage.mockImplementation((id: string) => new Promise((_, reject) => (fails[id] = reject)));
      renderPage('?ds=bugsink');
      await screen.findByText('SAMPLE-PYTHON-3');
      const loads = mockSources.bugsink.listIssues.mock.calls.length;
      fireEvent.click(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-2' }));
      fireEvent.click(screen.getByRole('button', { name: 'Mute SAMPLE-PYTHON-3' }));

      await act(async () => {
        fails.u1(new Error('first'));
        fails.u2(new Error('second'));
      });

      await waitFor(() => expect(screen.getByRole('button', { name: 'Resolve SAMPLE-PYTHON-2' })).toBeEnabled());
      expect(mockSources.bugsink.listIssues).toHaveBeenCalledTimes(loads + 1);
      expect(screen.getByText('SAMPLE-PYTHON-3: second')).toBeInTheDocument();
    });
  });
});
