# M9: Event Volume Series Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A user charts a project's or an issue's event volume in a time-series panel or Explore, and the App's issue list shows a sparkline of the last 24 hours per row while staying responsive.

**Architecture:** `pkg/bugsink` gains the two `event-counts` reads. A new `volume` query type turns the dashboard range into one Bugsink call on whole hours inside the 90-day limit, sums the hourly counts into steps of whole hours and answers one time-series frame, through a 60-second cache of raw counts. The issue list's sparkline cell runs that same query for its row's issue, through a limit of 4 queries in flight per data source, and cancels it when the row leaves the screen.

**Tech Stack:** Go 1.26 with `grafana-plugin-sdk-go` v0.299.0, React with `@grafana/ui` and `@grafana/runtime` 13.2.4 (`Sparkline`, `InteractiveTable`, `DataSourceWithBackend`), Jest, `@grafana/plugin-e2e` 4.0.0, the Compose stack (Bugsink 2.6.3, Grafana 13.3.1).

**Spec:** [`../bugsink_grafana_plugin_spec.md`](../bugsink_grafana_plugin_spec.md) (§6, §4.D). **Roadmap:** [`../implementation_roadmap.md`](../implementation_roadmap.md) (M9, Definition of done). **Findings:** [`../phase0_findings.md`](../phase0_findings.md) (Q5, Q9).

## Global Constraints

* Everything in the Global Constraints of the M2 plan holds: the token never leaves the backend; cursor paging only (this milestone reads no paged list); resources never answer 401; generated code untouched; nothing in `.config/`; plugin ids, types and `plugin.json` unchanged (no Grafana restart needed for it); SPDX headers; one-line comments; behaviour tests; Go commands on `./pkg/...`; ports 3300/8800/8810/8820; push after every commit, never amend; every test command in the foreground.
* Branch `m9/event-volume` in the worktree `.claude/worktrees/m9-event-volume`.
* Reuse `userMessage`, `failure`, `parseModel`, `validIssueID`, `cache`, `d.projectNames`, the query model, `ProjectPicker`, `CommitInput` and the e2e helpers in `tests/fixtures.ts`, `tests/reported.ts`, `tests/seeded.ts`; no parallel versions.
* A query that names nothing to read answers an empty frame with an info notice; wrong input answers 400 with a sentence.
* `QueryData` stays read-only. No task changes who may write or a write route (no "Authorization: yes" task). The M7/M8 row actions, busy rows, scope ref and messages of the issue list keep their behaviour and tests.
* Untrusted text (project names, Bugsink sentences) shows as plain text. Resource and query failures in the page raise no Grafana toast.
* The e2e run sends no new Bugsink event: it reuses the event of `tests/report-trace.setup.ts` (project `sample-python`, marker `m2-e2e`), so the budget stays at 5.
* A provisioned dashboard must not open in an error state.

## Facts already verified (2026-11-15, Bugsink 2.6.3 and Grafana 13.3.1 in the Compose stack, now 13:48Z)

* `GET projects/1/event-counts/` (token `a`×40, no parameters): 200 `{"start":"2026-11-14T14:00:00Z","end":"2026-11-15T14:00:00Z","interval_seconds":3600,"buckets":[{"timestamp":"2026-11-14T14:00:00Z","count":0},…]}`, 24 buckets ascending, zero-filled, the last the current hour (`13:00Z`, incomplete). `issues/{id}/event-counts/` has the same shape and accepts the uuid or the short id in any case (`sample-python-275` works).
* Timestamps in answers are always UTC with `Z`. A request time with an offset is converted (`12:30:00+05:30` read as `07:00Z`); an unescaped `+` is a space and fails; a bare `2026-11-15T12:00:00` is read as UTC. The generated client encodes params with `url.Values.Encode`.
* Range rules (all 400, body keyed by the parameter): `start` off the hour `{"start":["Must be on a UTC hour boundary."]}`; `end` off the hour `{"end":["Must be on a UTC hour boundary."]}`; `end <= start` `{"end":["Must be after start."]}`; `end` after the end of the current hour `{"end":["Cannot be later than the end of the current hour."]}`; more than 2160 hours `{"start":["The range cannot exceed 90 days."]}`; `start` before the start of the current hour minus 2160 hours `{"start":["Hourly counts are retained for 90 days."]}` (`08-17T13:00Z` accepted, `08-17T12:00Z` refused); `start=garbage` `{"start":["Datetime has wrong format. Use one of these formats instead: YYYY-MM-DDThh:mm[:ss[.uuuuuu]][+HH:MM|-HH:MM|Z]."]}`.
* Largest answer: 2160 buckets, 101630 bytes, 17 ms; `start` alone defaults `end` to the end of the current hour. No paging, no `Cache-Control`.
* Refusals: token `c`×40 on `projects/2/…` and on an issue of project 2: 403 `{"detail":"This token is not allowed to access this project."}`; token `d`×40: 403 `This token does not have the required capability: projects:read.` (project) and `…: issues:read.` (issue); no token: 401; `projects/99/…`: 404 `No Project matches the given query.`; unknown issue: 404 `No Issue matches the given query.`; `projects/abc/…`: 404 `Not found.`. The health check already reads `projects:read` and `issues:read`, so no new capability is probed.
* Recent events: the current hour's count equals the events digested so far (issue `SAMPLE-PYTHON-275`: 4 events with `ingested_at` after 13:00Z, bucket `13:00Z` = 4; 24-hour sum 54 = `digested_event_count`). Delay is the digest only.
* Grafana 13.3.1 bundle (`public/build/18631.*.js`): `backendSrv.fetch` returns an Observable whose teardown marks the request done in its queue and unsubscribes it; the queue runs at most 5 requests to `api/ds/query` or `api/datasources/proxy` at once (`http2Enabled ? 1000 : 5`) and never holds other requests (resources); an error alert is skipped when `showErrorAlert` is undefined and the URL is a data query, so a failing query raises no toast.
* `@grafana/runtime` 13.2.4 `DataSourceWithBackend.query` posts through `getBackendSrv().fetch` inside `switchMap`; `getResource`/`postResource` spread their options into `fetch`, whose `BackendSrvRequest` has `abortSignal` and `requestId` (a repeated `requestId` cancels the earlier call).
* `@grafana/ui` 13.2.4 exports `Sparkline` with props `{ width: number; height: number; sparkline: FieldSparkline; config?: FieldConfig<GraphFieldConfig>; showHighlights?: boolean; theme: GrafanaTheme2 }`; `FieldSparkline` is `{ y: Field; x?: Field; timeRange?; … }`; `y.state.range` (`min`, `max`) is read unguarded, and fewer than two values render nothing. It renders in Jest (`data-testid="uplot-main-div"`, checked with a throwaway test).
* `InteractiveTable` renders only the current page's rows (`tableInstance.page`), keys rows by `getRowId`, calls each column's `cell` as a component through `cell.render('Cell')` (a new function per render remounts the cell), and has `autoResetPage` false by default; its docs say `columns` and `data` "Must be memoized".
* SDK v0.299.0: `data.FrameTypeTimeSeriesMulti`, `data.VisTypeGraph`, `data.FieldConfig{DisplayNameFromDS, Interval}`; `backend.DataQuery.Interval` (`time.Duration`) and `MaxDataPoints`. Datasource `plugin.json` already has `"metrics": true`.
* Grafana's RBAC guide (`how-to-guides/app-plugins/implement-rbac-in-app-plugins.md`, fetched today): "Requires Grafana version 11.6.0 or later."; "To enable this protection, activate the `externalServiceAccounts` feature."; "The `externalServiceAccounts` feature only supports single organization setups."; "The backend service account and ID forwarding setup allow your plugin's backend to authenticate requests…"; the code reads `X-Grafana-Id`; "Assigning roles to specific users requires either Grafana Cloud or a Grafana Enterprise license." It names no `idForwarded` toggle and states no defaults.
* Not verified (Task 6 checks first): that an unsubscribed query aborts its HTTP request in the browser (`net::ERR_ABORTED`), and the URL Grafana 13.3.1 uses for the app's queries (`/api/ds/query` or `/apis/query.grafana.app/…`). Not verified and not relied on: that Grafana cancels the plugin's context when the browser aborts.

