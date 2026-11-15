# M6: Trace & Log Links Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** From an event, a user opens its trace in Tempo and its log lines in Loki: with **Open trace** and **Open logs** on the issue page's Event section and with data links on the trace id of event frames, for events from the Python and the JavaScript sample apps; an incoming `traceparent` keeps its trace id with the documented Python setup.

**Architecture:** The data source gains five link settings. `pkg/plugin/links.go` turns them into Grafana internal data links (a Tempo `traceId` query and a rendered LogQL template) with a time range of the event's timestamp ± the window. Event frames carry a `trace_id` field with those links and a hidden per-row `logs_query`; the event resource answers the same links in `links`. The browser copies the range from the SDK's `timeRange` key to the `range` key Grafana reads, and the Event section turns each link into an Explore address with `mapInternalLinkToExplore`. A Node.js sample app on port 8820 is the second SDK.

**Tech Stack:** Go 1.26, `grafana-plugin-sdk-go` v0.299.0; React 19, `@grafana/ui`/`@grafana/data`/`@grafana/runtime` 13.2.4; Jest; `@grafana/plugin-e2e` 4.0.0; the Compose stack (Bugsink 2.6.3, Grafana 13.3.1, Tempo 3.1.1, Loki 3.7.10, collector 0.165.0, Sentry Python SDK 2.75.0). New: the JavaScript sample app (Node.js 22, `@sentry/node` 11, OpenTelemetry JS). No new plugin dependency.

**Spec:** [`../bugsink_grafana_plugin_spec.md`](../bugsink_grafana_plugin_spec.md) (§5, §4.A, §4.D event detail, §4.F, §7, §10 open question). **Roadmap:** [`../implementation_roadmap.md`](../implementation_roadmap.md) (M6, Definition of done, open decisions). **Findings:** [`../phase0_findings.md`](../phase0_findings.md) (Q1, Q2, Q3, Q7, Other findings). **Previous plans:** [`m5-event-detail.md`](m5-event-detail.md) and [`m4-issue-detail-and-events.md`](m4-issue-detail-and-events.md) (contracts and As built).

## Global Constraints

* The API token never reaches the browser, a log line, an error message, a frame or a link.
* The backend never builds a cursor and never requests a URL taken from a response.
* Resources never answer 401: Bugsink failures are 502, invalid settings and invalid input 400, a refused caller 403. Resource errors are JSON `{"message": …}`.
* Reuse what exists instead of parallel code: `userMessage`, the instance cache, `d.projects`/`d.projectNames`, the query model, `parseEvent`, the router of `routes()` with `canonical`, `useSection`, `tests/fixtures.ts`, `tests/seeded.ts`, `tests/reported.ts`, the `report-event` setup project.
* Links are Grafana internal data links (data source uid + query); the plugin never writes an Explore URL by hand (spec §5).
* Every string from an event payload is untrusted: React text only, never HTML, Markdown, an `href` or a `src`. In a LogQL template it is inserted escaped (decision 4).
* Generated code stays in `pkg/bugsink/internal/gen/` and is not regenerated. Do not modify anything in `.config/`. Do not change a plugin id or type; this plan changes no `plugin.json`. Provisioning changes need `docker compose restart grafana`.
* Every hand-written source file starts with `SPDX-License-Identifier: GPL-3.0-or-later` (JSON exempt); `scripts/check_spdx.sh` passes. Committed fixtures hold no local variables, private-network IPs, user-agent strings, host names, memory addresses or temp paths.
* Comments are one line and describe the code as it is. Tests assert behaviour, not code structure.
* Go commands run on `./pkg/...`: `node_modules` contains a Go package.
* Host ports: `3300` Grafana, `8800` Bugsink, `8810` Python sample app, `8820` JavaScript sample app.
* End-to-end tests produce at most 5 Bugsink events per run; this milestone keeps 5 (decision 10). Every `tests/*.setup.ts` sets `setup.describe.configure({ retries: 0 })`; events are reported only from setup files.
* Work on branch `m6/trace-log-links` in the worktree `.claude/worktrees/m6-trace-log-links` (from `main` at 8728bf0, `node_modules` installed). Every implementer uses this branch and worktree; push after every commit; never amend.
* The running stack may mount another checkout's `dist/` and provisioning. Before the first stack check, run `docker compose up -d --build --wait` from the worktree (Compose project `bugsink-grafana-plugin`; volumes are kept). Never `docker compose down`.
* Run every test command in the foreground.

## Facts already verified (2026-11-15, Compose stack)

