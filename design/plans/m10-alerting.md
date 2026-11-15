# M10: Alerting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A user writes Grafana alert rules on Bugsink data: the event count of the current hour, the number of issues in a state or seen in the last N minutes, and new issues per interval, each safe to alert on.

**Architecture:** Three query types join `issues`, `events` and `volume`: `newIssues` (a time series from `first_seen`), `issueCount` and `eventCount` (one number each, in Grafana's numeric format, so a Threshold reads them without a Reduce). They reuse the issue list cache and the count cache. Every Bugsink call of an instance passes a limit of 8 in flight, identical concurrent cache misses share one call, and Bugsink's forwarded error text never carries the token. `alerting: true` makes the data source selectable in the alert rule editor.

**Tech Stack:** Go 1.26 with `grafana-plugin-sdk-go` v0.299.0, React with `@grafana/ui`/`@grafana/runtime` 13.2.4, Jest, `@grafana/plugin-e2e` 4.0.0, the Compose stack (Bugsink 2.6.3, Grafana 13.3.1).

**Spec:** [`../bugsink_grafana_plugin_spec.md`](../bugsink_grafana_plugin_spec.md) (§6, §3, §10). **Roadmap:** [`../implementation_roadmap.md`](../implementation_roadmap.md) (M10, Definition of done). **Findings:** [`../phase0_findings.md`](../phase0_findings.md) (Q9, Q10 item 8). **Previous:** [`m9-event-volume.md`](m9-event-volume.md) "As built" 19.

## Global Constraints

* Everything in the Global Constraints of the M2 plan holds: the token never reaches the browser, a log line, an error message or a frame; cursor paging only; resources never answer 401 (Bugsink failures 502, settings 400, refused caller 403); generated code untouched; nothing in `.config/`; plugin ids and types unchanged; SPDX headers; one-line comments; behaviour tests; Go commands on `./pkg/...`; ports 3300/8800/8810/8820; push after every commit, never amend; every test command in the foreground.
* Branch `m10/alerting` in the worktree `.claude/worktrees/m10-alerting`.
* Reuse `userMessage`, `failure`, `parseModel`, `issuesFilter`, `issueList`, `validIssueID`, `cache`, `volumeFrame`, `alignDown`, `ProjectPicker`, `CommitInput`, `captureLogs` and the e2e helpers; no parallel versions. No new Go or npm dependency.
* The datasource `plugin.json` gains `"alerting": true` only; after it changes, restart Grafana (`docker compose restart grafana`).
* `QueryData` stays read-only and reads no user: alert queries arrive without one. No task changes who may write or a write route (no "Authorization: yes" task). The M7/M8 write routes keep their check order and tests.
* A query that names nothing to read answers an empty frame with an info notice; wrong input answers 400 with a sentence; a Bugsink failure is an error, never a zero.
* E2E sends no new Bugsink event: it counts this run's four `sample-python` events from `tests/report-trace.setup.ts` and `tests/report-event-detail.setup.ts`; the budget stays at 5. Alert rules made by tests live in their own folder and are deleted by the test; the stack provisions no alert rule.
* A provisioned dashboard must not open in an error state.
* No new token capability: the three types read the issue list (`issues:read`) and `event-counts` (`projects:read`, `issues:read`), which the health check already probes.

## Facts already verified (2026-11-15, Bugsink 2.6.3 and Grafana 13.3.1 in the Compose stack, 15:00 to 15:25Z)

* Plugin docs (`tutorials/build-a-data-source-backend-plugin.md`): "`alerting` | If your backend data source supports alerting, set to `true`. Requires `backend` to be set to `true`."; then "Restart your Grafana instance." `/api/frontend/settings` today: `bugsink`, `bugsink-project`, `bugsink-triage` have `meta.alerting` false, Loki true.
* The provisioning API does not check that flag: `POST /api/v1/provisioning/alert-rules` (header `X-Disable-Provenance: true`) with a `volume` query on `bugsink` was accepted, and `PUT /api/v1/provisioning/folder/m10-probe/rule-groups/m10-probe` `{"interval":10}` set a 10 s interval and kept the rule. Grafana stored the query model with `"intervalMs":1000,"maxDataPoints":43200` and `relativeTimeRange {from: 3600, to: 0}`; the rule echo has `keep_firing_for` and `notification_settings`.
* That rule (Reduce `last`, Threshold `> 0`, `for: 0s`) was evaluated 2 s after creation and `GET /api/prometheus/grafana/api/v1/rules?folder_uid=m10-probe` showed rule `state: "firing"`, `health: "ok"` and one alert `state: "Alerting"` with labels `{alertname, grafana_folder, m10, project: "sample-python"}`: frame labels become alert instance labels.
* An error result: the same rule on `bugsink-project` with `project: 2` gave rule `health: "error"`, `lastError` `[sse.dataQueryError] failed to execute query [A]: Bugsink refused the request: This token is not allowed to access this project.` and one alert `state: "Error"` labelled `datasource_uid: bugsink-project, ref_id: A`. Bugsink's log shows 3 `event-counts` calls one second apart for that one evaluation: Grafana retries a failed evaluation (`max_attempts = 3`, `initial_retry_delay = 1s` in `defaults.ini`). Errors are not cached by the plugin, so each attempt calls Bugsink.
* `POST /api/v1/eval` answers 200 with per-query results and works for a data source whose `meta.alerting` is false. A `volume` frame reduces to a `numeric-multi` frame `{B: 7}` with the series' labels; an empty `volume` frame reduces to `null` (NaN), not 0; a `table` (issues) frame fails with `input data must be a wide series but got type long`. The eval output of a `volume` query with `intervalMs` 1000 carries no frame notices, so the one-hour step notice never reaches an alert.
* A data source's `numeric-multi` frame (one `number` field, one row; Loki instant query `sum(count_over_time(...[1h]))`) feeds a Threshold expression directly, without Reduce: eval gave `C = 1`. Not verified: an `int64` field in such a frame (Loki's is `float64`; SSE turned the `volume` frame's `int64` into `float64`); Task 7 checks first.
* Grafana docs: "Each time series or table row is evaluated as a separate alert instance." (`alerting/fundamentals/alert-rules/queries-conditions`); "The **No Data** state occurs when the alert rule query runs successfully but returns no data points at all."; "Grafana, by default, triggers a new `DatasourceError` alert" (`.../nodata-and-error-states`); "Rules can be evaluated concurrently or sequentially." and "You can set a Keep firing for period to avoid repeated firing-resolving-firing notifications caused by flapping conditions." (`.../alert-rule-evaluation`).
* Grafana 13.3.1 `defaults.ini` `[unified_alerting]`: `min_interval = 10s`, `evaluation_timeout = 30s`, `max_attempts = 3`; no setting limits concurrent rule evaluations. `[smtp] enabled = false`. The stack has no contact point; the root policy's receiver is `empty` (no integrations), so a firing rule sends nothing.
* `DELETE /api/folders/<uid>?forceDeleteRules=true` deleted the folder and its rule (200); for a missing folder it answers 404 `{"message":"folder not found","status":"not-found"}`. The probe folder `m10-probe` and rule `m10-probe-rule` were deleted; `GET /api/v1/provisioning/alert-rules` is `[]` again.
* SDK v0.299.0: `backend.FromAlertHeaderName = "FromAlert"`; `NewAlertForwarderMiddleware` is in the default handler chain of `datasource.Manage` and adds the header to the plugin's outgoing HTTP requests through `ContextualMiddleware` (in `httpclient.DefaultMiddlewares`), so Bugsink receives `FromAlert` on alert queries. `QueryData` reads no `PluginContext.User` today (grep). `data.FrameTypeNumericMulti = "numeric-multi"`.
* Bugsink: `first_seen` is the first event's `ingested_at` (issue `SAMPLE-PYTHON-278`: `first_seen 09:22:03.218754Z` = `ingested_at`, event `timestamp 09:22:03.201122Z`), so first and last seen are Bugsink's clock; `digest_order` desc follows `first_seen` desc. Every list sort is one of `digest_order`, `last_seen`, `digested_event_count`: none sorts by `first_seen`.
* Counting by paging: project 1 has 278 issues, all open (2 pages, 37 to 73 ms in all); all projects 286 open; `project=1&state=open&q=m2-e2e` 1 issue. `GET projects/1/event-counts/` with no parameters: 24 buckets, `start` 24 hours before `end`, `end` the end of Bugsink's current hour, the last bucket the hour in progress.
* `@grafana/ui` 13.2.4 `Tooltip` clones its child with `tabIndex: 0` ("tooltip trigger should be keyboard focusable"), which makes every sparkline chart a tab stop. `@grafana/plugin-e2e` 4.0.0 has `alertRuleEditPage` with `getQueryRow()`, `datasource.set()` and `evaluate()` (Preview, the eval endpoint) and the `toBeOK` matcher.

## Decisions in this plan, for review

1. **Three query types.** `newIssues` (New issues), `issueCount` (Issue count) and `eventCount` (Event count); the picker grows from three to six. "Open-issue count" and "issues seen in the last N minutes" are one type, `issueCount`: the issue list's `project`, `state` (default `unresolved`, as for `issues`), `q`, plus optional `minutes` (1 to 1440; absent means any time).
2. **Event count of the current hour = the hour in progress by Bugsink's clock, plus `hours`.** `eventCount` sums the last `hours` (1 to 24, default 1) of Bugsink's default window, whose last bucket is its hour in progress. The read sends no `start` and no `end`, so a skewed Grafana clock cannot get a 400 at the hour. At the top of the hour a one-hour count starts again from zero and a firing rule resolves; the README says so and recommends "Keep firing for" `1h` or `hours: 2` (the hour in progress and the full hour before).
3. **Numeric frames for current values** (contract below): one frame, `Meta.Type` `numeric-multi`, one row, one `int64` field `count`, so a Threshold is the whole condition and a dashboard stat shows it.
4. **Stable alert labels:** the new frames carry `project_id` (the id as text) or `issue` (as typed), never the project name, so a rename or an unreadable project list cannot turn one alert instance into another. They read no project list. All projects: no label.
5. **New issues, read and buckets:** the issue list with `state=all`, `q`, the project, sorted by `last_seen` descending and bounded at the window start (every issue first seen after a time was last seen after it), counted by `first_seen` into steps. Step = `Interval` rounded up to whole minutes, at least one minute and at least the range over 1440 points; steps aligned to UTC from 1970-01-01; the window starts at the aligned `from`, so every point is a whole step except the last, which is the step in progress; the series is zero-filled, so no new issues is 0, not No data. The model's `state` is ignored. No step notice.
6. **The page cap is an error, not a lower bound.** `issueCount` and `newIssues` read at most `MaxPages` (20 pages, 5000 issues); a list cut there answers 400 with the cap message. A lower bound would keep a `> N` rule above 5000 silently Normal and make any `< N` rule wrong, and notices never reach an alert.
7. **Freshness:** issue counts and new issues use the issue list cache (15 s, keyed by the filter, a list read since T answering later bounds); event counts use the count cache (60 s) keyed by kind, id and Grafana's current hour, storing an answer only when its last bucket is that hour, so no previous hour is served after the hour turns. Both lifetimes exceed Grafana's shortest interval (10 s): a rule can see a value up to 15 s or 60 s old.
8. **Load bounds:** a transport limit of **8** Bugsink requests waiting for an answer per instance (released when the headers arrive; Django has done its work by then), queued requests ending with their context. Identical concurrent cache misses (issue lists keyed by filter and bound, counts, the project list) share one call; when the shared call was cancelled, a caller whose own context is alive calls alone. Both with `sync` from the standard library.
9. **Per evaluation, at most:** `eventCount` 1 `event-counts` call (none within 60 s in the same hour); `issueCount` and `newIssues` 20 list pages (none within 15 s for the same filter); Grafana's three attempts triple a failing evaluation. `volume` is unchanged (1 call plus the cached project list).
10. **Queries behave the same from alerting:** the plugin does not read `FromAlert` or the user; `TestQueryNeverWrites` covers the three new types with no user and the header set.
11. **Forwarded Bugsink text** (Security): the client replaces the request's token (the Authorization header without `Bearer `) with `[redacted]` in every detail it keeps, then cuts the detail to 500 characters plus an ellipsis character. Hostile test values: the header echoed in `detail`, in a list body, in a field error, in an HTML body, and a token across the 500-character cut.
12. **Sparkline charts are no tab stops:** the chart drops its `Tooltip` and gets a `title` with the same text as its accessible name; the failure icon keeps its focusable tooltip, as it is the only way to read the message.
13. **No provisioned alert rules:** file provisioning of paused rules is not documented (the file-provisioning page names no `isPaused`), and an unpaused one would fire for ever on the dev stack. A new dashboard `Bugsink current values` shows the three types; the e2e makes and deletes its own rules.
14. **A count field upstream:** the request text is in the M10 report for the owner to file; the plan sends nothing.

## Query model (additions)

| Key | Used by | Values | Default |
| :--- | :--- | :--- | :--- |
| `queryType` | all | adds `newIssues`, `issueCount`, `eventCount` | `issues` |
| `project` | the three | project id | absent: all projects (`eventCount`: names nothing) |
| `state` | `issueCount` | as `issues` | `unresolved` |
| `q` | `newIssues`, `issueCount` | text, takes dashboard variables | empty |
| `issue` | `eventCount` | issue id, wins over `project` | absent |
| `minutes` | `issueCount` | JSON integer 1 to 1440 | absent: any time |
| `hours` | `eventCount` | JSON integer 1 to 24 | 1 |

## Frame and message contract

| Type | Frame | Value |
| :--- | :--- | :--- |
| `issueCount` | `Name` `Issues`, `numeric-multi` `{0, 1}`, field `count` `[]int64` (one row), labels `{project_id}` when a project is set, `DisplayNameFromDS` `Issues` | issues matching project, state and `q`, and with `minutes` those last seen at or after now minus `minutes` |
| `eventCount` | `Name` `Events`, as above, labels `{issue}` or `{project_id}` | sum of the last `hours` buckets of Bugsink's default window |
| `newIssues` | `volumeFrame("New issues", {project_id}, step, starts, totals, nil)` | issues with `first_seen` in each step |

| Condition | Answer |
| :--- | :--- |
| `eventCount` names neither project nor issue | empty `Events` frame (field, no row), info notice `An event count needs a project or an issue.`, no request |
| `minutes` not a whole number 1 to 1440 (`0`, `1441`, `2.5`, `"5"`, `-1`) | 400 `The minutes must be a whole number from 1 to 1440.` |
| `hours` not a whole number 1 to 24 (`0`, `25`, `1.5`, `"2"`) | 400 `The hours must be a whole number from 1 to 24.` |
| `issueCount` list cut at 20 pages | 400 `More than 5000 issues match, so they cannot be counted. Narrow the query with a project, a state or a search.` |
| `newIssues` list cut at 20 pages | 400 `More than 5000 issues were seen in the time range, so the new ones cannot be counted. Narrow the query with a project or a search, or shorten the time range.` |
| bad `issue`, `state`, `project` | the existing messages (`"a/b" is not an issue id.`, `Unknown state "bogus".`, `The project must be a number.`) |
| Bugsink failure or unreachable | the M2 message contract, error source downstream; never a frame |

## Review Focus

1. Bugsink down or refusing while a rule evaluates: the query is an error (rule Error, `DatasourceError`), never `0` or an empty frame that reads as "no issues" (Task 3 `TestCountQueriesNeverAnswerZeroOnFailure`, Task 4 `TestNewIssuesFailure`, Task 7 `a refused count puts its rule in Error`).
2. Grafana's and Bugsink's clocks apart at the top of the hour: the event count follows Bugsink's hour in progress, is never a 400, and a previous hour is never served from the cache after Grafana's hour turns (Task 3 `TestEventCountTopOfHour`, `TestEventCountCache`).
3. Many rules evaluating together: at most 8 Bugsink requests wait at once per instance, and 8 identical cold queries make one call (Task 2 `TestInstanceLimitsConcurrentCalls`, `TestIdenticalColdQueriesShareOneCall`).
4. A rule over a busy installation: past 5000 issues the count is the cap error, read once per attempt and not cached as a number (Task 3 `TestIssueCountCap`, Task 4 `TestNewIssuesCap`).
5. A proxy or Bugsink echoing the Authorization header in an error: no panel, resource answer or log line holds the token (Task 1 `TestForwardedTextNeverHoldsToken`).

## File structure

| File | Responsibility |
| :--- | :--- |
| `pkg/bugsink/errors.go`, `client.go` | redact and cut forwarded detail |
| `pkg/plugin/limit.go` (new), `flight.go` (new), `datasource.go`, `query.go`, `volume.go` | transport limit, shared misses, debug log of refused counts |
| `pkg/plugin/counts.go` (new) | `issueCount`, `eventCount`, numeric frame, number fields |
| `pkg/plugin/newissues.go` (new) | `newIssues` step, buckets, query |
| `src/datasource/plugin.json`, `types.ts`, `counts.ts` (new), `components/QueryEditor.tsx`, `CommitInput.tsx`, `NewIssuesQueryEditor.tsx`, `IssueCountQueryEditor.tsx`, `EventCountQueryEditor.tsx` (new) | alerting flag, model, editors, live rules |
| `src/pages/issues/SparklineCell.tsx`, `SparklineCell.test.tsx`, `src/pages/IssuesPage.test.tsx` | M9 leftovers |
| `docker/grafana/provisioning/dashboards/bugsink-alerting.json` (new), `tests/alerting.spec.ts` (new), `tests/fixtures.ts`, `tests/volume.spec.ts` | the demo |
| `src/README.md`, `CHANGELOG.md`, `docker/README.md`, `design/plans/m9-event-volume.md`, spec §6 | docs and deferred corrections |

---

### Task 1: Keep the API token out of forwarded Bugsink text (Security: yes)

**Files:** Modify `pkg/bugsink/errors.go:47-65`, `pkg/bugsink/client.go:174-177`, `pkg/bugsink/client_test.go`; Create `pkg/plugin/redaction_test.go`.

**Interfaces:** Produces `const maxDetailChars = 500`, `const redacted = "[redacted]"`; `newAPIError(status int, body []byte, token string) *APIError` (detail first, then the capability, then redaction and the cut); `readAPIError` takes the token from `resp.Request.Header.Get("Authorization")` without `Bearer ` (empty when `resp.Request` is nil; an empty token replaces nothing).

- [ ] **Step 1: Write failing tests.**
  - `client_test.go` `TestAPIErrorHidesToken`, a server answering 400 with the request's Authorization header echoed: `{"detail":"echo <header>"}` gives `Detail` `echo Bearer [redacted]`; `["echo","<token>"]` gives `echo [redacted]`; `{"q":["<token>"]}` gives `q: [redacted]`; a `text/html` body holding the token gives an empty `Detail`; `err.Error()` never contains the token. `TestAPIErrorIsCut`: a 2000-character detail gives 500 characters plus the ellipsis character; 495 `x` then the token gives no 6-character run of the token. A 403 naming `issues:read` keeps `Capability`.
  - `redaction_test.go` `TestForwardedTextNeverHoldsToken` (table: detail, list, HTML body × issues query, `GET issues/X-1`, `POST issues/X-1/resolve` with triage on and `X-Bugsink-Triage: 1`, Editor user): the panel error, the resource body and every `captureLogs` line hold no `secretToken`; the JSON cases show `[redacted]` in the message.
- [ ] **Step 2: Run** `go test ./pkg/...`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test -race ./pkg/...`, `go vet ./pkg/...`. Expected: PASS, existing message tests unchanged.
- [ ] **Step 5: Commit** `Keep the API token out of Bugsink's forwarded text` and push.

### Task 2: Bound concurrent Bugsink calls and share identical reads

**Files:** Create `pkg/plugin/limit.go`, `limit_test.go`, `flight.go`, `flight_test.go`; Modify `pkg/plugin/datasource.go` (`configure`, fields), `query.go` (`issueList`, `projects`), `volume.go` (`eventCounts`), `volume_query_test.go`, `query_test.go`.

**Interfaces:**

```go
const maxConcurrentCalls = 8
// limitTransport lets at most n requests wait for an answer at once; a queued request ends with its context's error.
func limitTransport(next http.RoundTripper, n int) http.RoundTripper // forwards CloseIdleConnections
type flight[V any] struct{ /* mutex, calls by key, each with done channel, result and dups count */ }
func newFlight[V any]() *flight[V]
// do runs fn once for concurrent callers of one key; after a cancelled shared call a caller with a live context runs fn alone.
func (g *flight[V]) do(ctx context.Context, key string, fn func(context.Context) (V, error)) (V, error)
// Datasource gains issueFlight *flight[bugsink.IssueList], countFlight *flight[[]bugsink.EventCount], projectFlight *flight[projectsResult]
```

- `configure` wraps `d.httpClient.Transport` (or `http.DefaultTransport` when nil) before `bugsink.New`. Flight keys: `filterKey(f)+"|"+from.UnixNano()`, the count cache key, `""`. Nothing is remembered after a call ends; caching stays in `cache`.
- `eventCounts` logs a refused answer: `backend.Logger.FromContext(ctx).Debug("Event counts refused", "reason", err.Error())` when `errors.Is(err, bugsink.ErrNotBugsink)`.

- [ ] **Step 1: Write failing tests** (`-race`; order of events stated for each):
  - `TestLimitTransport`: n 2, a server that holds every request until released: start 5 requests, wait until the server holds 2, assert it never holds more (atomic maximum) for 50 ms, release: all 5 succeed.
  - `TestLimitTransportQueuedCancel`: n 1, one request held; a second with a context cancelled while queued returns `context.Canceled` and never reaches the server; release the first: it succeeds.
  - `TestLimitTransportReleasesOnError`: n 1, a closed server: 10 sequential requests all fail promptly.
  - `TestFlightSharesOneCall`: the leader's `fn` blocks; start 7 followers and wait until `dups` is 7, then release: `fn` ran once and all 8 got its value.
  - `TestFlightSharesErrorsOnce`: the shared error reaches all callers; the next call runs `fn` again.
  - `TestFlightCancelledLeader`: hold the leader, start one follower, cancel the leader's context, release: the leader gets `context.Canceled`, the follower the value of its own `fn` run (2 runs).
  - `TestFlightFollowerCancelled`: a waiting follower whose context is cancelled returns `context.Canceled` at once; the leader still gets its value.
  - `TestIdenticalColdQueriesShareOneCall`: 8 concurrent identical `volume` queries against a server that holds `event-counts` until `countFlight` has 7 dups: one `event-counts` request, 8 equal frames; the same for 8 `issues` queries (one list request).
  - `TestInstanceLimitsConcurrentCalls`: 20 concurrent `volume` queries for 20 issues, a holding server: at most 8 held at once, all 20 answer.
  - `TestRefusedCountsAreLogged`: `interval_seconds: 86400`: one debug line `Event counts refused` whose `reason` contains `are not hourly` and no token; the panel message is unchanged.
- [ ] **Step 2: Run** `go test -race ./pkg/plugin/...`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test -race -count=3 ./pkg/...`, `go vet ./pkg/...`. Expected: PASS, M2 to M9 tests unchanged.
- [ ] **Step 5: Commit** `Bound concurrent Bugsink calls and share identical reads` and push.

### Task 3: The issue count and event count queries

**Files:** Create `pkg/plugin/counts.go`, `counts_test.go`; Modify `pkg/plugin/query.go` (`queryModel`, `parseModel`, `query`), `query_test.go` (`TestQueryNeverWrites`).

**Interfaces:**

```go
const (
	queryTypeIssueCount = "issueCount"
	queryTypeEventCount = "eventCount"
	maxCountMinutes = 1440
	maxCountHours   = 24
)
// queryModel gains Minutes json.RawMessage `json:"minutes"` and Hours json.RawMessage `json:"hours"`
// wholeNumber reads an optional JSON integer from lo to hi; absent or null gives 0, true.
func wholeNumber(raw json.RawMessage, lo, hi int) (int, bool)
type issueCountQuery struct{ Filter bugsink.IssueFilter; Minutes int } // Minutes 0: any time
func parseIssueCountQuery(m queryModel, project int) (issueCountQuery, string)
func (d *Datasource) queryIssueCount(ctx context.Context, m queryModel, project int) backend.DataResponse
type eventCountQuery struct{ ProjectID int; Issue string; Hours int }
func parseEventCountQuery(m queryModel, project int) (eventCountQuery, string)
func (d *Datasource) queryEventCount(ctx context.Context, m queryModel, project int) backend.DataResponse
// hourCounts reads Bugsink's default 24-hour window through the count cache, storing it only when its last bucket is Grafana's current hour.
func (d *Datasource) hourCounts(ctx context.Context, q eventCountQuery) ([]bugsink.EventCount, error)
func countLabels(projectID int, issue string) data.Labels
// countFrame builds a current-value frame with zero or one value.
func countFrame(name string, labels data.Labels, values []int64, notices []data.Notice) *data.Frame
```

- `issueCount` filter: `issuesFilter` with `Sort` `last_seen`, `Order` `desc` forced; `seenSince` = `d.now()` minus `Minutes` (zero for any time), through `issueList`; a `Truncated` list gives the cap message; the count is the issues with `LastSeen` not before `seenSince`. The dashboard range is not used. `hourCounts` calls the client with zero `start` and `end`, key `kind|id|hours|<Grafana's hour start, Unix>`, through `countFlight`.

- [ ] **Step 1: Write failing tests** in `counts_test.go`, `d.now` pinned to `2026-11-15T13:48Z`:
  - `TestIssueCountProject`: `{"queryType":"issueCount","project":1,"state":"open"}` with a range ending a year ago: requests carry `project=1&state=open&sort=last_seen&order=desc`, both pages are read, the frame is the contract's with value 251 and labels `{project_id: "1"}`.
  - `TestIssueCountSeenInLastMinutes`: `minutes` 5, `state` `all`: issues last seen 1, 4 and 6 minutes ago give 2; page 2 is not requested.
  - `TestIssueCountCap`: a server that always returns `next`: the cap message, status 400, 20 requests; a second query makes 20 more.
  - `TestIssueCountValidation`: the `minutes` values of the contract give its message and no request; `null` and absent count any time; `state: "bogus"` gives `Unknown state "bogus".`
  - `TestIssueCountSharesTheIssueCache`: an `issues` query `{"project":1,"state":"open"}` over the last day, then `issueCount` with `minutes` 60 within 15 s: one list read.
  - `TestEventCountProject`: `{"queryType":"eventCount","project":1}`: one request to `projects/1/event-counts/` with an empty query string; 24 buckets ending `13:00Z` with 7 there: value 7, labels `{project_id: "1"}`, name `Events`; `hours: 2` adds the `12:00Z` bucket.
  - `TestEventCountIssueWins`: `issue` `X-1` and `project` 1: `issues/X-1/event-counts/` only, labels `{issue: "X-1"}`.
  - `TestEventCountTopOfHour`: Bugsink's last bucket `12:00Z` (behind) gives that bucket's count; `14:00Z` (ahead) gives that one's; in both cases a second query within 60 s requests again.
  - `TestEventCountCache`: the same query twice within 60 s: one request; `d.now` moved to `14:00:05Z`: a new request.
  - `TestEventCountValidation`: the `hours` values of the contract give its message; `issue` `a/b` gives `"a/b" is not an issue id.`; neither project nor issue gives the empty frame and the info notice, no request.
  - `TestCountQueriesNeverAnswerZeroOnFailure`: a closed server and a 403 `This token is not allowed to access this project.` give errors with the M2 messages and statuses and no frames, for both types.
  - `TestQueryNeverWrites` gains rows `issueCount` and `eventCount`, and every row now sends `Headers: {"FromAlert": "true"}` with a `PluginContext` that has no `User`.
- [ ] **Step 2: Run** `go test -race ./pkg/plugin/...`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test -race ./pkg/...`, `go vet ./pkg/...`. Expected: PASS.
- [ ] **Step 5: Commit** `Add the issue count and event count queries` and push.

### Task 4: The new issues query

**Files:** Create `pkg/plugin/newissues.go`, `newissues_test.go`; Modify `pkg/plugin/query.go` (route), `query_test.go` (`TestQueryNeverWrites` row `newIssues`).

**Interfaces:**

```go
const (
	queryTypeNewIssues = "newIssues"
	maxNewIssuePoints  = 1440
)
// newIssueStep is interval rounded up to whole minutes, at least one minute and at least the range over maxNewIssuePoints.
func newIssueStep(from, to time.Time, interval time.Duration) time.Duration
// newIssueSeries counts issues by first_seen into epoch-aligned steps from start that begin before to; first_seen in [start, to).
func newIssueSeries(issues []bugsink.Issue, start, to time.Time, step time.Duration) (starts []time.Time, totals []int64)
func (d *Datasource) queryNewIssues(ctx context.Context, q backend.DataQuery, m queryModel, project int) backend.DataResponse
```

- Filter `{ProjectID: project, State: all, Query: m.Q, Sort: last_seen, Order: desc}`; `start` = `alignDown(from, step)`; read `issueList(ctx, filter, start)`; `Truncated` gives the cap message.

- [ ] **Step 1: Write failing tests:**
  - `TestNewIssueStep` (table): 1 h range, 1 s: 1 min; 7 d, 10 min: 10 min; 7 d, 30 s: 7 min; 90 d, 2 h: 2 h; 1 h, 90 s: 2 min.
  - `TestNewIssueSeries`: start `00:00`, to `00:25`, step 10 min, first seen `00:01`, `00:09`, `00:15`, `00:24:59`, `00:25` and `23:59` the day before: starts `00:00, 00:10, 00:20`, totals `2, 1, 1`; no issues: three zeros; to `00:20`: two points.
  - `TestNewIssuesQuery`: `{"queryType":"newIssues","project":1,"q":"x","state":"resolved"}`, range `10:05..10:35`, interval 10 min: requests carry `project=1&state=all&q=x&sort=last_seen&order=desc`; paging stops at the first issue last seen before `10:00`; frame `New issues`, `timeseries-multi`, labels `{project_id: "1"}`, `Config.Interval` 600000, zero-filled.
  - `TestNewIssuesAllZeroIsNotEmpty`: no issues in range: every point 0, length equals the step count.
  - `TestNewIssuesCap`: the cap message, 400, 20 requests.
  - `TestNewIssuesUsesTheIssueCache`: an `issues` query `{"state":"all","q":"x","project":1}` from `09:00`, then `newIssues` from `10:05` within 15 s: one list read.
  - `TestNewIssuesFailure`: a closed server gives `Cannot reach Bugsink: …` with status 502 and no frame.
- [ ] **Step 2: Run** `go test -race ./pkg/plugin/...`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test -race ./pkg/...`, `go vet ./pkg/...`. Expected: PASS.
- [ ] **Step 5: Commit** `Add the new issues query` and push.

### Task 5: Editors for the alerting queries and the alerting flag

**Files:** Modify `src/datasource/plugin.json` (`"alerting": true` after `"metrics": true`), `src/datasource/types.ts`, `components/QueryEditor.tsx`, `components/QueryEditor.test.tsx`, `components/CommitInput.tsx`, `components/CommitInput.test.tsx`; Create `src/datasource/counts.ts`, `counts.test.ts`, `components/NewIssuesQueryEditor.tsx`, `IssueCountQueryEditor.tsx`, `EventCountQueryEditor.tsx` with tests.

**Interfaces:**

```ts
export type QueryType = 'issues' | 'events' | 'volume' | 'newIssues' | 'issueCount' | 'eventCount';
// BugsinkQuery gains minutes?: number; hours?: number
export const MINUTES_MESSAGE = 'The minutes must be a whole number from 1 to 1440.'; // counts.ts
export const HOURS_MESSAGE = 'The hours must be a whole number from 1 to 24.';
export function wholeNumberError(text: string, min: number, max: number, message: string): string | undefined; // empty text is valid
// CommitInput gains validate?: (text: string) => string | undefined and description?: string
```

- Picker labels: `Issues`, `Events`, `Event volume`, `New issues`, `Issue count`, `Event count`. New issues: `Project`, `Search`. Issue count: `Project`, `State` (shows `Unresolved` when empty), `Search`, `Seen within (minutes)` (placeholder `any time`). Event count: `Project`, `Issue`, `Hours` (placeholder `1`, description `The hour in progress plus the full hours before it.`). An empty number field removes its key. While `validate` returns a message, the field shows it as its error and blur and Enter neither commit nor run.

- [ ] **Step 1: Write failing tests:** `counts.test.ts`: `''` valid, `'1'` and `'1440'` valid, `'0'`, `'1441'`, `'2.5'`, `'5m'`, `' 5'` give the message. `QueryEditor.test.tsx`: the picker offers the six labels; choosing `Issue count` calls `onChange` with `queryType: 'issueCount'` and runs; its editor shows `Project`, `State`, `Search`, `Seen within (minutes)` and not `Sort by`; `Event count` shows `Project`, `Issue`, `Hours`; `New issues` shows `Project`, `Search` and not `State`. Editor tests: typing `60` in minutes and Enter gives `minutes: 60` and one run; `61.5` shows `MINUTES_MESSAGE`, calls nothing on blur or Enter, and `State` still changes the query; clearing the field removes `minutes`; `hours` likewise with `HOURS_MESSAGE`; `CommitInput.test.tsx` covers `validate`. The flag itself is proven end to end (Task 7), not by a unit test.
- [ ] **Step 2: Run** `npm run test:ci`. Expected: FAIL.
- [ ] **Step 3: Implement.** Remind the owner: `plugin.json` changed, Grafana needs a restart.
- [ ] **Step 4: Run** `npm run test:ci`, `npm run typecheck`, `npm run lint`. Expected: PASS, Jest exits by itself.
- [ ] **Step 5: Commit** `Add editors for the alerting queries and allow alert rules` and push.

### Task 6: Issue list leftovers from M9

**Files:** Modify `src/pages/issues/SparklineCell.tsx`, `SparklineCell.test.tsx:76-89`, `src/pages/IssuesPage.test.tsx:344-361`.

- [ ] **Step 1: Write the tests.**
  - `the chart is not a tab stop`: the `img` named `7 events in the last 24 hours` has no `tabindex` attribute and `title` `7 events in the last 24 hours`; the failure icon keeps `tabindex="0"`.
  - `does not rebuild the chart when the context changes`: compare `screen.getByTestId('uplot-main-div').firstElementChild` before and after the rerender with `toBe`.
  - `shows no message when the list read it cancelled rejects`, in this order: first read pending (rejects on abort); pick `Muted`; the second read stays pending; `await waitFor` two calls and flush with `act`; assert no `alert`; resolve the second read; `MUTED-1` shows and still no `alert`.
- [ ] **Step 2: Prove the two rewritten tests can fail** with throwaway edits, not committed: without the `useMemo` in `SparklineCell` the rebuild test fails; a page that shows the error of an aborted read fails the cancelled-read test.
- [ ] **Step 3: Implement** the chart without `Tooltip`, with `title={label}`.
- [ ] **Step 4: Run** `npm run test:ci`, `npm run typecheck`, `npm run lint`. Expected: PASS.
- [ ] **Step 5: Commit** `Keep sparkline charts out of the tab order and tighten two M9 tests` and push.

### Task 7: Current values dashboard and end-to-end tests of the demo

**Files:** Create `docker/grafana/provisioning/dashboards/bugsink-alerting.json`, `tests/alerting.spec.ts`; Modify `tests/fixtures.ts` (receives `thisRunsIssue` from `tests/volume.spec.ts`, unchanged), `tests/volume.spec.ts`, `docker/README.md`.

**Interfaces:** dashboard uid `bugsink-alerting`, title `Bugsink current values`, time `now-7d..now`, data source `bugsink`; stat panels `Open issues in sample-python` `{queryType:'issueCount',project:1,state:'open'}`, `Events in sample-python this hour` `{queryType:'eventCount',project:1}`, `Issues seen in the last hour` `{queryType:'issueCount',state:'all',minutes:60}`; time series (bars) `New issues per day in sample-python` `{queryType:'newIssues',project:1}`, `interval: "1d"`.

- [ ] **Step 1: First contact with the running Grafana** (build, `docker compose restart grafana`; record each answer in the commit body; stop and report if one differs):
  - `/api/frontend/settings`: `bugsink` has `meta.alerting` true.
  - `POST /api/v1/eval` with A `{queryType:'eventCount',project:1,hours:2}` on `bugsink` (`relativeTimeRange` 600) and C a Threshold `gt 0` on A, condition C: A is `numeric-multi` with one number at least 1 and label `project_id: "1"`, C is 1. If SSE refuses the `int64` field, report it.
  - The same with A `{queryType:'issueCount',project:1,state:'open',q:'m2-e2e'}`: A at least 1; with `newIssues` and a Reduce `sum`: a number.
  - `python3 scripts/verify_stack.py` prints eleven `ok` lines.
- [ ] **Step 2: Write the tests** in `tests/alerting.spec.ts` (read `.config/AGENTS/e2e-testing.md` first; anonymous Admin requests; `test.describe.configure({ mode: 'serial' })`):
  - `the data source is offered for alert rules`: `alertRuleEditPage`, `getQueryRow()`, `datasource.set('Bugsink')`, `Query type` `Event count`, `Project` `sample-python`; `await expect(alertRuleEditPage.evaluate()).toBeOK()`.
  - `the demo rules fire`: `DELETE /api/folders/bugsink-e2e-alerts?forceDeleteRules=true` answers 200 or 404; `POST /api/folders` `{uid:'bugsink-e2e-alerts', title:'Bugsink e2e alerts'}`; `POST /api/v1/provisioning/alert-rules` with `X-Disable-Provenance: true` for `bugsink-e2e-events` (A `{queryType:'eventCount',project:1,hours:2}`) and `bugsink-e2e-open` (A `{queryType:'issueCount',project:1,state:'open',q:EVENT_MARKER}`), each with C a Threshold `gt 0` on A, `condition: 'C'`, `for: '0s'`, `noDataState: 'NoData'`, `execErrState: 'Error'`, group `bugsink-e2e`, `relativeTimeRange {from: 600, to: 0}`, data source uid from `readProvisionedDataSource`; `PUT …/rule-groups/bugsink-e2e` `{interval: 10}`. Then, in `toPass({ timeout: 60_000 })`, `GET /api/prometheus/grafana/api/v1/rules?folder_uid=bugsink-e2e-alerts`: both rules `state: 'firing'`, each with one alert `state: 'Alerting'` and label `project_id: '1'`. Also `POST /api/ds/query` `{queryType:'eventCount',issue:<thisRunsIssue friendlyId>,hours:2}` gives at least 1: the count includes this run's event. `hours: 2` keeps the test passing when an hour turns between the setup's events and the evaluation.
  - `a refused count puts its rule in Error`: in the same folder a rule `bugsink-e2e-refused` on `bugsink-project` with A `{queryType:'issueCount',project:2}`: within the `toPass` the rule's `health` is `error`, its `lastError` contains `This token is not allowed to access this project.` and its alert's state is `Error`.
  - `afterAll`: delete the folder with `forceDeleteRules=true`, expect 200; the rules endpoint then lists no group of that folder.
  - `the current values dashboard opens clean`: `readProvisionedDashboard({ fileName: 'bugsink-alerting.json' })`, `gotoDashboardPage`; no panel shows an error; the `Open issues in sample-python` stat shows a number of 1 or more.
  - Evidence only, not asserted: that the rules were Normal before this run's events (CI's `verify_stack.py` events count too).
