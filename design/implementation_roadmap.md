# Implementation Roadmap: Bugsink App Plugin for Grafana

**Spec:** [`bugsink_grafana_plugin_spec.md`](bugsink_grafana_plugin_spec.md). Section references (§) point there.

This roadmap replaces the phase table in spec §8 as the build order. The spec's phases group work by
layer in places (Phase 2 is the whole read-only UI); the milestones below are vertical slices instead.

## How to read this

* A **milestone** is one slice through every layer it needs (Bugsink client, backend handler, frontend,
  tests, docs) and ends with something a user can do in Grafana that they could not do before.
* Each milestone has a **demo**: the steps that show it works on the Compose stack. The demo is the
  acceptance test and becomes an end-to-end test.
* Each milestone gets its own task-level plan (`design/plans/m<N>-<name>.md`), written when the milestone
  starts, not before: later plans depend on what M0 finds and on what earlier milestones taught us.
* A milestone merges only when it is releasable: nothing half-wired, no feature flag hiding broken code.

## Definition of done (every milestone)

* Go unit tests for new client and frame code; frontend unit tests for new components and logic.
* The demo runs as a `@grafana/plugin-e2e` test against the Compose stack in CI.
* The health check reports any token capability the new feature needs (§4.A).
* End-to-end tests that produce events stay under Bugsink's ingestion throttle (about 1000 events per 5 minutes, spec §7).
* User documentation and `CHANGELOG.md` cover the new feature.
* The provisioned sample dashboard and data source in the Compose stack exercise it.

## Milestones

| # | Milestone | A user can now… | Spec |
| :--- | :--- | :--- | :--- |
| M0 | Dev stack & verification spike | (developer) run Bugsink, Grafana, Tempo and Loki locally and produce a linked error | §7, §8 Phase 0 |
| M1 | Connect | add a Bugsink data source and see "Save & test" pass or explain why not | §3, §4.A |
| M2 | Issues in dashboards | put a table of Bugsink issues on a dashboard or in Explore | §4.B, §4.F |
| M3 | Issue list page | browse and filter issues in the App | §4.D |
| M4 | Issue detail & events | open an issue and see its events, tags and history | §4.B, §4.C, §4.D |
| M5 | Event detail | read an event's stack trace, breadcrumbs, tags and contexts | §4.C, §4.D |
| M6 | Trace & log links | jump from an event to its Tempo trace and Loki logs | §5 |
| M7 | Basic triage | resolve, reopen, mute and unmute from Grafana | §4.E |
| M8 | Full triage | use release-aware resolve, timed and threshold mutes, with attribution | §4.E |
| M9 | Event volume series | chart event volume and see sparklines in the issue list | §6, §4.D |
| M10 | Alerting | write Grafana alert rules on Bugsink data | §6 |
| M11 | Release | install the signed plugin from the Grafana catalog | §8 Phase 6 |

### M0: Dev stack & verification spike

No plugin code. It answers the questions the later milestones are built on.

* **Scope:** Compose stack (Bugsink 2.6.2+, Grafana, Tempo, Loki, OpenTelemetry Collector); the Python
  sample app with a Sentry SDK and OpenTelemetry sharing trace context, producing errors on demand;
  two seeded Bugsink projects and four API tokens (global read, global triage, project-bound read, no capabilities); the OpenAPI schema from
  `/api/canonical/0/schema/` pinned in the repo.
* **Questions to answer**, written to `design/phase0_findings.md`. All eleven are answered there
  (Q1 to Q11, with evidence and the spec edits they caused), so the list below is the original outline:
  * Does the `trace` tag hold the OpenTelemetry trace id that Tempo has?
  * Where do environment and release live in event `data`?
  * How does a client learn a token's capabilities (needed by the M1 health check)?
  * What does a project-bound token get from each endpoint the plugin uses?
  * Does the pinned schema match every endpoint and parameter named in §4.B, §4.C, §4.E and §6?
  * Does a Go client generate cleanly from the schema?
* **Demo:** `docker compose up`, trigger an error in the sample app, find the event in Bugsink and its
  trace in Tempo under the same trace id.
* **Exit:** every question has an answer; the spec is corrected where a finding contradicts it.
  Done: see [`phase0_findings.md`](phase0_findings.md).

### M1: Connect

The walking skeleton: every layer exists and is wired, with the smallest feature that proves it.

