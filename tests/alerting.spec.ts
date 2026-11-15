// SPDX-License-Identifier: GPL-3.0-or-later
import type { APIRequestContext } from '@playwright/test';
import { expect, test, thisRunsIssue } from './fixtures';
import { EVENT_MARKER, PROJECT_READ } from './seeded';

const FOLDER = 'bugsink-e2e-alerts';
const GROUP = 'bugsink-e2e';
const PROVENANCE = { 'X-Disable-Provenance': 'true' };
const POLL = { timeout: 60_000 };

type Query = Record<string, unknown>;
type RuleState = {
  name: string;
  state: string;
  health: string;
  lastEvaluation: string;
  lastError?: string;
  alerts?: Alert[];
};
type Alert = { state: string; labels: Record<string, string> };

function ruleBody(title: string, datasourceUid: string, query: Query, threshold: number) {
  return {
    title,
    ruleGroup: GROUP,
    folderUID: FOLDER,
    orgID: 1,
    condition: 'C',
    for: '0s',
    noDataState: 'NoData',
    execErrState: 'Error',
    data: [
      {
        refId: 'A',
        datasourceUid,
        relativeTimeRange: { from: 600, to: 0 },
        model: { ...query, refId: 'A', datasource: { uid: datasourceUid }, intervalMs: 1000, maxDataPoints: 43200 },
      },
      {
        refId: 'C',
        datasourceUid: '__expr__',
        model: {
          refId: 'C',
          type: 'threshold',
          expression: 'A',
          conditions: [{ evaluator: { type: 'gt', params: [threshold] } }],
          datasource: { type: '__expr__', uid: '__expr__' },
        },
      },
    ],
  };
}

async function createRule(
  request: APIRequestContext,
  title: string,
  datasourceUid: string,
  query: Query,
  threshold = 0
) {
  const created = await request.post('/api/v1/provisioning/alert-rules', {
    headers: PROVENANCE,
    data: ruleBody(title, datasourceUid, query, threshold),
  });
  expect(created.status(), `creating the rule ${title}`).toBe(201);
}

async function evaluateEveryTenSeconds(request: APIRequestContext) {
  const url = `/api/v1/provisioning/folder/${FOLDER}/rule-groups/${GROUP}`;
  const group = await (await request.get(url)).json();
  const updated = await request.put(url, { headers: PROVENANCE, data: { ...group, interval: 10 } });
  expect(updated.ok(), `setting the ${GROUP} group to a 10 s interval`).toBe(true);
}

async function readRules(request: APIRequestContext): Promise<RuleState[]> {
  const response = await request.get(`/api/prometheus/grafana/api/v1/rules?folder_uid=${FOLDER}`);
  expect(response.ok()).toBe(true);
  const groups: Array<{ rules: RuleState[] }> = (await response.json()).data.groups;
  return groups.flatMap((group) => group.rules);
}

function ruleNamed(rules: RuleState[], name: string) {
  const rule = rules.find((candidate) => candidate.name === name);
  expect(rule, `rule ${name} is evaluated; rules seen: ${rules.map((r) => r.name).join(', ') || 'none'}`).toBeDefined();
  return rule!;
}

// evaluatedAt is the time of a rule's last evaluation in milliseconds, or NaN before its first.
function evaluatedAt(rule: RuleState) {
  const at = Date.parse(rule.lastEvaluation);
  return at > 0 ? at : NaN;
}

async function deleteFolder(request: APIRequestContext) {
  return request.delete(`/api/folders/${FOLDER}?forceDeleteRules=true`);
}

async function createFolder(request: APIRequestContext) {
  const folder = await request.post('/api/folders', { data: { uid: FOLDER, title: 'Bugsink e2e alerts' } });
  expect([200, 409], `creating the folder ${FOLDER}`).toContain(folder.status());
}