* Tempo, Loki and the collector run; Grafana has data sources uid `tempo` and `loki`. The Python app's `GET /handled?variant=m6-plan-probe` returned trace `19587cc4a34ca86466a4edd5b2ab198b`; Bugsink's event `63d927b0-…` has that `contexts.trace.trace_id`, tag `service: sample-python`, and `GET resources/events/<id>` answers `traceId`, `timestamp`, `project`, `environment`, `release`.
* `POST /api/ds/query` on `tempo` with `{queryType: "traceId", query: "<id>"}`: 200, frame `Trace` with 7 spans of `sample-python`, also with a range two days in the past; an unknown id: status 500, error `trace with id 00000000000000000000000000abcdef not found in the selected time range [...]; it may exist outside this range`. `queryType: "traceql"` with a bare id: 500 `invalid TraceQL query`.
* `POST /api/ds/query` on `loki` with `{queryType: "range", expr: "{service_name=\"sample-python\"} | trace_id=\"<id>\""}` over the last 15 minutes: one line.
* Grafana's Tempo plugin (module 13.4.2) builds its own trace links as `internal: {query: {query: "${__value.raw}", queryType: "traceId"}, datasourceUid, datasourceName}` and hides helper fields with `config.custom.hidden: true`.
* SDK v0.299.0 `data.InternalDataLink{Query any, DatasourceUID, DatasourceName, ExplorePanelsState, Transformations, Range *TimeRange}`; `Range` is serialised as `timeRange` (`{from, to}` RFC 3339). Grafana 13.3.1's panel link supplier passes `range: link.internal.range` to `mapInternalLinkToExplore` and no bundle reads `internal.timeRange`; Explore's supplier passes Explore's own range.
* `@grafana/data` 13.2.4 exports `mapInternalLinkToExplore(options: {link, internalLink, scopedVars, range?, field, replaceVariables})`; it interpolates the query and `panelsState`, never the range, and its href is `/explore?left=<state>` with `range: toURLRange(range.raw)` only when `range.raw` is set.
* `@grafana/runtime` 13.2.4 exports `DataSourcePicker` with props `current` (uid or ref), `onChange(ds)`, `onClear`, `type`, `noDefault`, `inputId`, `width`.
* Event list items have no payload (`{id, ingested_at, digested_at, issue, grouping, event_id, project, timestamp, digest_order}`); `events/lookup/` answers the same body as `events/{id}/`, `data` included (M4/M5 facts).
* `traceparent`, in the `sample-python` container with sentry-sdk 2.75.0 (`OTLPIntegration(setup_otlp_traces_exporter=True, collector_url=None, setup_propagator=True, capture_exceptions=False)`), a throwaway server behind `OpenTelemetryMiddleware`, header `00-0af7651916cd43dd8448eb211c80319c-…-01`: with `setup_propagator=True` the span and the event got a fresh id (`caef564d…`, both equal); with `setup_propagator=False` both got `0af7651916cd43dd8448eb211c80319c`; without the header both got equal fresh ids in both modes. The running app (default) ignored the header on `/handled` (`6e2d887e…`).
* `@sentry/node` 11.12.0 (2026-11-10; engines `>=22.12.0 <23` among others): `enableOpenTelemetrySetup` defaults to false; `openTelemetryIntegration()` attaches events to the active OpenTelemetry span, exports no spans and leaves propagation to the OpenTelemetry propagator. Bugsink project 2 is `sample-js`, DSN key `2`×32.
* CI runs `scripts/verify_stack.py` before `npm run e2e`. A run reports today: `/handled?variant=m2-e2e`, `/handled?variant=m4-trace`, `/chained?variant=m5-chained` twice, `/message?variant=m5-message`.
* **Not verified:** (a) the JavaScript app's event carries the OpenTelemetry trace id, Tempo and Loki hold its spans and log line, and its payload shape; (b) `/message` captured in a new thread still gets a trace id (absent from Tempo) and its thread frames; Task 1 Step 1 checks both first. (c) A dashboard table shows both links in a menu on a `trace_id` cell, keeps `${__data.fields.logs_query}` unescaped and hides `logs_query`; Task 6 checks (c) in its first test. No grafana.com page documents internal links built in a backend; the facts above come from the SDK source and Grafana's bundles.

## Decisions in this plan, for review

