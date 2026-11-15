# M2: Issues in Dashboards Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A user puts a table of Bugsink issues on a dashboard or in Explore, filtered by project, state and search text, limited to the dashboard time range, with a link from each row to the issue in Bugsink.

**Architecture:** `pkg/bugsink` gains `ListIssues`, which pages through `GET /issues/` and stops early when the list is sorted by `last_seen` descending. The data source instance now owns one Bugsink client and a small time-limited cache for its lifetime; `QueryData` turns an issues query into one table frame, and `CallResource` serves the project list to the query editor. The frontend adds a query editor and one config field.

**Tech Stack:** Go 1.26 with `grafana-plugin-sdk-go` v0.299.0, React with `@grafana/ui` 13.2.4 (`Combobox`), Jest, `@grafana/plugin-e2e` (Playwright), the M0 Compose stack (Bugsink 2.6.3, Grafana 13.3.1).

**Spec:** [`../bugsink_grafana_plugin_spec.md`](../bugsink_grafana_plugin_spec.md) (§3, §4.B, §4.F). **Roadmap:** [`../implementation_roadmap.md`](../implementation_roadmap.md) (M2, Definition of done). **Findings:** [`../phase0_findings.md`](../phase0_findings.md) (Q5, Q6, Q8).

## Global Constraints

* The API token never reaches the browser, a log line, an error message or a frame.
* The backend pages with the `cursor` taken from `next`; it never builds a cursor and never requests a URL taken from a response (spec §3).
* The plugin sends `sort` and `order` on every issue list request (spec §4.B).
* Generated code stays in `pkg/bugsink/internal/gen/` and is imported only by `pkg/bugsink`. This milestone does not regenerate it.
* Do not modify anything in `.config/`. Do not change a plugin id or type. A `plugin.json` change needs a Grafana restart.
* Every source file written by hand starts with `SPDX-License-Identifier: GPL-3.0-or-later` in a comment (JSON files are exempt); `scripts/check_spdx.sh` passes.
* Comments are one line and describe the code as it is. Tests assert behaviour, not code structure.
* Go commands run on `./pkg/...` (`go test ./pkg/...`, `go vet ./pkg/...`): `node_modules` contains a Go package.
* Host ports stay `3300` (Grafana), `8800` (Bugsink), `8810` (sample app).
* End-to-end tests of this milestone produce at most 5 Bugsink events per run (throttle, spec §7).
* Work on branch `m2/issues` in a worktree made with `git worktree add .claude/worktrees/m2-issues -b m2/issues plan/m2-issues`; push after every commit; never amend.
* Run every test command in the foreground.

## Facts already verified (2026-11-15, Bugsink 2.6.3 in the Compose stack)

* `GET issues/?project=1&sort=last_seen&order=desc` returns `{next, previous, results}` with 250 results; `next` is `http://…/api/canonical/0/issues/?cursor=<opaque>&order=desc&project=1&sort=last_seen`.
* An issue is `{id (uuid), friendly_id ("SAMPLE-PYTHON-2"), project (int), digest_order, last_seen, first_seen, digested_event_count, stored_event_count, calculated_type, calculated_value, transaction, is_resolved, is_resolved_unconditionally, is_resolved_by_next_release, fixed_at, events_at, is_muted, unmute_after (null or date-time), unmute_on_volume_based_conditions}`.
* Refusals of the issue list:

  | Request | Answer |
  | :--- | :--- |
  | `state=bogus` | 400 `{"state":["Must be 'all', 'open', 'unresolved', 'resolved', or 'muted'."]}` |
  | `sort=bogus` | 400 `{"sort":["Must be 'digest_order', 'last_seen', or 'digested_event_count'."]}` |
  | `order=up` | 400 `{"order":["Must be 'asc' or 'desc'."]}` |
  | `sort=digest_order` without `project`, global token | 400 (findings Q6); 200 for a project-bound token |
  | `project=99` | 404 `{"detail":"No Project matches the given query."}` |
  | `project=2` with the project-bound token of project 1 | 403 `{"detail":"This token is not allowed to access this project."}` |
  | `cursor=garbage` | 404 `{"detail":"Invalid cursor"}` |