* **Scope:** `create-plugin` scaffold of the app with the nested backend data source; Go client
  generated with `oapi-codegen` v2.8.2 from the unmodified schema (`ogen` fails on it, findings Q11);
  config editor (base URL, API token in `secureJsonData`; for a project-bound token `GET /projects/`
  returns its project, which the health check names; there is no project id field, findings Q5); `CheckHealth`
  for URL and token, the projects the token can see, and the capabilities it lacks (no endpoint reports them, findings Q4:
  one read per capability the configured features need; nothing is said about write capabilities until M7 adds a triage setting); Compose stack mounts and provisions the plugin; CI
  (lint, unit tests, build, end-to-end); license headers and `README.md`.
* **Demo:** add the data source with a valid token and see success; with a wrong URL, a bad token and a
  token without capabilities, see a message that names the problem; with a project-bound token, see
  success.
* **Plugin ids:** `ahmedismail-bugsink-app` and `ahmedismail-bugsink-datasource`.

### M2: Issues in dashboards

Built; see [`plans/m2-issues-in-dashboards.md`](plans/m2-issues-in-dashboards.md).

* **Scope:** issues query in `QueryData` (`project`, `state`, `q`, `sort`, `order`) returning a table
  frame with the fields in §4.B plus the project name, a status and the issue id; pagination with the
  cursor from `next` (250 per page, no total count, at most 20 pages); dashboard time range as "last
  seen within the range", stopping at the range start when sorted by `last_seen` descending (`sort` and
  `order` always sent; `sort=digest_order` only with a project); a 15-second cache per instance; a
  project list resource; query editor with project picker (a project-bound token sees its one
  project), state picker, search field, sort and order; link from each row to the issue in Bugsink's
  UI, built from the optional Browser URL setting.
* **Demo:** the provisioned dashboard `Bugsink issues`: a table panel showing unresolved issues of
  `sample-python` seen in the last 24 hours, filtered with `environment:development` (the sample app's
  environment); the same query in Explore.

### M3: Issue list page

Built; see [`plans/m3-issue-list-page.md`](plans/m3-issue-list-page.md). The page is plain React, runs the data
source's issues query, reuses the query editor as its filter bar and keeps its state in the URL.

* **Scope:** App page with data source picker; issue list with the columns of §4.D except sparkline and
  actions; project, status and time-range filters; sort by last seen and event count; row link to
  Bugsink; empty, loading and error states.
* **Demo:** open the App, pick an instance, filter to open issues of one project, sort by event count.
* **Not yet:** the Actions column (M7) and the sparkline (M9).

### M4: Issue detail & events

Built; see [`plans/m4-issue-detail-and-events.md`](plans/m4-issue-detail-and-events.md). The detail page reads through
resource routes, and the events query type runs in the data source.

* **Scope:** issue detail page (summary, tag values with counts (`events:read`), history, and the issue's events in
  Bugsink's arrival order, newest first); events query type in the data source (the events of one issue with `q`,
  lookup by SDK event id, and the events of a trace through the issue search) returning a table frame, with no
  time range and with bounds that add notices; the short id in the list opens the detail page and an icon opens
  Bugsink; `CallResource` routes for the detail reads; a provisioned `Bugsink trace` dashboard.
* **Demo:** click an issue in the list, see its summary, tags and history, and a list of its events; an
  issues table panel on a dashboard filtered by `trace:<id>`, and an events table panel that shows the
  events of the same trace from one query. Both run as e2e tests.
* **Not yet:** event content. An event row shows ids and timestamps only.

### M5: Event detail

Built; see [`plans/m5-event-detail.md`](plans/m5-event-detail.md). The backend parses the payload and serves a
normalised event through `GET events/{id}`.

* **Scope:** Event section on the issue page, parsed from the Sentry payload in `data`: exception chain and stack
  trace (in-app frames open with source lines, others folded), breadcrumb timeline, tags and contexts, with bounds
  that add notices; Bugsink's `stacktrace_md` as preformatted plain text when no exception has frames; the newest
  event shown by default, any other chosen from the Events table with `event=` in the address.
* **Demo:** open an issue and read the latest event's stack trace and breadcrumbs; switch to an older
  event; open a message event whose payload has no exception frames and see Bugsink's stack trace text. Runs as
  an e2e test.
* **Watch for:** payloads from different SDKs (Python and JavaScript differ in frame fields), chained
  exceptions, events with no exception at all (messages), very large payloads.

### M6: Trace & log links

Built; see [`plans/m6-trace-log-links.md`](plans/m6-trace-log-links.md). Links open Explore in the same tab.

