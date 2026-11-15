# M3: Issue List Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A user opens the Bugsink app, picks a Bugsink instance and browses its issues in a table, filtered by project, state and time range, sorted by last seen or event count, with a link from each row to the issue in Bugsink; the page's state lives in the URL.

**Architecture:** The app's root page runs the data source's existing issues query (M2's `QueryData`) through a new `BugsinkDataSource.listIssues`, which turns the `issues` frame into typed rows. The page is plain React with `@grafana/ui`: a data source picker, the M2 query editor as the filter bar, Grafana's time range picker and an `InteractiveTable` with one React cell per column. The backend changes only in remembering a failed project read.

**Tech Stack:** Go 1.26 with `grafana-plugin-sdk-go` v0.299.0; React 19, `@grafana/ui`/`@grafana/data`/`@grafana/runtime` 13.2.4, `react-router-dom` 6.30.6; Jest; `@grafana/plugin-e2e` 4.0.0; the Compose stack (Bugsink 2.6.3, Grafana 13.3.1). No new dependency.

**Spec:** [`../bugsink_grafana_plugin_spec.md`](../bugsink_grafana_plugin_spec.md) (§3, §4.B, §4.D). **Roadmap:** [`../implementation_roadmap.md`](../implementation_roadmap.md) (M3, Definition of done). **Previous plan:** [`m2-issues-in-dashboards.md`](m2-issues-in-dashboards.md) (frame and message contracts, As built).

## Global Constraints

* The API token never reaches the browser, a log line, an error message or a frame.
* The backend pages with the `cursor` taken from `next`; it never builds a cursor and never requests a URL taken from a response. It sends `sort` and `order` on every issue list request.
* Resources never answer 401: Bugsink failures are 502, invalid settings 400, a refused caller 403.
* Reuse what exists instead of parallel code: `userMessage`, the instance caches, `d.projects`, the query model and `DEFAULT_QUERY`, the M2 `QueryEditor`, `getProjects`, `tests/fixtures.ts`, `tests/seeded.ts`, the `report-event` setup project.
* The page shows the backend's message text verbatim; it never rewrites a message of the M2 message contract.
* Generated code stays in `pkg/bugsink/internal/gen/` and is not regenerated. Do not modify anything in `.config/`. Do not change a plugin id or type; a `plugin.json` change needs a Grafana restart (this plan changes none).
* Every hand-written source file starts with `SPDX-License-Identifier: GPL-3.0-or-later` (JSON exempt); `scripts/check_spdx.sh` passes.
* Comments are one line and describe the code as it is. Tests assert behaviour, not code structure.
* Go commands run on `./pkg/...`: `node_modules` contains a Go package.
* Host ports stay `3300` (Grafana), `8800` (Bugsink), `8810` (sample app).
* End-to-end tests produce at most 5 Bugsink events per run; this milestone adds none (the `report-event` setup reports one).
* Work on branch `m3/issue-list` in the worktree `.claude/worktrees/m3-issue-list` (created from `main`, `node_modules` installed). Every implementer uses this same branch and worktree; push after every commit; never amend.
* The running stack mounts the main checkout's `dist/` and provisioning. Before the first stack check, run `docker compose up -d --wait` from the worktree (the Compose project is named `bugsink-grafana-plugin`, so Grafana is recreated with the worktree's mounts; data volumes are kept). Never `docker compose down`.
* Run every test command in the foreground.

## Facts already verified (2026-11-15, Compose stack)

