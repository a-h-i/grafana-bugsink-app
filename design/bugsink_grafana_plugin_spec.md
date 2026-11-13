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
* **Frontend (`@grafana/ui`, `@grafana/scenes`, React):**
  * App pages: issue list, issue detail, event detail, triage controls.
  * Config editor: Bugsink base URL, API token, project scope, Tempo and Loki data sources, link templates, triage settings.
  * Query editor: query type (issues, events) and their parameters.
* **Backend (`grafana-plugin-sdk-go`):**
  * Typed client generated from the pinned OpenAPI schema.
  * Follows Bugsink's `next` cursor URLs for pagination; never builds cursors itself.
  * Short-lived cache per (data source, request) to absorb dashboard refreshes and App page re-renders.
  * Converts responses into DataFrames and attaches data links (§5).
  * Serves time series from Bugsink's hourly event counts (§6).

---

## 4. Functional Requirements

### A. Authentication & Configuration
* **Token:** a Bugsink API token in `secureJsonData`. Recommended: a global service token with `projects:read`, `issues:read`, `events:read`, plus `issues:triage` and `issues:comment` when triage is enabled.
* **Project-bound tokens** are supported: a project-bound token cannot list projects, so the config editor asks for the project id instead.
* **Health check:** confirms the base URL, the token, and which configured features the token's capabilities allow (for example "triage enabled but token lacks `issues:triage`").

### B. Issue Queries
`GET /issues/` supports, as of 2.6.2:
* `project` (optional; omitted lists every project the token can see), `state` (`all`, `open`, `unresolved`, `resolved`, `muted`), `q` (plain-text title search plus `key:value` tag filters, e.g. `environment:production release:1.4.2`), `sort` (`digest_order`, `last_seen`, `digested_event_count`), `order`, `cursor`.
* **Time range:** "issues seen in the dashboard range". The plugin sorts by `last_seen` descending and stops paging once `last_seen` falls before the range start.
* **Filters** map directly onto `state` and `q`; the query editor offers a project picker, a state picker and a free search field.
* **Issue fields used:** `friendly_id`, `project`, `calculated_type`, `calculated_value`, `transaction`, `first_seen`, `last_seen`, `digested_event_count`, `stored_event_count`, `is_resolved`, `is_resolved_by_next_release`, `is_muted`, `unmute_after`.
* **Issue extras:** tag values with counts (`issues/{id}/tags/`), issue history (state changes with author names), hourly event counts (`issues/{id}/event-counts/`).

### C. Event Queries
* **Event list** for one issue (`issue` filter, `q` search, newest first): `id`, `event_id`, `timestamp`, `ingested_at`.
* **Lookup by SDK event id** (`events/lookup/?project=&event_id=`), for links that carry the Sentry event id.
* **Search by trace:** Bugsink indexes `contexts.trace.trace_id` as the tag `trace`, so `q=trace:<id>` finds the events of one trace. This allows the reverse link, from a Tempo trace to its Bugsink events.
* **Event detail:** the full Sentry event payload (`data`). The plugin reads the exception, stack frames, breadcrumbs, tags, contexts (`trace_id`), environment and release from it. The Sentry event format is a stable public protocol.
* **Stack trace display:** rendered from the structured frames in `data`. Bugsink's `stacktrace_md` is displayed as Markdown only as a fallback and is never parsed.

### D. App UI
* **Issue list:**
  * Columns: Type (`calculated_type`), Message (`calculated_value`), Transaction, Project, Events (`digested_event_count`), First seen, Last seen, Status, Actions.
  * Filters: project, status, time range. Sort: last seen, event count.
  * Row link to the issue in Bugsink's own UI.
  * Event-count sparkline per row from `event-counts` (one call per visible row; see §6).
* **Issue detail:** summary, event list, latest event shown by default.
* **Event detail:**
  * Exception message and formatted stack trace (in-app frames emphasized, others collapsible).
  * Breadcrumb timeline.
  * Tags and contexts (`environment`, `release`, `trace_id`, user-defined tags).
  * **Open trace** and **Open logs** buttons (§5), hidden when the event has no `trace_id` or the target data source is not configured.

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

