// SPDX-License-Identifier: GPL-3.0-or-later
import React from 'react';
import { Route, Routes } from 'react-router-dom';
import { IssuePage } from './IssuePage';
import { IssuesPage } from './IssuesPage';

export function App(): React.JSX.Element {
  return (
    <Routes>
      <Route path="issues/:issueId" element={<IssuePage />} />
      <Route path="*" element={<IssuesPage />} />
    </Routes>
  );
}