- [ ] **Step 3: Run** `npm run build && npm run build:backend && docker compose restart grafana && npm run e2e`. Expected: every test passes, twice in a row, and `GET /api/v1/provisioning/alert-rules` is `[]` after each run.
- [ ] **Step 4: Run** `npm run lint`, `npm run typecheck`, `scripts/check_spdx.sh`. Expected: PASS.
- [ ] **Step 5:** Describe the dashboard in `docker/README.md`, and that the stack has no contact point (receiver `empty`), so a rule made there notifies nobody.
- [ ] **Step 6: Commit** `Provision a current values dashboard and test alerting end to end` and push.

### Task 8: Documentation and deferred corrections

**Files:** Modify `src/README.md`, `CHANGELOG.md`, `design/plans/m9-event-volume.md:349,352,357`, `design/bugsink_grafana_plugin_spec.md:191-192`.

- [ ] **Step 1: Write** in `src/README.md` a section "Alerting": the three types and their fields; which values they return and how old (15 s issue counts, 60 s event counts); the hourly limit (spec §6 sentence) and the top of the hour (Keep firing for `1h` or `hours: 2`); a Threshold reads Issue count and Event count directly, New issues and Event volume need a Reduce; labels `project_id`/`issue`; Bugsink failures put the rule in Error, an Event count with nothing named and an empty Event volume window are No data, no new issues is 0; the 5000-issue cap and its message; Bugsink requests per evaluation (decision 9) and Grafana's three attempts; at most 8 requests at once per data source; the two demo rules as a recipe. In "Querying event volume": its labels use the project name, so prefer Event count for alerts. In `CHANGELOG.md`: the three query types, alerting, the request limit, the redacted Bugsink text, sparklines out of the tab order.
- [ ] **Step 2: Correct** the deferred wording: in spec line 191 replace "A range of 90 days or more (the `Last 90 days` preset included) is cut" (in `src/README.md` line 268 "A time range of 90 days or more (the `Last 90 days` preset included) is cut") by "A range whose start, rounded down to its step, is older than the oldest hour Bugsink keeps (90 days before the end of the current hour) is cut: every range of 90 days or more, the `Last 90 days` preset included, and some a little shorter"; spec line 192 "says each point counts several hours" becomes "says how many hours each point counts"; M9 plan line 349 gets the same 90-day wording; line 352 becomes "With 30 rows, page 1 sends 25 volume queries and page 2 sends 5 more, 30 in all; the queries the old limiter started before cancelling them never reached the network."; line 357's second sentence becomes "a succeeded or failed action restarts no sparkline (pinned at page level), and the cancelled-read test was rewritten in M10 so that it can fail."
- [ ] **Step 3: Run** `scripts/check_spdx.sh`, `scripts/check_licenses.sh`, `npm run lint`, `grep -n "90 days or more (the" src/README.md design/bugsink_grafana_plugin_spec.md` (no output). Expected: PASS.
- [ ] **Step 4: Commit** `Document alerting` and push.