async function deleteRuleTitled(request: APIRequestContext, title: string) {
  const listed = await request.get('/api/v1/provisioning/alert-rules');
  expect(listed.ok(), 'listing the alert rules').toBe(true);
  const rules: Array<{ uid: string; title: string; folderUID: string }> = await listed.json();
  for (const rule of rules.filter((candidate) => candidate.title === title && candidate.folderUID === FOLDER)) {
    const deleted = await request.delete(`/api/v1/provisioning/alert-rules/${rule.uid}`, { headers: PROVENANCE });
    expect(deleted.status(), `deleting the leftover rule ${title}`).toBe(204);
  }
}

const DASHBOARD_PANELS: Array<{ title: string; reads: (query: Query) => boolean }> = [
  { title: 'Open issues in sample-python', reads: (q) => q.queryType === 'issueCount' && q.state === 'open' },
  { title: 'Events in sample-python this hour', reads: (q) => q.queryType === 'eventCount' },
  { title: 'Issues seen in the last hour', reads: (q) => q.queryType === 'issueCount' && q.minutes === 60 },
  { title: 'New issues per day in sample-python', reads: (q) => q.queryType === 'newIssues' },
];

test.describe('alerting', () => {
  test.describe.configure({ mode: 'serial' });

  test.afterAll(async ({ request }) => {
    const deleted = await deleteFolder(request);
    expect([200, 404], 'the e2e folder and its rules are deleted').toContain(deleted.status());
    const rules = await readRules(request);
    expect(
      rules.map((rule) => rule.name),
      'no rule of the e2e folder is left'
    ).toEqual([]);
  });

  test('the data source is offered for alert rules', async ({ alertRuleEditPage, page }) => {
    const row = await alertRuleEditPage.getQueryRow();
    await row.datasource.set('Bugsink');
    await page.getByRole('radio', { name: 'Event count' }).click();
    await page.getByRole('combobox', { name: 'Project' }).click();
    await page.getByRole('option', { name: 'sample-python' }).click();
    await expect(alertRuleEditPage.evaluate()).toBeOK();
  });

  test('the demo rules fire', async ({ request, readProvisionedDataSource }) => {
    test.setTimeout(120_000);
    const bugsink = await readProvisionedDataSource({ fileName: 'datasources.yaml', name: 'Bugsink' });
    const issue = await thisRunsIssue(request);

    const cleared = await deleteFolder(request);
    expect([200, 404], 'clearing the e2e folder').toContain(cleared.status());
    await createFolder(request);

    await createRule(request, 'bugsink-e2e-events', bugsink.uid, { queryType: 'eventCount', project: 1, hours: 2 });
    await createRule(
      request,
      'bugsink-e2e-events-high',
      bugsink.uid,
      { queryType: 'eventCount', project: 1, hours: 2 },
      100_000
    );
    await createRule(request, 'bugsink-e2e-open', bugsink.uid, {
      queryType: 'issueCount',
      project: 1,
      state: 'open',
      q: EVENT_MARKER,
    });
    await evaluateEveryTenSeconds(request);

    await expect(async () => {
      const rules = await readRules(request);
      for (const name of ['bugsink-e2e-events', 'bugsink-e2e-open']) {
        const rule = ruleNamed(rules, name);
        expect(rule.state, `${name}: ${rule.health} ${rule.lastError ?? ''}`).toBe('firing');
        expect(
          rule.alerts?.map((alert) => [alert.state, alert.labels.project_id]),
          `${name}: one Alerting instance for project 1`
        ).toEqual([['Alerting', '1']]);
      }
      const high = ruleNamed(rules, 'bugsink-e2e-events-high');
      expect(
        evaluatedAt(high),
        'bugsink-e2e-events-high was evaluated at or after bugsink-e2e-events'
      ).toBeGreaterThanOrEqual(evaluatedAt(ruleNamed(rules, 'bugsink-e2e-events')));
      expect(high.health, `bugsink-e2e-events-high: ${high.lastError ?? ''}`).toBe('ok');
      expect(high.state, 'bugsink-e2e-events-high stays below its threshold').toBe('inactive');
      expect(
        (high.alerts ?? []).filter((alert) => alert.state !== 'Normal'),
        'bugsink-e2e-events-high has no firing instance'
      ).toEqual([]);
    }, 'both demo rules fire within a minute and the high threshold does not').toPass(POLL);

    await expect(async () => {
      const counted = await request.post('/api/ds/query', {
        data: {
          from: 'now-10m',
          to: 'now',
          queries: [
            {
              refId: 'A',
              datasource: { uid: bugsink.uid },
              queryType: 'eventCount',
              issue: issue.friendlyId,
              hours: 2,
            },
          ],
        },
      });
      expect(counted.ok(), 'the event count query answers').toBe(true);
      const result = (await counted.json()).results.A;
      expect(result.error, 'the event count query has no error').toBeUndefined();
      expect(result.frames[0].data.values[0][0], "the count includes this run's event").toBeGreaterThanOrEqual(1);
    }, "the event count of this run's issue reaches 1").toPass(POLL);
  });

  test('a refused count puts its rule in Error', async ({ request, readProvisionedDataSource }) => {
    test.setTimeout(120_000);
    const bounded = await readProvisionedDataSource({ fileName: 'datasources.yaml', name: 'Bugsink project-bound' });
    await createFolder(request);
    await deleteRuleTitled(request, 'bugsink-e2e-refused');
    await createRule(request, 'bugsink-e2e-refused', bounded.uid, { queryType: 'issueCount', project: 2 });
    await evaluateEveryTenSeconds(request);

    await expect(async () => {
      const rule = ruleNamed(await readRules(request), 'bugsink-e2e-refused');
      expect(rule.health, 'the refused rule is in error').toBe('error');
      expect(rule.lastError).toContain('This token is not allowed to access this project.');
      expect(rule.lastError).not.toContain(PROJECT_READ);
      expect(rule.alerts?.map((alert) => alert.state)).toEqual(['Error']);
    }, 'the refused rule reaches Error within a minute').toPass(POLL);
  });

  test('the current values dashboard opens clean', async ({
    gotoDashboardPage,
    page,
    readProvisionedDashboard,
    selectors,
  }) => {
    const dashboard = await readProvisionedDashboard({ fileName: 'bugsink-alerting.json' });
    const answers = DASHBOARD_PANELS.map(({ reads }) =>
      page.waitForResponse(
        (response) =>
          response.url().includes('/api/ds/query') &&
          (response.request().postDataJSON()?.queries ?? []).some((query: Query) => reads(query))
      )
    );
    const dashboardPage = await gotoDashboardPage({ uid: dashboard.uid });
    for (const [i, answered] of answers.entries()) {
      const { title } = DASHBOARD_PANELS[i];
      const response = await answered;
      expect(response.status(), `${title}: status`).toBe(200);
      const result = (await response.json()).results.A;
      expect(result.error, `${title}: error`).toBeUndefined();
      expect(result.frames?.length, `${title}: frames`).toBeGreaterThan(0);
      expect(result.frames[0].schema?.meta?.notices, `${title}: notices`).toBeUndefined();
    }
    const open = dashboardPage.getPanelByTitle('Open issues in sample-python');
    await expect(open.locator).toContainText(/[1-9]\d*$/);

    for (const { title } of DASHBOARD_PANELS) {
      const panel = dashboardPage.getPanelByTitle(title);
      await expect(panel.locator, title).toBeVisible();
      await expect(panel.getErrorIcon(), title).toHaveCount(0);
      await expect(panel.getByGrafanaSelector(selectors.components.Panels.Panel.status('warning')), title).toHaveCount(
        0
      );
      await expect(panel.locator.getByText('No data'), title).toHaveCount(0);
    }
  });
});