* `q=environment:development` matches the sample app's issues; `q=environment:production` matches none: the sample app reports `environment="development"` (`sample-apps/python/app.py:46`). `q=LookupError` matches by title.
* Bugsink's page for an issue is `<base>/issues/issue/<uuid>/event/last/` (302 to the login page without a session; `<base>/issues/issue/<uuid>/` is 404).
* The stack holds 262 issues in project 1 (`sample-python`) and 1 in project 2 (`sample-js`), most last seen on 2026-11-13. The sample app's `GET /handled?variant=<text>` reports one handled error.
* Grafana in the Compose stack reaches Bugsink at `http://bugsink:8000`; a browser reaches it at `http://localhost:8800`. A link built from the data source URL would not open.
* SDK v0.299.0: `backend.ErrDataResponseWithSource(status, src, message)`, `backend.DownstreamError(err)`, `backend/resource/httpadapter`, `data.DataLink{Title, URL, TargetBlank}` in `data.FieldConfig.Links`, `data.Notice{Severity, Text}` in `data.FrameMeta.Notices`.
* `@grafana/ui` 13.2.4 exports `Combobox` and `ComboboxOption`.
* `@grafana/plugin-e2e` has the option `provisioningRootDir` (default `provisioning`), which `readProvisionedDashboard` and `readProvisionedDataSource` read from.

## Decisions in this plan, for review

1. **Time range means "last seen within the range".** A row is kept when `from <= last_seen <= to`. Spec §4.B names only the range start; with a range in the past that would list every issue seen since, which is misleading. Issues last seen after `to` are skipped and paging continues.
2. **A new optional setting, `Browser URL`** (`jsonData.browserUrl`): the address of Bugsink as the browser reaches it, used for issue links. Empty means the Bugsink URL. Without it the links are dead whenever Grafana reaches Bugsink by an internal address, as in the Compose stack.
3. **Query defaults:** state `unresolved`, sort `last_seen`, order `desc`, all projects.
4. **Project picker:** "All projects" plus the projects the token can see. For a project-bound token that list is its one project; the picker is not locked. "Arrival order" (`digest_order`) is offered only when a project is chosen.
5. **Page limit:** a query reads at most 20 pages (5000 issues) and then says so in a warning notice on the frame.
6. **Cache:** 15 seconds, per data source instance, at most 32 entries. An issue list fetched for range start T answers later queries with the same filter and a range start at or after T. Concurrent identical queries that all miss each fetch; they are not merged.
7. **Only `q` takes template variables.** A project variable needs variable queries, which are not in this milestone.
8. **The demo filters on `environment:development`,** the sample app's environment; the roadmap's demo says `production`.
9. **One query type.** The model carries `queryType: "issues"`; the editor shows no query type picker until M4 adds events.

## Query model

JSON of one query, as saved in a dashboard:

| Key | Values | Default |
| :--- | :--- | :--- |
| `queryType` | `issues` | `issues` (also when empty) |
| `project` | project id, or absent for all projects | absent |
| `state` | `all`, `open`, `unresolved`, `resolved`, `muted` | `unresolved` |
| `q` | text | empty |
| `sort` | `last_seen`, `digested_event_count`, `digest_order` | `last_seen` |
| `order` | `asc`, `desc` | `desc` |

## Frame contract

One frame named `issues`, `Meta.PreferredVisualization` table, with these fields in this order. A query with no matches returns the frame with all fields and no rows.

| Field name | Display name | Type | Source |
| :--- | :--- | :--- | :--- |
| `friendly_id` | Issue | string | `friendly_id`; carries the link |
| `calculated_type` | Type | string | `calculated_type` |
| `calculated_value` | Message | string | `calculated_value` |
| `transaction` | Transaction | string | `transaction` |
| `project` | Project | string | the project's name; its id as text when the project list cannot be read |
| `status` | Status | string | `resolved` when `is_resolved`, else `muted` when `is_muted`, else `open` |
| `digested_event_count` | Events | int64 | `digested_event_count` |
| `stored_event_count` | Stored events | int64 | `stored_event_count` |
| `first_seen` | First seen | time | `first_seen` |
| `last_seen` | Last seen | time | `last_seen` |
| `is_resolved` | Resolved | bool | `is_resolved` |
| `is_resolved_by_next_release` | Resolved by next release | bool | `is_resolved_by_next_release` |
| `is_muted` | Muted | bool | `is_muted` |
| `unmute_after` | Unmute after | nullable time | `unmute_after` |
| `project_id` | Project ID | int64 | `project` |
| `id` | (none) | string | `id` |

* Link on `friendly_id`: title `Open in Bugsink`, URL `<browser URL>/issues/issue/${__data.fields.id}/event/last/`, opens in a new tab.
* When the page limit cut the list: one notice, severity warning, text `Only the first 5000 issues were read. Narrow the search or the time range.`

## Message contract

Query errors (`QueryData`, per query) and resource errors (`CallResource`, JSON `{"message": …}`):

