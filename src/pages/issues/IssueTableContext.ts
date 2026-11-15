// SPDX-License-Identifier: GPL-3.0-or-later
import { createContext, useContext } from 'react';
import { BugsinkDataSource } from '../../datasource/datasource';
import { IssueRow, TriageAction } from '../../datasource/types';

export type VolumeSource = Pick<BugsinkDataSource, 'issueVolume'>;

export interface IssueTableValue {
  source: VolumeSource;
  issueHref: (row: IssueRow) => string;
  busyIds?: ReadonlySet<string>;
  disabled?: boolean;
  onAction?: (row: IssueRow, action: TriageAction) => void;
}

export const IssueTableContext = createContext<IssueTableValue | null>(null);

export function useIssueTable(): IssueTableValue {
  const value = useContext(IssueTableContext);
  if (value === null) {
    throw new Error('IssueTableContext is missing');
  }
  return value;
}