* `state=open` excludes muted issues, `state=unresolved` includes them: issue `SAMPLE-PYTHON-5` muted with the triage token was absent from `open`, present in `unresolved` and `muted`; it was unmuted afterwards (`is_muted: false`, `is_resolved: false`).
* The stack holds no resolved or muted issue; `project=1&state=open&sort=digested_event_count&order=desc` starts with event counts 106, 22, 20, 9, 3.
* `POST /api/ds/query` with `{"queries":[{"refId":"A","datasource":{"uid":"bugsink"},"project":1,"state":"open","sort":"digested_event_count","order":"desc"}],"from":"now-7d","to":"now"}` returns the M2 frame (16 fields, 263 rows); field `friendly_id` has `config.links[0]` = `{title: "Open in Bugsink", targetBlank: true, url: "http://localhost:8800/issues/issue/${__data.fields.id}/event/last/"}`.
* The same with `project: 99` answers HTTP 400 with `{"results":{"A":{"error":"Bugsink refused the request: No Project matches the given query.","errorSource":"downstream","status":404,"frames":[]}}}`.
* The project-bound token (`c`×40) lists only project 1 in `GET projects/` and only project-1 issues in `GET issues/` without `project`. The token without capabilities gets 403 `This token does not have the required capability: projects:read.` (and `issues:read.` on `issues/`).
* The health check already probes `projects:read`, `issues:read` and `events:read` (`pkg/plugin/health.go:45-49`); M3 needs no other capability.
* Grafana 13.3.1's `getList` adds `-- Grafana --`, `-- Mixed --` and `-- Dashboard --` only when the filter has neither `pluginId` nor `alerting`, and keeps an instance when `pluginId` matches its `meta` (minified `public/build/18631.*.js`). The frontend's `DataSourceInstanceSettings.url` of a proxy-access data source is `/api/datasources/proxy/uid/<uid>`, not the Bugsink URL (same file).
* `DataSourceWithBackend.query` applies `applyTemplateVariables`, posts to `/api/ds/query` and turns a failed fetch into a response with `error` (`node_modules/@grafana/runtime/dist/esm/utils/DataSourceWithBackend.mjs:63-215`).
* `@grafana/ui` 13.2.4 exports `InteractiveTable` (`columns`, `data`, `getRowId`, `pageSize`; a column without `sortType` is not sortable; `cell` renders a custom cell), `TimeRangePicker` (requires `value`, `onChange`, `onChangeTimeZone`, `onMoveBackward`, `onMoveForward`, `onZoom`), `RefreshPicker` (`onRefresh`, `isLoading`, `noIntervalPicker`), `EmptyState` (`variant`: `call-to-action` | `not-found` | `completed`), `LoadingPlaceholder`, `Combobox`, `TextLink`, `Alert`, `Stack`.
* `@grafana/data` 13.2.4 exports `dateMath.parse`, `toURLRange`, `dateTimeFormat`, `DataFrameView`, `createDataFrame`, `FieldType`. `@grafana/runtime` exports `PluginPage` (with `actions`), `getDataSourceSrv`, `config` (`config.bootData.user.timezone`); its `DataSourcePicker` is marked `@internal`.
* `@grafana/scenes` is not a dependency. Its latest version is 9.2.2, Apache-2.0, with peers `@grafana/* >=12.4` and 13 runtime dependencies (`react-grid-layout`, `react-select`, `lodash`, …).
* `react-router` is a webpack external (`.config/bundler/externals.ts:26`); `react-router-dom` 6.30.6 is bundled and uses Grafana's router.
* `@grafana/plugin-e2e` `NavigateOptions` has `queryParams?: URLSearchParams`; the breadcrumb selector is `data-testid ${title} breadcrumb` (e2e-selectors, since 9.4.0).
* **Not verified:** that the breadcrumb selector finds the app's breadcrumb in Grafana 13.3.1, and that `useSearchParams` writes survive a reload inside a plugin page. Task 5 Step 1 checks both first.

## Decisions in this plan, for review