| Condition | Message | Query status / error source | Resource status |
| :--- | :--- | :--- | :--- |
| Settings invalid (any M1 settings message, or the browser URL) | the health check's message for it | 400, downstream | 400 |
| Unknown `queryType`, `state`, `sort` or `order` | `Unknown <key> "<value>".` | 400, downstream | n/a |
| `project` is not a number | `The project must be a number.` | 400, downstream | n/a |
| Bugsink 401 | `Bugsink rejected the API token.` | 502, downstream | 502 |
| Bugsink 403 naming a capability | `The API token lacks <capability>.` | 403, downstream | 502 |
| Any other Bugsink answer with a detail | `Bugsink refused the request: <detail>` | the answer's status, downstream | 502 |
| Any other Bugsink answer | `Bugsink answered HTTP <status>.` | 502, downstream | 502 |
| Not a Bugsink answer (`ErrNotBugsink`) | `The answer did not look like a Bugsink API.` | 502, downstream | 502 |
| Timeout or cancelled request | `Bugsink did not answer in time.` | 504, downstream | 502 |
| Network failure | `Cannot reach Bugsink: <root cause>.` | 502, downstream | 502 |

New health check message: `The browser URL must start with http:// or https://.` (status error). All M1 health messages are unchanged.

## Review Focus

1. A resource call must never answer 401 to the browser: Grafana's frontend treats a 401 as an expired session. Bugsink's 401 becomes 502 (Task 4 test).
2. A dashboard range in the past: issues last seen after `to` are skipped, paging continues to the range start, and nothing newer than `to` is returned (Task 3 test).
3. A Bugsink that keeps returning `next`: the read stops at 20 pages with the notice, and a repeated cursor is an error, not a loop (Task 1 tests).
4. A saved query whose project is gone or not the token's: the panel shows Bugsink's own sentence, and the editor shows `Project <id>` instead of an empty picker (Task 3 and Task 5 tests).
5. `q` with spaces, quotes, `&` and `#` reaches Bugsink as one `q` parameter, unchanged (Task 1 test); a cached result is never served to a query with a different filter (Task 3 test).

## File structure

| File | Responsibility |
| :--- | :--- |
| `pkg/bugsink/issues.go` (new) | `Issue`, `IssueFilter`, `ListIssues` |
| `pkg/bugsink/client.go` | shared paging loop, `BaseURL` |
| `pkg/bugsink/errors.go` | field errors of a 400 become `Detail` |
| `pkg/plugin/datasource.go` | instance: settings, client, cache, `Dispose`, `CheckHealth` |
| `pkg/plugin/cache.go` (new) | time-limited cache |
| `pkg/plugin/messages.go` (new) | the message contract as one function |
| `pkg/plugin/query.go` (new) | `QueryData`, query model, issue fetch through the cache |
| `pkg/plugin/frames.go` (new) | issues to frame |
| `pkg/plugin/resources.go` (new) | `CallResource`, `GET projects` |
| `src/datasource/types.ts`, `datasource.ts`, `module.ts` | query model, defaults, template variables, project list |
| `src/datasource/components/QueryEditor.tsx` (new) | query editor |
| `src/datasource/components/ConfigEditor.tsx` | Browser URL field |
| `docker/grafana/provisioning/dashboards/` (new) | dashboard provider and the sample dashboard |
| `tests/issues-query.spec.ts` (new) | the demo as end-to-end tests |

---

### Task 1: List issues in the Bugsink client

**Files:**
- Create: `pkg/bugsink/issues.go`, `pkg/bugsink/issues_test.go`
- Modify: `pkg/bugsink/client.go`, `pkg/bugsink/errors.go`, `pkg/bugsink/client_test.go`

**Interfaces:**
- Consumes: `gen.Client.IssuesList(ctx, *gen.IssuesListParams)`, `gen.PaginatedIssueList`, `gen.Issue`; the existing `decodeJSON`, `cursorOf`.
- Produces:

```go
type IssueState string // StateAll "all", StateOpen "open", StateUnresolved "unresolved", StateResolved "resolved", StateMuted "muted"
type IssueSort string  // SortLastSeen "last_seen", SortEventCount "digested_event_count", SortDigestOrder "digest_order"
type Order string      // Asc "asc", Desc "desc"

type IssueFilter struct {
	ProjectID int // 0 lists every project the token can see
	State     IssueState
	Query     string
	Sort      IssueSort
	Order     Order
}

type Issue struct {
	ID, FriendlyID                       string
	ProjectID                            int
	Type, Value, Transaction             string
	FirstSeen, LastSeen                  time.Time
	DigestedEventCount, StoredEventCount int
	IsResolved, IsResolvedByNextRelease  bool
	IsMuted                              bool
	UnmuteAfter                          *time.Time
}

type IssueList struct {
	Issues    []Issue
	SeenSince time.Time // every issue last seen at or after it is in Issues; zero when the list is not bounded by time
	Truncated bool      // the page limit was reached before the list ended
}

const MaxPages = 20

// ListIssues pages through the issue list; sorted by last_seen descending it stops at the first issue last seen before seenSince.
func (c *Client) ListIssues(ctx context.Context, f IssueFilter, seenSince time.Time) (IssueList, error)

// BaseURL is the Bugsink address without the API path or a trailing slash.
func (c *Client) BaseURL() string
```