* **Scope:** config editor fields for the Tempo and Loki data sources, time window, logs query and
  service tag; `trace_id` and `logs_query` fields on event frames, with internal data links on `trace_id`
  for a `trace:<id>` or event id query (an issue-only query has none); **Open trace**, **Open logs** and
  **Open event in Bugsink** on the event detail, the first two hidden when the trace id is not 32
  lowercase hexadecimal characters or there is no target, and shown without checking Tempo; the Python
  sample app with `setup_propagator=False`; the JavaScript sample app, a Node server on port 8820, as the
  second SDK; documentation of the SDK trace-context prerequisite, the reverse link from a Tempo trace
  to its Bugsink events, events whose trace Tempo does not have, `traceparent` propagation and the
  logs query rules; the M5 leftovers (message with exceptions, `Unknown exception`).
* **Demo:** trigger an error in each sample app, open the event, land on the right trace in Tempo and on
  the matching log lines in Loki; a dashboard row of the provisioned `Bugsink trace` dashboard links to
  the trace's logs; the message captured in a new thread (a trace id Tempo does not have) shows the
  buttons and Tempo reports the trace as not found; a request with a `traceparent` header keeps its
  trace id with the documented setup; the data source without targets shows no buttons. An event
  with no trace id shows no buttons in unit tests only, because neither SDK sends such an event.

### M7: Basic triage

* **Scope:** `CallResource` routes for resolve, reopen, mute and unmute; the caller check in the backend
  (org role Editor or higher by default); triage on/off per data source; health check for
  `issues:triage`; inline Resolve and Mute on the issue list; the four actions on the issue detail;
  state shown from the issue Bugsink returns.
* **Demo:** as an Editor, resolve an issue from the list and reopen it from the detail page; as a Viewer,
  see no controls and get a refusal when calling the route directly; with triage switched off, the same.

### M8: Full triage

* **Scope:** resolve in latest and next release; mute for a period and until a volume threshold, with
  their forms; attribution comment when the token has `issues:comment` (on by default, can be switched off); Grafana RBAC
  plugin actions where available.
* **Demo:** mute an issue for two days, resolve another in the next release, and see a comment in Bugsink
  naming the Grafana user for each.

### M9: Event volume series

* **Scope:** time-series query types for event volume per project and per issue from `event-counts`;
  hour-boundary handling, the 90-day limit and re-bucketing to coarser intervals; the sparkline column in
  the issue list, one call per visible row, cached.
* **Demo:** a time-series panel of a project's event volume over seven days; the issue list shows a
  sparkline per row and stays responsive with a full page of issues.

### M10: Alerting

* **Scope:** `alerting: true`; new issues per interval from `first_seen`; current-value queries
  (open-issue count, issues seen in the last N minutes, event count of the current hour); documentation
  of the hourly-resolution limit. The open-issue count pages through the issue list (250 per page, no
  count field), capped at a fixed number of pages and cached for a short time; a count field is
  requested from Bugsink upstream.
* **Demo:** an alert rule that fires when a project's event count in the current hour passes a
  threshold, and one that fires when open issues exceed a number; both fire after the sample app
  produces errors.

### M11: Release

* **Scope:** complete user documentation; plugin signing; release pipeline in CI; plugin validator
  clean; catalog submission.
* **Demo:** install the signed build on a clean Grafana from the release artifact and repeat the M1 demo.

## Order and dependencies

```text
M0 → M1 → M2 → M3 → M4 → M5 → M6
                │              
                ├→ M7 → M8      (needs M3; M4 for the detail-page actions)
                └→ M9 → M10     (needs M2; M3 for the sparkline)
```

M0 to M5 are a fixed sequence. After M5 the three branches (links, triage, series) are independent and
can be reordered by what is most useful; the numbering is the suggested order, with links first because
correlation is the plugin's main reason to exist. A first catalog release is possible after M6, with
M7 to M10 following as minor versions.

## Licensing

* The repository is licensed under the GNU General Public License, version 3 or later
  (`GPL-3.0-or-later`), as a personal project of Ahmed Ismail.
* From M1 on: the SPDX identifier goes in `package.json`, and source files carry an
  `SPDX-License-Identifier: GPL-3.0-or-later` header.
* Dependencies must have GPLv3-compatible licenses. The Grafana SDKs and UI packages are Apache-2.0,
  which is compatible. M1 adds a dependency license check to CI. The check covers what the plugin ships;
  the dev-stack sample apps are outside it.
* Bugsink's OpenAPI schema is committed under Bugsink's own license, not the GPL: it lives in its own
  directory with a notice naming its source, version and license.

## Open decisions

All nine decisions are made on 2026-11-15 and are in the milestones above; spec §10 lists them. Among them: the client generated from Bugsink's PolyForm Shield schema ships without asking for permission (risk accepted by the owner).

| Decision | Needed by | Notes |
| :--- | :--- | :--- |
| `traceparent` propagation with the OTLP integration | M6 | Settled: `setup_propagator=False` keeps an incoming `traceparent`. Spec §10. |
