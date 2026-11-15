// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { App } from './App';

const mockSource = {
  getProjects: jest.fn().mockResolvedValue([]),
  listIssues: jest.fn().mockResolvedValue({ rows: [] }),
  canTriage: jest.fn().mockReturnValue(false),
  getIssue: jest.fn(),
  getIssueTags: jest.fn(() => new Promise(() => {})),
  getIssueHistory: jest.fn(() => new Promise(() => {})),
  listEvents: jest.fn(() => new Promise(() => {})),
};

jest.mock('@grafana/runtime', () => ({
  config: { bootData: { user: { timezone: 'utc' } } },
  getDataSourceSrv: () => ({
    getList: () => [{ uid: 'bugsink', name: 'Bugsink' }],
    get: () => Promise.resolve(mockSource),
  }),
  PluginPage: ({ actions, children }: { actions?: React.ReactNode; children: React.ReactNode }) => (
    <div>
      <div>{actions}</div>
      {children}
    </div>
  ),
}));

function renderApp(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]} future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
      <Routes>
        <Route path="/a/ahmedismail-bugsink-app/*" element={<App />} />
      </Routes>
    </MemoryRouter>
  );
}

describe('App', () => {
  beforeEach(() => {
    mockSource.getIssue.mockReset().mockReturnValue(new Promise(() => {}));
    mockSource.listIssues.mockClear();
  });

  it('shows the issue list at the root', async () => {
    renderApp('/a/ahmedismail-bugsink-app?ds=bugsink');

    expect(await screen.findByText('No issues match these filters in this time range.')).toBeInTheDocument();
    expect(mockSource.getIssue).not.toHaveBeenCalled();
  });

  it('shows the issue page under issues/:issueId', async () => {
    renderApp('/a/ahmedismail-bugsink-app/issues/X-1?ds=bugsink');

    expect(await screen.findByText('Loading issue...')).toBeInTheDocument();
    expect(mockSource.getIssue).toHaveBeenCalledWith('X-1');
    expect(mockSource.listIssues).not.toHaveBeenCalled();
  });
});