- [ ] **Step 1: Write failing tests** in `issues_test.go` against an `httptest.Server`, in the style of `client_test.go`:
  - `TestListIssuesSendsFilter`: filter `{ProjectID: 1, State: StateUnresolved, Query: `environment:prod "a b" &x=1 #y`, Sort: SortLastSeen, Order: Desc}` produces a request whose parsed query is exactly `project=1`, `state=unresolved`, `sort=last_seen`, `order=desc` and `q` equal to the input string; with `ProjectID: 0` and `Query: ""` the request has no `project` and no `q` parameter.
  - `TestListIssuesMapsFields`: one result copied from the verified issue above maps to every `Issue` field; `unmute_after: null` gives a nil `UnmuteAfter`, a date-time gives its value.
  - `TestListIssuesFollowsCursor`: two pages; the second request carries the `cursor` of the first page's `next` and the same filter parameters; all issues return in order; `Truncated` is false.
  - `TestListIssuesStopsAtSeenSince`: `SortLastSeen`/`Desc`, pages of issues with falling `last_seen`, `seenSince` in the middle of page 1: only issues at or after `seenSince` return, page 2 is never requested, `SeenSince` equals the argument.
  - `TestListIssuesReadsEverythingForOtherSorts`: with `SortEventCount` (and with `SortLastSeen`/`Asc`), `seenSince` set, every page is requested, no issue is dropped, `SeenSince` is zero.
  - `TestListIssuesStopsAtMaxPages`: a server that always returns a fresh `next`: exactly `MaxPages` requests, `Truncated` true, no error.
  - `TestListIssuesRepeatedCursor`: a `next` that repeats the cursor is an error wrapping `ErrNotBugsink`.
  - `TestAPIErrorFieldErrors` in `client_test.go`: a 400 with body `{"sort":["Must be 'digest_order', 'last_seen', or 'digested_event_count'."]}` gives `APIError{Status: 400, Detail: "sort: Must be 'digest_order', 'last_seen', or 'digested_event_count'."}`; several keys join with a space in key order; a body with `detail` behaves as before.
  - `TestBaseURL`: `New("http://h:8000/api/canonical/0/", …)` and `New("http://h:8000", …)` both give `http://h:8000`; `New("https://h/bugsink/", …)` gives `https://h/bugsink`.
- [ ] **Step 2: Run** `go test ./pkg/bugsink/...`. Expected: FAIL, undefined `ListIssues`, `IssueFilter`, `BaseURL`.
- [ ] **Step 3: Implement.** Move the paging loop of `ListProjects` into one unexported helper that both lists use (cursor from `next`, repeat detection, `MaxPages`, a per-page callback that can stop the loop); `ListProjects` keeps its behaviour and its tests. `ListIssues` sends `sort` and `order` always, and `project`, `state`, `q` only when set.
- [ ] **Step 4: Run** `go test ./pkg/bugsink/...` and `go vet ./pkg/...`. Expected: PASS.
- [ ] **Step 5: Check against the stack:** a throwaway `go run` or test with the seeded global read token and `IssueFilter{ProjectID: 1, Sort: SortLastSeen, Order: Desc}`, zero `seenSince`, returns 262 or more issues in two pages. Do not commit the throwaway.
- [ ] **Step 6: Commit** `Add issue listing to the Bugsink client` and push.

### Task 2: The instance owns its client, settings and cache

**Files:**
- Create: `pkg/plugin/cache.go`, `pkg/plugin/cache_test.go`
- Modify: `pkg/plugin/datasource.go`, `pkg/plugin/datasource_test.go`, `pkg/plugin/health.go`

**Interfaces:**
- Consumes: `bugsink.New`, `(*bugsink.Client).BaseURL`.
- Produces:

```go
// Datasource is one Bugsink data source instance.
type Datasource struct {
	client        *bugsink.Client // nil when settingsErr is set
	settingsErr   string          // the health check's message for invalid settings, or empty
	url           string          // the Bugsink URL as entered, for messages
	browserURL    string          // base of links to Bugsink's pages, no trailing slash
	healthTimeout time.Duration
	// plus the HTTP client and the caches
}

func NewDatasource(ctx context.Context, s backend.DataSourceInstanceSettings) (instancemgmt.Instance, error)
func (d *Datasource) Dispose()

const cacheTTL = 15 * time.Second
const cacheMaxEntries = 32

type cache[V any] struct{ /* mutex, entries, now func() time.Time */ }
func newCache[V any](ttl time.Duration, maxEntries int) *cache[V]
func (c *cache[V]) get(key string) (V, bool)
func (c *cache[V]) set(key string, value V)
```