* **Who may triage:** the backend checks the calling Grafana user before forwarding a triage call. The default is org role Editor or higher; Grafana RBAC plugin actions are used where available. Triage can be switched off per data source.
* **Attribution:** all calls run as the token's Bugsink user. When the token has `issues:comment`, the plugin adds an issue comment naming the Grafana user and the action (configurable).
* **Inline actions** on the issue list: Resolve and Mute. The other actions are in the issue detail page.
* The issue's state shown after an action comes from the issue object Bugsink returns.

### F. Dashboard Integration
* **Table:** issue rows and event rows, with the same links as the App pages.
* **Time series and alerting:** open, see §6.

---

## 5. Trace & Log Linking

### Prerequisite
Links only work when the `trace_id` in the Bugsink event matches the trace stored in Tempo. That holds when the application's Sentry SDK shares the OpenTelemetry trace context (for example, Sentry's OpenTelemetry integration or propagator). Otherwise the SDK uses its own trace id, which Tempo does not know. The plugin's documentation explains this setup; Phase 0 verifies it.

### Link construction
* Links are Grafana **internal data links** (target data source UID + query) on the event frames. Grafana builds the Explore URL, so the plugin does not hand-write URLs.
* **Time range:** the event's `timestamp` ± a configurable window (default 15 minutes), not "last hour".
* **Tempo:** a trace-by-ID query for `trace_id`.
* **Loki:** a configurable LogQL template. Default:
  ```text
  {service_name="${service}"} | trace_id="${trace_id}"
  ```
  * Template variables: `${trace_id}`, `${service}`, `${project}`, `${environment}`, `${release}`, `${tag:<name>}`.
  * `${service}` comes from a configurable source (an event tag, or the Bugsink project name; default: tag `service`, falling back to the project name).
  * The default uses structured metadata (`| trace_id=`). A line filter (`|= "${trace_id}"`) is a one-line template change for setups that only have the id in the log line.
* Links belong to events, not issues: `trace_id` is per event, and fetching the latest event for every issue row would cost one call per row.

---

## 6. Time Series & Alerting
Bugsink 2.6.2 keeps hourly event counters per installation, project and issue, updated at digest time, so they include events later evicted by retention.

* **Endpoints:** `GET /projects/{id}/event-counts/` and `GET /issues/{id}/event-counts/`, with `start` and `end` on UTC hour boundaries, at most 90 days back. The response holds one bucket per hour, zero-filled; the current hour is included and incomplete.
* **Series offered by the data source:**
  * Event volume per project, and per issue, from `event-counts` (hourly; the plugin re-buckets to coarser intervals, never finer).
  * New issues per interval, from `first_seen` in the issue list.
  * Current values for alert rules: open-issue count, issues seen in the last N minutes, event count of the current hour.
* **Alerting:** the data source declares `alerting: true` and serves these series to Grafana alert rules. Volume alerts work at hourly resolution: a rule can compare the current, incomplete hour against a threshold or the previous hours, but cannot react to a five-minute spike.
* **Beyond 90 days:** hourly counts are kept for a fixed 90 days, independent of event retention. Older history is the issue's lifetime totals (`digested_event_count`, `first_seen`, `last_seen`) plus the events retention has kept, which are a sample. The plugin does not build count series from stored events.
* **Left with Bugsink:** new-issue, regression and unmute notifications, which Bugsink sends itself.
* **Limits to raise with Bugsink upstream** (only if they hurt in practice):
  * Counts for many issues in one request: a dashboard of N issues costs N calls today.
  * Resolution finer than one hour.

## 7. Development & Test Environment
* **Docker Compose** stack: Bugsink, Grafana (plugin mounted), Tempo, Loki, and an OpenTelemetry Collector.
* **Sample apps** (one Python, one JavaScript) instrumented with a Sentry SDK pointed at Bugsink and OpenTelemetry pointed at the collector, sharing trace context. They produce errors on demand.
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

## 10. Open Questions
* Whether issue comments as triage attribution should be on by default.