## Not in this milestone

* A Bugsink count field: requested upstream by the owner; the page-through stays until Bugsink has one.
* Provisioned alert rules and contact points (decision 13).
* An alert-specific path for `FromAlert`: alert queries are ordinary reads.
* Event counts for all projects: Bugsink has no installation-wide counts, and summing N projects costs N calls.
* M9 leftovers that only touch dashboards: a warning when a step is wider than 90 days, and whether a cut of less than one step should warn; the volume series' name-based labels stay (README advice instead).
* M11: Refresh reloading the sparklines; the issue-list and volume end-to-end cache race.

## After the last task

* Whole-branch review against this plan and spec §6, then one fix wave.
* From the worktree: `npm ci`, `npm run build`, `npm run build:backend`, `go test -race ./pkg/...`, `npm run test:ci`, `npm run e2e`, `python3 scripts/verify_stack.py`.
* As-built updates (the fix wave, not a task): spec §6 lines 195-197 (the three types, their values, freshness, the cap error, labels, the top of the hour), §3 line 59-61 (the request limit and shared misses), §4.D line 112 (the chart's `title`, no tab stop), §10 line 286 (concurrent calls: 8 per instance in the backend) and new §10 rows for decisions 1, 2, 4, 6 and 11; the roadmap's M10 entry; an `## As built` section here.
* The merge into `main` happens only when the owner asks for it.

## As built

1. Ruling: the plan's decisions were accepted as written.
2. Every Bugsink request passes a redacting transport that also covers transport errors and errors raised while an answer's body is read or closed, because Go's HTTP client quotes upstream bytes there (Ruling: security fixes land as soon as the file is free, not in the fix wave).
3. The transport drops a 3xx answer's `Location`, because net/http quotes an unparsable one in its error before any redirect check, so the outcome of a redirect is unchanged.
4. Before redaction a detail is pre-cut to a length that grows with the token, because a cut at 500 plus the token's length leaked part of a long repeated token, and after it the text is cut to 500 characters plus an ellipsis.
5. The token is matched in any ASCII letter case and inside words; percent-encoded or base64 forms, non-ASCII case variants and net/http's own stderr line are not redacted.
6. A request's `url.Error` names only the request's scheme, host and path, so it never holds a cursor taken from Bugsink's `next` or a search; that text has the token replaced by `[redacted]` in any ASCII letter case, as written and in its path-escaped form, and is cut to 500 characters plus an ellipsis, whatever put the token in the path. The issue id of an event lookup answer must match the rule for issue ids from users before it is read, otherwise the query fails with `not a Bugsink API response`. `errors.Is`, `errors.As` and `Timeout()` still reach the cause.
7. The limit of 8 frees a slot when the answer's headers arrive and serves waiting requests first come first served, reads, writes and health probes alike, with no slot reserved for writes; during an instance swap Bugsink can briefly see 16.
8. A cancelled leader of a shared read makes its waiting callers call again, one extra Bugsink call per cancelled leader.
9. The issue-list flight key holds the cache generation, so a read started before a triage clear is neither stored nor shared with a later caller.
10. Ruling: the count field stays `int64`, which Grafana's Threshold accepts without a Reduce.
11. New issues answers 400 `The time range must end after it starts.` for a reversed or empty range, and 502 for an issue without a valid `first_seen`.
12. New issues grows the step by a minute when alignment would give more than 1440 steps, so 7 days at a 30-second interval gives 8-minute steps, not the table's 7.
13. New issues does not count an issue first seen in the future or outside the window, and its window start moves at most once a minute, so repeated evaluations share the cached list.
14. An event count answer with fewer buckets than `hours`, an empty list included, is the error `The answer did not look like a Bugsink API.`, never 0; Bugsink 2.6.3 always sends one zero-filled bucket per hour of its 24-hour default window, also for an issue with no events in it.
15. Ruling: with neither project nor issue the Event count editor shows `An event count needs a project or an issue.` under Project and its empty choice reads `No project`; the sentence never blocks save or run, and the backend keeps its empty frame with an info notice, so a rule saved through the API without a target reads No data.
16. Ruling: the sparkline chart is an image with a native `title` instead of Grafana's `Tooltip`, which made it a tab stop, and the failure icon keeps its focusable tooltip.
17. Ruling: the sparkline markup task and the documentation task were reviewed in the whole-branch review instead of on their own.
18. Grafana 13.3.1 offers the three data sources in the rule editor, reaches Alerting, Normal, Error and No data within 10 to 15 seconds at a 10-second interval, runs alert queries as `grafana_scheduler` with no user, and kept the seeded tokens out of its last 5000 log lines.
19. Not checked live: the `FromAlert` header reaching Bugsink and the rules' annotations.
20. Ruling: the end-to-end tests make one folder and four rules at a 10-second interval through the provisioning API, the two demo rules, a twin event count rule with threshold 100000 that must stay Normal, and a refused rule that must reach Error, and delete the folder after the tests; the refused test clears its own leftover rule.
21. Ruling: the dashboard test waits for each panel's query answer and requires status 200, no error and no notice before it checks each panel for error and warning icons and `No data`.
22. The README states that Grafana tries a failing evaluation up to 3 times in all, that an event count makes no call within a minute only while Bugsink's hour is Grafana's, that a New issues Reduce range must exceed the evaluation interval plus a minute, that the `issue` label is the issue as typed, and that a slow many-page count can exceed Grafana's 30-second evaluation timeout.
23. The README and the stack README say the repository provisions no alert rules, contact points or notification policies, and that Grafana's default policy and disabled email make a rule on the stack notify nobody until someone adds a contact point.
24. Estimated load per data source: about 67 Bugsink calls a minute for 20 rules at 10 seconds, 27 at one minute, 100 to 300 with five users on dashboards, about 1500 in the worst case, and up to 360 failed calls a minute for 20 rules while Bugsink is down.
25. Left for later milestones:
    * M11: a reserved slot or priority for writes and the health check, which can wait tens of seconds behind alert reads on a slow Bugsink; the health message when the plugin's own queue times out; the issue cache's size and memory under alert load (32 lists of up to 5000 issues); rounding the minutes bound so identical cold counts share a read; the hours out of the event-count key; a contiguity check on event-count buckets; the same `All projects` label on the Event volume editor; Refresh reloading the sparklines; the issue-list and volume end-to-end cache race; the redaction of the SDK tracing middleware's span if plugin tracing is ever switched on.
    * M11, test hardening: the memory test reading the process-wide `TotalAlloc`; 2-second contexts for follow-up calls; a follower after a leader panic; the new issues alignment test that also passes without alignment; distinct bodies for the three failure cases; the unused `ds.now`; the Hours "other controls keep working" test.
    * Bugsink upstream: the issue list has no total count, so counts page through it, and cursor paging can skip an issue that gets an event while later pages are read.