- `NewDatasource` returns an error only when the SDK cannot build HTTP options; invalid user settings set `settingsErr` so the health check can explain them. `jsonData` is `{"browserUrl": string}`; an empty `browserUrl` gives `client.BaseURL()`; a value that is not `http://` or `https://` with a host sets `settingsErr` to `The browser URL must start with http:// or https://.`; a trailing slash is trimmed.
- `CheckHealth` returns `settingsErr` when set and otherwise runs the M1 probes on `d.client`. Its messages and their order of precedence do not change.
- `Dispose` closes the HTTP client's idle connections.

- [ ] **Step 1: Write failing tests.**
  - `cache_test.go` with an injected clock: a value is returned before `ttl` and not after; `set` on a full cache drops expired entries first and then the oldest; two keys do not see each other's values; concurrent `get`/`set` pass `go test -race`.
  - `datasource_test.go`: the existing M1 health tests keep passing through `NewDatasource` (settings in, message out); new cases: `browserUrl` empty gives `browserURL` equal to the base URL without the API path; `browserUrl: "http://localhost:8800/"` gives `http://localhost:8800`; `browserUrl: "javascript:alert(1)"` and `"localhost:8800"` give the browser URL health message; two health checks on one instance reuse one HTTP client.
- [ ] **Step 2: Run** `go test -race ./pkg/plugin/...`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test -race ./pkg/...` and `go vet ./pkg/...`. Expected: PASS.
- [ ] **Step 5: Commit** `Give the data source instance its client, settings and cache` and push.

### Task 3: The issues query

**Files:**
- Create: `pkg/plugin/query.go`, `pkg/plugin/query_test.go`, `pkg/plugin/frames.go`, `pkg/plugin/frames_test.go`, `pkg/plugin/messages.go`, `pkg/plugin/messages_test.go`
- Modify: `pkg/plugin/datasource.go` (interface assertion)

**Interfaces:**
- Consumes: Task 1's `ListIssues`, `IssueList`, `IssueFilter`; Task 2's `Datasource`, `cache`; `ListProjects`.
- Produces:

```go
func (d *Datasource) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error)

// issuesFrame builds the frame of the frame contract.
func issuesFrame(issues []bugsink.Issue, projectNames map[int]string, browserURL string, truncated bool) *data.Frame

// userMessage renders an error from the Bugsink client as the message contract's text and status.
func userMessage(err error) (message string, status backend.Status)

// projects returns the project list through the cache; Task 4 uses it.
func (d *Datasource) projects(ctx context.Context) ([]bugsink.Project, error)
```

- Each query in the request is answered on its own: one failing query does not fail the others.
- The cache key of an issue list is the `IssueFilter`. A cached `IssueList` answers a query when its `SeenSince` is zero or not after the query's `from`; otherwise the list is fetched with `seenSince = from` and stored. Rows are then filtered to `from <= last_seen <= to`.
- Project names come from `d.projects`; when that fails, the `project` field holds the id as text and the query still succeeds.

- [ ] **Step 1: Write failing tests**, with `Datasource` built by `NewDatasource` against an `httptest.Server` that serves `projects/` and `issues/`:
  - `frames_test.go` `TestIssuesFrame`: field names, display names, types and order equal the frame contract; values of one resolved, one muted and one open issue, including `status`, a nil and a set `unmute_after`, and the project name; the link on `friendly_id` has title `Open in Bugsink`, URL `http://b/issues/issue/${__data.fields.id}/event/last/` and `TargetBlank`; no other field has a link; `truncated` adds the notice with the contract's text; no issues gives all fields with length 0.
  - `messages_test.go`: one case per row of the message contract.
  - `query_test.go`:
    - `TestQueryDefaults`: model `{}` requests `state=unresolved&sort=last_seen&order=desc` and no `project`.
    - `TestQueryModel`: `{"project":1,"state":"muted","q":"x","sort":"digested_event_count","order":"asc"}` reaches Bugsink unchanged.
    - `TestQueryRejectsUnknownValues`: `state: "bogus"` gives `Unknown state "bogus".` with status 400 and no request to Bugsink; the same for `sort`, `order` and `queryType`.
    - `TestQueryTimeRange`: issues last seen before `from`, inside the range and after `to`: only the inside ones are rows.
    - `TestQueryPastRangeKeepsPaging`: page 1 holds only issues newer than `to`, page 2 holds issues in range: page 2 is requested and its issues are the rows.
    - `TestQueryCache`: the same query twice within the TTL makes one issue list request; a later query with a later `from` makes none; an earlier `from` makes one; a query that differs in any one of project, state, `q`, sort or order makes one.
    - `TestQueryErrors`: Bugsink 403 with a capability, 403 `This token is not allowed to access this project.`, 404 `No Project matches the given query.` and a closed server give the contract's message and status, with error source downstream; the response never contains the token.
    - `TestQuerySettingsError`: an instance with invalid settings answers every query with its settings message and status 400.
    - `TestQueryProjectsUnreadable`: `projects/` answers 403: rows carry the project id as text.
    - `TestQueryTwoQueries`: one valid and one invalid query in a request: the valid one has its frame, the invalid one its error.