1. **The page runs the issues query, not a new resource route.** One code path keeps the time range rule, page limit notice, cache, validation and messages identical for dashboards and the app; M4's detail reads and M7's triage calls get their own resource routes, as the roadmap says.
2. **Plain React with `@grafana/ui`, not `@grafana/scenes`.** Every later column is a React cell (M7's buttons, M9's sparkline, M4's link to the detail page), which a Scenes table panel cannot host, so Scenes would only add variables and URL sync; its variables are interpolated strings and only `q` takes variables (M2 decision 7); and it is a new dependency with 13 transitive ones. Spec §3 names Scenes; the as-built step updates it.
3. **The filter bar is the M2 `QueryEditor`.** Panels and the app share one set of controls, so it also brings Search, Order and Arrival order beyond the roadmap's filters.
4. **Sort only through `Sort by`;** column headers are not sortable. The sort is Bugsink's, so it has one control.
5. **An `Issue` column (friendly id) carries the row link,** as in the dashboard table, left of the §4.D columns. Its URL is the `friendly_id` field's link with `${__data.fields.id}` replaced by the row's id: the frontend cannot know the Browser URL's default (see Facts).
6. **25 rows per page, paged in the browser.** The query already returns at most 5000 rows, and a page bounds M9's per-visible-row calls.
7. **Defaults:** the query defaults of M2 (`unresolved`, `last_seen`, `desc`, all projects); time range `now-7d` to `now`, because an issue list is browsed for open work and a week covers a quiet weekend; the instance marked default, else the first by name.
8. **Picking another instance drops `project`,** and a `digest_order` sort becomes `last_seen`: project ids belong to one instance.
9. **The data source picker is a `Combobox` over `getList({ pluginId })`,** not runtime's `@internal` `DataSourcePicker`; with `pluginId` Grafana adds no built-in data sources, so M1's client-side type filter goes.
10. **A failed project read is remembered for the cache TTL (15 s),** except a cancelled or timed-out one. Every page load runs a query and a project read; a token without `projects:read` would otherwise repeat the failing call each time.
11. **Loading:** the first load shows a placeholder; later loads keep the table and spin the refresh button, as Grafana panels do.
12. **Time zone:** the user's Grafana preference; a change in the picker lasts for the visit and is not in the URL.
13. **A second provisioned data source, `Bugsink project-bound`** (uid `bugsink-project`, token `c`×40), so the demo has an instance to pick and the project-bound case is exercised.

## URL contract

The page reads these query parameters and, on its first render and on every change, replaces the URL entry with the full state (`replace`, so Back leaves the page).

| Parameter | Value | Absent or invalid |
| :--- | :--- | :--- |
| `ds` | data source uid | the default instance; an unknown uid shows the not-found message |
| `project` | positive integer | all projects (a value that is not a positive integer is ignored) |
| `state`, `sort`, `order` | as the query model | the query default; an unknown value is sent and the backend's message shows |
| `q` | text | no search |
| `from`, `to` | Grafana time (`now-7d`, `now`, or epoch milliseconds) | `now-7d` / `now` when either does not parse |

## Page contract

| Condition | What the page shows |
| :--- | :--- |
| No Bugsink data source | `EmptyState` `call-to-action`, message `No Bugsink data source yet.`, link `Add a Bugsink data source` to `/connections/datasources/new` |
| `ds` names no Bugsink instance | `Alert` error, title `The data source "<uid>" was not found.`; the picker is empty, no query runs |
| First load | `LoadingPlaceholder` text `Loading issues...` |
| Query failed | `Alert` error whose title is the backend's message; no table |
| Frame has a notice | `Alert` warning whose title is the notice text, above the table |
| No rows | `EmptyState` `not-found`, message `No issues match these filters in this time range.` |

Table columns, in order: `Issue` (`friendly_id`, a `TextLink` to the row URL, new tab), `Type`, `Message`, `Transaction`, `Project`, `Events` (`digested_event_count`), `First seen`, `Last seen` (`dateTimeFormat` in the page's time zone, `YYYY-MM-DD HH:mm:ss`), `Status` (`Open`, `Resolved`, `Muted`).

## Review Focus

1. Two filter changes in quick succession whose answers arrive out of order: the table shows the answer to the latest state, never the older one (Task 4 test).
2. A stale or hand-edited link (`ds` of a deleted instance, `project=abc`, `from=garbage`): the page explains the missing instance or falls back to defaults; it never renders blank or throws (Task 4 tests).
3. Switching instance while a project is chosen: the project and an arrival-order sort are dropped, and the editor forgets the previous instance's projects and error (Task 4 and Task 2 tests).
4. A project read cancelled because the user left the page is not remembered as a failure for the next caller (Task 1 test).
5. Bugsink refuses the query (project not the token's, `issues:read` missing): the page shows the backend's sentence, not a generic error and not the empty state (Task 4 test).

## File structure

| File | Responsibility |
| :--- | :--- |
| `pkg/plugin/query.go`, `pkg/plugin/datasource.go` | project reads remembered with their error |
| `src/datasource/types.ts` | `IssueRow`, `IssueList`, `IssueStatus` |
| `src/datasource/issueRows.ts` (new) | `issues` frame to rows |
| `src/datasource/datasource.ts` | `listIssues` |
| `src/datasource/components/QueryEditor.tsx` | forgets projects and error on a data source change |
| `src/pages/issues/IssueTable.tsx` (new) | the table, its cells and the empty state |
| `src/pages/issues/listState.ts` (new) | URL parameters to page state and back |
| `src/pages/issues/timeRange.ts` (new) | time range parsing, shift and zoom |
| `src/pages/IssuesPage.tsx` (new), `src/module.tsx` | the root page; replaces `src/pages/Home.tsx` |
| `docker/grafana/provisioning/datasources/datasources.yaml` | second Bugsink instance |
| `tests/issue-list.spec.ts` (new), `tests/fixtures.ts` | the demo end to end; replaces `tests/app-home.spec.ts` |

---

### Task 1: Remember failed project reads

**Files:**
- Modify: `pkg/plugin/query.go` (`projects`, `projectNames`), `pkg/plugin/datasource.go` (`projectCache` type), `pkg/plugin/query_test.go`, `pkg/plugin/resources_test.go`

**Interfaces:**
- Consumes: `cache[V]`, `isTimeout(err)` (`messages.go`), `(*bugsink.Client).ListProjects`.
- Produces: unchanged `func (d *Datasource) projects(ctx context.Context) ([]bugsink.Project, error)`; `projectCache *cache[projectsResult]` with `type projectsResult struct { projects []bugsink.Project; err error }`.

- [ ] **Step 1: Write failing tests** with the fake Bugsink of `query_test.go`:
  - `TestProjectsFailureIsRemembered`: `projects/` answers 403 `This token does not have the required capability: projects:read.`; two issue queries and one `GET projects` resource call within the TTL make exactly one `projects/` request; both queries have rows with the project id as text; the resource call answers 502 `The API token lacks projects:read.`.
  - `TestProjectsFailureExpires`: same server, the project cache's clock moved past `cacheTTL`: the next `d.projects` call requests `projects/` again.
  - `TestProjectsCancelledReadIsNotRemembered`: `d.projects` with a cancelled context fails; the following call with a live context requests `projects/` and returns both projects.
- [ ] **Step 2: Run** `go test -race ./pkg/plugin/...`. Expected: FAIL (two `projects/` requests in the first test).
- [ ] **Step 3: Implement.** Store the result with its error unless `isTimeout(err)`; a cached error is returned as it was.
- [ ] **Step 4: Run** `go test -race ./pkg/...` and `go vet ./pkg/...`. Expected: PASS, M2 tests unchanged.
- [ ] **Step 5: Commit** `Remember a failed project read for the cache lifetime` and push.

### Task 2: Issue rows from the issues query; the editor follows its data source

**Files:**
- Create: `src/datasource/issueRows.ts`, `src/datasource/issueRows.test.ts`
- Modify: `src/datasource/types.ts`, `src/datasource/datasource.ts`, `src/datasource/datasource.test.ts`, `src/datasource/components/QueryEditor.tsx`, `src/datasource/components/QueryEditor.test.tsx`

**Interfaces:**
- Consumes: the M2 frame contract; `DataSourceWithBackend.query`.
- Produces:

```ts
export type IssueStatus = 'open' | 'resolved' | 'muted';
export interface IssueRow {
  id: string;
  friendlyId: string;
  type: string;
  message: string;
  transaction: string;
  project: string;      // name, or the id as text when projects are unreadable
  projectId: number;
  status: IssueStatus;
  events: number;       // digested_event_count
  firstSeen: number;    // epoch milliseconds
  lastSeen: number;
  url?: string;         // the issue's page in Bugsink
}
export interface IssueList { rows: IssueRow[]; notice?: string }

// issueRows.ts
export function issueRows(frame: DataFrame): IssueRow[]

// in BugsinkDataSource
listIssues(query: BugsinkQuery, range: TimeRange): Promise<IssueList>
```

- `listIssues` runs `this.query` with one target `{ ...DEFAULT_QUERY, ...query, refId: 'issues', datasource: this.getRef() }`, `range`, `rangeRaw: range.raw`, `app: 'ahmedismail-bugsink-app'`, a fresh `requestId`, `interval: '1m'`, `intervalMs: 60000`, `scopedVars: {}`, `timezone: 'browser'`, `startTime: Date.now()`; it resolves the first frame's rows and `meta.notices[0].text`, and rejects with `new Error(message)` where message is `error.message`, else `error.data.message`, else `The issues could not be loaded.`.
- `issueRows` reads fields by name (`friendly_id`, `calculated_type`, `calculated_value`, `transaction`, `project`, `project_id`, `status`, `digested_event_count`, `first_seen`, `last_seen`, `id`); `url` is `friendly_id`'s first link URL with `${__data.fields.id}` replaced by `encodeURIComponent(id)`, absent when the field has no link.
- `QueryEditor`: when `datasource` changes, `projects` becomes `[]` and `loadError` `undefined` before the new list loads.

- [ ] **Step 1: Write failing tests.**
  - `issueRows.test.ts`: a frame built with `createDataFrame` from the M2 contract (one open, one muted, one resolved issue; link URL `http://localhost:8800/issues/issue/${__data.fields.id}/event/last/`) maps to `IssueRow`s with every field; id `a/b` gives `…/issues/issue/a%2Fb/event/last/`; a frame without links gives no `url`; a frame with all fields and no values gives `[]`.
  - `datasource.test.ts` (spy on `query` with `of(...)`): `listIssues({refId: 'x', project: 1}, range)` sends one target equal to `{queryType: 'issues', state: 'unresolved', sort: 'last_seen', order: 'desc', project: 1, refId: 'issues', datasource: ds.getRef()}` and the given `range`; resolves rows and `notice: 'Only the first 5000 issues were read. Narrow the search or the time range.'` from a frame notice; a response `{data: [], error: {message: 'Bugsink refused the request: No Project matches the given query.'}}` rejects with that message; `{data: [], error: {status: 502}}` rejects with `The issues could not be loaded.`.
  - `QueryEditor.test.tsx` `forgets the previous data source's projects`: render with a data source whose `getProjects` rejects `{data: {message: 'Bugsink rejected the API token.'}}`, rerender with one resolving `[{id: 3, name: 'other'}]`: the error text is gone, the Project picker offers `All projects` and `other` only.
- [ ] **Step 2: Run** `npm run test:ci`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `npm run test:ci`, `npm run typecheck`, `npm run lint`. Expected: PASS, Jest exits by itself.
- [ ] **Step 5: Commit** `Read issue rows from the issues query and reset the editor on a data source change` and push.

### Task 3: The issue table

**Files:**
- Create: `src/pages/issues/IssueTable.tsx`, `src/pages/issues/IssueTable.test.tsx`

**Interfaces:**
- Consumes: `IssueRow` (Task 2).
- Produces: `export const PAGE_SIZE = 25;` and `export function IssueTable({ rows, timeZone }: { rows: IssueRow[]; timeZone: TimeZone }): React.JSX.Element`, rendering the columns of the page contract with `InteractiveTable` (`getRowId` = `id`, `pageSize` = `PAGE_SIZE`, no `sortType`), or the no-rows `EmptyState` when `rows` is empty.

- [ ] **Step 1: Write failing tests** in `IssueTable.test.tsx`:
  - the column headers are exactly `Issue`, `Type`, `Message`, `Transaction`, `Project`, `Events`, `First seen`, `Last seen`, `Status`, in that order, and none is a sort button;
  - one row renders its cells: link `SAMPLE-PYTHON-2` with `href` equal to the row's `url` and `target="_blank"`, `LookupError`, the message, transaction, `sample-python`, `22`, and with `timeZone: 'utc'` the times `2026-11-13 23:06:27` and `2026-11-15 05:45:33`;
  - statuses render as `Open`, `Muted`, `Resolved`;
  - a row without `url` shows its friendly id as text, not a link; empty message and transaction cells render empty;
  - 30 rows render 25 data rows and a pagination control; the next page shows the other 5;
  - no rows shows `No issues match these filters in this time range.`
- [ ] **Step 2: Run** `npm run test:ci`. Expected: FAIL.
- [ ] **Step 3: Implement.** Read the `InteractiveTable` story on developers.grafana.com/ui first.
- [ ] **Step 4: Run** `npm run test:ci`, `npm run typecheck`, `npm run lint`. Expected: PASS.
- [ ] **Step 5: Commit** `Add the issue table` and push.

### Task 4: The issue list page

**Files:**
- Create: `src/pages/IssuesPage.tsx`, `src/pages/IssuesPage.test.tsx`, `src/pages/issues/listState.ts`, `src/pages/issues/listState.test.ts`, `src/pages/issues/timeRange.ts`, `src/pages/issues/timeRange.test.ts`
- Modify: `src/module.tsx` (`setRootPage(IssuesPage)`)
- Delete: `src/pages/Home.tsx`, `src/pages/Home.test.tsx`, `tests/app-home.spec.ts` (Task 5 brings its checks back)

**Interfaces:**
- Consumes: `listIssues`, `getProjects`, `QueryEditor`, `IssueTable`, `DEFAULT_QUERY`.
- Produces:

```ts
// listState.ts
export interface ListState { ds?: string; query: BugsinkQuery; from: string; to: string }
export const DEFAULT_FROM = 'now-7d';
export const DEFAULT_TO = 'now';
export function readListState(params: URLSearchParams): ListState  // query has refId 'issues' and DEFAULT_QUERY's values where absent
export function writeListState(state: ListState): URLSearchParams  // ds, project, state, q, sort, order, from, to; project and q only when set

// timeRange.ts
export function toTimeRange(from: string, to: string): TimeRange        // epoch ms digits or dateMath; defaults when either fails
export function shiftTimeRange(range: TimeRange, direction: -1 | 1): TimeRange  // by half the span, absolute
export function zoomOutTimeRange(range: TimeRange): TimeRange          // doubles the span about its centre, absolute

// IssuesPage.tsx
export function IssuesPage(): React.JSX.Element
```

- Layout: `PluginPage` (no `pageNav`) with `actions` = `TimeRangePicker` + `RefreshPicker` (`noIntervalPicker`, `isLoading` while a query runs); body = `Combobox` `aria-label="Data source"` (instance names, value uid), then `QueryEditor` (`onChange` writes the state, `onRunQuery` does nothing), then the page contract's alerts, then `IssueTable`.
- State lives in the URL through `useSearchParams` (`setSearchParams(writeListState(next), { replace: true })`). A query runs whenever the instance, query, `from`, `to` or a refresh changes; an answer to an earlier state is dropped. The range written back uses `toURLRange`.

- [ ] **Step 1: Write failing tests.**
  - `listState.test.ts`: empty params give `ds` undefined, `DEFAULT_QUERY`'s values and `now-7d`/`now`; `project=2&state=open&q=a%20b&sort=digested_event_count&order=asc&from=now-24h&to=now` round-trips through `writeListState`; `project=abc`, `project=0` and `project=-1` give no project; `state=bogus` is kept.
  - `timeRange.test.ts`: `toTimeRange('now-7d', 'now')` spans 7 days with `raw` kept; `'1763164800000'` parses as that instant; `toTimeRange('garbage', 'now')` gives the default range; shifting a 2-hour range back moves both ends by 1 hour; zooming out a 2-hour range gives 4 hours with the same centre.
  - `IssuesPage.test.tsx` (`MemoryRouter` with `initialEntries`, a route that prints `location.search`; `@grafana/runtime` mocked: `getDataSourceSrv` with `getList` returning instances `Bugsink` (uid `bugsink`) and `Bugsink EU` (uid `eu`) and `get(uid)` returning stub data sources with `getProjects` and `listIssues` mocks, `PluginPage` rendering `actions` and children, `config.bootData.user.timezone = 'utc'`):
    - `getList` is called with `{ pluginId: 'ahmedismail-bugsink-datasource' }`; with no instances the no-data-source empty state and link show.
    - no params: `bugsink` is chosen and the URL becomes `ds=bugsink&state=unresolved&sort=last_seen&order=desc&from=now-7d&to=now`; with `Bugsink EU` marked `isDefault`, `eu` is chosen.
    - `?ds=eu&project=2&state=open&sort=digested_event_count&order=desc&from=now-24h&to=now` calls `eu`'s `listIssues` with those values and a range whose `raw` is `now-24h`/`now`; the pickers show `Open` and `Event count`.
    - choosing State `Muted` writes `state=muted` and calls `listIssues` again; Refresh calls it again with the same arguments.
    - from `?ds=bugsink&project=2&sort=digest_order`, choosing `Bugsink EU` writes `ds=eu`, no `project`, `sort=last_seen`, and loads `eu`'s projects.
    - `?ds=nosuch` shows `The data source "nosuch" was not found.` and calls no `listIssues`.
    - `?project=abc&from=garbage` queries without a project over `now-7d`/`now`.
    - a pending `listIssues` shows `Loading issues...`; a rejection with `Bugsink refused the request: No Project matches the given query.` shows that alert and neither table nor empty-state message; a result with a notice shows the notice above the rows.
    - out of order: the first `listIssues` (state `unresolved`) resolves after the second (state `open`); the table shows the second's rows only.
- [ ] **Step 2: Run** `npm run test:ci`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `npm run test:ci`, `npm run typecheck`, `npm run lint`. Expected: PASS.
- [ ] **Step 5: Check in the stack:** `npm run build`, `docker compose up -d --wait` from the worktree, `docker compose restart grafana`; `GET http://localhost:3300/a/ahmedismail-bugsink-app` answers 200 and `GET http://localhost:3300/public/plugins/ahmedismail-bugsink-app/module.js` contains `Loading issues...`.
- [ ] **Step 6: Commit** `Add the issue list page` and push.

### Task 5: Second instance and end-to-end tests of the demo

**Files:**
- Create: `tests/issue-list.spec.ts`
- Modify: `docker/grafana/provisioning/datasources/datasources.yaml`, `tests/fixtures.ts`, `docker/README.md`

**Interfaces:**
- Consumes: everything above, built into `dist/`; `BUGSINK_BROWSER_URL`, `EVENT_MARKER` from `tests/seeded.ts`.
- Produces: provisioned data source `Bugsink project-bound`, uid `bugsink-project`, `url: http://bugsink:8000`, `jsonData.browserUrl: http://localhost:8800`, `secureJsonData.apiToken` `c`×40 with the public-token comment; `gotoPage: (path?: string, queryParams?: URLSearchParams) => Promise<AppPage>`.

- [ ] **Step 1: Check the unverified facts first:** with the stack up from the worktree and the Task 4 build, open `http://localhost:3300/a/ahmedismail-bugsink-app` in a Playwright test or browser: the URL gains `ds=bugsink&…` and keeps it after a reload, and `data-testid Bugsink breadcrumb` is present once. If either fails, stop and report.
- [ ] **Step 2: Provision** the second data source; `docker compose restart grafana`; `GET http://localhost:3300/api/datasources/uid/bugsink-project/health` reports success naming `sample-python`.
- [ ] **Step 3: Write the tests** in `tests/issue-list.spec.ts` (read `.config/AGENTS/e2e-testing.md` first; pick options by exact name, since the config tests create data sources with similar names):
  - `the page shows its title once`: heading `Bugsink` count 1; `getByGrafanaSelector(selectors.components.Breadcrumbs.breadcrumb('Bugsink'))` count 1.
  - `the demo: open issues of one project by event count`: `gotoPage('')`; choose `Data source` `Bugsink`, `State` `Open`, `Project` `sample-python`, `Sort by` `Event count`; the URL contains `ds=bugsink`, `project=1`, `state=open`, `sort=digested_event_count`; every `Project` cell reads `sample-python` and every `Status` cell `Open`; the `Events` cells of the first page do not increase; the first row's `Issue` link `href` matches `^http://localhost:8800/issues/issue/[0-9a-f-]{36}/event/last/$` with `target="_blank"`.
  - `the state survives a reload`: `gotoPage('', new URLSearchParams('ds=bugsink&project=1&state=open&sort=digested_event_count&order=desc&from=now-24h&to=now'))`, `page.reload()`; the pickers show `sample-python`, `Open`, `Event count`; with `q=${EVENT_MARKER}` added, a row's message contains `m2-e2e` (retry with `toPass()` for the digest).
  - `a project-bound instance offers its one project`: choose `Bugsink project-bound`; the `Project` options are exactly `All projects`, `sample-python`.
  - `no matching issues`: params `ds=bugsink&q=environment:nosuchenv`: `No issues match these filters in this time range.`
  - `a refused query explains itself`: params `ds=bugsink&project=99`: `Bugsink refused the request: No Project matches the given query.`
  - `an unknown data source`: params `ds=nosuch`: `The data source "nosuch" was not found.`
- [ ] **Step 4: Run** `npm run build && npm run build:backend && docker compose restart grafana && npm run e2e`. Expected: every test passes, twice in a row.
- [ ] **Step 5: Run** `npm run lint`, `npm run typecheck`, `scripts/check_spdx.sh`, `python3 scripts/verify_stack.py`. Expected: PASS.
- [ ] **Step 6:** In `docker/README.md`, list `Bugsink project-bound` under the Grafana section and say the app page opens at `http://localhost:3300/a/ahmedismail-bugsink-app`.
- [ ] **Step 7: Commit** `Provision a project-bound instance and test the issue list end to end` and push.

### Task 6: Documentation and changelog

**Files:**
- Modify: `src/README.md`, `CHANGELOG.md`

- [ ] **Step 1: Write** in `src/README.md` a section `## Browsing issues in the app` before `## Querying issues`: the data source picker; that the filters are the query editor's controls; the time range picker and its default of the last 7 days; the columns and the link to Bugsink; that the address bar holds the whole state, so a link reopens the same list; and the limits of `### Limits` (5000 issues, results up to 15 seconds old, `Arrival order` needs a project). In `CHANGELOG.md` under 0.1.0: `Issue list page in the app: pick a Bugsink instance, filter by project, state, search and time range, sort by last seen or event count, link to each issue in Bugsink; the page state is kept in the URL.`
- [ ] **Step 2: Run** `scripts/check_spdx.sh`, `scripts/check_licenses.sh`, `npm run lint`. Expected: PASS.
- [ ] **Step 3: Commit** `Document the issue list page` and push.

## Not in this milestone

The Actions column (M7), the sparkline (M9), an in-app issue detail page and routing (M4), sortable column headers, the page number in the URL, a project dashboard variable, merging concurrent identical cache misses, and the resource mux rebuilt per call (M4).

## After the last task

* Whole-branch review against this plan and spec §4.D, then one fix wave.
* From the worktree: `npm ci`, `npm run build`, `npm run build:backend`, `go test -race ./pkg/...`, `npm run test:ci`, `npm run e2e`, `python3 scripts/verify_stack.py`.
* Update spec §3 (plain React and `@grafana/ui`, no Scenes) and §4.D (the `Issue` column, the five states as the status filter, the URL state) and the roadmap's M3 entry to what was built, and add an `## As built` section here.
* The merge into `main` happens only when the owner asks for it.

## As built

1. All six tasks ran as planned on `m3/issue-list`; the code tasks each had a review, the documentation task was reviewed in the whole-branch review, then one fix wave.
2. The provisioned `Bugsink` data source is Grafana's default (`isDefault: true`). Without it the page's default instance was whichever Bugsink data source sorted first by name, which other end-to-end tests create in parallel.
3. Parameters the page does not own (`orgId`, a bare `?kiosk`) are kept in the URL exactly as written; the page compares raw strings, so it cannot rewrite in a loop.
4. `Search` no longer runs the query on blur, in the editor everywhere: blur applies the text, Enter applies and runs. In Explore a press on the Run button blurred the field, started the query and then cancelled it (reproduced with a real mouse, three of three times).
5. A later load keeps the old rows and shows a loading bar above the table; the first load and a data source change show the placeholder.
6. An unknown `ds` and a data source that fails to load show an error instead of loading forever; an invalid `from` or `to` is replaced in the URL by the default range.
7. `listIssues` reads the deprecated `response.error` on purpose: a failed fetch sets only that field.
8. The tests' `AppPage` is built in `tests/fixtures.ts`, because plugin-e2e's `gotoAppPage` drops query parameters. The Explore test waits for the query that Enter starts before it presses Run.
9. Left for later milestones: typing each column in `issueRows.ts` (M4); memoising the table's columns and cancelling superseded requests (M9).
