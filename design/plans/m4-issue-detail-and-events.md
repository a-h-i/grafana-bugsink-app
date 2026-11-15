# M4: Issue Detail and Events Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A user clicks an issue in the app's list and sees its summary, tag values, history and events, and goes back to the same list; on a dashboard, an events query lists the events of one issue, one SDK event id or one trace.

**Architecture:** `pkg/bugsink` gains reads for one issue, its tags, its history, its events and an event lookup. The data source serves the three issue reads as resource routes from a router built once per instance, and answers a new `events` query type in `QueryData`, which for `trace:<id>` searches the issues first and then reads their events. The app gets routes: the M3 list at the root and `issues/:issueId` for the detail page, which reads the three routes and runs the events query.

**Tech Stack:** Go 1.26 with `grafana-plugin-sdk-go` v0.299.0; React 19, `@grafana/ui`/`@grafana/data`/`@grafana/runtime` 13.2.4, `react-router-dom` 6.30.6; Jest; `@grafana/plugin-e2e` 4.0.0; the Compose stack (Bugsink 2.6.3, Grafana 13.3.1). No new dependency (`github.com/google/uuid` moves from indirect to direct).

**Spec:** [`../bugsink_grafana_plugin_spec.md`](../bugsink_grafana_plugin_spec.md) (§3, §4.B extras, §4.C, §4.D). **Roadmap:** [`../implementation_roadmap.md`](../implementation_roadmap.md) (M4, Definition of done). **Previous plans:** [`m2-issues-in-dashboards.md`](m2-issues-in-dashboards.md) (frame and message contracts), [`m3-issue-list-page.md`](m3-issue-list-page.md) (URL and page contracts, As built).

## Global Constraints

* The API token never reaches the browser, a log line, an error message or a frame.
* The backend pages with the `cursor` taken from `next`; it never builds a cursor and never requests a URL taken from a response. It sends `sort` and `order` on every issue list request and `order=desc` on every event list request.
* Resources never answer 401: Bugsink failures are 502, invalid settings and invalid input 400, a refused caller 403. Resource errors are JSON `{"message": …}`.
* Reuse what exists instead of parallel code: `userMessage`, `cache[V]`, `d.projects`/`d.projectNames`, `listPages`, `issueOf`, the query model and `DEFAULT_QUERY`, `listState.ts`, `IssueTable`, `tests/fixtures.ts`, `tests/seeded.ts`, the `report-event` setup project.
* Pages show the backend's message text verbatim; they never rewrite a message of the M2 message contract.
* Generated code stays in `pkg/bugsink/internal/gen/` and is not regenerated. Do not modify anything in `.config/`. Do not change a plugin id or type; a `plugin.json` change needs a Grafana restart (this plan changes none).
* Every hand-written source file starts with `SPDX-License-Identifier: GPL-3.0-or-later` (JSON exempt); `scripts/check_spdx.sh` passes.
* Comments are one line and describe the code as it is. Tests assert behaviour, not code structure.
* Go commands run on `./pkg/...`: `node_modules` contains a Go package.
* Host ports stay `3300` (Grafana), `8800` (Bugsink), `8810` (sample app).
* End-to-end tests produce at most 5 Bugsink events per run; this milestone adds one (`/handled?variant=m4-trace`), so a run produces two.
* Work on branch `m4/issue-detail` in the worktree `.claude/worktrees/m4-issue-detail` (created from `main` at a441190, `node_modules` installed). Every implementer uses this same branch and worktree; push after every commit; never amend.
* The running stack may mount another checkout's `dist/` and provisioning. Before the first stack check, run `docker compose up -d --wait` from the worktree (Compose project `bugsink-grafana-plugin`; data volumes are kept). Never `docker compose down`.
* Run every test command in the foreground.

## Facts already verified (2026-11-15, Compose stack, tokens `a`/`c`/`d`×40)

* `GET issues/{id}/` takes the uuid or the short id (`SAMPLE-PYTHON-2`) and returns the M2 issue object. Refusals: project-bound token on a project-2 issue 403 `This token is not allowed to access this project.`; no-capability token 403 `…capability: issues:read.`; unknown uuid 404 `{"detail":"No Issue matches the given query."}`; `nosuch` 400 with body `["Invalid issue identifier."]`, a JSON array, which `newAPIError` today turns into `Bugsink answered HTTP 400.`. `issues/{id}/tags/`, `issue-history/?issue=` and `events/?issue=` refuse the same way (tags and events name `events:read`).
* `issues/{id}/tags/` is `{next, previous, results: [{id, key, value, count}]}`, ordered by id, not by count, and includes Bugsink's "mostly unique" keys: `SAMPLE-PYTHON-263` (135 events) has a full first page of 250 values, 243 of them `trace`, `trace.span`, `trace.ctx` with count 1, and `next` set. The global token gets `results: []` for the project-2 issue.
* `issue-history/?issue=` is newest first; items `{id, issue, project, triggering_event, user, user_display_name, timestamp, kind, comment, metadata}`. On throwaway `SAMPLE-PYTHON-264` with the triage token: mute gave `kind: muted, metadata: {mute_unconditionally: true}`, unmute `unmuted`, a comment `manual_annotation` with its text; all with `user` and `user_display_name` null. The issue was left unmuted and unresolved. No `issue` is 400 `{"issue":["This field is required."]}`.
* `events/?issue=` items `{id, ingested_at, digested_at, issue, grouping, event_id, project, timestamp, digest_order}`; newest first by `digest_order`, `order=asc` reverses, `order=up` 400 `{"order":["Must be 'asc' or 'desc'."]}`, `cursor=garbage` 404 `Invalid cursor`. `q=environment:development` and `q=failure` match all 26 events of `SAMPLE-PYTHON-2`, `q=nomatchtext` none.
* Page size is 250 for events, tags and history (installed Bugsink source: `events/api_views.py:25`, `issues/api_views.py:41,421`). No issue in the stack has more than 250 events, so event paging is verified from the source and covered by fake-server tests only.
* `events/lookup/?project=&event_id=` returns the event with `data`; it accepts the dashed and the 32-hex form. Another project's event in project 1: 404 `No Event matches the given query.`; `event_id=notauuid` 404 `Not found.`; `project=99` 404 `No Project matches the given query.`; project-bound token with `project=2` 403.
* Trace: `GET :8810/handled?variant=m4-plan-probe` returned `{"trace_id": "64a4…b229", "event_id": "426ac644…1b"}` (hex). After the digest, `issues/?q=trace:<trace_id>&state=all&sort=last_seen&order=desc` returned exactly that issue, and `events/?issue=<it>&q=trace:<trace_id>` one event whose `event_id` is the dashed form. `issues/?q=trace:` (empty) is 200 with no results; the project-bound token with `project=2` gets 403.
* Bugsink's event page is `<base>/issues/issue/<issue uuid>/event/<event uuid>/` (302 to the login page; `issues/urls.py:40`).
* Grafana serves `/a/ahmedismail-bugsink-app/issues/SAMPLE-PYTHON-2` (200, the app root handles sub-paths). `GET /api/datasources/uid/bugsink/resources/issues/SAMPLE-PYTHON-2` reaches the plugin today and gets its `{"message":"Not found."}`.
* App routes are relative `<Route path="…">` elements under the root page; pages not in the nav need no `plugin.json` entry (grafana.com "Build an app plugin").
* `@grafana/runtime` 13.2.4 `PluginPage` takes `pageNav?: NavModelItem` and `actions`; `@grafana/ui` 13.2.4 exports `LinkButton`, `RadioButtonGroup`, `Stack`, `Text`, `InteractiveTable`, `EmptyState`, `TextLink`; `@grafana/data` exports `getDefaultTimeRange`; `react-router-dom` 6.30.6 has `Routes`, `Route`, `Link`, `useParams`. `@grafana/plugin-e2e` `gotoDashboardPage` takes `queryParams`.
* SDK: `httpadapter.New(http.Handler) backend.CallResourceHandler` builds the request URL from `req.Path` plus the query (`backend/resource/httpadapter/handler.go:38-52`).
* Generated client: `IssuesRetrieve(ctx, id string)`, `IssuesTagsList(ctx, id, *IssuesTagsListParams{Cursor, Key})`, `IssueHistoryList(ctx, *IssueHistoryListParams{Cursor, Issue, Order})`, `EventsList(ctx, *EventsListParams{Cursor, Issue, Order, Q})`, `EventsLookupRetrieve(ctx, *EventsLookupRetrieveParams{EventId openapi_types.UUID, Project int})`. `EventList.Id`/`EventId` are pointers; `TurningPoint` fields are all pointers, author in `UserDisplayName`. Path ids are escaped but `..` is not, so an id must be validated before it reaches the client.
* The health check already probes `issues:read` and `events:read` (`pkg/plugin/health.go:45-49`); M4 needs no other capability.
* **Not verified:** that `pageNav` makes the detail page's title and breadcrumb the issue's short id in Grafana 13.3.1. Task 7 Step 1 checks it first.