- [ ] **Step 2: Run** `go test -race ./pkg/plugin/...`. Expected: FAIL.
- [ ] **Step 3: Implement.** `var _ backend.QueryDataHandler = (*Datasource)(nil)`.
- [ ] **Step 4: Run** `go test -race ./pkg/...`, `go vet ./pkg/...`. Expected: PASS.
- [ ] **Step 5: Check in the stack:** `npm run build:backend`, `docker compose restart grafana`, then `POST http://localhost:3300/api/ds/query` with one query on data source uid `bugsink` (`{"queries":[{"refId":"A","datasource":{"uid":"bugsink"},"project":1,"state":"all"}],"from":"now-30d","to":"now"}`) returns a frame with the contract's fields and rows.
- [ ] **Step 6: Commit** `Add the issues query` and push.

### Task 4: The project list resource

**Files:**
- Create: `pkg/plugin/resources.go`, `pkg/plugin/resources_test.go`
- Modify: `pkg/plugin/datasource.go` (interface assertion)

**Interfaces:**
- Consumes: `d.projects`, `userMessage`.
- Produces: `func (d *Datasource) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error`, serving `GET projects` as `200 [{"id":1,"name":"sample-python"}]` (an empty list is `[]`, not `null`), errors as `{"message": …}` with the message contract's resource status. Any other path is 404; any other method on `projects` is 405.

- [ ] **Step 1: Write failing tests:** the list as JSON in Bugsink's order; an empty list is `[]`; Bugsink 401 gives status 502 and `Bugsink rejected the API token.`; Bugsink 403 with a capability gives 502 and `The API token lacks projects:read.`; invalid settings give 400 and the settings message; `GET other` is 404; `POST projects` is 405; two calls within the TTL make one request to Bugsink.
- [ ] **Step 2: Run** `go test -race ./pkg/plugin/...`. Expected: FAIL.
- [ ] **Step 3: Implement** with `httpadapter.New` over an `http.ServeMux`.
- [ ] **Step 4: Run** `go test -race ./pkg/...`. Expected: PASS.
- [ ] **Step 5: Check in the stack:** rebuild, restart Grafana, `GET http://localhost:3300/api/datasources/uid/bugsink/resources/projects` returns both seeded projects.
- [ ] **Step 6: Commit** `Add the project list resource` and push.

### Task 5: Query editor and the Browser URL setting

**Files:**
- Create: `src/datasource/components/QueryEditor.tsx`, `src/datasource/components/QueryEditor.test.tsx`, `src/datasource/datasource.test.ts`
- Modify: `src/datasource/types.ts`, `src/datasource/datasource.ts`, `src/datasource/module.ts`, `src/datasource/components/ConfigEditor.tsx`, `src/datasource/components/ConfigEditor.test.tsx`

**Interfaces:**
- Consumes: the query model and `GET resources/projects` (Task 4).
- Produces:

```ts
export type IssueState = 'all' | 'open' | 'unresolved' | 'resolved' | 'muted';
export type IssueSort = 'last_seen' | 'digested_event_count' | 'digest_order';
export type SortOrder = 'asc' | 'desc';
export interface BugsinkQuery extends DataQuery {
  project?: number;
  state?: IssueState;
  q?: string;
  sort?: IssueSort;
  order?: SortOrder;
}
export const DEFAULT_QUERY: Partial<BugsinkQuery> = { queryType: 'issues', state: 'unresolved', sort: 'last_seen', order: 'desc' };
export interface BugsinkOptions extends DataSourceJsonData { browserUrl?: string }
export interface Project { id: number; name: string }

// in BugsinkDataSource
getDefaultQuery(): Partial<BugsinkQuery>
applyTemplateVariables(query: BugsinkQuery, scopedVars: ScopedVars): BugsinkQuery // interpolates q only
getProjects(): Promise<Project[]>
```

