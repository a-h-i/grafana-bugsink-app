# Project Spec: Bugsink App Plugin for Grafana

## 1. Purpose & Core Goals
* **Unified UI:** View and triage Bugsink issues inside Grafana, next to the LGTM stack.
* **Trace & Log Correlation:** Jump from a Bugsink event to its Tempo trace and Loki logs through the event's `trace_id`.
* **Dual Interface:** An App page for developer triage, plus a data source for dashboards (and alerting, see §6).
* **Sentry migration path:** Teams moving from Sentry to Bugsink keep an issue browser that sits beside their traces and logs.
* **General purpose:** Nothing is specific to one installation. Label names, queries, links and token scopes are configurable, so the plugin can be published to the Grafana plugin catalog.

### Non-goals
* Event ingestion or SDK configuration (Bugsink and the Sentry SDKs own that).
* Replacing Bugsink's own UI for administration (projects, teams, tokens, releases).

---

## 2. Target Versions
* **Bugsink:** 2.6.2 or later: token capabilities, issue search and state filters, cross-project issue lists, tag counts and hourly event counts in the canonical API at `/api/canonical/0/`.
* The behaviour described here was checked on Bugsink 2.6.3 only (`design/phase0_findings.md`).
* The public API docs page lagged behind releases (it showed 2.6.0 after 2.6.2 shipped); the installation's own schema is the source of truth.
* **Grafana:** the current major release; the exact minimum follows from the SDK features used and is set in `plugin.json`.
* The API contract is pinned in the repo as the OpenAPI schema served by Bugsink at `/api/canonical/0/schema/`, refreshed when the supported Bugsink version moves.

---

## 3. Architecture

```text
┌──────────────────────────────────────────────────────────────┐
│ Bugsink App plugin (one package)                             │
│  ┌──────────────────────────┐   ┌─────────────────────────┐  │
│  │ App pages (frontend only)│   │ Dashboards / Explore    │  │
│  │ Issue list, issue detail,│   │ Table panels, (alerting)│  │
│  │ event detail, triage     │   │                         │  │
│  └────────────┬─────────────┘   └────────────┬────────────┘  │
│               │ resource calls               │ queries       │
│               └──────────────┬───────────────┘               │
│                              ▼                               │
│   Nested Bugsink data source (Go backend, one per instance)  │
│   - QueryData: issues / events → DataFrames with data links  │
│   - CallResource: detail reads, triage actions               │
│   - CheckHealth: token and capability check                  │
│   - Token kept in secureJsonData, never sent to the browser  │
└──────────────────────────────┬───────────────────────────────┘
                               ▼ REST (Bearer token)
                 ┌───────────────────────────┐
                 │ Bugsink /api/canonical/0/ │
                 └───────────────────────────┘
```