## Decisions in this plan, for review

1. **In the app, the short id opens the detail page; an icon beside it opens Bugsink.** The roadmap wants a click to open the detail and the spec wants a row link to Bugsink; both stay. Dashboard frames keep their link to Bugsink. The as-built step updates spec §4.D.
2. **The detail URL carries the list's query string** (`/a/<app>/issues/<id>?ds=…&state=…`). The detail page reads only `ds`; `Back to issues` returns to the root with the same string, so the list's state survives the visit, a reload and a shared link.
3. **Detail reads are resource routes and are not cached;** the event list on the page runs the events query, as the M3 list runs the issues query. M7 shows the state Bugsink returns, which a cached read would hide.
4. **Route shapes for later milestones:** `GET issues/{id}` returns the issue JSON that M7's `POST issues/{id}/<action>` routes also return; M9 adds `GET issues/{id}/event-counts`, M5 `GET events/{id}`. Lists are wrapped (`{"tags": […], "truncated": …}`) so a flag can be added without reshaping.
5. **Tags are grouped by key in the backend:** keys in Bugsink's order (first seen first), the 5 values with the highest count per key, and the number of other values. The trace keys alone fill pages; the backend reads at most 20 pages (5000 values) and says so.
6. **History shows kind, time, author and comment;** `metadata` and the triggering event are left for M8 and M5.
7. **Events query modes,** first match wins: `eventId` (lookup, needs a project), `issue` (that issue's events, filtered by `q`), no issue with a `trace:<id>` term in `q` (the fan-out). Anything else is a message. `state`, `sort` and `order` are ignored for events.
8. **The events query does not use the dashboard time range.** Bugsink's event list has no time filter and is in arrival order, so a range filter would read every page; an issue, an event id or a trace already scopes the list.
9. **Bounds:** one issue reads at most 4 pages (1000 events); the fan-out reads one page of matching issues (state `all`, last seen first), keeps the first 10, and reads one page (250 events) of each. Each bound reached adds a notice (see the frame contract). A failing request fails the query.
10. **Fan-out rows are grouped by issue** (last seen first), each issue's events newest first; no re-sort across issues (spec §4.C order rule). A table panel can sort by Timestamp.
11. **The query type picker** (`Issues`, `Events`) is in the panel and Explore editor only; the app's filter bar is the issues editor without it.
12. **`q`, `issue` and `eventId` take dashboard variables;** the demo dashboard uses a textbox variable `trace`.
13. **An issue id is a uuid or a short id, `^[A-Za-z0-9_-]{1,100}$`;** anything else is 400 `"<id>" is not an issue id.` before any request.
14. **The refresh button retries a failed data source load** (deferred from M3), through one `useDataSource` hook that both pages use.

## Resource routes

| Route | 200 body |
| :--- | :--- |
| `GET projects` | unchanged (M2) |
| `GET issues/{id}` | `{"id","friendlyId","projectId","project","type","message","transaction","status","firstSeen","lastSeen","events","storedEvents","isResolved","isResolvedByNextRelease","isMuted","unmuteAfter","bugsinkUrl"}`; `project` is the name or the id as text, `status` as in the M2 frame, times RFC 3339, `unmuteAfter` null or a time, `bugsinkUrl` `<browser URL>/issues/issue/<uuid>/event/last/` |
| `GET issues/{id}/tags` | `{"tags":[{"key":"environment","values":[{"value":"development","count":26}],"otherValues":0}],"truncated":false}`; values by count descending, then value; `tags` is `[]` when empty |
| `GET issues/{id}/history` | `{"history":[{"id":276,"kind":"manual_annotation","timestamp":"…","user":null,"comment":"m4 plan probe"}],"truncated":false}`; newest first; `user` is `user_display_name` |

Errors: invalid settings 400 with the settings message; invalid `{id}` 400 `"<id>" is not an issue id.`; any Bugsink or network failure 502 with `userMessage`'s text; an unknown path 404 `Not found.`; another method on a known path 405 `Method not allowed.` with `Allow: GET`.

## Events query

Query model additions (M2 table otherwise unchanged): `queryType` `issues` | `events`; `issue` (text, uuid or short id); `eventId` (text, dashed or 32-hex uuid). `project` and `q` are shared.

| Condition | Message (status 400, downstream) |
| :--- | :--- |
| `issue` set and not an issue id | `"<issue>" is not an issue id.` |
| `eventId` set and not a uuid | `The event ID must be a UUID.` |
| `eventId` set, no project | `Looking up an event ID needs a project.` |
| no `eventId`, no `issue`, no `trace:<id>` term in `q` | `An events query needs an issue, an event ID or trace:<id> in Search.` |

Frame `events`, `Meta.PreferredVisualization` table, fields in this order; no rows keeps every field:

| Field name | Display name | Type | Source |
| :--- | :--- | :--- | :--- |
| `timestamp` | Timestamp | time | `timestamp` |
| `event_id` | Event ID | string | `event_id`; carries the link |
| `issue` | Issue | string | the issue's short id |
| `project` | Project | string | project name, or the id as text |
| `ingested_at` | Ingested at | time | `ingested_at` |
| `id` | (none) | string | Bugsink's event id |
| `issue_id` | (none) | string | `issue` |
| `project_id` | Project ID | int64 | `project` |

* Link on `event_id`: title `Open in Bugsink`, URL `<browser URL>/issues/issue/${__data.fields.issue_id}/event/${__data.fields.id}/`, new tab.
* Notices, severity warning: `Only the newest 1000 events were read.`; `This trace has events in more than 10 issues. Only the 10 last seen were read.`; `Only the newest 250 events of <short id> in this trace were read.` (one per issue cut).

## Detail page contract

Route `issues/:issueId` under the app root. `PluginPage` with `pageNav` `{ text: <short id, or issueId until the issue loads> }`; actions: `Back to issues` (`Button`, icon `arrow-left`, navigates to the root with the same query string), `Open in Bugsink` (`LinkButton`, external, `target="_blank"`, `bugsinkUrl`, shown once the issue loads), `RefreshPicker` (`noIntervalPicker`; reloads every section and retries a failed data source load). Times use the user's time zone, `YYYY-MM-DD HH:mm:ss`. No data source, unknown `ds` and a failed data source load show the list page's messages.

| Section | Loading | Failed | Empty | Content |
| :--- | :--- | :--- | :--- | :--- |
| Summary | `Loading issue...` | `Alert` error, the message | n/a | heading `<type>: <message>` (type alone when the message is empty); rows `Status` (`Open`/`Muted`/`Resolved`), `Project`, `Transaction`, `Events`, `Stored events`, `First seen`, `Last seen`, `Unmute after` (only when set), `Resolved by next release` `Yes` (only when true) |
| Tags | `Loading tags...` | same | `No tags.` | one row per key: `Tag`, `Values` as `development (26), staging (3)` then ` and 4 more values` (` and 1 more value`); truncated: warning `Only the first 5000 tag values were read.` |
| History | `Loading history...` | same | `No history.` | per entry: time, label, ` by <user>` when set, the comment below when not empty; labels `First seen`, `Resolved`, `Muted`, `Regressed`, `Unmuted`, `Reopened`, `Resolved in the next release` (`next_materialized`), `Comment` (`manual_annotation`), any other kind as written; truncated: warning `Only the newest 5000 history entries were read.` |
| Events | `Loading events...` | same | `No stored events.` | columns `Event ID` (external link to the row's url), `Timestamp`, `Ingested at`; Bugsink's order; 25 rows per page; each notice as a warning above |

Each section loads and fails on its own. In the list's `Issue` column, the short id is a link to `issues/<uuid><list query string>` in the same tab, followed by an icon `LinkButton` with accessible name `Open <short id> in Bugsink` (new tab), absent when the row has no url.

## Review Focus

1. An issue id that is not one (`..`, `a/b`, `../projects`, empty) in the URL or a saved query never reaches another Bugsink endpoint: 400 `"<id>" is not an issue id.` without a request (Task 2 and Task 3 tests).
2. A token without `events:read`, or a project-bound token on another project's issue: each section shows its own Bugsink sentence, and summary and history still render when only `events:read` is missing (Task 6 test).
3. A trace across many issues, or an issue with thousands of events: the reads stop at the bounds of decision 9 and the notices say so (Task 3 tests).
4. Back from the detail page restores the list exactly, including parameters the page does not own (`orgId`, `kiosk`) (Task 5 test).
5. A Bugsink 400 whose body is a JSON array (`["Invalid issue identifier."]`) shows Bugsink's sentence, not `Bugsink answered HTTP 400.` (Task 1 test).

## File structure

| File | Responsibility |
| :--- | :--- |
| `pkg/bugsink/client.go`, `errors.go`, `issues.go` | `listPages` page limit as a parameter, list bodies as detail, `GetIssue`, `FirstIssues` |
| `pkg/bugsink/details.go` (new) | tags, history |
| `pkg/bugsink/events.go` (new) | event list, lookup |
| `pkg/plugin/health.go` | prober uses the new `GetIssue` |
| `pkg/plugin/resources.go`, `datasource.go` | router built once, the three issue routes |
| `pkg/plugin/events.go` (new), `query.go`, `frames.go` | events query, fan-out, events frame |
| `src/datasource/types.ts`, `datasource.ts`, `issueRows.ts`, `eventRows.ts` (new) | detail types, resource reads, `listEvents`, typed rows |
| `src/datasource/components/` | `QueryEditor` (type picker), `IssuesQueryEditor`, `EventsQueryEditor`, `ProjectPicker`, `CommitInput` |
| `src/pages/App.tsx`, `routes.ts`, `useDataSource.ts`, `NoDataSource.tsx` (new), `src/module.tsx` | routes, paths, instance choice |
| `src/pages/IssuesPage.tsx`, `issues/IssueTable.tsx` | hook, link to the detail page |
| `src/pages/IssuePage.tsx`, `src/pages/issue/` (new) | the detail page and its sections |
| `docker/grafana/provisioning/dashboards/bugsink-trace.json` (new) | the trace demo dashboard |
| `tests/issue-detail.spec.ts`, `tests/events-query.spec.ts` (new) | the demo end to end |

---

### Task 1: Detail and event reads in the Bugsink client

**Files:**
- Create: `pkg/bugsink/details.go`, `pkg/bugsink/details_test.go`, `pkg/bugsink/events.go`, `pkg/bugsink/events_test.go`
- Modify: `pkg/bugsink/client.go`, `pkg/bugsink/errors.go`, `pkg/bugsink/issues.go`, `pkg/bugsink/client_test.go`, `pkg/bugsink/issues_test.go`, `pkg/plugin/health.go`, `pkg/plugin/health_test.go`, `pkg/plugin/frames.go`, `go.mod`, `go.sum`

**Interfaces:**
- Consumes: the generated operations in Facts; `listPages`, `decodeJSON`, `issueOf`, `deref`.
- Produces:

```go
const PageSize = 250 // results per page of every Bugsink list
func listPages[T any](ctx context.Context, maxPages int, fetch …, onPage …) (truncated bool, err error) // ListProjects, ListIssues pass MaxPages

func (c *Client) GetIssue(ctx context.Context, id string) (Issue, error) // replaces the probe-only GetIssue
func (c *Client) FirstIssues(ctx context.Context, f IssueFilter, n int) (issues []Issue, more bool, err error) // one page; more when it held over n or had next

type IssueTag struct{ Key, Value string; Count int }
type TagList struct{ Tags []IssueTag; Truncated bool }
func (c *Client) ListIssueTags(ctx context.Context, issueID string) (TagList, error) // MaxPages

type TurningPoint struct{ ID int; Kind string; Timestamp time.Time; User, Comment string } // User is user_display_name, empty when null
type History struct{ Entries []TurningPoint; Truncated bool }
func (c *Client) ListIssueHistory(ctx context.Context, issueID string) (History, error) // newest first, MaxPages

type Event struct{ ID, EventID, IssueID string; ProjectID int; Timestamp, IngestedAt time.Time }
type EventList struct{ Events []Event; Truncated bool }
func (c *Client) ListEvents(ctx context.Context, issueID, query string, maxPages int) (EventList, error) // sends order=desc, q only when set
func (c *Client) LookupEvent(ctx context.Context, projectID int, eventID uuid.UUID) (Event, error)
```

- [ ] **Step 1: Write failing tests** against an `httptest.Server`, in the style of `issues_test.go`:
  - `TestAPIErrorListBody` (`client_test.go`): a 400 with body `["Invalid issue identifier."]` gives `APIError{Status: 400, Detail: "Invalid issue identifier."}`; two strings join with a space; `[1]` gives an empty detail.
  - `TestGetIssue`: the verified issue object maps to every `Issue` field; the request path is `/api/canonical/0/issues/SAMPLE-PYTHON-2/`; a 404 `No Issue matches the given query.` is an `*APIError` with that detail.
  - `TestFirstIssues`: a page of 12 issues and `n` 10 gives 10 issues and `more`; a page of 3 with `next` set gives 3 and `more`; a page of 3 without `next` gives 3, not `more`; exactly one request, carrying `q`, `state=all`, `sort`, `order`.
  - `TestListIssueTags`: two pages map to `IssueTag`s in order; the second request carries the first page's cursor; an endless server stops at `MaxPages` with `Truncated`.
  - `TestListIssueHistory`: `user_display_name: null` gives `User ""`, a name gives it; `comment` and `kind` copied; the request carries `issue=<id>`.
  - `TestListEvents`: the request has `issue`, `order=desc` and `q` exactly as given (with spaces and `&`); no `q` when empty; `maxPages` 4 against an endless server makes 4 requests and `Truncated`; fields map, including dashed `event_id`.
  - `TestLookupEvent`: the request carries `project=1` and the dashed `event_id`; the event's `data` is ignored; 404 `No Event matches the given query.` is an `*APIError`.
  - `health_test.go`: the fake prober's `GetIssue` returns `(bugsink.Issue, error)`; every health case keeps its message.
- [ ] **Step 2: Run** `go test ./pkg/...`. Expected: FAIL, undefined `ListEvents`, `ListIssueTags`, `FirstIssues`.
- [ ] **Step 3: Implement.** `detailOf`'s caller also accepts a JSON array of strings; `frames.go`'s `MaxPages*250` becomes `MaxPages*PageSize`; `go mod tidy` makes `github.com/google/uuid` direct.
- [ ] **Step 4: Run** `go test -race ./pkg/...`, `go vet ./pkg/...`. Expected: PASS, M2 and M3 tests unchanged.
- [ ] **Step 5: Commit** `Add issue detail and event reads to the Bugsink client` and push.

### Task 2: Resource router built once, with the issue routes

**Files:**
- Modify: `pkg/plugin/resources.go`, `pkg/plugin/resources_test.go`, `pkg/plugin/datasource.go`, `pkg/plugin/query_test.go` (fake Bugsink serves the detail endpoints)

**Interfaces:**
- Consumes: Task 1's `GetIssue`, `ListIssueTags`, `ListIssueHistory`; `d.projectNames`, `userMessage`, `statusOf`.
- Produces: `Datasource.resources backend.CallResourceHandler`, set in `NewDatasource` from `httpadapter.New(d.routes())`; `func (d *Datasource) routes() *http.ServeMux`; `func validIssueID(id string) bool` (`^[A-Za-z0-9_-]{1,100}$`); `func issueJSON(i bugsink.Issue, project, browserURL string) issueBody` (M7 reuses it); `const maxTagValues = 5`. The routes and bodies of the resource contract.

- [ ] **Step 1: Write failing tests** in `resources_test.go`:
  - `TestIssueResource`: `GET issues/SAMPLE-PYTHON-2` returns 200 with exactly the contract's keys, `project` `alpha`, `status` `open`, `unmuteAfter` null, `bugsinkUrl` `http://localhost:8800/issues/issue/<uuid>/event/last/` for browser URL `http://localhost:8800`.
  - `TestIssueTagsResource`: values `environment: development 26, staging 3`, `trace` with 7 values of count 1, `release` with 1 value, in that id order: keys `environment`, `trace`, `release`; `trace` has 5 values and `otherValues` 2; `environment`'s values are by count; a truncated tag list gives `"truncated": true`; no tags gives `"tags":[]`.
  - `TestIssueHistoryResource`: entries newest first; a null author is JSON `null`, a name is a string; no entries gives `"history":[]`.
  - `TestIssueRoutesRejectBadIDs`: `issues/a.b`, `issues/a.b/tags` and `issues/a%20b/history` answer 400 `"a.b" is not an issue id.` (and `"a b"`); `issues/..` and `issues/../projects` never answer 200; the fake Bugsink records no request in any case.
  - `TestIssueRoutesErrors`: Bugsink 403 `This token is not allowed to access this project.` gives 502 `Bugsink refused the request: This token is not allowed to access this project.`; 403 naming `events:read` on tags gives 502 `The API token lacks events:read.`; 401 gives 502 `Bugsink rejected the API token.`; invalid settings give 400; no body contains the token.
  - `TestResourceRouting` (extends M2's): `GET other` is 404 with body `{"message":"Not found."}`; `POST projects` and `POST issues/X-1` are 405 with `{"message":"Method not allowed."}` and `Allow: GET`; `GET issues/X-1/other` is 404.
  - `TestConcurrentResourceCalls`: 10 concurrent `GET issues/X-1` calls on one instance all answer 200 under `-race`.
  - `TestIssueRoutesNotCached`: two `GET issues/X-1` calls make two Bugsink requests.
- [ ] **Step 2: Run** `go test -race ./pkg/plugin/...`. Expected: FAIL.
- [ ] **Step 3: Implement.** One helper registers `GET <pattern>` and `<pattern>` (the 405 answer) for each route; the catch-all `/` answers 404.
- [ ] **Step 4: Run** `go test -race ./pkg/...`, `go vet ./pkg/...`. Expected: PASS.
- [ ] **Step 5: Check in the stack:** `npm run build:backend`, `docker compose up -d --wait`, `docker compose restart grafana`; `GET http://localhost:3300/api/datasources/uid/bugsink/resources/issues/SAMPLE-PYTHON-2/tags` lists `environment` with `development`; the same on uid `bugsink-project` for `SAMPLE-JS-1` answers 502 `Bugsink refused the request: This token is not allowed to access this project.`
- [ ] **Step 6: Commit** `Serve issue detail reads from a router built once per instance` and push.

### Task 3: The events query

**Files:**
- Create: `pkg/plugin/events.go`, `pkg/plugin/events_test.go`
- Modify: `pkg/plugin/query.go`, `pkg/plugin/frames.go`, `pkg/plugin/frames_test.go`, `pkg/plugin/datasource.go` (`eventCache`), `pkg/plugin/query_test.go`

**Interfaces:**
- Consumes: Task 1's `GetIssue`, `FirstIssues`, `ListEvents`, `LookupEvent`, `PageSize`; Task 2's `validIssueID`; `cache[V]`, `userMessage`, `d.projectNames`.
- Produces:

```go
const (queryTypeEvents = "events"; maxEventPages = 4; maxTraceIssues = 10; maxTraceEventPages = 1)
// eventsQuery is a validated events query; the first set of EventID (uuid.Nil when absent), Issue, trace term in Query decides the mode.
type eventsQuery struct{ ProjectID int; Issue, Query string; EventID uuid.UUID }
type eventRow struct{ event bugsink.Event; issue string } // issue is the short id
type eventsResult struct{ rows []eventRow; notices []string }

func parseEventsQuery(m queryModel, project int) (eventsQuery, string)
func (d *Datasource) events(ctx context.Context, q eventsQuery) (eventsResult, error) // through eventCache
func eventsFrame(rows []eventRow, projectNames map[int]string, browserURL string, notices []string) *data.Frame
```

- `queryModel` gains `Issue string \`json:"issue"\`` and `EventID string \`json:"eventId"\``. `query()` dispatches on the query type; `queryType` accepts `issues` and `events`; the issues path is unchanged. A trace term is `(^|\s)trace:\S`. The fan-out sends `IssueFilter{ProjectID, State: StateAll, Query: q, Sort: SortLastSeen, Order: Desc}` and the whole `q` to each event list.
- `eventCache *cache[eventsResult]` (TTL and size as the other caches), keyed by mode and every input; errors are not stored.

- [ ] **Step 1: Write failing tests** with the extended fake Bugsink:
  - `frames_test.go` `TestEventsFrame`: names, display names, types and order equal the frame contract; one row's values; the link on `event_id` has title `Open in Bugsink`, URL `http://b/issues/issue/${__data.fields.issue_id}/event/${__data.fields.id}/`, `TargetBlank`; no other field has a link; notices become warning notices in order; no rows keeps every field.
  - `events_test.go`:
    - `TestEventsQueryValidation`: one case per row of the events message table, each with status 400 and no Bugsink request; `issue: "../projects"` is the first row's message.
    - `TestEventsOfIssue`: `{queryType: "events", issue: "SAMPLE-PYTHON-2", q: "environment:development"}` requests `issues/SAMPLE-PYTHON-2/` once, then `events/?issue=<uuid>&q=environment%3Adevelopment&order=desc`; rows in the fake's order with `issue` `SAMPLE-PYTHON-2`; `state`, `sort`, `order` and the time range in the request change nothing.
    - `TestEventsOfIssueBound`: an endless event list makes 4 event requests; notice `Only the newest 1000 events were read.`
    - `TestEventLookup`: `{eventId: "426ac644fafb475d905046d654730f1b", project: 1}` requests `events/lookup/?project=1&event_id=426ac644-fafb-475d-9050-46d654730f1b`, then the event's issue; one row. Bugsink 404 `No Event matches the given query.` gives `Bugsink refused the request: No Event matches the given query.` with status 404.
    - `TestEventsOfTrace`: `{q: "trace:abc"}` with two matching issues makes one issue list request (`q=trace:abc`, `state=all`, `sort=last_seen`, `order=desc`) and one event request per issue with `q=trace:abc`; rows are the first issue's events, then the second's; no notice.
    - `TestEventsOfTraceBounds`: 12 matching issues: events of the first 10 only and the more-than-10 notice; an issue whose event page has `next`: its notice names its short id, and its second page is never requested.
    - `TestEventsOfTraceWithProject`: `project: 2` reaches the issue search as `project=2`; a 403 there is the query's error.
    - `TestEventsCache`: the same events query twice within the TTL makes requests once; a different `q` makes them again; a failed query is not stored.
    - `TestTwoQueryTypes`: an issues and an events query in one request each get their own frame.
- [ ] **Step 2: Run** `go test -race ./pkg/plugin/...`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test -race ./pkg/...`, `go vet ./pkg/...`. Expected: PASS.
- [ ] **Step 5: Check in the stack:** rebuild, restart Grafana; `POST /api/ds/query` with `{"queries":[{"refId":"A","datasource":{"uid":"bugsink"},"queryType":"events","issue":"SAMPLE-PYTHON-2"}],"from":"now-1h","to":"now"}` returns the events frame with 26 or more rows; with `"q":"trace:64a4aa9ecb93c8a06e40a1f9b986b229"` and no issue, one row for `SAMPLE-PYTHON-264`.
- [ ] **Step 6: Commit** `Add the events query` and push.

### Task 4: Events and issue details in the frontend data source and query editor

**Files:**
- Create: `src/datasource/eventRows.ts`, `src/datasource/eventRows.test.ts`, `src/datasource/components/IssuesQueryEditor.tsx` (moved from `QueryEditor.tsx` with `git mv`, and its test), `src/datasource/components/EventsQueryEditor.tsx`, `EventsQueryEditor.test.tsx`, `ProjectPicker.tsx`, `ProjectPicker.test.tsx`, `CommitInput.tsx`, `CommitInput.test.tsx`, `QueryEditor.tsx` and `QueryEditor.test.tsx` (new content)
- Modify: `src/datasource/types.ts`, `src/datasource/datasource.ts`, `src/datasource/datasource.test.ts`, `src/datasource/issueRows.ts`, `src/datasource/issueRows.test.ts`, `src/pages/IssuesPage.tsx` (imports `IssuesQueryEditor`)

**Interfaces:**
- Consumes: the resource and frame contracts (Tasks 2, 3).
- Produces:

```ts
export type QueryType = 'issues' | 'events';
// BugsinkQuery gains: issue?: string; eventId?: string;
export interface EventRow { id: string; eventId: string; issue: string; issueId: string; project: string; projectId: number; timestamp: number; ingestedAt: number; url?: string }
export interface EventList { rows: EventRow[]; notices: string[] }
export interface IssueDetail { id: string; friendlyId: string; projectId: number; project: string; type: string; message: string; transaction: string; status: IssueStatus; firstSeen: string; lastSeen: string; events: number; storedEvents: number; isResolved: boolean; isResolvedByNextRelease: boolean; isMuted: boolean; unmuteAfter: string | null; bugsinkUrl: string }
export interface IssueTag { key: string; values: Array<{ value: string; count: number }>; otherValues: number }
export interface IssueTags { tags: IssueTag[]; truncated: boolean }
export interface HistoryEntry { id: number; kind: string; timestamp: string; user: string | null; comment: string }
export interface IssueHistory { history: HistoryEntry[]; truncated: boolean }

// BugsinkDataSource
getIssue(id: string): Promise<IssueDetail>            // getResource(`issues/${encodeURIComponent(id)}`)
getIssueTags(id: string): Promise<IssueTags>
getIssueHistory(id: string): Promise<IssueHistory>
listEvents(query: BugsinkQuery): Promise<EventList>   // events query, getDefaultTimeRange(), refId 'events'
export function eventRows(frame: DataFrame): EventRow[]
export function ProjectPicker(props: { datasource: BugsinkDataSource; value?: number; onChange: (project?: number) => void }): React.JSX.Element
export function CommitInput(props: { label: string; value?: string; placeholder?: string; onCommit: (value: string) => void; onRunQuery: () => void }): React.JSX.Element
export function IssuesQueryEditor(props: Props): React.JSX.Element // the M3 editor, without a type picker
export function EventsQueryEditor(props: Props): React.JSX.Element
export function QueryEditor(props: Props): React.JSX.Element       // picker, then the editor of the type
```

- `listIssues` and `listEvents` share one private method that runs a target and returns the frames or throws the M3 failure message (`The events could not be loaded.` for events).
- `issueRows` types each column (`values<string>`, `values<number>`), no `any`. `eventRows` builds `url` from `event_id`'s link, replacing `${__data.fields.issue_id}` and `${__data.fields.id}` with the encoded row values.
- `applyTemplateVariables` interpolates `q`, `issue` and `eventId`.
- `QueryEditor`: `RadioButtonGroup` labelled `Query type` with `Issues`, `Events`; a change calls `onChange({ ...query, queryType })` and `onRunQuery`. `EventsQueryEditor`: `Project` (`ProjectPicker`; a change runs), `Issue` (placeholder `SAMPLE-PYTHON-2`), `Search` (placeholder `trace:4bf92f3577b34da6a3ce929d0e0e4736`), `Event ID` (placeholder `426ac644fafb475d905046d654730f1b`). `CommitInput` applies on blur when changed; Enter applies and runs (M3's Search rule). `ProjectPicker` holds M3's project loading, `Project <id>` fallback and error text.

- [ ] **Step 1: Write failing tests.**
  - `issueRows.test.ts`: the M3 cases still pass.
  - `eventRows.test.ts`: a frame of the events contract maps to `EventRow`s; the url is `http://localhost:8800/issues/issue/<issue uuid>/event/<event uuid>/`; ids with `/` are encoded; no link gives no url; no values gives `[]`.
  - `datasource.test.ts`: `getIssue('SAMPLE-PYTHON-2')` calls `getResource('issues/SAMPLE-PYTHON-2')`, likewise `…/tags`, `…/history`; `listEvents({refId: 'x', issue: 'X-1'})` sends one target `{queryType: 'events', issue: 'X-1', refId: 'events', datasource: ds.getRef()}`; resolves rows and every notice text in order; an error response rejects with its message; `applyTemplateVariables` replaces `$t` in `q`, `issue` and `eventId`.
  - `QueryEditor.test.tsx`: `Query type` shows `Issues` for `{}` and renders the issues controls; choosing `Events` calls `onChange` with `queryType: 'events'` and the same `project` and `q`, and runs; `{queryType: 'events'}` renders `Issue`, `Search`, `Event ID`, `Project`.
  - `EventsQueryEditor.test.tsx`: typing in `Issue` calls nothing until blur, then `onChange` with `issue`; Enter in `Event ID` applies and runs once; picking `sample-js` sets `project: 2` and runs.
  - `CommitInput.test.tsx`: blur without a change calls nothing; Enter without a change runs without `onCommit`.
  - `ProjectPicker.test.tsx` and `IssuesQueryEditor.test.tsx`: the M3 editor tests split between them, including `forgets the previous data source's projects`.
- [ ] **Step 2: Run** `npm run test:ci`. Expected: FAIL.
- [ ] **Step 3: Implement.** Read the `RadioButtonGroup` page on developers.grafana.com/ui first.
- [ ] **Step 4: Run** `npm run test:ci`, `npm run typecheck`, `npm run lint`. Expected: PASS, Jest exits by itself.
- [ ] **Step 5: Commit** `Add events and issue details to the frontend data source and a query type picker` and push.

### Task 5: App routes, the shared data source choice and the link to the detail page

**Files:**
- Create: `src/pages/App.tsx`, `src/pages/App.test.tsx`, `src/pages/routes.ts`, `src/pages/useDataSource.ts`, `src/pages/NoDataSource.tsx`, `src/pages/IssuePage.tsx` (summary only), `src/pages/IssuePage.test.tsx`, `src/pages/issue/IssueSummary.tsx`, `src/pages/issue/IssueSummary.test.tsx`
- Modify: `src/module.tsx` (`setRootPage(App)`), `src/pages/IssuesPage.tsx`, `src/pages/IssuesPage.test.tsx`, `src/pages/issues/IssueTable.tsx`, `src/pages/issues/IssueTable.test.tsx`

**Interfaces:**
- Consumes: Task 4's `getIssue`, `IssueDetail`; `readListState`.
- Produces:

```ts
// routes.ts
export const APP_ROOT = '/a/ahmedismail-bugsink-app';
export function issuePath(id: string, search: string): string // `${APP_ROOT}/issues/${encodeURIComponent(id)}${search}`
export function listPath(search: string): string              // `${APP_ROOT}${search}`
// useDataSource.ts
export interface DataSourceChoice { instances: DataSourceInstanceSettings[]; uid?: string; unknown: boolean; source?: BugsinkDataSource; failed: boolean }
export function useDataSource(requested: string | undefined, attempt: number): DataSourceChoice // a new attempt reloads a failed instance
export function App(): React.JSX.Element      // <Routes>: 'issues/:issueId' → IssuePage, '*' → IssuesPage
export function IssuePage(): React.JSX.Element
export function IssueSummary({ issue, timeZone }: { issue: IssueDetail; timeZone: TimeZone }): React.JSX.Element
// IssueTable gains: issueHref: (row: IssueRow) => string
```

- `IssuesPage` uses `useDataSource(urlState.ds, refresh)` and `NoDataSource`, keeping every M3 behaviour; `IssueTable` gets `issueHref = (row) => issuePath(row.id, location.search)`. In-app links are `react-router-dom` `Link`s; `Back to issues` uses `useNavigate`.
- `IssuePage` follows the detail page contract for the page frame and the Summary section in this task.

- [ ] **Step 1: Write failing tests** (`MemoryRouter`, the `@grafana/runtime` mocks of `IssuesPage.test.tsx`):
  - `IssueTable.test.tsx`: the `Issue` cell is a link named `SAMPLE-PYTHON-2` whose `href` is `issueHref`'s value and has no `target`; beside it a link named `Open SAMPLE-PYTHON-2 in Bugsink` with the row url and `target="_blank"`; a row without url has only the first link; the M3 column, paging and empty-state tests still pass.
  - `IssuesPage.test.tsx`: the M3 tests pass; `get(uid)` rejecting shows `The data source "Bugsink" could not be loaded.`, and Refresh calls `get` again and then shows the table.
  - `App.test.tsx`: `/a/ahmedismail-bugsink-app?ds=bugsink` renders the list; `/a/ahmedismail-bugsink-app/issues/X-1?ds=bugsink` calls `getIssue('X-1')` on `bugsink`.
  - `IssuePage.test.tsx`: from `issues/X-1?orgId=1&kiosk&ds=eu&state=open&q=a%20b`, `getIssue('X-1')` runs on `eu`; `Back to issues` goes to `/a/ahmedismail-bugsink-app?orgId=1&kiosk&ds=eu&state=open&q=a%20b` exactly; pending shows `Loading issue...`; a rejection `{data: {message: 'Bugsink refused the request: No Issue matches the given query.'}}` shows that alert; `Open in Bugsink` has `bugsinkUrl` and `target="_blank"`; `?ds=nosuch` shows `The data source "nosuch" was not found.` and calls no read; no instances shows `No Bugsink data source yet.`
  - `IssueSummary.test.tsx`: heading `LookupError: failure outside any span`; rows `Open`, `sample-python`, `/no-trace`, `26`, `26` and the two times in `utc`; `Unmute after` only with a value; `Resolved by next release` `Yes` only when true; an empty message gives heading `LookupError`.
- [ ] **Step 2: Run** `npm run test:ci`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `npm run test:ci`, `npm run typecheck`, `npm run lint`. Expected: PASS.
- [ ] **Step 5: Check in the stack:** `npm run build`, restart Grafana; `GET http://localhost:3300/public/plugins/ahmedismail-bugsink-app/module.js` contains `Back to issues`.
- [ ] **Step 6: Commit** `Route the app to an issue detail page and link the list to it` and push.

### Task 6: Tags, history and events on the detail page

**Files:**
- Create: `src/pages/issue/IssueTags.tsx`, `IssueHistory.tsx`, `EventTable.tsx`, each with a `.test.tsx`
- Modify: `src/pages/IssuePage.tsx`, `src/pages/IssuePage.test.tsx`

**Interfaces:**
- Consumes: Task 4's `getIssueTags`, `getIssueHistory`, `listEvents`, `EventRow`; Task 5's `IssuePage`, `PAGE_SIZE`.
- Produces: `IssueTags({ tags }: { tags: IssueTags })`, `IssueHistory({ history, timeZone })`, `EventTable({ rows, timeZone })`, rendering the contract's sections; `IssuePage` loads the four sections independently, each keyed to the instance, issue id and refresh, dropping answers to an earlier key.

- [ ] **Step 1: Write failing tests.**
  - `IssueTags.test.tsx`: `environment` with `development (26), staging (3)`; `trace` with `otherValues` 76 ends ` and 76 more values`, with 1 ` and 1 more value`; truncated shows `Only the first 5000 tag values were read.`; no tags shows `No tags.`
  - `IssueHistory.test.tsx`: each kind's label, unknown kind as written; ` by Ada` only with a user; the comment `m4 plan probe` under `Comment`; truncated warning; `No history.`
  - `EventTable.test.tsx`: headers exactly `Event ID`, `Timestamp`, `Ingested at`; the event id links to the row url in a new tab; rows in the given order; 30 rows page at 25; `No stored events.`
  - `IssuePage.test.tsx`: the three reads run with `X-1` on the chosen instance and `listEvents` with `{queryType: 'events', issue: 'X-1'}`; each section's loading text shows while pending; `getIssueTags` and `listEvents` rejecting with `The API token lacks events:read.` show that message in Tags and Events while Summary and History render; notices from `listEvents` show above the table; Refresh reruns all four.
- [ ] **Step 2: Run** `npm run test:ci`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `npm run test:ci`, `npm run typecheck`, `npm run lint`. Expected: PASS.
- [ ] **Step 5: Commit** `Show tags, history and events on the issue detail page` and push.

### Task 7: Trace dashboard and end-to-end tests of the demo

**Files:**
- Create: `docker/grafana/provisioning/dashboards/bugsink-trace.json`, `tests/issue-detail.spec.ts`, `tests/events-query.spec.ts`
- Modify: `tests/seeded.ts`, `tests/issue-list.spec.ts`, `docker/README.md`

**Interfaces:**
- Consumes: everything above, built into `dist/`.
- Produces: dashboard uid `bugsink-trace`, title `Bugsink trace`, time `now-24h` to `now`, textbox variable `trace` (label `Trace ID`, empty default), table panel `Issues in this trace` (`{queryType: 'issues', state: 'all', q: 'trace:$trace', sort: 'last_seen', order: 'desc'}`), table panel `Events in this trace` (`{queryType: 'events', q: 'trace:$trace'}`); `tests/seeded.ts` exports `TRACE_MARKER = 'm4-trace'`.

- [ ] **Step 1: Check the unverified fact first:** with the Task 6 build, open `/a/ahmedismail-bugsink-app/issues/SAMPLE-PYTHON-2?ds=bugsink`: the page heading and the last breadcrumb read `SAMPLE-PYTHON-2`. If not, stop and report.
- [ ] **Step 2: Provision** the dashboard; restart Grafana; `GET http://localhost:3300/api/dashboards/uid/bugsink-trace` answers 200.
- [ ] **Step 3: Write the tests** (read `.config/AGENTS/e2e-testing.md` first; digest waits retry with `toPass()`):
  - `issue-detail.spec.ts` `click an issue to see its detail`: `gotoPage('', new URLSearchParams('ds=bugsink&project=1&state=all&q=m2-e2e&from=now-24h&to=now'))`; click the first row's `SAMPLE-PYTHON-` link; the URL path matches `/a/ahmedismail-bugsink-app/issues/[0-9a-f-]{36}` and keeps the query string; Summary contains `ValueError` and `m2-e2e`; Tags contains `environment` and `development`; History contains `First seen`; the first Events row's link matches `^http://localhost:8800/issues/issue/[0-9a-f-]{36}/event/[0-9a-f-]{36}/$`.
  - `back to the list keeps its state`: from that detail page, `Back to issues`; the URL equals the list's; `Search` shows `m2-e2e`.
  - `a project-bound token on another project's issue`: `gotoPage('issues/SAMPLE-JS-1', new URLSearchParams('ds=bugsink-project'))`: Summary, Tags, History and Events each show `Bugsink refused the request: This token is not allowed to access this project.`
  - `an unknown issue`: `issues/00000000-0000-0000-0000-000000000000?ds=bugsink` shows `Bugsink refused the request: No Issue matches the given query.`
  - `events-query.spec.ts`, `beforeAll`: `GET ${SAMPLE_APP_URL}/handled?variant=m4-trace` once, keeping `trace_id` and `event_id` (format the dashed form for assertions).
  - `issues and events of one trace`: `gotoDashboardPage({ uid: 'bugsink-trace', queryParams: new URLSearchParams({ 'var-trace': traceId }) })`; panel `Issues in this trace` contains `m4-trace`; panel `Events in this trace` contains the dashed event id and `SAMPLE-PYTHON-`.
  - `look up an event by its ID in Explore`: pick `Bugsink`, `Query type` `Events`, `Project` `sample-python`, `Event ID` = the hex id, Enter; the table contains the dashed id.
  - `issue-list.spec.ts` demo test: the first row's `Open … in Bugsink` link has the Bugsink pattern and `target="_blank"`; its short-id link points at `/a/ahmedismail-bugsink-app/issues/`.
- [ ] **Step 4: Run** `npm run build && npm run build:backend && docker compose restart grafana && npm run e2e`. Expected: every test passes, twice in a row.
- [ ] **Step 5: Run** `npm run lint`, `npm run typecheck`, `scripts/check_spdx.sh`, `python3 scripts/verify_stack.py`. Expected: PASS.
- [ ] **Step 6:** In `docker/README.md`, describe `Bugsink trace`: paste a trace id from `curl http://localhost:8810/handled` into `Trace ID`.
- [ ] **Step 7: Commit** `Provision a trace dashboard and test issue detail and events end to end` and push.

### Task 8: Documentation and changelog

**Files:**
- Modify: `src/README.md`, `CHANGELOG.md`

- [ ] **Step 1: Write** in `src/README.md`: the intro no longer says events are unavailable; under `## Browsing issues in the app`, that the short id opens the detail page and the icon opens Bugsink; a section `## Issue detail page` (summary fields, tags grouped by key with the top 5 values, history, events in Bugsink's arrival order with links to Bugsink, `Back to issues` keeps the list, Refresh, each section's own error, the tag and history limits); a section `## Querying events` (the `Query type` picker, the three modes and their order, that `q`, `Issue` and `Event ID` take variables, the frame's columns, that the dashboard time range does not apply, the bounds and their notices, the trace dashboard as the example); `events:read` named for tags and events. In `CHANGELOG.md` under 0.1.0: `Issue detail page in the app: summary, tag values with counts, history and the issue's events; the list's short id opens it.` and `Events query: the events of one issue, one event looked up by its SDK id, or every event of a trace, as a table.`
- [ ] **Step 2: Run** `scripts/check_spdx.sh`, `scripts/check_licenses.sh`, `npm run lint`. Expected: PASS.
- [ ] **Step 3: Commit** `Document the issue detail page and the events query` and push.

## Not in this milestone

Event content and an in-app event page (M5), triage controls (M7), event-count sparklines (M9), history `metadata` and triggering events, an in-app data link from dashboard frames, a time-range filter for events, cursor paging of events on the page beyond 1000, caching of detail reads, sorting the merged trace rows by timestamp.

## After the last task

* Whole-branch review against this plan and spec §4.B extras, §4.C, §4.D, then one fix wave.
* From the worktree: `npm ci`, `npm run build`, `npm run build:backend`, `go test -race ./pkg/...`, `npm run test:ci`, `npm run e2e`, `python3 scripts/verify_stack.py`.
* Update spec §4.C (modes, bounds, no time range, fan-out order) and §4.D (the short id opens the detail page, the icon opens Bugsink, the detail page as built) and the roadmap's M4 entry, and add an `## As built` section here.
* The merge into `main` happens only when the owner asks for it.

## As built

1. The events query answers an empty table with one info notice, `An events query needs an issue, an event ID or trace:<id> in Search.`, when it has no event id, issue or `trace:<id>` term, instead of the 400 in the events message table; the other three messages stay 400.
2. This replaces decision 7's "anything else is a message", so the provisioned `Bugsink trace` dashboard opens on "No data" before a trace id is typed.
3. `notrace:abc` and `trace:` count as naming nothing to read.
4. When the summary read fails, the page shows its message alone and none of the Tags, History and Events sections; otherwise each of those sections has its heading in the page and shows its own error under it.
5. Resource reads for the editors and the page pass `showErrorAlert: false`, so a failed read shows its inline message and raises no Grafana toast.
6. Switching the query type to Issues with no project and Arrival order selected changes Sort by to Last seen.
7. The trace event of the e2e run is reported by `tests/report-trace.setup.ts` in the `report-event` project and written to the gitignored `playwright/.trace.json`, so a run reports 2 events in total.
8. The Explore event lookup in the e2e test retries until Bugsink has digested the event.
9. The Project field narrows the trace's issue search and is not used with Issue; the user README says so.
10. The user README no longer links to the dev stack's README, because it ships inside the plugin package.
11. Spec §4.C and §4.D, the §10 decisions table and the roadmap entry state what was built.
12. Left for later milestones:
    * M5: an events table empty state like the Tags and History ones, and theme spacing in place of the inline pixel styles.
    * M7: a client-side issue id check before write routes, shared status labels and time format, keeping the summary on refresh and replacing it from action answers, the "other controls keep working" editor test, and tests for `rel` on Open in Bugsink and for Back without a query string.
    * M8: renaming the `describe` helper in `IssueHistory.tsx`.
    * M11: a spinner while a data source retry is in flight.