- Query editor controls, by accessible name: `Project` (`Combobox`: `All projects`, then one option per project; a saved id that is not in the list shows as `Project <id>`), `State` (All, Open, Unresolved, Resolved, Muted), `Search` (text input, placeholder `environment:production release:1.4.2`; the query changes and runs on blur and on Enter, not on each key), `Sort by` (Last seen, Event count, and Arrival order only while a project is chosen), `Order` (Descending, Ascending). Every picker change calls `onChange` and `onRunQuery`. Choosing `All projects` while the sort is `digest_order` sets the sort to `last_seen`. When the project list fails to load, the editor shows the error's message as text and the other controls keep working.
- Config editor: field `Browser URL`, input id `bugsink-browser-url`, placeholder `https://bugsink.example.com`, description `The address of Bugsink as your browser reaches it, used for links to issues. Leave empty when it is the same as the Bugsink URL.`, bound to `jsonData.browserUrl`.

- [ ] **Step 1: Write failing tests.**
  - `datasource.test.ts`: `getDefaultQuery()` equals `DEFAULT_QUERY`; `applyTemplateVariables` replaces `$env` in `q` through the template service and leaves `project`, `state`, `sort`, `order` as they are; `getProjects` reads `projects` through `getResource`.
  - `QueryEditor.test.tsx` with a stub data source whose `getProjects` resolves `[{id: 1, name: 'sample-python'}, {id: 2, name: 'sample-js'}]`: the five controls render with their names and the defaults' labels; picking `sample-js` calls `onChange` with `project: 2` and runs the query; picking `All projects` removes `project`; typing in `Search` calls nothing until blur, then `onChange` with `q` and `onRunQuery` once; Enter does the same; `Arrival order` is absent without a project and present with one; `{project: 2, sort: 'digest_order'}` then `All projects` gives `sort: 'last_seen'`; `project: 99` shows `Project 99`; a rejected `getProjects` with message `Bugsink rejected the API token.` shows that text.
  - `ConfigEditor.test.tsx`: `Browser URL` shows `jsonData.browserUrl`; typing calls `onOptionsChange` with the new `jsonData.browserUrl` and the other options unchanged.
- [ ] **Step 2: Run** `npm run test:ci`. Expected: FAIL.
- [ ] **Step 3: Implement,** and register the editor with `.setQueryEditor(QueryEditor)`. Read the current `Combobox` and query editor documentation on grafana.com first (`.config/AGENTS/instructions.md`).
- [ ] **Step 4: Run** `npm run test:ci`, `npm run typecheck`, `npm run lint`. Expected: PASS, and Jest exits by itself.
- [ ] **Step 5: Commit** `Add the issues query editor and the Browser URL setting` and push.

### Task 6: Sample dashboard and end-to-end tests of the demo

**Files:**
- Create: `docker/grafana/provisioning/dashboards/dashboards.yaml`, `docker/grafana/provisioning/dashboards/bugsink-issues.json`, `tests/issues-query.spec.ts`
- Modify: `docker/grafana/provisioning/datasources/datasources.yaml`, `playwright.config.ts`, `tests/seeded.ts`, `tests/datasource-config.spec.ts`, `docker/README.md`

**Interfaces:**
- Consumes: everything above, built into `dist/`.
- Produces: provisioned data source `Bugsink` with `jsonData.browserUrl: http://localhost:8800`; dashboard uid `bugsink-issues`, title `Bugsink issues`, time range `now-24h` to `now`, one table panel `Unresolved issues in sample-python` with the query `{project: 1, state: 'unresolved', q: 'environment:development', sort: 'last_seen', order: 'desc'}`; `tests/seeded.ts` exports `SAMPLE_APP_URL` (`process.env.SAMPLE_APP_URL || 'http://localhost:8810'`) and `BUGSINK_BROWSER_URL` (`http://localhost:8800`); Playwright option `provisioningRootDir: 'docker/grafana/provisioning'`.