* **Packaging:** an app plugin (`ahmedismail-bugsink-app`) with a nested backend data source plugin (`ahmedismail-bugsink-datasource`), shipped and signed as one package under the grafana.com organization `ahmedismail`. The app has no backend of its own; every Bugsink call goes through a data source instance.
* **Multiple Bugsink instances:** each is one data source instance. The App pages have a data source picker.
* **Frontend (React with `@grafana/ui`; no Scenes: the App's columns and controls are React cells and components):**
  * App pages: issue list, issue detail, event detail, triage controls.
  * Config editor: Bugsink base URL, API token, project scope, Tempo and Loki data sources, link time window, logs query and service tag, triage settings.
  * Query editor: query type (issues, events) and their parameters.
* **Backend (`grafana-plugin-sdk-go`):**
  * Typed client generated from the pinned OpenAPI schema.
  * Pages with the cursor Bugsink returns in `next`; never builds cursors itself, and never requests a URL taken from a response. Never follows a redirect, for reads or writes: a redirect is a Bugsink failure reported with its status, never with its `Location`.
  * Short-lived cache per (data source, request) to absorb dashboard refreshes and App page re-renders.
  * Converts responses into DataFrames and attaches data links (§5).
  * Serves time series from Bugsink's hourly event counts (§6).

---

## 4. Functional Requirements

### A. Authentication & Configuration
* **Token:** a Bugsink API token in `secureJsonData`. Recommended: a global service token with `projects:read`, `issues:read`, `events:read`, plus `issues:triage` and `issues:comment` when triage is enabled.
* **Project-bound tokens** are supported: `GET /projects/` returns only the token's own project. The config editor discovers the project from it and has no project id field.
* **Browser URL** (optional, `jsonData.browserUrl`): the address of Bugsink as the browser reaches it, used for links to Bugsink's pages. Empty means the Bugsink URL. It must be `http` or `https` and carry no credentials, query string or fragment.
* **Link settings** (all in `jsonData`, none secret): `tempoDatasourceUid` and `lokiDatasourceUid` (empty: that link is not offered), `linkWindowMinutes` (a whole number from 1 to 1440; empty means 15), `logsQuery` (empty means the default in §5) and `serviceTag` (empty means the project name; default `service`). A window outside 1 to 1440, a logs query with a substituted variable outside a double-quoted string, inside a comment (`#`, `//` or `/* */`, outside strings) or in a double-quoted string cut by a line break, or a link key of the wrong type makes the settings invalid: the health check and every query and resource answer with that message.
* **Switches** (booleans in `jsonData`): `triageEnabled` (off by default) turns on the triage routes and controls of §4.E; `hideStacktraceText` (off by default) drops Bugsink's stack trace text in the backend (§4.C); `attributionComments` (absent or `null` means on, `false` off) writes the attribution comment of §4.E and has an effect only with triage on. A value that is not a boolean makes the settings invalid. The config editor shows `Attribution comments` under the `Triage` heading, after the `Triage` switch, always rendered.
* **Health check:** confirms the base URL, the token, and which configured features the token's capabilities allow (for example "triage enabled but token lacks `issues:triage`").
  * No endpoint reports a token's capabilities. `GET /api/0/` returns `auth.scopes`, but it needs `debug-files:upload` and returned `["org:ci"]` for the tokens tried, one with `debug-files:upload` and one with it plus `issues:read` (observed by hand, findings Q4). Each operation needs one (`x-bugsink-required-capability` in the schema), so a client learns a capability only by calling an operation that needs it and reading the refusal.
  * The health check makes at least one read per capability that the configured features need (`projects:read` reads every page of projects), and no others (with triage off, nothing is checked for triage). With triage on it also probes `issues:triage` with one `POST issues/00000000-0000-0000-0000-000000000000/resolve/`: Bugsink 2.6.3 checks the capability before it looks up the issue, so a 404 means the token has it, a 403 naming it means it lacks it, and nothing changes; any other answer, a 200 included, is reported as an unexpected answer to that check.
  * With triage and attribution on, it then probes `issues:comment` with one `POST issue-comments/` holding `{"issue":"00000000-0000-0000-0000-000000000000","comment":""}`, which Bugsink refuses twice over (no such issue, a blank comment), so nothing is written. A 400 means the token has it and the success message ends ` Actions are attributed by a comment.`; a 403 naming `issues:comment` keeps the status OK and ends ` It lacks issues:comment, so actions are not attributed.`; a 401, a connection error or a timeout are the usual errors; a 2xx means Bugsink accepted the probe and is the error `Bugsink answered HTTP <n> to a check for issues:comment.`; any other answer (an older Bugsink's 404 or 405, a 500, a 403 naming no capability or another one) keeps the status OK and ends ` Bugsink answered HTTP <n> to a check for issues:comment, so actions may not be attributed.`. Both probes are free of side effects.
  * A 401 means a missing or bad token. A 403 whose body names a capability (`This token does not have the required capability: projects:read.`) means that capability is missing; a 403 without one (`This token is not allowed to access this project.`) means the project is not the token's.

### B. Issue Queries
`GET /issues/` supports, as of 2.6.3:
* `project` (optional; omitted lists every project the token can see), `state` (`all`, `open`, `unresolved`, `resolved`, `muted`), `q` (plain-text title search plus `key:value` tag filters, e.g. `environment:production release:1.4.2`), `sort` (`digest_order`, `last_seen`, `digested_event_count`), `order`, `cursor`.
* **Sort defaults:** `digest_order` needs a single project (400 without one for a global token). Without `sort`, a single-project list is `digest_order` ascending and a list across projects is `last_seen` descending, so the plugin sends `sort` and `order` explicitly.
* **Paging:** 250 results per page; `{next, previous, results}` with no total count. A query reads at most 20 pages (5000 issues) and then says so in a warning on the frame.
* **Time range:** issues last seen within the dashboard range (`from <= last_seen <= to`). Sorted by `last_seen` descending, the plugin stops paging once `last_seen` falls before the range start; with any other sort it reads every page (up to the limit) and filters. Issues last seen after the range end are skipped.
* **Filters** map directly onto `state` and `q`; the query editor offers a project picker, a state picker and a free search field. Defaults: all projects, state `unresolved`, `last_seen` descending. Only `q` takes dashboard variables.
* **Frame:** besides the fields below, a row carries the project's name, a `status` (`resolved`, `muted` or `open`) and the issue `id`; the `friendly_id` field links to the issue's page in Bugsink (`<browser URL>/issues/issue/<id>/event/last/`).
* **Cache:** 15 seconds per data source instance; a list fetched for one range start answers later queries with the same filter and a later range start.
* **Issue fields used:** `friendly_id`, `project`, `calculated_type`, `calculated_value`, `transaction`, `first_seen`, `last_seen`, `digested_event_count`, `stored_event_count`, `is_resolved`, `is_resolved_by_next_release`, `is_muted`, `unmute_after`.
* **Issue extras:** tag values with counts (`issues/{id}/tags/`, needs `events:read`), issue history (state changes and comments with author names, `GET /issue-history/?issue=`), hourly event counts (`issues/{id}/event-counts/`).

### C. Event Queries
* **Modes:** the events query type takes `project`, `q`, `issue` and `eventId`; the first that applies wins: `eventId` (lookup; needs a project), `issue` (that issue's events, narrowed by `q`; `project` unused), a `trace:<id>` term in `q` (the fan-out; `project` narrows its issue search). With none of the three the answer is an empty table with an info notice, not an error; a bad issue id, a bad event id and a lookup without a project are 400. `state`, `sort` and `order` are ignored.
* **Time range:** the dashboard time range does not apply to events; an issue, an event id or a trace already scopes the list.
* **Bounds:** one issue reads at most 4 pages (1000 events); the fan-out reads one page of matching issues (state `all`, last seen first), keeps the first 10 and reads one page (250 events) of each. Each bound reached adds a warning notice, and a failing request fails the query. Results are cached for 15 seconds.
* **Event list** for one issue (`issue` is required, `q` search): `id`, `event_id`, `timestamp`, `ingested_at`. The list is ordered by `digest_order` (arrival order), newest first by default; there is no sort by event `timestamp`. The UI keeps this order and shows the event `timestamp` in a column; it does not re-sort, because the API pages in arrival order.
* **Trace ids and links on event frames:** the frame has a `trace_id` field (display `Trace ID`) after `ingested_at` and a `logs_query` field last, hidden in every table through `hideFrom` (`viz`, `legend`, `tooltip`). `trace_id` holds the `trace:<id>` term's id for every row of a query with one, and the payload's id for an event id lookup; an issue-only query leaves it empty and carries no links. `trace_id` carries the `Open trace` and `Open logs` links only when every row has a trace id of 32 lowercase hexadecimal characters and at least one target is set. `logs_query` holds the logs query rendered per row, because the rows of one trace can belong to different projects. The links carry the range of the first to the last timestamp plus and minus the window, in the SDK's `timeRange`.
* **Lookup by SDK event id** (`events/lookup/?project=&event_id=`), for links that carry the Sentry event id.
* **Search by trace:** Bugsink indexes `contexts.trace.trace_id` as the tag `trace`. `GET /issues/?q=trace:<id>` finds the issues holding events of one trace, with or without `project`; `GET /events/?issue=<id>&q=trace:<id>` finds the events within one issue. Events cannot be searched by trace across issues (`issue` is required). The reverse link, from a Tempo trace to its Bugsink events, therefore goes trace, issue, events. The events query type does this itself: given `trace:<id>` and no issue, it finds the issues and then their events, and returns one frame. The fan-out groups rows by issue, last seen issue first, each issue's events newest first; it does not re-sort across issues.
* **Event detail:** the full Sentry event payload (`data`). The backend parses it field by field (`GET events/{id}`) and serves a normalised event, so the browser never gets the raw payload; a field of the wrong type is left empty, never an error. The plugin reads the exceptions, stack frames, breadcrumbs, tags, contexts (`trace_id`), environment, release, transaction and message from it. The Sentry event format is a stable public protocol. Reads are not cached.
* **Event bounds:** at most 10 MiB of Bugsink's answer; the last 10 exceptions of a chain, the 250 frames nearest the error of each, 5 source lines on each side of a frame's line, the newest 100 breadcrumbs, the first 200 entries by key of each tags, contexts, context fields and breadcrumb data list, values cut at 1024 characters and Bugsink's stack trace text cut at 100 KB. Each bound reached adds a warning notice.
* **Stack trace display:** rendered from the structured frames in `data`, without local variables. Bugsink's `stacktrace_md` is shown as preformatted plain text, not rendered Markdown, only as a fallback when no exception has frames; it is Bugsink's verbatim text, is never parsed, and can include local variables. With `hideStacktraceText` on, the event resource drops that text, and its 100 KB cut notice, in the backend and adds the notice `Bugsink's stack trace text is hidden for this data source.` when there was text; the section then shows `No stack trace.`

### D. App UI
* **Issue list:**
  * Columns: Type (`calculated_type`), Message (`calculated_value`), Transaction, Project, Events (`digested_event_count`), First seen, Last seen, Status, Actions.
  * Actions shows only when triage is on for the data source and the user's org role may triage (§4.E): Resolve and Mute for an open row, Resolve for a muted row, nothing for a resolved row. The buttons of an issue stay disabled while an action on it runs, and every button is disabled while the list loads. An answer that arrives after the list changed does not change a row. One message shows above the table, keyed to the list load it belongs to: a failure in the current list reloads the list once and shows through that reload; a failure or an attribution warning (prefixed with the short id) that arrives after a new search shows on the new list; the message is dropped when the page leaves its load, so it does not come back when the same search comes back.
  * An `Issue` column (`friendly_id`): in the app, the short id opens the issue detail page and an icon beside it opens the issue in Bugsink in a new tab; in dashboard frames the field links to Bugsink.
  * Filters: project, status (all, open, unresolved, resolved, muted), search text, time range. Sort: last seen, event count, and arrival order within one project. The filter bar is the data source's query editor; the page runs the data source's issues query.
  * 25 rows per page, paged in the browser. The page's state (data source, filters, sort, time range) lives in the URL; parameters the page does not own, such as `orgId` and `kiosk`, are kept as written.
  * The default instance is Grafana's default data source when it is a Bugsink one, else the first by name. Switching instance clears the project.
  * The detail page's address carries the list's query string, and Back to issues returns to the list with it.
  * Event-count sparkline per row from `event-counts` (one call per visible row; see §6).
* **Issue detail:** a page at `issues/<id>` with a Back to issues button, an Open in Bugsink button and a Refresh button. Five reads load at the same time (the Event read waits for the Events list when the address names no event): the Event (see Event detail below), the summary (type and message, status, project, transaction, event counts, first and last seen, unmute after and resolved by next release when set), Tags (the 5 values with the highest count per key and the number of other values; needs `events:read`), History (kind, time, author, comment, and the details Bugsink's `metadata` holds: release, next release, mute period, volume threshold, unmute time; newest first) and Events (the issue's events, 25 per page, newest first). When the summary cannot be read, its message shows alone; otherwise each of the other four shows its own error under its heading. Failed reads raise no Grafana toast. Refresh reads every section again and keeps the shown content until the new answers arrive. With triage on and a role that may triage, the actions for the issue's state show under the summary (§4.E): `Mute for a period` and `Mute until a threshold` open a modal (`Number of periods`, default 1; `Period`, default Days; `Events`, default 10, for the threshold) that runs the backend's rules and messages live and disables `Mute` while one fails; an open modal closes when the page moves to another issue or data source. After an action, an attribution warning shows under the buttons; a failure shows there and the summary and History are read again; both belong to the issue and data source they came from.
* **Event detail:** an Event section on the issue page, between the summary and the Events table.
  * The `event` address parameter holds Bugsink's id of the event on show; without it the newest event of the issue shows. Event ID in the Events table opens an event in the section. An `event` that belongs to another issue shows `This event belongs to another issue.`
  * Identifying rows, then the actions, then the message when the event has one, then the exception chain with its stack trace (in-app frames open with source lines, others folded); Bugsink's stack trace text when no exception has frames. An exception without a type is headed `Unknown exception`.
  * Breadcrumb timeline.
  * Tags and contexts (`environment`, `release`, `trace_id`, user-defined tags).
  * **Open trace** and **Open logs** buttons (§5), hidden when the event has no linkable `trace_id` (32 lowercase hexadecimal characters) or the target data source is not configured, and **Open event in Bugsink**, which opens the event in Bugsink in a new tab. The buttons open Explore in the same tab. The plugin does not check that the trace exists in Tempo.
  * An event captured with no active span, such as one captured in a new thread, still carries a `trace_id` (the SDK's own), which Tempo does not have (§5), so a link can lead to a missing trace; Tempo's own not-found message shows.

### E. Triage
Bugsink has one endpoint per action under `/api/canonical/0/issues/{id}/`:

| UI action | Endpoint | Body |
| :--- | :--- | :--- |
| Resolve | `resolve/` | none |
| Resolve in latest release | `resolve-latest/` | none |
| Resolve in next release | `resolve-next/` | none |
| Reopen | `reopen/` | none |
| Mute | `mute/` | none |
| Mute for a period | `mute-for/` | `period_name`, `nr_of_periods` |
| Mute until a volume threshold | `mute-until/` | `period_name`, `nr_of_periods`, `gte_threshold` |
| Unmute | `unmute/` | none |

* **Who may triage:** the backend checks the calling Grafana user before forwarding a triage call: the org role from Grafana's plugin context must be exactly `Editor` or `Admin`. Anonymous sessions and service accounts count by their org role, so an anonymous Editor or Admin can triage. Triage is off by default and switched on per data source.
* **No Grafana RBAC plugin actions.** On Grafana OSS a backend plugin cannot check an action for the caller: SDK v0.299.0 hands the plugin only `Login`, `Name`, `Email` and `Role` and has no permission API, and the plugin-tools RBAC guide's backend check needs `github.com/grafana/authlib/authz` (a new dependency), a managed service account from an `iam` block, the `externalServiceAccounts` feature toggle ("The `externalServiceAccounts` feature only supports single organization setups."), the user's id token in `X-Grafana-Id` (`idForwarded` toggle) and Grafana 11.6.0 or later. The guide also says: "Assigning roles to specific users requires either Grafana Cloud or a Grafana Enterprise license." On OSS, plugin roles could at most be granted to basic roles, which the org-role check already does. Neither toggle is on in the dev stack. Not verified: whether Grafana OSS 13.3.1 registers plugin roles granted to basic roles.
* **Routes:** `POST issues/{id}/resolve`, `resolve-latest`, `resolve-next`, `reopen`, `mute`, `mute-for`, `mute-until` and `unmute` on the data source's resources, all served by one handler. The checks run in this order: path segments (400 for a dot, empty or encoded-slash segment), method (405), caller (403 `Only editors and admins can triage issues.`), settings (400), switch (403 `Triage is switched off for this data source.`), the header `X-Bugsink-Triage: 1` (403 `Triage calls need the header X-Bugsink-Triage: 1.`), id (400, `^[A-Za-z0-9_-]{1,100}$`), then for the two mute routes only the body (400), then Bugsink. A refused call reaches no Bugsink endpoint and no refusal reads the body. The app sends the header; a page on another site cannot add it to a request through the browser, so it cannot triage through a visitor's Grafana session. A page that uses DNS rebinding to reach Grafana under its own name can add the header, but its request carries no Grafana session, so it gets only what anonymous access allows.
* **Mute bodies:** at most 1024 bytes (`The request body is larger than 1024 bytes.`); exactly one JSON object and nothing after it, with exactly `period` and `periods` for `mute-for` and also `threshold` for `mute-until`; a repeated, unknown or missing key, `null`, a string number, a fraction or an exponent is refused with `The request body must be a JSON object with period and periods.` (or `... with period, periods and threshold.`). `period` is one of `minute`, `hour`, `day`, `week`, `month`, `year` (`The period must be minute, hour, day, week, month or year.`); `periods` a whole number from 1 to 100 (`The number of periods must be a whole number from 1 to 100.`; 100 years stays inside Bugsink's date range); `threshold` a whole number from 1 to 1000000 (`The threshold must be a whole number from 1 to 1000000.`). Bugsink gets a body rebuilt from the validated values: `period_name`, `nr_of_periods` and, for `mute-until`, `gte_threshold`. The bodiless routes never read the body, and no route reads the query string or forwards a header.
* **Answers:** 200 with the issue as `GET issues/{id}` serves it; any Bugsink failure is 502 with Bugsink's sentence, for example `Bugsink refused the request: Issue is already resolved.` Every answer carries `Cache-Control: no-store`. Every call sent to Bugsink empties the instance's issue list cache, and a list read that started before is not stored. One info log line per call sent holds the action, the id, the Grafana login, the org role, Bugsink's status and, when a comment was sent, Bugsink's status for it (0 without an answer); never the token, the body or the comment text. `QueryData` has no write path.
* **Transitions** (Bugsink 2.6.3): an open issue offers Resolve, Resolve in latest release, Resolve in next release, Mute, Mute for a period and Mute until a threshold; a muted issue the three resolves (which also unmute it) and Unmute; a resolved issue Reopen. Resolve in latest release is offered without knowing the project's releases; on a project without one Bugsink answers 400 `Project has no releases.`, shown as `Bugsink refused the request: Project has no releases.` (from Bugsink's source and a unit test; not seen live, both seeded projects have releases). Bugsink has no release picker, so the plugin has none.
* **Attribution:** every action runs with the token, and Bugsink's history records it without a user. With `attributionComments` on (the default), after Bugsink accepted the action the plugin sends one comment (`POST /issue-comments/`, body `issue`, the uuid from Bugsink's answer, and `comment`) whatever the token's capabilities: `<what> from Grafana by "<login>".`, `<what>` being `Resolved`, `Resolved in the latest release`, `Resolved in the next release`, `Reopened`, `Muted`, `Muted for <n> <unit>`, `Muted until <t> or more events in <n> <unit>` or `Unmuted` (unit singular for 1). The login comes only from Grafana's plugin context, never from the request; it is the Grafana login, which some sign-in methods make an email address, and a service account shows as `sa-<org>-<name>`. Inside the quotes every Unicode quotation mark, `"` included, becomes `'`, every control, format, line or paragraph separator character becomes U+FFFD, and a login over 100 characters is cut with an ellipsis appended inside the quotes. An empty login (anonymous access) is written `an anonymous user`, without quotes. Everyone who can see the issue in Bugsink can read the comment. If the comment fails, the action stands: no retry and no undo, and the answer is 200 with the issue and `warning: "The action succeeded, but the attribution comment was not written: <reason>"`, which the list shows with the short id and the issue page under the buttons.
* **Inline actions** on the issue list: Resolve and Mute. The other actions are in the issue detail page. Triage is in the App only, not in dashboard tables or Explore.
* The issue's state shown after an action comes from the issue object Bugsink returns. After a failure the message shows inline, the list loads again and the issue page reads the summary and History again.

### F. Dashboard Integration
* **Table:** issue rows and event rows, with the same links as the App pages. The `Trace ID` cell of an events table carries `Open trace` and `Open logs` in its cell menu; in Explore, a link followed from the table uses Explore's time range.
* **Time series and alerting:** open, see §6.

---

## 5. Trace & Log Linking

### Prerequisite
Links only work when the `trace_id` in the Bugsink event matches the trace stored in Tempo. That holds when the application's Sentry SDK shares the OpenTelemetry trace context (for example, Sentry's OpenTelemetry integration or propagator). Otherwise the SDK uses its own trace id, which Tempo does not know. The plugin's documentation explains this setup.

Verified setup (Python, FastAPI, Sentry SDK 2.75, OpenTelemetry SDK 1.45):
* Sentry's `OTLPIntegration(setup_otlp_traces_exporter=False)` makes the event's `contexts.trace.trace_id` the OpenTelemetry trace id; the app exports its spans to Tempo itself.
* In a FastAPI/Starlette app with Sentry's default ASGI integration, the OpenTelemetry ASGI middleware must be the outermost wrapper (`OpenTelemetryMiddleware(app)`). With the FastAPI instrumentor, Sentry captured unhandled errors after the request span had ended and the event carried a trace id Tempo does not have (observed by hand, see phase0_findings.md, Q1; not reproduced by a script).
* An event raised with no active span still carries a trace id and span id from the SDK's propagation context, and Bugsink tags it `trace`; Tempo has no such trace. The plugin cannot tell such an id from a real one without asking Tempo, so the link is offered and Tempo reports the trace as not found; the documentation explains the case.
* The integration's default propagator ignores an incoming W3C `traceparent` header and starts a new trace. With `setup_propagator=False` the span and the event keep the incoming trace id (tested with Sentry SDK 2.75.0), so the documented setup is `OTLPIntegration(setup_otlp_traces_exporter=False, setup_propagator=False)`.

Verified setup (Node.js, Sentry SDK `@sentry/node` 11, OpenTelemetry JS 2): `Sentry.openTelemetryIntegration()` attaches events to the active OpenTelemetry span and leaves propagation and span export to the app's own OpenTelemetry setup; the sample app keeps an incoming `traceparent`. The sample app drops the default Dedupe integration, which compares the first exception, the constant cause, and would drop every later handled event.

### Link construction
* Links are Grafana **internal data links** (target data source UID + query) on the event frames. Grafana builds the Explore URL, so the plugin does not hand-write URLs.
* **Time range:** the event's `timestamp` ± a configurable window (default 15 minutes), not "last hour".
* **Tempo:** a trace-by-ID query for `trace_id` (`queryType: traceId`), offered only for a trace id of 32 lowercase hexadecimal characters.
* **Loki:** a configurable LogQL template. Default:
  ```text
  {service_name="${service}"} | trace_id="${trace_id}"
  ```
  * Template variables: `${trace_id}`, `${service}`, `${project}`, `${environment}`, `${release}`, `${tag:<name>}`. The backend renders the template in one pass, per row; a variable the row does not know renders empty (in a table, `${environment}`, `${release}` and `${tag:…}` are known only for an event id query), and an unknown `${…}` stays as written.
  * Every variable must sit inside a double-quoted string; a template that does not is refused as an invalid setting. A value is escaped for a double-quoted LogQL string, and `$` and `[` are written as `\u0024` and `\u005b`, so Grafana's template interpolation in Explore finds no variable in an event value. Values are not escaped for a regular expression.
  * `${service}` comes from a configurable source (an event tag, or the Bugsink project name; default: tag `service`, falling back to the project name).
  * The default uses structured metadata (`| trace_id=`): through the OpenTelemetry Collector's OTLP/HTTP export, Loki keeps `trace_id` as structured metadata, not a label, and by default indexes only a fixed set of resource attributes (such as `service_name`) as labels. A line filter (`|= "${trace_id}"`) is a one-line template change for setups that only have the id in the log line.
* **Range transport:** the backend sends the range in the SDK's `timeRange`; the browser copies it to `internal.range`, the key Grafana reads, and the Event section turns each link into an Explore address with `mapInternalLinkToExplore` (never a hand-written URL).
* Links belong to events, not issues: `trace_id` is per event, and fetching the latest event for every issue row would cost one call per row.

---

## 6. Time Series & Alerting
Bugsink (2.6.2 and later; verified on 2.6.3) keeps hourly event counters per installation, project and issue, updated at digest time, so they include events later evicted by retention.

* **Endpoints:** `GET /projects/{id}/event-counts/` and `GET /issues/{id}/event-counts/`, with `start` and `end` on UTC hour boundaries, within the last 90 days (an off-boundary `start`, a longer range or a `start` older than 90 days is a 400). Both are optional: `end` defaults to the end of the current hour and `start` to 24 hours before it. The response is `{start, end, interval_seconds: 3600, buckets: [{timestamp, count}]}` in ascending order, one bucket per hour, zero-filled; the current hour is included and incomplete.
* **Series offered by the data source:**
  * Event volume per project, and per issue, from `event-counts` (hourly; the plugin re-buckets to coarser intervals, never finer).
  * New issues per interval, from `first_seen` in the issue list.
  * Current values for alert rules: open-issue count, issues seen in the last N minutes, event count of the current hour. The issue list has no count field (250 per page), so the data source pages through the list (capped at a fixed number of pages) and caches the count for a short time.
* **Alerting:** the data source declares `alerting: true` and serves these series to Grafana alert rules. Volume alerts work at hourly resolution: a rule can compare the current, incomplete hour against a threshold or the previous hours, but cannot react to a five-minute spike.
* **Beyond 90 days:** hourly counts are kept for a fixed 90 days, independent of event retention. Older history is the issue's lifetime totals (`digested_event_count`, `first_seen`, `last_seen`) plus the events retention has kept, which are a sample. The plugin does not build count series from stored events.
* **Left with Bugsink:** new-issue, regression and unmute notifications, which Bugsink sends itself.
* **Limits to raise with Bugsink upstream** (only if they hurt in practice):
  * Counts for many issues in one request: a dashboard of N issues costs N calls today.
  * Resolution finer than one hour.
  * A count on the issue list, which would replace the page-through.

## 7. Development & Test Environment
* **Docker Compose** stack: Bugsink, Grafana (plugin mounted), Tempo, Loki, and an OpenTelemetry Collector.
* **Sample apps** (one Python, one JavaScript, a Node server on port 8820) instrumented with a Sentry SDK pointed at Bugsink and OpenTelemetry pointed at the collector, sharing trace context. They produce errors on demand.
* **Ingestion throttle:** Bugsink throttles ingestion at about 1000 events per 5 minutes and answers 429 (observed by hand; the limits were not measured), so tests that produce events keep to a small budget per run.
* **Tests:** Go unit tests for the client and frame conversion; frontend unit tests; end-to-end tests with `@grafana/plugin-e2e` against the Compose stack, run in CI.

---

## 8. Development Phases
The build order, split into vertical milestones, is in [`implementation_roadmap.md`](implementation_roadmap.md). The table below is the original outline.

| Phase | Deliverable | Scope |
| :--- | :--- | :--- |
| **Phase 0** | **Verification spike** | Compose stack with Bugsink 2.6.2+ and sample apps. Confirm the `trace` tag holds the OpenTelemetry trace id that Tempo has, and where environment and release live in event `data`. Pin the OpenAPI schema. Findings written to `design/`. |
| **Phase 1** | **Scaffold & issue table** | `create-plugin` app with nested backend data source, config editor, health check, issue query → table frame. |
| **Phase 2** | **App UI (read-only)** | Issue list, issue detail, event list, event detail (stack trace, breadcrumbs, tags). |
| **Phase 3** | **Trace & log links** | Tempo and Loki internal links on event frames and the event detail page. |
| **Phase 4** | **Triage** | Triage actions through `CallResource`, role check, attribution comment, UI controls. |
| **Phase 5** | **Time series & alerting** | Event-count and new-issue series, alerting support, issue-list sparklines. |
| **Phase 6** | **Release** | Documentation (including the SDK trace-context setup), plugin signing, catalog submission, CI release pipeline. |

---

## 9. License
GNU General Public License, version 3 or later (`GPL-3.0-or-later`). A personal open source project of Ahmed Ismail.

---

## 10. Decisions and Open Questions

Decided on 2026-11-15:

| Question | Decision | Section |
| :--- | :--- | :--- |
| How the health check learns a token's capabilities | At least one read per capability the configured features need; with triage on, `issues:triage` is probed by a resolve on an issue id that cannot exist, which Bugsink 2.6.3 refuses for the capability before any lookup; with triage and attribution on, `issues:comment` is probed by a blank comment on that id, which Bugsink refuses twice over; both probes are free of side effects | §4.A |
| Config editor for a project-bound token | Discovers the project from `GET /projects/`; no project id field | §4.A |
| Event list order in the UI | Bugsink's arrival order, newest first; no re-sorting | §4.C |
| Events query for `trace:<id>` | Fans out in the data source: the issues, then their events | §4.C |
| An event whose trace is not in Tempo | The buttons show whenever a trace id exists; Tempo reports the missing trace | §4.D, §5 |
| Attribution comments | On by default (`attributionComments` absent or `null`), switched off per data source with `false`; the plugin always tries when it is on and Triage is on, and a token without `issues:comment` gets a warning after each action | §4.A, §4.E |
| Open-issue count for alert rules | Page through the issue list and cache; ask Bugsink upstream for a count | §6 |
| What the dashboard time range means for the issues query | Issues last seen within the range, both ends | §4.B |
| Links to Bugsink when Grafana reaches it by an internal address | An optional Browser URL setting | §4.A |
| What an issue row's short id opens in the app | The detail page; an icon beside it opens Bugsink. Dashboard frames keep their link to Bugsink | §4.D |
| An events query that names nothing to read | An empty table with an info notice, not an error, so the provisioned trace dashboard opens clean | §4.C |
| The dashboard time range for events | Not applied; an issue, an event id or a trace scopes the list | §4.C |
| When the issue summary cannot be read | Its message shows alone and the other sections are not shown | §4.D |
| How the App pages are built and get their data | Plain React with `@grafana/ui`; the issue list runs the data source's issues query | §3, §4.D |
| What the Event section shows when no exception has frames | Bugsink's own stack trace text as plain text, possibly with local variables; the structured view never shows locals | §4.C, §4.D |
| Where the Python setup stands on `traceparent` | The documented setup is `setup_propagator=False`, which keeps an incoming `traceparent`; the sample app uses it | §5 |
| Trace ids on event frames | Only where the query names them: the `trace:<id>` term or an event id lookup; an issue-only query has none and no links | §4.C |
| Which trace ids get links | Only 32 lowercase hexadecimal characters, in the resource and the frames, so no dead link and no raw value in a Tempo query | §4.C, §5 |
| How the logs query is rendered | In Go, per row, in one pass, into a `logs_query` field marked hidden; variables only inside double quotes; `$` and `[` in values written as `\u` escapes | §5 |
| Where the link range comes from | The backend; the browser copies `timeRange` to `range`; Explore applies its own range | §5 |
| Where Open trace and Open logs open | Explore in the same tab; Back from Explore can need more than one press: Explore adds its own history entries | §4.D |
| The JavaScript sample app | A Node server on port 8820 with `@sentry/node` and `openTelemetryIntegration()`; its transitive `sentry@0.49.0` (FSL-1.1-Apache-2.0) is accepted for the dev stack because the image is not published and the licence check covers what the plugin ships | §7 |
| The Message and an exception without a type | The message shows above the exception whenever it is not empty; an untyped exception is headed `Unknown exception` | §4.D |
| Where the event payload is parsed | In the backend, bounded, one normalised event for the browser and for M6 | §4.C |
| An `event` address that names another issue's event | An error under Event, not the event | §4.D |
| Who may triage | Org role exactly `Editor` or `Admin`, read from Grafana's plugin context | §4.E |
| Anonymous sessions and service accounts | They triage by their org role; the config editor and the README warn that anonymous Editor or Admin access lets anyone who can open Grafana triage | §4.E |
| Cross-site triage requests | Every triage route requires the header `X-Bugsink-Triage: 1`, which the app sends; without it the answer is 403 and nothing reaches Bugsink | §4.E |
| The triage switch | Off by default per data source, so existing data sources stay read-only and nothing new is probed | §4.A, §4.E |
| Where triage writes happen | Only in resource calls; `QueryData` has no write path, so alerting, public dashboards and reports cannot triage | §4.E |
| Redirects from Bugsink | Never followed, for reads and writes; reported as a Bugsink failure with the status, never with the `Location` | §3 |
| Bugsink's stack trace text | Shown by default; the `hideStacktraceText` switch drops it in the backend | §4.C |
| How `logs_query` is hidden | Through `hideFrom` on the field, so every table hides it without a dashboard override | §4.C |
| Comments in the logs query | A substituted variable inside a `#`, `//` or `/* */` comment outside strings is refused; a lone `/` is division | §4.A |
| End-to-end identities | Basic-auth Grafana users made by a setup project, because the stack's login form is disabled | §7 |
| Grafana RBAC plugin actions | Not built: a backend plugin on Grafana OSS cannot check an action for the caller without the `externalServiceAccounts` and `idForwarded` toggles (off by default, one organization only), a new dependency and a managed service account; assigning plugin roles to specific users needs Grafana Cloud or Enterprise. Revisit when an OSS backend plugin can check an action without extra toggles | §4.E |
| Whom the attribution comment names | The Grafana login from the plugin context, in double quotes, with every quotation mark made an apostrophe, control and separator characters replaced and a cut at 100 characters; `an anonymous user` unquoted for an empty login; never the name or the email field | §4.E |
| A comment that fails after the action | The action stands, no retry and no undo; 200 with the issue and a `warning` | §4.E |
| An unexpected answer to the `issues:comment` probe | The status stays OK with ` Bugsink answered HTTP <n> to a check for issues:comment, so actions may not be attributed.`, because attribution is optional and an older Bugsink must not turn a working data source red; a 2xx stays an error, because the probe may have written something | §4.A |
| Mute terms | 1 to 100 periods of minute, hour, day, week, month or year, a threshold of 1 to 1000000 events; a body of at most 1024 bytes holding exactly those fields, the numbers as JSON integers; the browser runs the same rules and messages live and the backend is the authority | §4.E |
| A release to resolve in | Only the latest or the next release, as Bugsink offers; no release picker | §4.E |
| History details | Parsed in the backend from the documented `metadata` keys into typed fields; a value of the wrong type is left empty; a `mute_until` entry without a valid threshold keeps no period; nothing else from `metadata` reaches the browser | §4.D |
| Code generated from the PolyForm Shield schema, compiled into the GPL plugin | Shipped without asking for permission; the owner accepted the risk on 2026-11-15. The generated client is confined to one package (`pkg/bugsink/internal/gen`), so a hand-written client can replace it if Bugsink B.V. objects | §9 |

Still open:
* None.