## Decisions in this plan, for review

1. **One query type, `volume` ("Event volume").** It reuses the `project` and `issue` fields: `issue` wins (project unused), else `project`; with neither, an empty frame and the info notice `An event volume query needs a project or an issue.` One picker entry; the same rule as the events query.
2. **Step:** `step` = Grafana's `Interval` rounded up to whole hours, at least one hour. `MaxDataPoints` is not read: Grafana derives `Interval` from it, and 2160 points is the hard bound.
3. **Alignment and window:** steps start at multiples of `step` since 1970-01-01T00:00Z (so a day step runs midnight to midnight UTC). The window is `alignDown(from)` to `alignUp(to)`, so every point is a whole step; it is then clamped: the end to the end of the current hour (the last point is the incomplete step in progress, as Bugsink's current hour is), the start to `alignUp(oldest)` with `oldest` = end of the current hour minus 2160 hours, so the first point is never a cut step. Series are dense: Bugsink zero-fills, sums of zeros are zero.
4. **90-day cut:** when the start was moved, the warning notice `Bugsink keeps hourly event counts for 90 days, so the series starts later than the time range.`; when nothing of the range is left, the frame is empty with that notice and nothing is requested. A range wholly after the current hour is an empty frame without a notice and no request.
5. **`end` is left out when the window reaches the current hour.** Bugsink then uses the end of its own current hour, so a Grafana clock a little ahead of Bugsink's does not turn the panel into a 400; points are grouped by the timestamps Bugsink returns, so one hour more or less at the end is harmless. Left: a full 90-day query in the seconds after an hour turns, with Bugsink's clock ahead, gets Bugsink's 400 sentence.
6. **Step notice** (info) whenever `step` differs from `Interval`: `Each point counts one hour of events: Bugsink counts events per hour.` (or `… counts <n> hours of events: …`). The provisioned panel sets a minimum interval of `1h`, so it opens without notices.
7. **Frame:** one frame per query in the time-series multi format (contract below): `time` and `count` (int64), the series named by the project's name or the issue as typed.
8. **Cache:** a separate count cache per instance, 60 seconds, at most 128 entries, holding Bugsink's hourly counts keyed by kind, id, window start and window end; two queries with different steps over the same window share one call. Errors are never stored. Triage does not clear it: an action changes no count.
9. **Sparkline:** the last 24 hours, 24 hourly points ending with the current hour, through the `volume` query of the row's issue (no new resource route, no second JSON shape). One query per visible row (at most 25, the page size); rows on other pages are not read.
10. **Concurrency:** at most 4 sparkline queries in flight per data source, queued in the browser by `BugsinkDataSource`; Grafana 13.3.1 runs at most 5 data queries at once without HTTP/2, so one slot stays free for the list's own query. No backend limit: the browser limit and the cache bound a page, and dashboards are bounded by their panels.
11. **Cancellation:** `issueVolume` and `listIssues` take an `AbortSignal`; abort removes a queued call before it is sent and unsubscribes a running one; the promise rejects with an error named `AbortError`. A sparkline is owned by its cell: the cell's effect aborts on unmount (paging, a search that drops the row, leaving the page) and on a data source change. The list load aborts its superseded request in its cleanup (M3 leftover). Sparkline answers write only into their own cell, so they do not use the list's scope ref.
12. **Memoised columns** (M3 leftover): `columns` and `rows` are memoised; values that change per render (busy ids, disabled, `onAction`, `issueHref`, the data source) reach the cells through `IssueTableContext`, so a triage action re-renders but never remounts a row's cells and never restarts its sparkline.
13. **Cell states** (table below): loading, failure (icon with the message in a tooltip, no toast), no events (text), otherwise the line. A failure stays in its cell.
14. **Docs corrections deferred to M9** go in Task 7 with the README: the RBAC sentences quote the guide above; the roadmap's M8 demo says what the tests check.

## Query model (additions)

| Key | Values | Default |
| :--- | :--- | :--- |
| `queryType` | `issues`, `events`, `volume` | `issues` |
| `project` | project id; used by `volume` when `issue` is empty | absent |
| `issue` | issue uuid or short id, `^[A-Za-z0-9_-]{1,100}$`; takes dashboard variables (existing) | absent |

## Frame contract (`volume`)

* One frame, `Name` = series name; `Meta.Type` `timeseries-multi`, `Meta.TypeVersion` `{0, 1}`, `Meta.PreferredVisualization` `graph`; `Meta.Notices` as below.
* Field `time`: `[]time.Time`, UTC step starts, ascending; `Config.Interval` = step in milliseconds.
* Field `count`: `[]int64`; `Labels` `{project: <name>}` or `{issue: <as typed>}`; `Config.DisplayNameFromDS` = series name.
* Series name: the project's name from `d.projectNames`, its id as text when the list cannot be read; for an issue, the `issue` field as typed.
* Empty window or nothing named: the same two fields with no rows.

| Condition | Notice severity | Text |
| :--- | :--- | :--- |
| Neither issue nor project | info | `An event volume query needs a project or an issue.` |
| Start moved by the 90-day limit | warning | `Bugsink keeps hourly event counts for 90 days, so the series starts later than the time range.` |
| `step != Interval` | info | `Each point counts one hour of events: Bugsink counts events per hour.` / `Each point counts <n> hours of events: Bugsink counts events per hour.` |
| `issue` fails the pattern (hostile values `a/b`, `..`, `%2F`, `SAMPLE PYTHON`) | 400 error | `"<issue>" is not an issue id.` (no Bugsink request) |
| Bugsink failure | per the M2 message contract | e.g. 403 `Bugsink refused the request: This token is not allowed to access this project.`, 403 `The API token lacks issues:read.`, 404 `Bugsink refused the request: No Issue matches the given query.` |

**Bounds of one query:** one `event-counts` call (none on a cache hit or an empty window), plus the cached project list for a project series' name; at most 2160 points.

## Sparkline cell contract (column `Last 24 hours`, after `Events`)

| State | Shows | Accessible name |
| :--- | :--- | :--- |
| Loading (queued or in flight) | an empty box of 120×24 px | `Loading the last 24 hours` |
| Failure | `exclamation-triangle` icon, message in a tooltip | `Events per hour unavailable: <message>` |
| All counts zero | text `No events` | `No events in the last 24 hours` |
| Otherwise | `Sparkline` 120×24, x the `time` values, y the counts with `state.range` set | `<total> events in the last 24 hours` (role `img`) |

## Review Focus

1. Grafana's and Bugsink's clocks a little apart at the hour: a window reaching the current hour sends no `end`, and an answer with one extra or one missing hour still groups into correct steps (Task 2 `TestRebucketToleratesEdgeHours`, Task 3 `TestVolumeQueryProject`).
2. A time range longer than 90 days or wholly older: no Bugsink 400, the series starts at the cut with the warning, a wholly older range makes no request (Task 2 `TestVolumeWindow`, Task 3 `TestVolumeQueryOutsideRetention`).
3. Paging, searching or leaving while sparklines load: superseded calls are aborted, queued ones never sent, no answer lands in another row, no React warning (Task 5 `paging aborts the previous page`, `unmount aborts and logs nothing`).
4. One row's failure (deleted issue, token without `issues:read`): only that cell shows the message; no toast; the list and its actions keep working (Task 5 `one failing row stays in its cell`).
5. A triage action while sparklines load: busy rows, messages and statuses behave as in M7/M8, and no sparkline is requested again (Task 5 `triage does not restart sparklines`).

## File structure

| File | Responsibility |
| :--- | :--- |
| `pkg/bugsink/counts.go` (new) | `EventCount`, `ProjectEventCounts`, `IssueEventCounts` |
| `pkg/plugin/volume.go` (new) | window, step, re-bucketing, frame, notices, the `volume` query and its cache read |
| `pkg/plugin/query.go`, `datasource.go` | route `volume`; `countCache`, `now` on the instance |
| `src/datasource/types.ts`, `volume.ts` (new), `limiter.ts` (new), `datasource.ts` | query type, frame to series, the in-flight limit, `issueVolume`, cancellable `listIssues` |
| `src/datasource/components/QueryEditor.tsx`, `VolumeQueryEditor.tsx` (new) | type picker entry and the volume editor |
| `src/pages/issues/IssueTable.tsx`, `SparklineCell.tsx` (new), `src/pages/IssuesPage.tsx` | memoised columns, context, sparkline column, aborting list loads |
| `docker/grafana/provisioning/dashboards/bugsink-volume.json` (new) | the demo dashboard |
| `tests/volume.spec.ts` (new), `tests/issue-list.spec.ts`, `tests/triage.spec.ts` | e2e of the demo; column index; M8 leftovers |
| `src/README.md`, `CHANGELOG.md`, `docker/README.md`, spec §4.E/§10, roadmap M8 | docs and deferred corrections |

---

### Task 1: Event counts in the Bugsink client

**Files:** Create `pkg/bugsink/counts.go`, `pkg/bugsink/counts_test.go`.

**Interfaces:**
- Consumes: `gen.Client.ProjectsEventCountsRetrieve`, `gen.Client.IssuesEventCountsRetrieve`, `gen.EventCounts`; `decodeJSON`, `ErrNotBugsink`.
- Produces:

```go
// EventCount is the number of events Bugsink digested in one UTC hour.
type EventCount struct {
	Hour  time.Time
	Count int
}

// ProjectEventCounts reads a project's hourly counts from start up to end; a zero end leaves end to Bugsink, the end of its current hour.
func (c *Client) ProjectEventCounts(ctx context.Context, projectID int, start, end time.Time) ([]EventCount, error)

// IssueEventCounts is ProjectEventCounts for one issue, by uuid or short id.
func (c *Client) IssueEventCounts(ctx context.Context, issueID string, start, end time.Time) ([]EventCount, error)
```

- [ ] **Step 1: Write failing tests** against an `httptest.Server`, in the style of `events_test.go`:
  - `TestProjectEventCounts`: start `2026-11-08T13:00Z`, end `2026-11-15T14:00Z` request `/api/canonical/0/projects/1/event-counts/` with parsed query exactly `start=2026-11-08T13:00:00Z`, `end=2026-11-15T14:00:00Z`; a zero end sends no `end`; the verified answer maps to `[]EventCount` in order with `Hour` in UTC.
  - `TestIssueEventCounts`: `SAMPLE-PYTHON-2` requests `/api/canonical/0/issues/SAMPLE-PYTHON-2/event-counts/`.
  - `TestEventCountsSendUTC`: a start given in `+02:00` is sent with `Z`.
  - `TestEventCountsNotHourly`: `interval_seconds: 86400` is an error wrapping `ErrNotBugsink`.
  - `TestEventCountsRefusals`: 400 `{"start":["The range cannot exceed 90 days."]}` gives `APIError{Status: 400, Detail: "start: The range cannot exceed 90 days."}`; 403 naming `issues:read` gives `Capability: "issues:read"`.
- [ ] **Step 2: Run** `go test ./pkg/bugsink/...`. Expected: FAIL, undefined `ProjectEventCounts`.
- [ ] **Step 3: Implement** with `start.UTC()` and `end.UTC()`.
- [ ] **Step 4: Run** `go test -race ./pkg/...`, `go vet ./pkg/...`. Expected: PASS.
- [ ] **Step 5: Commit** `Add event counts to the Bugsink client` and push.

### Task 2: The volume window, steps and frame

**Files:** Create `pkg/plugin/volume.go`, `pkg/plugin/volume_test.go`.

**Interfaces:**
- Consumes: `bugsink.EventCount`.
- Produces (pure functions; Task 3 wires them):

```go
const maxVolumeHours = 2160

// volumeWindow is the hours one volume query reads from Bugsink.
type volumeWindow struct {
	Start, End time.Time     // UTC hours, Start inclusive, End exclusive
	Step       time.Duration // whole hours, at least one
	ToNow      bool          // End is the end of the current hour, so no end parameter is sent
	Cut        bool          // the 90-day limit moved Start after the range start
}
func (w volumeWindow) empty() bool
func windowFor(from, to, now time.Time, interval time.Duration) volumeWindow
func rebucket(counts []bugsink.EventCount, step time.Duration) (starts []time.Time, totals []int64)
func stepNotice(step, interval time.Duration) (data.Notice, bool)
func volumeFrame(name string, labels data.Labels, step time.Duration, starts []time.Time, totals []int64, notices []data.Notice) *data.Frame
```

- [ ] **Step 1: Write failing tests**, `now` = `2026-11-15T13:48Z` (current hour ends `14:00Z`, oldest `2026-08-17T14:00Z`):
  - `TestVolumeWindow` (table): `now-7d..now`, interval 10m: `08T13:00..15T14:00`, step 1h, `ToNow`, not `Cut`; interval 90m: step 2h, start `08T12:00`; interval 24h: start `08T00:00`, end `15T14:00` (`ToNow`); `now-180d`, interval 3h: start `2026-08-17T15:00Z`, `Cut`; `10T10:30..10T12:30`, interval 1m: `10:00..13:00`, not `ToNow`; `now-1h..now+1h`: end `14:00Z`, `ToNow`; a range wholly before `oldest`: `empty()`, `Cut`; a range wholly after `14:00Z`: `empty()`, not `Cut`; interval 5h: start a multiple of 18000 s since the epoch; every window spans at most `maxVolumeHours`.
  - `TestRebucket`: hours `00..05Z` counts `1..6` with step 3h give starts `00:00, 03:00` and totals `6, 15`; step 1h keeps them; no counts give empty slices.
  - `TestRebucketToleratesEdgeHours`: step 2h over `00..04Z` with an extra hour `04:00` (Bugsink ahead) gives starts `00, 02, 04` with the extra hour alone in `04`; a missing last hour gives the shorter series; neither panics.
  - `TestStepNotice`: (1h, 10m) gives info `Each point counts one hour of events: Bugsink counts events per hour.`; (3h, 150m) gives `… counts 3 hours of events: …`; (3h, 3h) gives none.
  - `TestVolumeFrame`: name, field names `time` and `count`, types `[]time.Time` and `[]int64`, `Config.Interval` 3600000 for 1h, labels and `DisplayNameFromDS` on `count`, `Meta.Type` `timeseries-multi`, `TypeVersion` `{0, 1}`, `PreferredVisualization` `graph`, notices kept; no starts give both fields with length 0.
- [ ] **Step 2: Run** `go test ./pkg/plugin/...`. Expected: FAIL.
- [ ] **Step 3: Implement.** Alignment on Unix seconds (`t.Unix() - t.Unix()%stepSeconds`); `rebucket` groups by the aligned hour of each count, ascending.
- [ ] **Step 4: Run** `go test -race ./pkg/...`, `go vet ./pkg/...`. Expected: PASS.
- [ ] **Step 5: Commit** `Add the event volume window and frame` and push.

### Task 3: The volume query

**Files:** Modify `pkg/plugin/volume.go`, `pkg/plugin/query.go`, `pkg/plugin/datasource.go`; Test `pkg/plugin/volume_query_test.go` (new).

**Interfaces:**
- Consumes: Task 1, Task 2; `parseModel`, `validIssueID`, `userMessage`, `failure`, `d.projectNames`, `newCache`.
- Produces:

```go
const (
	queryTypeVolume      = "volume"
	countCacheTTL        = time.Minute
	countCacheMaxEntries = 128
)
type volumeQuery struct {
	ProjectID int
	Issue     string // wins over ProjectID
}
func parseVolumeQuery(m queryModel, project int) (q volumeQuery, invalid string)
func (d *Datasource) queryVolume(ctx context.Context, q backend.DataQuery, m queryModel, project int) backend.DataResponse
// eventCounts reads the window's hourly counts through the count cache; a failed read is not stored.
func (d *Datasource) eventCounts(ctx context.Context, q volumeQuery, w volumeWindow) ([]bugsink.EventCount, error)
// Datasource gains: countCache *cache[[]bugsink.EventCount]; now func() time.Time (time.Now in NewDatasource)
```

- `parseModel` accepts `volume`; `query` sends it to `queryVolume`. The cache key is `kind|id|start|end` with the computed end also when it is not sent.

- [ ] **Step 1: Write failing tests**, with `NewDatasource` against an `httptest.Server` serving `projects/`, `projects/1/event-counts/` and `issues/…/event-counts/`, `d.now` pinned to `2026-11-15T13:48Z`:
  - `TestVolumeQueryProject`: `{"queryType":"volume","project":1}`, range `now-7d..now`, interval 10m: one request with `start=2026-11-08T13:00:00Z` and no `end`; the frame contract with display name `sample-python`, label `project=sample-python`, values from the answer, and the one-hour step notice.
  - `TestVolumeQueryIssue`: `{"queryType":"volume","issue":"SAMPLE-PYTHON-2","project":1}` requests `issues/SAMPLE-PYTHON-2/event-counts/` only; display name and label `issue` are `SAMPLE-PYTHON-2`; the project list is not read.
  - `TestVolumeQueryPastRange`: `10T10:30..10T12:30` sends `end=2026-11-10T13:00:00Z`.
  - `TestVolumeQuerySteps`: interval 3h: totals summed per 3 hours, `Config.Interval` 10800000, no step notice.
  - `TestVolumeQueryOutsideRetention`: `now-180d`, interval 3h: `start=2026-08-17T15:00:00Z` and the warning notice; a range wholly older than 90 days and one wholly in the future: no request, empty frame, the warning only for the older one.
  - `TestVolumeQueryNamesNothing`: `{"queryType":"volume"}`: no request, empty frame, the info notice.
  - `TestVolumeQueryValidation`: issue `a/b`, `..`, `%2F`, `SAMPLE PYTHON` give 400 `"<issue>" is not an issue id.` and no request; `project: "x"` gives `The project must be a number.`
  - `TestVolumeQueryErrors`: Bugsink 403 `This token is not allowed to access this project.` gives status 403 and `Bugsink refused the request: This token is not allowed to access this project.`; 403 naming `issues:read` gives `The API token lacks issues:read.`; 404 gives `Bugsink refused the request: No Issue matches the given query.`; 400 `{"end":["Cannot be later than the end of the current hour."]}` gives 400 `Bugsink refused the request: end: Cannot be later than the end of the current hour.`; error source downstream; the response never holds the token.
  - `TestVolumeCache`: the same query twice makes one request; intervals 10m and 1h over the same window make one; 61 seconds later (cache clock) makes another; a 502 then a 200 makes two.
  - `TestVolumeProjectNameUnreadable`: `projects/` answers 403: display name `1`.
- [ ] **Step 2: Run** `go test -race ./pkg/plugin/...`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test -race ./pkg/...`, `go vet ./pkg/...`. Expected: PASS, existing query tests unchanged.
- [ ] **Step 5: Commit** `Add the event volume query` and push.

### Task 4: Editor and cancellable data source reads

**Files:** Create `src/datasource/volume.ts`, `volume.test.ts`, `limiter.ts`, `limiter.test.ts`, `components/VolumeQueryEditor.tsx`; Modify `types.ts`, `datasource.ts`, `datasource.test.ts`, `components/QueryEditor.tsx`, `components/QueryEditor.test.tsx`.

**Interfaces:**
- Consumes: the `volume` query and frame contract (Task 3).
- Produces:

```ts
export type QueryType = 'issues' | 'events' | 'volume';
export interface VolumeSeries { times: number[]; counts: number[]; total: number }
export function volumeSeries(frame: DataFrame): VolumeSeries; // volume.ts, reads fields `time` and `count`
export function limiter(max: number): <T>(task: (signal: AbortSignal) => Promise<T>, signal: AbortSignal) => Promise<T>; // limiter.ts
export const SPARKLINE_CONCURRENCY = 4; // datasource.ts
// BugsinkDataSource
issueVolume(id: string, signal: AbortSignal): Promise<VolumeSeries>;
listIssues(query: BugsinkQuery, range: TimeRange, signal?: AbortSignal): Promise<IssueList>;
export function VolumeQueryEditor(props: Props): React.JSX.Element; // Project picker, then Issue (CommitInput, placeholder SAMPLE-PYTHON-2)
```

- `issueVolume` refuses an id failing `isIssueId` without a request, then runs through the instance's limiter one query `{ queryType: 'volume', issue: id, refId: 'volume' }` with `from` = start of the current hour minus 23 hours, `to` = now, `intervalMs: 3600000`, `maxDataPoints: 24`. `runTarget` subscribes to `this.query(…)` instead of `lastValueFrom` and unsubscribes on abort; an abort rejects with an error whose `name` is `AbortError`.
- The type picker gains `Event volume` (value `volume`), which shows `VolumeQueryEditor`.

- [ ] **Step 1: Write failing tests.**
  - `limiter.test.ts`: with max 4 and 6 tasks, 4 start; settling one starts the fifth; aborting a queued task rejects it with `AbortError` and it never starts; aborting a running task aborts its signal and starts the next.
  - `volume.test.ts`: a frame with `time` `[t0, t1]` and `count` `[2, 5]` gives `{ times: [t0, t1], counts: [2, 5], total: 7 }`; a frame without those fields gives empty arrays and total 0.
  - `datasource.test.ts`: `issueVolume('a1', signal)` calls `query` once with the target above, `intervalMs` 3600000, `maxDataPoints` 24, `from` the start of the hour 23 hours before the current one and `to` now (fake timers); a response `error` rejects with its message; `issueVolume('a/b')` rejects `"a/b" is not an issue id.` without a query; six calls start 4 subscriptions; aborting a running call runs the stub Observable's teardown; `listIssues` with an aborted signal runs the teardown and rejects with `AbortError`; the existing `listIssues` and `listEvents` tests pass unchanged.
  - `QueryEditor.test.tsx`: the picker offers `Issues`, `Events`, `Event volume`; choosing `Event volume` calls `onChange` with `queryType: 'volume'` and runs the query; then `Project` and `Issue` show and `State`, `Sort by`, `Search`, `Event ID` do not; typing an issue applies on blur and runs on Enter.
- [ ] **Step 2: Run** `npm run test:ci`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `npm run test:ci`, `npm run typecheck`, `npm run lint`. Expected: PASS, and Jest exits by itself.
- [ ] **Step 5: Commit** `Add the event volume editor and cancellable data source reads` and push.

### Task 5: The sparkline column

**Files:** Create `src/pages/issues/SparklineCell.tsx`, `SparklineCell.test.tsx`; Modify `src/pages/issues/IssueTable.tsx`, `IssueTable.test.tsx`, `src/pages/IssuesPage.tsx`, `src/pages/IssuesPage.test.tsx`.

**Interfaces:**
- Consumes: `issueVolume`, `listIssues(…, signal)`, `VolumeSeries` (Task 4); `Sparkline`, `useTheme2`, `Tooltip`, `Icon` from `@grafana/ui`.
- Produces:

```tsx
export type VolumeSource = Pick<BugsinkDataSource, 'issueVolume'>;
// IssueTable props gain: source: VolumeSource
export const IssueTableContext: React.Context<{
  source: VolumeSource; issueHref: (row: IssueRow) => string;
  busyIds?: ReadonlySet<string>; disabled?: boolean; onAction?: (row: IssueRow, action: TriageAction) => void;
}>;
export function SparklineCell({ id }: { id: string }): React.JSX.Element; // reads source from IssueTableContext
```

- `columns` is `useMemo` over `timeZone` and whether Actions show; the Issue, Actions and `Last 24 hours` cells read the rest from the context. `IssuesPage` memoises `rows` and passes `source`; its load effect makes an `AbortController`, passes its signal to `listIssues` and aborts it in the cleanup beside `scope.active = false`. Nothing else in the page changes.
- `SparklineCell`'s effect depends on `source` and `id`, aborts in its cleanup and ignores answers after it; it renders the cell contract states.

- [ ] **Step 1: Write failing tests.**
  - `SparklineCell.test.tsx`: pending shows `Loading the last 24 hours`; `{ counts: [0, 3, 4], total: 7 }` shows `7 events in the last 24 hours` and `uplot-main-div`; all zero shows `No events` and `No events in the last 24 hours`; a rejection `Bugsink refused the request: No Issue matches the given query.` shows `Events per hour unavailable: Bugsink refused the request: No Issue matches the given query.`; a new `source` aborts the old signal and calls again.
  - `IssueTable.test.tsx`: header `Last 24 hours` after `Events`; 30 rows call `issueVolume` 25 times, once per first-page id; `paging aborts the previous page`: page 2 aborts every pending page-1 signal and calls for the 5 new ids; `one failing row stays in its cell`: one rejection, the other rows show their sparklines and the table and buttons still work; `triage does not restart sparklines`: re-render with new `busyIds`, `disabled` and `onAction` keeps the `issueVolume` call count and disables the busy row's buttons; `unmount aborts and logs nothing`: every pending signal is aborted and `console.error` (spied) is not called when the aborted promises reject.
  - `IssuesPage.test.tsx`: mock sources gain `issueVolume`; a new search aborts the previous `listIssues` signal; unmount aborts the current one; a failing `issueVolume` shows no `Alert` above the table; every existing M7/M8 test passes unchanged.
- [ ] **Step 2: Run** `npm run test:ci`. Expected: FAIL.
- [ ] **Step 3: Implement.** Build `FieldSparkline` from `VolumeSeries`: `x` a time field of `times`, `y` a number field of `counts` with `state: { range: { min, max, delta } }`.
- [ ] **Step 4: Run** `npm run test:ci`, `npm run typecheck`, `npm run lint`. Expected: PASS.
- [ ] **Step 5: Commit** `Add the sparkline column to the issue list` and push.

### Task 6: Volume dashboard and end-to-end tests

**Files:** Create `docker/grafana/provisioning/dashboards/bugsink-volume.json`, `tests/volume.spec.ts`; Modify `tests/issue-list.spec.ts` (`COLUMN.status` 9 to 10), `tests/triage.spec.ts`, `docker/README.md`.

**Interfaces:**
- Consumes: everything above, built into `dist/`; `readReported('trace')` (`eventId` of this run's `m2-e2e` event).
- Produces: dashboard uid `bugsink-volume`, title `Bugsink event volume`, time `now-7d..now`, one `timeseries` panel `Events per hour in sample-python`, data source `bugsink`, `interval: "1h"`, `fieldConfig.defaults.custom.drawStyle: "bars"`, target `{ refId: 'A', queryType: 'volume', project: 1 }`.

- [ ] **Step 1: First contact with the running Grafana** (build, `docker compose restart grafana`, then record each answer in the commit message's body):
  - `POST /api/ds/query` `{"queries":[{"refId":"A","datasource":{"uid":"bugsink"},"queryType":"volume","project":1,"intervalMs":3600000}],"from":"now-7d","to":"now"}`: one frame, fields `time` and `count`, `meta.type` `timeseries-multi`, 169 rows, no notice.
  - The same with `intervalMs` 60000: the one-hour notice; with `from` `now-180d`: the warning and a first time at the end of the current hour minus 2160 hours or later.
  - On `bugsink-project`, `"project":2`: status 403, `Bugsink refused the request: This token is not allowed to access this project.`
  - In the browser: the URL the app's queries post to; paging the issue list while sparkline queries are held shows the aborted ones as failed requests (`net::ERR_ABORTED`). If either differs from the Facts, stop and report.
  - `python3 scripts/verify_stack.py` prints eleven `ok` lines.
- [ ] **Step 2: Write the tests** in `tests/volume.spec.ts` (read `.config/AGENTS/e2e-testing.md` first; retries on digestion use `toPass()`; a local `thisRunsIssue(bugsink)` finds the `m2-e2e` issue of project 1 whose newest event id equals `readReported('trace').eventId`):
  - `the volume dashboard draws this run's events`: open the provisioned dashboard; the panel shows no error icon; the `api/ds/query` answer for refId `A` has 168 or 169 points, and at least one of the last two counts is above 0 (reload in `toPass`).
  - `issue volume in Explore`: Explore, data source `Bugsink`, `Event volume`, Issue = this run's short id, Enter; the answer has a count above 0 in the last two points and no error.
  - `sparklines show for the rows`: the app with `ds=bugsink&project=1&state=all&q=m2-e2e&from=now-24h&to=now`: the row of this run's issue reaches the name `/^[1-9]\d* events in the last 24 hours$/`; then without `q`, every visible row's `Last 24 hours` cell reaches `/events in the last 24 hours$/` and none shows `Events per hour unavailable`.
  - `the list stays usable while sparklines load`: route the query URL and hold every request whose body has `"queryType":"volume"`; with `project=1&state=all&from=now-7d`: rows show, each with `Loading the last 24 hours`, never more than 4 held at once; typing `m2-e2e` in Search and Enter shows the filtered rows while held; releasing the held requests (ignoring ones the page aborted) shows sparklines.
  - In `tests/issue-list.spec.ts`: `COLUMN.status` is 10 and the demo test sees the header `Last 24 hours`.
  - In `tests/triage.spec.ts` (M8 leftovers): the threshold mute's second-newest History item is checked on its title (`strong`) with `toHaveText(/^Muted until 10 or more events in 1 hour/)` and `not.toContainText('from Grafana by')`; after the closing Unmute (threshold mute) and the closing Reopen (resolve in the latest release), `await expect(commentWarning(page)).toHaveCount(0)`.
- [ ] **Step 3: Run** `npm run build && npm run build:backend && docker compose restart grafana && npm run e2e`. Expected: every test passes, twice in a row.
- [ ] **Step 4: Run** `npm run lint`, `npm run typecheck`, `scripts/check_spdx.sh`. Expected: PASS.
- [ ] **Step 5:** Describe the dashboard in `docker/README.md`.
- [ ] **Step 6: Commit** `Provision an event volume dashboard and test event volume end to end` and push.

### Task 7: Documentation and deferred corrections

**Files:** Modify `src/README.md`, `CHANGELOG.md`, `design/bugsink_grafana_plugin_spec.md` (§4.E line 137, §10 line 267), `design/implementation_roadmap.md` (M8 demo, lines 189-190).

- [ ] **Step 1: Write** in `src/README.md` a section "Querying event volume": the `Event volume` type, Project and Issue (Issue wins; it takes dashboard variables), the series (points in whole hours, steps aligned to UTC so a day runs from midnight UTC, the current hour incomplete, at most 90 days with the warning text, the step notice and the advice to set the panel's Min interval to `1h`), the empty answer and its notice, that results can be up to 60 seconds old, and that each query is one Bugsink request needing `projects:read` (project) or `issues:read` (issue). In "Browsing issues in the app": the `Last 24 hours` column, one request per visible row, at most 4 at a time, the four cell states. In `CHANGELOG.md`: the volume query, the sparkline column, cancelled superseded list loads.
- [ ] **Step 2: Correct** the RBAC sentences: in `src/README.md` (Who may triage) replace "without the `externalServiceAccounts` and `idForwarded` feature toggles, which are off by default and support one organization only" by "Grafana's guide to RBAC in app plugins checks an action in the backend with the `externalServiceAccounts` feature, which "only supports single organization setups", a service account from the plugin's `iam` section and the user's ID token forwarded in the `X-Grafana-Id` header"; in spec line 137 replace "(`idForwarded` toggle)" by "(the guide's "ID forwarding setup")" and "Neither toggle is on in the dev stack." by "The dev stack has neither."; in spec line 267 replace "the `externalServiceAccounts` and `idForwarded` toggles (off by default, one organization only)" by "the `externalServiceAccounts` feature (which "only supports single organization setups") and ID forwarding". In the roadmap's M8 demo replace "see in Bugsink and in the History section a comment `... from Grafana by "m7-editor".` for each" by "see Bugsink's newest comment `... from Grafana by "m7-editor".` after each action, and the History section's two newest entries, the comment then the action, after the mute for two days, the threshold mute and the resolve in the latest release".
- [ ] **Step 3: Run** `scripts/check_spdx.sh`, `scripts/check_licenses.sh`, `npm run lint`, `grep -n idForwarded src/README.md design/bugsink_grafana_plugin_spec.md` (no output). Expected: PASS.
- [ ] **Step 4: Commit** `Document event volume` and push.

## Not in this milestone

* M10: `alerting: true`, current-value queries, new issues per interval.
* A series for all projects: Bugsink's API has no installation-wide counts, and summing N projects costs N calls.
* A sparkline over the list's time range (up to 2160 hours per row), a backend-wide limit on concurrent Bugsink calls, and merging identical concurrent cache misses.

## After the last task

* Whole-branch review against this plan, spec §6 and §4.D, then one fix wave.
* From the worktree: `npm ci`, `npm run build`, `npm run build:backend`, `go test -race ./pkg/...`, `npm run test:ci`, `npm run e2e`, `python3 scripts/verify_stack.py`.
* As-built updates (the fix wave, not a task): spec §6 lines 185-187 (the verified range rules: `end` at most the end of the current hour and after `start`, at most 2160 hours, `start` no older than the current hour minus 90 days, offsets accepted; the step, alignment, cut and notices; the 60-second cache), §4.D line 112 (last 24 hours, 4 in flight, the cell states), §3 line 61 if the cache wording changes, and new §10 rows for decisions 1, 3, 5, 9 and 10; the roadmap's M9 entry; an `## As built` section here.
* The merge into `main` happens only when the owner asks for it.

## As built

1. The `Interval` of a query with no interval counts as one hour, and the step is capped at 2^20 hours so a huge interval cannot overflow.
2. An empty frame carries no step notice (decision 6 said "whenever"); it carries the 90-day warning or the naming notice when those apply.
3. Bugsink's answers are bounded: at most 2160 buckets, hourly counts up to 1e9, on the hour and ascending; anything else is refused as not coming from Bugsink.
4. The 90-day bound is one constant, `bugsink.MaxEventCountBuckets`, used by both the client and the window.
5. Ruling: steps are aligned to UTC from 1970-01-01, so a day step runs midnight to midnight UTC and a week step starts on a Thursday; the README says so.
6. Ruling: the sparkline shows the last 24 hours as the 23 full hours plus the current one, which is Bugsink's own default window.
7. Ruling: at most 4 sparkline queries are in flight per data source, the count cache lasts one minute, and Refresh does not reload the charts.
8. Ruling: there is no backend limit on concurrent Bugsink calls yet, and identical concurrent cache misses are not merged (8 parallel cold queries make 1 to 8 calls).
9. A range whose start, rounded down to its step, is older than the oldest hour Bugsink keeps (90 days before the end of the current hour) shows the 90-day warning: every range of 90 days or more, the `Last 90 days` preset included, and some a little shorter.
10. A project series also reads the project list (cached 15 seconds) for its name; an empty window or a cache hit makes no `event-counts` call.
11. The limiter frees the slot of an aborted running task one microtask later, so that when a page's cells abort in one pass their queued neighbours are removed before a slot frees; a task that throws synchronously frees its slot, and an already aborted signal never starts the task.
12. With 30 rows, page 1 sends 25 volume queries and page 2 sends 5 more, 30 in all; the queries the old limiter started before cancelling them never reached the network.
13. Measured load: a page of 25 rows makes 25 `event-counts` calls cold and none within a minute; a page change while 4 run aborts those 4 and sends 25.
14. Grafana cancelling the plugin's context when the browser aborts a request is inferred from the mechanism and was not seen live.
15. Every sparkline state sits in one 120 by 24 box and has a text alternative: the loading and `No events` states are images, not live regions, the failure icon is reachable by keyboard, and the chart's native `title` gives the total (since M10, so the chart is not a tab stop).
16. A chart is built once per answer, so a triage action does not rebuild the 25 charts; `IssueTableContext` lives in its own file with a hook that throws outside a table.
17. The issue list's triage behaviour is unchanged: a succeeded or failed action restarts no sparkline (pinned at page level), and a cancelled list read shows no message (its test was rewritten in M10 so that it can fail).
18. The end-to-end limit test records the highest number of queries held at once and needs more than four issues in the list, failing with a message that names `scripts/verify_stack.py` when it has fewer; the dashboard test asserts the answer carries no notices.
19. Left for later milestones:
    * M10: a debug log of refused counts; a backend limit on concurrent Bugsink calls and merging of identical misses, since alert rules evaluate in parallel; the warning when a step is wider than 90 days; whether a cut of less than one step should warn.
    * M10: what an alert rule cannot rely on in a volume series: the last point is the step in progress and so partial; the first point can start before the range by up to one step; values can be 60 seconds old; steps are UTC-aligned from 1970-01-01, so day and week steps do not follow the dashboard's time zone; clock skew at the hour can end the series one hour late; an alert query without an interval gets the one-hour notice on every evaluation; an empty window reads as "No data", not zero.
    * M11: Refresh reloading the sparklines; the issue-list and volume end-to-end cache race.