- [ ] **Step 1: Provision.** Add the dashboard provider (path `/etc/grafana/provisioning/dashboards`, already inside the mounted directory) and the dashboard JSON; add `browserUrl` to the data source. `docker compose up -d --wait`, `docker compose restart grafana`, `python3 scripts/verify_stack.py` prints six `ok` lines.
- [ ] **Step 2: Write the tests** in `tests/issues-query.spec.ts` (read `.config/AGENTS/e2e-testing.md` first). A `beforeAll` calls `GET ${SAMPLE_APP_URL}/handled?variant=m2-e2e` once, so every run reports one event to the same issue. Assertions that wait for Bugsink to digest the event retry with `expect(...).toPass()`.
  - `sample dashboard lists the issue`: open the provisioned dashboard (`readProvisionedDashboard({ fileName: 'bugsink-issues.json' })`, `gotoDashboardPage`); the panel shows a row whose message contains `m2-e2e`; the row's issue link has an `href` matching `^http://localhost:8800/issues/issue/[0-9a-f-]{36}/event/last/$`.
  - `query editor in a new panel`: `panelEditPage`, data source `Bugsink`, table visualization, time range last 24 hours; choose project `sample-python`, type `m2-e2e` in `Search` and press Enter; the panel data contains `m2-e2e`. Then search `environment:nosuchenv`: the panel has no data and no error.
  - `project picker lists the token's projects`: in the same panel editor, the `Project` picker offers `All projects`, `sample-python` and `sample-js`.
  - `the same query in Explore`: `explorePage`, data source `Bugsink`, the same project and search, run; the result table contains `m2-e2e`.
  - `a refused query explains itself`: open a panel whose saved query has `project: 99` (set through the panel's query JSON or a second panel on the sample dashboard); the panel error contains `No Project matches the given query.`
  - In `tests/datasource-config.spec.ts`: `invalid browser URL`: valid URL and token, `Browser URL` `localhost:8800`, save and test: error alert `The browser URL must start with http:// or https://.`
- [ ] **Step 3: Run** `npm run build && npm run build:backend && docker compose restart grafana && npm run e2e`. Expected: every test passes, twice in a row (the second run meets an existing issue).
- [ ] **Step 4: Run** `npm run lint`, `npm run typecheck`, `scripts/check_spdx.sh`. Expected: PASS.
- [ ] **Step 5:** Describe the dashboard in `docker/README.md` (what it shows, that it needs a fresh event: `curl http://localhost:8810/handled`).
- [ ] **Step 6: Commit** `Provision a sample dashboard and test the issues query end to end` and push.

### Task 7: Documentation and changelog

**Files:**
- Modify: `src/README.md`, `CHANGELOG.md`, `README.md` (only if a development command changed)

- [ ] **Step 1: Write** in `src/README.md`: a section "Querying issues" (the five controls and what each sends; the table's columns; the link to Bugsink; that `q` takes dashboard variables), and the limits a user meets: rows are issues last seen within the dashboard time range; a query reads at most 5000 issues and says so when it stops there; results can be up to 15 seconds old; `Arrival order` needs a project. In the configuration section: `Browser URL`, with the Docker case as the example. In `CHANGELOG.md` under 0.1.0: the issues query, the query editor, the Browser URL setting.
- [ ] **Step 2: Run** `scripts/check_spdx.sh`, `scripts/check_licenses.sh`, `npm run lint`. Expected: PASS.
- [ ] **Step 3: Commit** `Document the issues query` and push.

## After the last task

* Whole-branch review against this plan and spec §4.B, then one fix wave.
* From the worktree: `npm ci`, `npm run build`, `npm run build:backend`, `go test -race ./pkg/...`, `npm run test:ci`, `npm run e2e`, `python3 scripts/verify_stack.py`.
* Update spec §4.A (Browser URL) and §4.B (time range rule, page limit) and the roadmap's M2 entry to what was built, and add an `## As built` section here.
* The merge into `main` happens only when the owner asks for it.

## As built

1. All seven tasks ran as planned on `m2/issues`, each with its own review, then one whole-branch review (ready to merge, no critical or important findings) and one fix wave.
2. `NewDatasource` never returns an error: every settings problem, including a failure to build the HTTP client, becomes the health check's message.
3. A browser URL that carries credentials, a query string or a fragment is invalid, with the same message as a wrong scheme.
4. A list cut off at the page limit is not cached: its `SeenSince` is not a guarantee then.
5. `MaxPages` also bounds the project list, which returns what it has at the limit.
6. Field errors of a Bugsink 400 (`{"sort": ["…"]}`) become the error's detail, so a refused parameter is named in the panel.
7. Message contract changes from the final review: a Bugsink 401 on a query has status 502, not 401; timeouts and cancellations say `Bugsink did not answer in time.`; a `project` that is not a number says `The project must be a number.`
8. A saved `project` of `0` or `null` means all projects, in the backend and in the editor.
9. The cache drops expired entries on every write.
10. The project picker lists the projects by name. The `id` column is visible in tables and documented.
11. The end-to-end tests report one event per run from a Playwright setup project (`tests/report-event.setup.ts`), and the refused-query test has its own provisioned dashboard (`bugsink-e2e-refused.json`).
12. Left for later milestones: failed project reads are not cached (M3); the resource mux is rebuilt per call and its 404/405 bodies are untested (M4); a project dashboard variable; merging concurrent identical cache misses.