1. **Settings** in `jsonData`: `tempoDatasourceUid`, `lokiDatasourceUid` (empty: that link is not offered), `linkWindowMinutes` (whole minutes 1–1440, absent means 15), `logsQuery` (empty means the spec's default), `serviceTag` (absent means `service`; empty means the project name only). No secret, so no `secureJsonData`.
2. **Trace ids on frames only where the query names them:** the `trace:<id>` term's id for every row of a query with one, the payload's id for an event-id lookup (`LookupEvent` now parses the payload it already receives); an issue-only query leaves `trace_id` empty. Reading each listed event would cost one call per row (spec §5's rule).
3. **Links on `trace_id` only when every row has a trace id** and at least one target is set, so no frame offers a dead link.
4. **The LogQL template is rendered in Go, per row**, into a hidden `logs_query` field (`config.custom.hidden`); the frame's Loki link query is `${__data.fields.logs_query}`, because one trace's rows can belong to different projects. Values are escaped for a LogQL double-quoted string (`\` and `"`); `${service}` is the `serviceTag` tag, else the project name; variables a row does not know (`${environment}`, `${release}`, `${tag:…}` outside a lookup) render empty; an unknown `${…}` stays as written; one pass, so a value containing `${trace_id}` is not expanded.
5. **Link range:** the backend computes it (the event's timestamp ± window; for a frame, its first to last timestamp ± window) and sends it in the SDK's `timeRange`; the browser copies it to `internal.range`, the key Grafana reads. In Explore, Grafana applies Explore's own range to data links; the README says so.
6. **Tempo link query** `{refId: "A", queryType: "traceId", query: <id>}`, as Grafana's own trace links; Loki `{refId: "A", queryType: "range", expr: <LogQL>}`. Titles `Open trace`, `Open logs`.
7. **Event section actions:** `Open trace` and `Open logs` open Explore in the same tab; `Open event in Bugsink` (new tab, `bugsinkUrl`) closes the M5 leftover. The plugin does not ask Tempo whether the trace exists (spec §10).
8. **The documented Python setup sets `setup_propagator=False`** (verified above), and the sample app does; this settles the spec §10 open question.
9. **The JavaScript sample app is a Node.js server** (`node:http`, `@sentry/node` 11 with `openTelemetryIntegration()`, its own OpenTelemetry tracer and logger providers): the same shape as the Python setup, no bundler. Its handled error has a `cause`, so the second SDK's chained exceptions, `colno` and display order are exercised.
10. **E2E events per run (5):** `/handled?variant=m2-e2e` with a `traceparent` header (replaces both the M2 event and the M4 trace event; `report-event.setup.ts` goes); `/chained?variant=m5-chained` twice; `/message?variant=m5-message`, now captured outside any span so it is also the "trace Tempo does not have" example; JavaScript `/handled?variant=m6-js`.
11. **M5 leftovers:** `Message` shows whenever the message is not empty, above `Exception`; an exception with an empty type is headed `Unknown exception`; a breadcrumb time without a zone is read as UTC (the protocol's zone); the Event section shows `Loading event...` while the summary loads; the duplicate-key test asserts `console.error`.
12. **Health check:** no new capability (links use `events:read`, already probed); a bad window is a settings message. Provisioned `Bugsink` gets both targets, `Bugsink project-bound` none (the e2e case for hidden buttons). Neither SDK sends an event without a trace id, so the demo's "no `trace_id`, no buttons" is pinned by unit tests (Task 3, Task 5) and the e2e shows hidden buttons through the data source without targets.

## Settings contract

| Key | Editor field (input id) | Default | Invalid |
| :--- | :--- | :--- | :--- |
| `tempoDatasourceUid` | `Tempo data source`, `DataSourcePicker type="tempo"` (`bugsink-tempo`) | none | not checked |
| `lokiDatasourceUid` | `Loki data source`, `type="loki"` (`bugsink-loki`) | none | not checked |
| `linkWindowMinutes` | `Time window`, number, suffix `minutes` (`bugsink-link-window`) | 15 (absent or `null`) | `The link time window must be a whole number of minutes from 1 to 1440.` (health check error; queries and resources 400, as every settings message) |
| `logsQuery` | `Logs query` (`bugsink-logs-query`), placeholder the default | `{service_name="${service}"} \| trace_id="${trace_id}"` | not checked |
| `serviceTag` | `Service tag` (`bugsink-service-tag`) | `service` | not checked |

Fields sit under an h3 `Trace and log links`, after the API token. Descriptions: Tempo `Open trace runs a trace ID query here.`; Loki `Open logs runs the logs query here.`; window `Links open this many minutes before and after the event.`; logs query `LogQL for Open logs. Variables: ${trace_id}, ${service}, ${project}, ${environment}, ${release}, ${tag:<name>}. Leave empty for the default.`; service tag `The event tag that holds the service name for ${service}. When it is empty or the event lacks the tag, the Bugsink project name is used.`

## Frame and resource contract

* Events frame (M4 contract otherwise unchanged): `trace_id` (display `Trace ID`, string) after `ingested_at`; `logs_query` (no display name, string, `config.custom.hidden: true`) last. Every row has both, `""` when unknown.
* Links on `trace_id` (decision 3), in this order: `Open trace` → Tempo, query `${__value.raw}`; `Open logs` → Loki, expr `${__data.fields.logs_query}`; both with `timeRange` per decision 5. Not `TargetBlank`.
* `GET events/{id}` gains `links`: `[{"title": "Open trace", "internal": {"query": {"refId":"A","queryType":"traceId","query":"<traceId>"}, "datasourceUid": "tempo", "timeRange": {"from": "<timestamp − window>", "to": "<timestamp + window>"}}}, {"title": "Open logs", …}]`; `[]` without a trace id or a target.

## Review Focus

1. A tag value, project name or release with `"`, `\` or `"} |= "` cannot break out of the LogQL string, and a value holding `${trace_id}` is not expanded (Task 2 test).
2. An event older than an hour: Open logs carries the event's time ± window, not Explore's default hour, both on the Event section and on a dashboard link (Task 3 and Task 4 tests).
3. Settings saved before M6 (no link keys) keep every query and the health check working, with defaults; `linkWindowMinutes` of `0`, `1441`, `2.5` or `"15"` gives the window message, not `The data source settings are invalid.` (Task 2 test).
4. No dead links: an issue-only events query, a frame with one row lacking a trace id, an event without a trace id and a data source without targets offer no link and no button (Task 3 and Task 5 tests).
5. A trace across two projects: each row's `logs_query` names its own project's service (Task 3 test).

## File structure

| File | Responsibility |
| :--- | :--- |
| `sample-apps/javascript/` (new) | `app.mjs`, `package.json`, `package-lock.json`, `Dockerfile`, `.dockerignore` |
| `sample-apps/python/app.py`, `docker-compose.yaml`, `scripts/verify_stack.py` | `setup_propagator=False`, `/message` outside any span, the `sample-js` service, JavaScript and `traceparent` checks |
| `pkg/bugsink/eventdetail.go`, `events.go`, `testdata/events/node-handled.json` (new) | `LookupEvent` parses the payload, zone-less times, the Node.js fixture |
| `pkg/plugin/links.go` (new), `datasource.go` | link settings, template rendering, internal links |
| `pkg/plugin/events.go`, `frames.go`, `resources.go`, `query.go` | trace ids on rows, the frame fields and links, `links` on the event |
| `src/datasource/types.ts`, `links.ts` (new), `datasource.ts`, `components/ConfigEditor.tsx` | settings types and fields, link ranges, Explore hrefs |
| `src/pages/event/EventDetailView.tsx`, `StackTrace.tsx`, `src/pages/IssuePage.tsx` | the buttons and the M5 leftovers |
| `docker/grafana/provisioning/datasources/datasources.yaml` | the provisioned targets (Task 3) |
| `tests/*` | the setup events, the demo end to end |

---

### Task 1: The JavaScript sample app and the Python propagation setup

**Files:**
- Create: `sample-apps/javascript/{app.mjs,package.json,package-lock.json,Dockerfile,.dockerignore}`, `pkg/bugsink/testdata/events/node-handled.json`
- Modify: `sample-apps/python/app.py`, `docker-compose.yaml`, `scripts/verify_stack.py`, `pkg/bugsink/eventdetail_test.go`

**Interfaces:**
- Produces: service `sample-js` on `127.0.0.1:${SAMPLE_JS_PORT:-8820}` → container 8000, env `SENTRY_DSN=http://22222222222222222222222222222222@bugsink:8000/2`, `OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318`, health check `node -e` fetching `/healthz`, `depends_on` like `sample-python`. Endpoints: `GET /healthz` → `{"status":"ok"}`; `GET /handled?variant=` → inside a server span continued from the request headers (W3C `traceparent`): an OpenTelemetry log record `handling a request that fails with a handled error`, breadcrumb `{category: "app", message: "about to fail"}`, `captureException(new Error(message("handled failure", variant), { cause: new TypeError("order total is not a number") }))`, answers `{"trace_id", "event_id"}` (32 hex). `message()` writes `(variant v<variant>)` as the Python app does. Sentry: environment `development`, release `sample-js@0.1.0`, tag `service: sample-js`, `integrations: [Sentry.openTelemetryIntegration()]`, no `tracesSampleRate`. OpenTelemetry: resource `service.name=sample-js`, `deployment.environment=development`, `service.version=0.1.0`; `NodeTracerProvider` with `BatchSpanProcessor(new OTLPTraceExporter())`, registered; a `LoggerProvider` with `BatchLogRecordProcessor(new OTLPLogExporter())` set as the global logger provider. Exact dependency versions in the lock file, installed with `npm ci --omit=dev` on a pinned `node:22.x-slim` image.
- Python: `OTLPIntegration(setup_otlp_traces_exporter=False, setup_propagator=False)`; `GET /message` runs `capture_message` in a new `threading.Thread` (as `/no-trace` does) and still returns `{event_id}`.

- [ ] **Step 1: Check the unverified facts first:** `docker compose up -d --build --wait sample-js sample-python`. `curl 'http://localhost:8820/handled?variant=m6-plan'`; with the global read token, `events/lookup/?project=2&event_id=<id>` (retry for the digest) has `contexts.trace.trace_id` equal to the answer's `trace_id`, `platform` `node`, two exceptions; `tempo/api/traces/<id>` through the Grafana proxy has spans of `sample-js`; `{service_name="sample-js"} | trace_id="<id>"` finds the log line. `curl 'http://localhost:8810/message?variant=m6-plan'`: its event has a trace id Tempo does not have and `threads` frames. If any fails, stop and report.
- [ ] **Step 2: Write the failing tests** in `eventdetail_test.go`: `TestParseNodeHandled` on `node-handled.json`: `Platform` `node`; two exceptions in protocol order, `TypeError` (`order total is not a number`) then `Error`; the in-app frame of `app.mjs` has `Lineno`, `Colno` and a `ContextLine`; `Environment` `development`, `Release` `sample-js@0.1.0`, a 32-hex `TraceID`; tag `service` `sample-js`; breadcrumb `about to fail` with a time. `TestFixturesAreScrubbed`: no fixture file contains `server_name`, an IPv4 address, `/tmp/`, `/home/`, `0x` followed by 8 hex digits or `Mozilla/`. Run `go test ./pkg/bugsink/...`. Expected: FAIL, the fixture does not exist.
- [ ] **Step 3: Capture the fixture:** save Step 1's JavaScript event answer as `node-handled.json`, removing `data.server_name`, `data.modules`, `data.contexts.{os,device,culture,cloud_resource}`, `data.request`, `data.user` and frame `vars`. Run `go test ./pkg/bugsink/...`. Expected: PASS.
- [ ] **Step 4: `verify_stack.py`:** keep the six checks; add `traceparent kept` (Python `/handled` with a random `traceparent` answers that trace id) and, for `sample-js`, `/handled`, its Bugsink event's trace id (project 2), Tempo spans and Loki lines (`service_name="sample-js"`), reusing the existing helpers with the sample URL, project and service as parameters. `python3 scripts/verify_stack.py` prints eleven `ok` lines.
- [ ] **Step 5: Run** `go test -race ./pkg/...`, `scripts/check_spdx.sh`, and `npm run e2e` (M5's message test still passes). Expected: PASS.
- [ ] **Step 6: Commit** `Add the JavaScript sample app and honour traceparent in the Python app` and push.

### Task 2: Link settings, the LogQL template and the event lookup payload

**Files:**
- Create: `pkg/plugin/links.go`, `pkg/plugin/links_test.go`
- Modify: `pkg/plugin/datasource.go`, `pkg/plugin/datasource_test.go`, `pkg/bugsink/events.go`, `pkg/bugsink/events_test.go`, `pkg/bugsink/eventdetail.go`, `pkg/bugsink/eventdetail_test.go`

**Interfaces:**
- Consumes: `bugsink.EventDetail`, `bugsink.KeyValue`, `parseEvent`.
- Produces:

```go
const (defaultLinkWindow = 15 * time.Minute; defaultLogsQuery = `{service_name="${service}"} | trace_id="${trace_id}"`; defaultServiceTag = "service")
const msgLinkWindow = "The link time window must be a whole number of minutes from 1 to 1440."
type linkSettings struct {
	tempoUID, lokiUID string
	window            time.Duration
	logsQuery         string
	serviceTag        string // empty uses the project name
}
type logsVars struct {
	TraceID, Service, Project, Environment, Release string
	Tags                                            []bugsink.KeyValue
}
func parseLinkSettings(jsonData json.RawMessage) (s linkSettings, invalid string) // defaults applied; invalid is msgLinkWindow or empty
func renderLogsQuery(template string, v logsVars) string                      // decision 4
func (s linkSettings) vars(traceID, project string, e *bugsink.EventDetail) logsVars // e nil: only trace id and project known
func (s linkSettings) exploreLinks(traceQuery, logsExpr string, from, to time.Time) []data.DataLink // Tempo then Loki, each only with its uid
// Datasource gains `links linkSettings`, set by configure after the browser URL check.
func (c *Client) LookupEvent(ctx context.Context, projectID int, eventID uuid.UUID) (EventDetail, error) // was Event; parses data like GetEvent
```

- [ ] **Step 1: Write failing tests.**
  - `links_test.go` `TestParseLinkSettings`: `{}` gives no uids, 15 minutes, the default query, tag `service`; all five keys set are read; `"serviceTag": ""` gives an empty tag; `linkWindowMinutes` `0`, `1441`, `2.5`, `"15"` give `msgLinkWindow`; `null` gives 15.
  - `TestRenderLogsQuery`: the default with trace `abc` and service `api` is `{service_name="api"} | trace_id="abc"`; every variable and `${tag:region}` (first tag with that key) render; a missing tag, environment and release render empty; `${foo}` stays; service `a"} |= "x\` renders `a\"} |= \"x\\`; a project named `${trace_id}` renders literally.
  - `TestLinkVars`: with tag `service` present it is the service; without it, or with `serviceTag` empty, the project name is; nil detail leaves environment, release and tags empty.
  - `TestExploreLinks`: both uids give `Open trace` with `Internal.DatasourceUID` `tempo`, query `{"refId":"A","queryType":"traceId","query":<traceQuery>}`, then `Open logs` with `{"refId":"A","queryType":"range","expr":<logsExpr>}`; both carry `Range{from, to}`; only one uid gives one link; none gives none.
  - `datasource_test.go`: the M1/M2 settings cases still pass; window `0` makes the health check answer `msgLinkWindow`; settings with no link keys keep `CheckHealth` green.
  - `events_test.go` `TestLookupEventParsesPayload`: the lookup answer of the `python-handled` fixture gives `TraceID`, `Tags`, `Environment` and the `Event` fields.
  - `eventdetail_test.go` `TestParseBreadcrumbTimeWithoutZone`: `"2026-11-15T07:52:59.387"` gives `2026-11-15T07:52:59.387Z`; RFC 3339 and epoch cases unchanged.
- [ ] **Step 2: Run** `go test -race ./pkg/...`. Expected: FAIL, undefined `parseLinkSettings`, `renderLogsQuery`.
- [ ] **Step 3: Implement.** Render with one `regexp.ReplaceAllStringFunc` over `\$\{([^}]*)\}`. `eventByID` keeps compiling by reading `.Event` of the lookup until Task 3.
- [ ] **Step 4: Run** `go test -race ./pkg/...`, `go vet ./pkg/...`. Expected: PASS.
- [ ] **Step 5: Commit** `Add trace and log link settings and render the logs query` and push.

### Task 3: Links on event frames and on the event resource

**Files:**
- Modify: `pkg/plugin/events.go`, `pkg/plugin/events_test.go`, `pkg/plugin/frames.go`, `pkg/plugin/frames_test.go`, `pkg/plugin/query.go`, `pkg/plugin/query_test.go`, `pkg/plugin/resources.go`, `pkg/plugin/resources_test.go`, `docker/grafana/provisioning/datasources/datasources.yaml`

**Interfaces:**
- Consumes: Task 2's `linkSettings`, `vars`, `renderLogsQuery`, `exploreLinks`, `LookupEvent`.
- Produces:

```go
type eventRow struct {
	event   bugsink.Event
	issue   string               // the short id
	traceID string               // from the query's trace term or the payload; empty when unknown
	detail  *bugsink.EventDetail // set for an event id lookup
}
func traceIDOf(query string) string // the id of the first trace:<id> term, or empty
func eventsFrame(rows []eventRow, projectNames map[int]string, browserURL string, notices []string, links linkSettings) *data.Frame
func eventJSON(e bugsink.EventDetail, project, browserURL string, links linkSettings) eventBody // adds Links []data.DataLink `json:"links"`
```

- [ ] **Step 1: Write failing tests.**
  - `events_test.go`: `traceIDOf` of `trace:abc`, `x trace:abc trace:def`, `notrace:abc`, `trace:` gives `abc`, `abc`, `""`, `""`; a trace query's rows all carry its id; an issue query with `q=trace:abc` too; an issue query without one carries `""`; an event-id lookup carries the payload's id and its detail, with no extra Bugsink request (two requests: lookup and issue).
  - `frames_test.go` `TestEventsFrameLinks`: fields end `…, ingested_at, trace_id, id, issue_id, project_id, logs_query`; `trace_id` display `Trace ID`; `logs_query` has `Custom["hidden"] == true`; rows from two projects (`sample-python`, `sample-js`) under one trace get `{service_name="sample-python"} | trace_id="t"` and `{service_name="sample-js"} | trace_id="t"`; the links equal Task 2's contract with `${__value.raw}` and `${__data.fields.logs_query}`; timestamps 10:00 and 10:05 with a 15-minute window give `Range` 09:45 to 10:20; one row with `""`, no rows, or no uids give no links.
  - `query_test.go`: an events query through `QueryData` with the trace term returns those fields and links (settings with both uids).
  - `resources_test.go` `TestEventResourceLinks`: from `python-unhandled` with both uids, `links` holds both entries with the fixture's trace id, the rendered default query for `sample-python` and `timeRange` of its timestamp ± 15 minutes; without uids, and for `message-only` (no trace id), `links` is `[]`, not `null`.
- [ ] **Step 2: Run** `go test -race ./pkg/plugin/...`. Expected: FAIL.
- [ ] **Step 3: Implement.** The frame range is the rows' earliest and latest `timestamp` ± `links.window`; a lookup row's vars use its detail, other rows `vars(traceID, projectName, nil)`.
- [ ] **Step 4: Run** `go test -race ./pkg/...`, `go vet ./pkg/...`. Expected: PASS.
- [ ] **Step 5: Check in the stack:** `npm run build:backend`; add `tempoDatasourceUid: tempo` and `lokiDatasourceUid: loki` to the `Bugsink` entry of `datasources.yaml` (Task 6 keeps them); `docker compose restart grafana`; the events query for `trace:19587cc4a34ca86466a4edd5b2ab198b` on `bugsink` returns `trace_id` with two links whose `internal.timeRange` brackets the event; `GET resources/events/63d927b0-00ff-4eb1-bcc4-cedcb4678c84` has two `links`; the same on `bugsink-project` has `[]`.
- [ ] **Step 6: Commit** `Link event frames and events to Tempo and Loki` and push.

### Task 4: Link settings in the config editor and link ranges in the browser

**Files:**
- Create: `src/datasource/links.ts`, `src/datasource/links.test.ts`
- Modify: `src/datasource/types.ts`, `src/datasource/datasource.ts`, `src/datasource/datasource.test.ts`, `src/datasource/components/ConfigEditor.tsx`, `src/datasource/components/ConfigEditor.test.tsx`

**Interfaces:**
- Consumes: the settings and the frame and resource contracts.
- Produces:

```ts
export interface BugsinkOptions extends DataSourceJsonData { browserUrl?: string; tempoDatasourceUid?: string; lokiDatasourceUid?: string; linkWindowMinutes?: number; logsQuery?: string; serviceTag?: string }
export interface ExploreLink { title: string; internal: { datasourceUid: string; query: Record<string, unknown>; timeRange: { from: string; to: string } } }
// EventDetail gains links: ExploreLink[]
// links.ts
export function linkRange(timeRange: { from: string; to: string }): TimeRange // dateTime from, to and raw
export function withLinkRanges(frames: DataFrame[]): DataFrame[] // every field link with internal.timeRange gains internal.range
export function exploreHref(link: ExploreLink): string // mapInternalLinkToExplore, range from timeRange, replaceVariables identity
// BugsinkDataSource
query(request: DataQueryRequest<BugsinkQuery>): Observable<DataQueryResponse> // super.query, data mapped with withLinkRanges
```

- [ ] **Step 1: Write failing tests.**
  - `links.test.ts`: `withLinkRanges` on a frame whose `trace_id` links carry `timeRange` `2026-11-15T09:45:00Z`/`10:20:00Z` sets `internal.range.raw.from.valueOf()` to that instant, leaves links without `timeRange` and other fields untouched, and does not mutate its input; `exploreHref` of an `Open logs` link starts with `/explore?left=`, and its decoded state has `datasource: 'loki'`, `queries[0].expr` verbatim (quotes and `|` intact) and `range` `{from: '<ms>', to: '<ms>'}` of the two instants.
  - `datasource.test.ts`: `query` passes frames through `withLinkRanges` (a mocked backend answer with a `timeRange` link comes out with `range`); `listEvents` still returns its rows.
  - `ConfigEditor.test.tsx` (mock `DataSourcePicker` from `@grafana/runtime` as a labelled stub): heading `Trace and log links`; the five fields of the settings contract with their labels, descriptions and defaults (`15`, `service`, empty query with the default as placeholder); picking Tempo and Loki stores their uids; clearing a picker removes the uid; typing `30` in `Time window` stores the number 30, clearing it removes the key; editing `Service tag` to empty stores `""`; other options unchanged in every call.
- [ ] **Step 2: Run** `npm run test:ci`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `npm run test:ci`, `npm run typecheck`, `npm run lint`. Expected: PASS, Jest exits by itself.
- [ ] **Step 5: Commit** `Add the link settings to the config editor and carry link ranges to Grafana` and push.

### Task 5: Open trace, Open logs and Open in Bugsink on the Event section; M5 leftovers

**Files:**
- Modify: `src/pages/event/EventDetailView.tsx`, `EventDetailView.test.tsx`, `src/pages/event/StackTrace.tsx`, `StackTrace.test.tsx`, `src/pages/IssuePage.tsx`, `src/pages/IssuePage.test.tsx`

**Interfaces:**
- Consumes: Task 4's `ExploreLink`, `exploreHref`, `EventDetail.links`.
- Produces: after the event's rows, a `Stack` of `LinkButton`s (variant secondary): one per `event.links` entry, named by its title, `href={exploreHref(link)}`, same tab; then `Open event in Bugsink` (`event.bugsinkUrl`, `target="_blank"`, `rel="noreferrer"`, icon `external-link-alt`). h3 `Message` before h3 `Exception` whenever `message` is not empty. h4 `Unknown exception` for an empty type. On the issue page the Event section is pending while the event has loaded and the summary has not.

- [ ] **Step 1: Write failing tests.**
  - `EventDetailView.test.tsx`: two links render `Open trace` and `Open logs` with `/explore?left=` hrefs and no `target`; `links: []` renders neither; `Open event in Bugsink` has `bugsinkUrl`, `target="_blank"`, `rel="noreferrer"`; an event with exceptions and message `order 42 failed` shows h3 `Message` with the text before h3 `Exception`; the duplicate-notice test spies on `console.error` and expects no call mentioning `same key` (it fails when notices are keyed by text).
  - `StackTrace.test.tsx`: an exception with `type: ''` and value `boom` shows h4 `Unknown exception` and `boom`.
  - `IssuePage.test.tsx`: `getEvent` resolves while `getIssue` is pending: the Event section shows `Loading event...`; once `getIssue` resolves the event shows.
- [ ] **Step 2: Run** `npm run test:ci`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `npm run test:ci`, `npm run typecheck`, `npm run lint`. Expected: PASS.
- [ ] **Step 5: Check in the stack:** `npm run build`, `docker compose restart grafana`; on the issue page of the probe event `63d927b0-…`, `Open logs` opens Explore on Loki with the rendered query and the event's range, showing one line.
- [ ] **Step 6: Commit** `Open the event's trace and logs from the issue page` and push.

### Task 6: Provisioned targets and end-to-end tests of the demo

**Files:**
- Create: `tests/report-js.setup.ts`, `tests/trace-links.spec.ts`
- Delete: `tests/report-event.setup.ts`
- Modify: `tests/report-trace.setup.ts`, `tests/seeded.ts`, `tests/fixtures.ts`, `tests/events-query.spec.ts`, `tests/datasource-config.spec.ts`, `.gitignore` (`/playwright/.js-event.json`)

**Interfaces:**
- Consumes: everything above, built into `dist/`; the sample apps of Task 1.
- Produces: relies on Task 3's provisioning (`Bugsink` with `tempoDatasourceUid: tempo`, `lokiDatasourceUid: loki`; `Bugsink project-bound` without them). `tests/seeded.ts`: `SAMPLE_JS_URL` (`process.env.SAMPLE_JS_URL || 'http://localhost:8820'`), `JS_MARKER = 'm6-js'`; `TRACE_MARKER` removed (its uses read `EVENT_MARKER`). `report-trace.setup.ts` sends `GET /handled?variant=m2-e2e` with `traceparent: 00-<random 32 hex>-<random 16 hex>-01`, expects the answer's `trace_id` to equal the header's, writes `trace` `{traceId, eventId}`. `report-js.setup.ts` writes `js-event` `{traceId, eventId}` from `GET ${SAMPLE_JS_URL}/handled?variant=m6-js`. `openFirstIssue(page, name = /^SAMPLE-PYTHON-/)`.

- [ ] **Step 1: Write the setups and tests** (read `.config/AGENTS/e2e-testing.md` first; digest waits retry with `toPass()`; the Event section is identified by its `Trace ID` row equal to the reported id, reloading until it is):
  - `a dashboard row links to the trace's logs` (first, settles fact (c)): `bugsink-trace` with `var-trace=<traceId>`; click the `Trace ID` cell of `Events in this trace`; the menu offers `Open trace` and `Open logs`; `Open logs` lands on Explore showing `handling a request that fails with a handled error`; no `Logs query` column header is visible.
  - `a traceparent reaches the event, and Open trace and Open logs land on it`: issue list `q=m2-e2e`, `openFirstIssue`; Event shows `Trace ID` = the header's id; `Open trace` lands on Explore showing `GET /handled` and `sample-python`; back, `Open logs` shows the log line.
  - `a JavaScript event parses and links`: `project=2&q=m6-js`, `openFirstIssue(page, /^SAMPLE-JS-/)`; headings `Error` and `TypeError`, `app.mjs` visible, breadcrumb `about to fail`; `Open trace` shows `sample-js`; `Open logs` shows the line.
  - `an event whose trace Tempo lacks`: issue `q=m5-message`; `Open trace` is visible; Explore shows `not found`.
  - `no trace buttons without targets`: `ds=bugsink-project`, issue `q=m2-e2e`; Event loaded, no `Open trace` or `Open logs` link; `Open event in Bugsink` matches `^http://localhost:8800/issues/issue/[0-9a-f-]{36}/event/[0-9a-f-]{36}/$`.
  - `datasource-config.spec.ts` `invalid link time window`: valid URL and token, `Time window` `2000`, save and test: `The link time window must be a whole number of minutes from 1 to 1440.`
- [ ] **Step 2: Run** `npm run build && npm run build:backend && docker compose up -d --wait && docker compose restart grafana && npm run e2e`. Expected: every test passes, twice in a row; the run reports exactly the five events of decision 10.
- [ ] **Step 3: Run** `npm run lint`, `npm run typecheck`, `scripts/check_spdx.sh`, `python3 scripts/verify_stack.py`. Expected: PASS.
- [ ] **Step 4: Commit** `Provision Tempo and Loki links and test them end to end` and push.

### Task 7: Documentation and changelog

**Files:**
- Modify: `src/README.md`, `CHANGELOG.md`, `docker/README.md`, `design/plans/m5-event-detail.md` (line 304)

- [ ] **Step 1: Write** in `src/README.md`: a section `Trace and log links` (the five settings; the buttons and `Open event in Bugsink`; the `Trace ID` column and its links in dashboards, filled for `trace:<id>` and event ID queries only; the window; template variables, escaping and what renders empty in dashboards; Explore uses its own range for links in Explore); `Linking errors to traces` (the prerequisite: the Sentry SDK shares the OpenTelemetry trace context; Python: `OTLPIntegration(setup_otlp_traces_exporter=False, setup_propagator=False)` and the ASGI middleware outermost; Node.js: your own tracer provider plus `Sentry.openTelemetryIntegration()`; events captured outside a span carry a trace id Tempo does not have, and Tempo then says not found; an incoming `traceparent` is honoured with `setup_propagator=False` and ignored by the default; the reverse link: the `Bugsink trace` dashboard, an events query with `trace:<id>`); the Event section now shows `Message` with exceptions. `CHANGELOG.md` under 0.1.0: `Open trace and Open logs on events, links on the trace ID of event tables, and the Tempo, Loki, time window, logs query and service tag settings.` `docker/README.md`: the JavaScript app (port, endpoints, `curl` example), `/message` now outside any span, the eleven `verify_stack.py` lines, the provisioned targets. In the M5 plan's "Not in this milestone", drop "a warning when `event` names another issue's event" (built, As built 13).
- [ ] **Step 2: Run** `scripts/check_spdx.sh`, `scripts/check_licenses.sh`, `npm run lint`. Expected: PASS.
- [ ] **Step 3: Commit** `Document trace and log links` and push.

## Not in this milestone

A browser JavaScript app and source-mapped frames (the Node.js app sends unminified frames); checking that a trace exists in Tempo (spec §10); trace ids for issue-only events queries (one call per event); links on issue frames (spec §5); Explore applying the event's range to links opened inside Explore (Grafana uses Explore's range); validating the LogQL template; probing Tempo or Loki from the health check; Sentry's `sentry-trace` header propagation; a JavaScript `/no-trace` endpoint.

## After the last task

* Whole-branch review against this plan and spec §5, §4.A and §4.D, then one fix wave.
* From the worktree: `npm ci`, `npm run build`, `npm run build:backend`, `go test -race ./pkg/...`, `npm run test:ci`, `npm run e2e`, `python3 scripts/verify_stack.py`.
* Update spec §4.A (link settings), §4.C and §4.F (`trace_id` and `logs_query` on event frames, the links), §4.D (buttons, `Open event in Bugsink`, `Message` with exceptions, `Unknown exception`), §5 (verified Python setup with `setup_propagator=False`, the Node.js setup, `traceId` queries, the range transport, per-row logs query, escaping and empty variables), §7 (the JavaScript app on 8820), §10 (close the open question, add this plan's decisions), the roadmap's M6 entry and open-decisions table, and add an `## As built` section here.
* The merge into `main` happens only when the owner asks for it.

## As built

1. Ruling: the plan's 12 decisions stand, with the changes below.
2. The JavaScript sample app's Sentry setup drops the default Dedupe integration, because it compares the first exception, the constant cause, and would drop every later handled event.
3. The lookup of an event by id makes 3 Bugsink requests (the lookup, the issue and the project list), not 2.
4. A link key of the wrong JSON type, for example a numeric `tempoDatasourceUid`, answers `The data source settings are invalid.`, and a blank `logsQuery` means the default.
5. Ruling: a logs query with a substituted variable outside a double-quoted string is refused as an invalid setting with `In the logs query, put every variable inside double quotes.`, through the health check and every query and resource; values are not escaped for regular expressions, and the field description and the README say so.
6. Ruling: `$` and `[` in a substituted value are written as a backslash escape (a dollar sign as `\u0024`, an opening square bracket as `\u005b`), so Grafana's template interpolation in Explore finds no variable in an event value.
7. Ruling: Tempo and Loki links are offered only for a trace id of 32 lowercase hexadecimal characters, in the event resource and in the frames; any other id shows as text and gets no links.
8. An empty service tag value falls back to the project name, so the default query never reads `{service_name=""}`.
9. The Time window field shows its placeholder `15` when empty and can be emptied.
10. Ruling: `trace:` is case-sensitive in Bugsink; the README says so and the plugin does not change the case.
11. Ruling: the plugin sends no trace id for an issue-only events query, so that query has no links; the owner was told.
12. Ruling: Explore applies its own time range to a link followed inside Explore; the backend's range applies to links followed from a dashboard.
13. Ruling: Open trace and Open logs open Explore in the same tab. Back from Explore can need more than one press: Explore adds its own history entries; the README says so, and opening them in a new tab is left to the owner.
14. The "no `trace_id`, no buttons" demo line is pinned by unit tests only, because neither SDK sends an event without a trace id; the example of a trace Tempo lacks is the message captured in a new thread.
15. The e2e run sends 5 events, every setup project keeps `retries: 0`, and one dashboard test reads the Open logs link and checks that its range is the event plus and minus 15 minutes.
16. The dashboard table in Grafana 13.3.1 shows the `logs_query` field even though the plugin marks it hidden (the column sits to the right and is only clipped in a narrow panel), so the README says the field is marked hidden and a table can still show it.
17. `scripts/verify_stack.py` prints eleven lines; the JavaScript app's `/handled` is called with a random `traceparent` and passes only when the answered trace id equals it, which shows the app keeps an incoming trace id; checks name the check they need.
18. The M5 leftovers are done: the message shows above the exceptions, an exception without a type is headed `Unknown exception`, a breadcrumb time without a zone is read as UTC, and `Open event in Bugsink` opens an event chosen by `event=`.
19. Ruling: the sample app's transitive dependency `sentry@0.49.0` (FSL-1.1-Apache-2.0, through `@sentry/node`) is accepted for the dev stack: the image is not published, and the licence check covers what the plugin ships; `docker/README.md` and the roadmap say so, and publishing the image would need a different pin.
20. Left for later milestones:
    * M11: live validation in the config editor for every setting (URL, Browser URL, time window, logs query); a scoped locator at `trace-links.spec.ts` for the Explore text check; `gotoPage` instead of `page.goto` in that spec; a check of the Bugsink event id behind `Open event in Bugsink`; one shared `listParams` and `dashed` for the e2e specs; the `Could not login` noise in e2e runs; stripping the quotes of a `trace:"..."` term once Bugsink's handling of quoted terms is checked.
