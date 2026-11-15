# M5: Event Detail Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** On the issue page a user reads the latest event's exception chain and stack trace (in-app frames open, other frames folded), its breadcrumbs, tags and contexts, switches to an older event, and sees Bugsink's stack trace text for an event whose frames the plugin does not read.

**Architecture:** `pkg/bugsink` gains `GetEvent`, which reads `GET events/{id}/` and parses the Sentry payload in `data` into a typed, bounded `EventDetail`. The data source serves it as `GET events/{id}`, a normalised JSON without the raw payload. The issue page gets an Event section that shows the event named by the URL's `event` parameter, else the newest row of the Events list.

**Tech Stack:** Go 1.26 with `grafana-plugin-sdk-go` v0.299.0; React 19, `@grafana/ui`/`@grafana/data`/`@grafana/runtime` 13.2.4, `react-router-dom` 6.30.6; Jest; `@grafana/plugin-e2e` 4.0.0; the Compose stack (Bugsink 2.6.3, Grafana 13.3.1, Sentry Python SDK 2.75.0). No new dependency.

**Spec:** [`../bugsink_grafana_plugin_spec.md`](../bugsink_grafana_plugin_spec.md) (§4.C event detail and stack trace display, §4.D event detail, §5 for what M6 needs). **Roadmap:** [`../implementation_roadmap.md`](../implementation_roadmap.md) (M5, Definition of done). **Previous plans:** [`m4-issue-detail-and-events.md`](m4-issue-detail-and-events.md) (resource and page contracts, As built), [`m3-issue-list-page.md`](m3-issue-list-page.md) (URL rules).

## Global Constraints

* The API token never reaches the browser, a log line, an error message or a frame.
* The backend never builds a cursor and never requests a URL taken from a response.
* Resources never answer 401: Bugsink failures are 502, invalid settings and invalid input 400, a refused caller 403. Resource errors are JSON `{"message": …}`.
* Reuse what exists instead of parallel code: `userMessage`, `d.projectNames`, `decodeJSON`, the router of `routes()` with `canonical`, `useDataSource`, `useSection`, `EventTable`, `tests/fixtures.ts`, `tests/seeded.ts`, the `report-event` setup project.
* Pages show the backend's message text verbatim. Resource reads pass `showErrorAlert: false`.
* Every string from an event payload is untrusted: it is rendered as React text only, never as HTML, Markdown, an `href` or a `src`.
* Generated code stays in `pkg/bugsink/internal/gen/` and is not regenerated. Do not modify anything in `.config/`. Do not change a plugin id or type; this plan changes no `plugin.json`.
* Every hand-written source file starts with `SPDX-License-Identifier: GPL-3.0-or-later` (JSON exempt); `scripts/check_spdx.sh` passes.
* Comments are one line and describe the code as it is. Tests assert behaviour, not code structure.
* Go commands run on `./pkg/...`: `node_modules` contains a Go package.
* Host ports stay `3300` (Grafana), `8800` (Bugsink), `8810` (Python sample app).
* End-to-end tests produce at most 5 Bugsink events per run. This milestone adds three (two `/chained`, one `/message`), so a run produces five: the budget is full.
* Work on branch `m5/event-detail` in the worktree `.claude/worktrees/m5-event-detail` (created from `main` at b68f736, `node_modules` installed). Every implementer uses this same branch and worktree; push after every commit; never amend.
* The running stack may mount another checkout's `dist/` and provisioning. Before the first stack check, run `docker compose up -d --wait` from the worktree (Compose project `bugsink-grafana-plugin`; data volumes are kept). Never `docker compose down`.
* Run every test command in the foreground.

## Facts already verified (2026-11-15, Compose stack, tokens `a`/`c`/`d`×40)

* `GET events/{id}/` (Bugsink's event uuid) and `events/lookup/?project=&event_id=` answer the same body: top-level keys `id, ingested_at, digested_at, issue, grouping, event_id, project, timestamp, digest_order, data, stacktrace_md`.
* `data` keys of a sample-app event: `level, exception, event_id, timestamp, contexts, transaction, transaction_info, tags, breadcrumbs, extra, modules, request, release, environment, server_name, sdk, platform`. `tags` is a dict (`{"service": "sample-python"}`), `contexts` holds `trace {trace_id, span_id, parent_span_id}` and `runtime`, `breadcrumbs` is `{values: [...]}` with RFC 3339 `timestamp`, `type`, `category`, `level`, `message`, `data`.
* Refusals of `events/{id}/`: unknown uuid 404 `No Event matches the given query.`; `nosuch` 404 `Not found.`; project-bound token on a project-2 event 403 `This token is not allowed to access this project.`; no-capability token 403 `…capability: events:read.` (every path). The health check already probes `events:read`.
* Python frames carry `filename, abs_path, function, module, lineno, pre_context, context_line, post_context, vars, in_app`. The `/error` event `7b5d5499-1cf8-4734-8381-14cfb03da60e` has 26 frames, the last 3 (`app.py`) `in_app: true`, the 23 Starlette/FastAPI/anyio frames `false`, mechanism `{type: starlette, handled: false}`; answer 74 511 bytes, `stacktrace_md` 34 145 characters. A script outside the app gets `in_app` absent on every frame.
* Chained (`raise RuntimeError(...) from JSONDecodeError`, probe event `d81681656e9f46a997bfd6e5f1e6f088` in project 1): `exception.values` is `[JSONDecodeError (module json.decoder, 4 frames), RuntimeError (1 frame)]`, cause first; mechanism `{type: generic, handled: true}` on both, no `exception_id`.
* `capture_message` with `attach_stacktrace=True` (probe event `505273ff3fb8468eae657d560be585e7`): no `exception`, `message` is a plain string, frames under `threads.values[0].stacktrace.frames`; `stacktrace_md` renders those frames under `# <message>`. The project-2 event `97adbabe-2dcd-4773-b24f-5088549ae235` has only `message, level, event_id` and `stacktrace_md` `_No stacktrace available._`.
* Bugsink renders `stacktrace_md` from `exception` values, else `threads`, else event-level `stacktrace` (`events/utils.py:70-88`); Python events in payload order, other platforms with exceptions and frames reversed, chain separator `During handling of the above exception, another exception occurred:` (`events/markdown_stacktrace.py:170-210`). Its own header says the Markdown "is not a stable interface … not something you'd parse".
* Size: Bugsink's `MAX_EVENT_SIZE` defaults to 1 MiB (`bugsink/app_settings.py:52`, env `MAX_EVENT_SIZE`); a 3 MB event got `413 Request Entity Too Large` and was dropped. A 140-frame event just under the limit answered 1 855 632 bytes (`data` 934 922, `stacktrace_md` 920 529). `decodeJSON` reads at most `maxBodyBytes` (10 MiB) and turns a cut body into `ErrNotBugsink`.
* Sentry protocol (develop.sentry.dev, Stack Trace and Breadcrumbs interfaces): frames are oldest first, the last frame raised; frame fields include `colno`, `raw_function`, `package`, `instruction_addr`, `platform`; a breadcrumb `timestamp` is RFC 3339 or numeric epoch seconds; `breadcrumbs` may be a bare array. The Exception page is gone from the site (404).
* `@grafana/data` 13.2.4 `renderMarkdown(str, {breaks})` sanitises with an `xss` whitelist that keeps `<img src>`, `<iframe>` (sandboxed) and `style`; rendering a real `stacktrace_md` turns `__call__` into bold `call` and runs the unfenced code lines into one paragraph. `@grafana/ui` `RenderUserContentAsHTML` uses DOMPurify's html profile (keeps `img`).
* `@grafana/ui` 13.2.4 exports `Collapse` (`label`, `isOpen`, `onToggle`; children render only while open), `InteractiveTable`, `Badge`, `Stack`, `Text`, `useStyles2`; `@emotion/css` 11.10.6 is a dependency.
* Generated client: `EventsRetrieve(ctx, id openapi_types.UUID)`; `EventDetail.Data` is `interface{}`, so `GetEvent` decodes into its own struct with `json.RawMessage`.
* `GET /api/datasources/uid/bugsink/resources/events/<uuid>` reaches the plugin today and gets `{"message":"Not found."}` 404.
* **Not verified:** that the sample app's `/chained` frames are `in_app` true for `app.py` and false for `json/`, and that `attach_stacktrace=True` adds no `threads` to its exception events. Task 6 Step 1 checks both first. JavaScript SDK payloads are not produced in this stack (M6 brings the app); Task 1's JavaScript fixture follows the protocol pages above.

## Decisions in this plan, for review

1. **The backend parses the payload** (`pkg/bugsink`, Go) and serves a normalised event: the browser never gets the raw `data` (up to 1 MiB and more, with locals, headers and modules), the bounds live in one place, the parser is tested with table tests over captured payloads, and M6 reuses `EventDetail` for trace and log links on event frames.
2. **Bounds:** the backend reads at most 10 MiB of Bugsink's answer (10× Bugsink's default event limit); beyond that the Event section shows `The answer from Bugsink is larger than 10 MB.` and the Events row keeps its Bugsink link. The normalised event keeps the last 10 exceptions of a chain, the 250 frames nearest the raise per exception, 5 context lines on each side, the newest 100 breadcrumbs, values cut at 1024 characters and `stacktrace_md` cut at 100 000 bytes; each cut adds a notice.
3. **`stacktrace_md` is shown as preformatted plain text, not rendered Markdown.** Grafana's Markdown sanitiser keeps remote images and iframes from application-controlled text, and Bugsink's own Markdown is unfenced code that renders garbled (Facts). Spec §4.C says "displayed as Markdown"; the as-built step updates it.
4. **When the fallback shows:** exception frames are parsed; `stacktraceMd` is sent only when no exception has frames and the payload has frames in `threads` or `stacktrace`, which the plugin does not parse. An event with no frames anywhere shows `No stack trace.`
5. **Display order follows Bugsink:** `platform: python` in payload order (cause first, innermost frame last), every other platform with exceptions and frames reversed; separators `During handling of the above exception, another exception occurred:` (Python) and `Caused by:` (others).
6. **Folding:** consecutive frames that are not `in_app: true` fold into one `Collapse` labelled `<n> other frames`; when no frame of an exception is in-app, every frame shows (as Bugsink does).
7. **Not shown:** frame locals (`vars`), `request`, `user`, `extra`, `modules`, `sdk`, thread stacks as structure. They are not in the spec's list, and locals and headers are the bulk and the sensitive part of a payload.
8. **The selected event lives in the URL** as `event=<Bugsink event uuid>`; absent means the newest row of the Events list. A row's Event ID links to it in the page; an icon beside it opens Bugsink. `Back to issues` drops `event` and keeps every other parameter verbatim.
9. **Layout:** Summary, Event, Events, Tags, History. Inside Event the sub-headings are `Exception`, `Message`, `Stack trace`, `Breadcrumbs`, `Event tags`, `Contexts`, so no heading repeats a page section's name.
10. **Event detail reads are not cached** (as M4 decision 3); an event does not change, but nothing reads it twice within 15 seconds.
11. **Test payloads:** the Python sample app gets `/chained` and `/message` (with `attach_stacktrace=True`) for the end-to-end demo; the other SDK shapes, the no-frames message, a very large payload and malformed fields are Go fixtures and generated payloads.
12. **No dashboard change.** Event content is an app page feature; the end-to-end tests exercise it through the provisioned `Bugsink` data source, and no new token capability is needed (`events:read` is already probed).

## Event resource contract

`GET events/{id}`, `{id}` a uuid; anything else 400 `"<id>" is not an event id.` before any request. 200 body (camelCase; times RFC 3339):

| Key | Value |
| :--- | :--- |
| `id`, `eventId`, `issueId` | Bugsink's event uuid, the SDK id (dashed), the issue uuid |
| `projectId`, `project` | id; name, or the id as text |
| `timestamp`, `ingestedAt` | Bugsink's top-level times |
| `platform`, `level`, `message`, `transaction`, `environment`, `release`, `traceId` | strings, `""` when absent; `message` from `logentry.formatted`, `logentry.message`, then `message` (string, or its `formatted`/`message`); `traceId` from `contexts.trace.trace_id` |
| `tags` | `[{key, value}]` by key; from a dict or a list of pairs |
| `contexts` | `[{name, fields: [{key, value}]}]` by name, fields by key |
| `exceptions` | protocol order: `[{type, value, module, mechanism, handled (bool or null), frames, framesOmitted}]`; frame `{filename, function, module, lineno, colno, inApp, preContext, contextLine, postContext}`; `filename` is `filename`, else `abs_path`, `module`, `package`; `lineno`/`colno`/`contextLine` null when absent; `inApp` true only for `in_app: true` |
| `breadcrumbs` | oldest first: `[{timestamp (or null), type, category, level, message, data: [{key, value}]}]` |
| `stacktraceMd` | string or null (decision 4) |
| `notices` | strings, in this order: `Only the last 10 exceptions of the chain are shown.`; `Only the 250 frames nearest the error are shown for <type>.` (per exception); `Only the newest 100 breadcrumbs are shown.`; `Values longer than 1024 characters are cut.`; `The stack trace text is cut at 100000 characters.` |
| `bugsinkUrl` | `<browser URL>/issues/issue/<issueId>/event/<id>/` |

A value that is not a string is its compact JSON text. A field of the wrong type is treated as absent, never as an error. Errors: invalid settings 400; any Bugsink or network failure 502 with `userMessage`'s text. New message contract row: `ErrTooLarge` gives `The answer from Bugsink is larger than 10 MB.` (query status 502, resource 502).

## Event section contract

| State | Shows |
| :--- | :--- |
| No event id (no `event`, Events list empty, failed or loading) | no Event section |
| Loading / failed | `Loading event...` / `Alert` error with the message (fallback `The event could not be loaded.`) |
| Loaded | h2 `Event`; rows `Event ID`, `Timestamp`, then `Level`, `Environment`, `Release`, `Transaction`, `Trace ID` when not empty; each notice as a warning; h3 `Exception` with the chain (h4 `<type>`, the value, `Badge` `Unhandled` when `handled` is false), or h3 `Message` with the message; h3 `Stack trace` when there are no exceptions: `stacktraceMd` in a `<pre>`, else `No stack trace.`; h3 `Breadcrumbs` (columns `Time`, `Type`, `Category`, `Level`, `Message`; `Message` is the message, else `key=value` pairs joined by `, `; `No breadcrumbs.`); h3 `Event tags` (`Tag`, `Value`; `No tags.`); h3 `Contexts` (`Context`, `Key`, `Value`; `No contexts.`) |

A frame header reads `<filename>:<lineno>:<colno> in <function>`, leaving out what is absent; an in-app frame's header is bold and its source shows in a `<pre>` as `<line number> | <code>`, the context line inside `<mark>`. Times use the user's time zone, `YYYY-MM-DD HH:mm:ss` (breadcrumbs `YYYY-MM-DD HH:mm:ss.SSS`).

## Review Focus

1. Application text that looks like HTML or Markdown (`<img src=x onerror=…>`, `<script>`, `[x](javascript:…)`, `__call__`) in an exception value, source line, breadcrumb, tag, context or `stacktraceMd` shows as typed and creates no element (Task 4 test).
2. A pathological payload (300 frames, 12 chained exceptions, 150 breadcrumbs, a 200 000-character minified line) comes back bounded with its notices, and an answer over 10 MiB says it is too large instead of `The answer did not look like a Bugsink API.` (Task 1 and Task 2 tests).
3. Payload shapes the Python SDK does not send (numeric breadcrumb timestamps, `colno`, bare-list `exception` and `breadcrumbs`, tag pairs, `logentry`, fields of the wrong type, `data: null`) parse without an error (Task 1 tests).
4. A stale or hand-edited `event` (`nosuch`, `../projects`) never reaches Bugsink: 400 `"nosuch" is not an event id.` shows in the Event section while the other sections render (Task 2 and Task 5 tests).
5. Clicking two events quickly: the Event section shows the last one clicked, never an earlier answer (Task 5 test).

## File structure

| File | Responsibility |
| :--- | :--- |
| `pkg/bugsink/client.go`, `errors.go` | `ErrTooLarge` from `decodeJSON` |
| `pkg/bugsink/eventdetail.go` (new), `testdata/events/*.json` (new) | `GetEvent`, the Sentry payload parser and its bounds |
| `pkg/plugin/resources.go`, `messages.go` | `GET events/{id}`, its body and notices, the too-large message |
| `src/datasource/types.ts`, `datasource.ts` | `EventDetail` types, `getEvent` |
| `src/pages/event/` (new) | `displayOrder.ts`, `StackTrace.tsx`, `Breadcrumbs.tsx`, `EventDetailView.tsx` |
| `src/pages/IssuePage.tsx`, `routes.ts`, `issue/EventTable.tsx`, `issue/IssueTags.tsx`, `issue/IssueHistory.tsx` | the Event section, event selection, deferred M4 fixes |
| `sample-apps/python/app.py` | `/chained`, `/message`, `attach_stacktrace` |
| `tests/reported.ts` (renamed from `trace-event.ts`), `tests/report-event-detail.setup.ts`, `tests/event-detail.spec.ts` (new) | the demo end to end |

---

### Task 1: Read and parse an event in the Bugsink client

**Files:**
- Create: `pkg/bugsink/eventdetail.go`, `pkg/bugsink/eventdetail_test.go`, `pkg/bugsink/testdata/events/{python-unhandled,python-handled,python-chained,python-message-threads,message-only,javascript-error}.json`
- Modify: `pkg/bugsink/client.go`, `pkg/bugsink/errors.go`, `pkg/bugsink/client_test.go`

**Interfaces:**
- Consumes: `gen.Client.EventsRetrieve`, `decodeJSON`, `Event`.
- Produces:

```go
var ErrTooLarge = errors.New("Bugsink's answer is larger than the read limit") // from decodeJSON past maxBodyBytes
const (MaxExceptions = 10; MaxFrames = 250; MaxContextLines = 5; MaxBreadcrumbs = 100; MaxValueLength = 1024; MaxStacktraceMD = 100_000)
type KeyValue struct{ Key, Value string }
type EventContext struct{ Name string; Fields []KeyValue }
type Frame struct{ Filename, Function, Module string; Lineno, Colno *int; InApp bool; PreContext []string; ContextLine *string; PostContext []string }
type Exception struct{ Type, Value, Module, Mechanism string; Handled *bool; Frames []Frame; FramesOmitted int }
type Breadcrumb struct{ Timestamp *time.Time; Type, Category, Level, Message string; Data []KeyValue }
type EventDetail struct {
	Event
	Platform, Level, Message, Transaction, Environment, Release, TraceID string
	Tags        []KeyValue
	Contexts    []EventContext
	Exceptions  []Exception // protocol order
	Breadcrumbs []Breadcrumb
	StacktraceMD    string // set only per decision 4
	ExceptionsCut, BreadcrumbsCut, ValuesCut, StacktraceMDCut bool
}
func (c *Client) GetEvent(ctx context.Context, id uuid.UUID) (EventDetail, error)
func parseEvent(data json.RawMessage, stacktraceMD string) EventDetail // fills everything but Event
```

- Fixtures: copy the API answers named in Facts (`python-unhandled` = `7b5d5499-…`, `python-handled` = the newest event of `SAMPLE-PYTHON-263`, `python-chained` and `python-message-threads` by lookup of the probe event ids in project 1, `message-only` = `97adbabe-…`), removing `data.modules` and every frame's `vars`. `javascript-error.json` is written by hand: `platform: javascript`, `exception` a bare list `[TypeError (mechanism {type: chained, source: cause, exception_id: 1, parent_id: 0}), Error (mechanism {type: onunhandledrejection, handled: false, exception_id: 0})]`, frames with `filename` `http://localhost:8820/assets/index.js`, `function`, `lineno`, `colno`, `in_app: true`, no context lines; `breadcrumbs` a bare list with numeric `timestamp` `1763193600.25`; `tags` `[["browser","Chrome 131"],["service","sample-js"]]`; `contexts.browser`; `logentry {formatted: "x"}`.

- [ ] **Step 1: Write failing tests** in `eventdetail_test.go` (fixtures through an `httptest.Server`, generated payloads through `parseEvent`):
  - `TestGetEventRequest`: path `/api/canonical/0/events/<uuid>/`; `Event` fields from the top level; 404 `No Event matches the given query.` is an `*APIError` with that detail.
  - `TestParsePythonUnhandled`: 26 frames in payload order, the last three `InApp` with `Filename` `app.py`, `Function` `_fail` last with `Lineno` 60 and its context; `Mechanism` `starlette`, `Handled` false; `Environment` `development`, `Release` `sample-python@0.1.0`, `TraceID` the context's id, `Tags` `[{service sample-python}]`, `Contexts` names `runtime`, `trace` in that order; two breadcrumbs with times; `StacktraceMD` empty.
  - `TestParseChained`: two exceptions, `JSONDecodeError` first with `Module` `json.decoder`.
  - `TestParseMessageWithThreads`: no exceptions, `Message` set, `StacktraceMD` equal to the fixture's text; `message-only`: `Message` `probe event for project 2`, `StacktraceMD` empty, no error.
  - `TestParseJavaScript`: exceptions from the bare list, `Colno` set, `ContextLine` nil; breadcrumb time `2025-11-15T08:00:00.25Z`; tags `browser`, `service` by key; `Message` `x`.
  - `TestParseBounds` (generated): 12 exceptions keep the last 10 and set `ExceptionsCut`; 300 frames keep the last 250 with `FramesOmitted` 50; 8 lines of `pre_context` keep the last 5, of `post_context` the first 5; 150 breadcrumbs keep the newest 100 and set `BreadcrumbsCut`; a 200 000-character `context_line` and a 5000-character tag value become 1024 characters plus `…` with `ValuesCut`; a 150 000-byte thread fallback is cut to 100 000 bytes on a UTF-8 boundary with `StacktraceMDCut`.
  - `TestParseTolerance`: `data` `null`, `[]` and `"x"`; `tags: 5`, `exception: {"values": "x"}`, `lineno: "7"`, `in_app: "yes"`, `contexts: {"trace": 3}`, a non-string tag value `{"a":1}` (becomes `{"a":1}`): no panic, the bad field is empty, the rest parses.
  - `TestDecodeTooLarge` (`client_test.go`): a 200 answer of `maxBodyBytes + 1` bytes gives `errors.Is(err, ErrTooLarge)`; exactly `maxBodyBytes` of valid JSON decodes.
- [ ] **Step 2: Run** `go test ./pkg/bugsink/...`. Expected: FAIL, undefined `GetEvent`, `parseEvent`, `ErrTooLarge`.
- [ ] **Step 3: Implement.** Decode each `data` field from its own `json.RawMessage` so one bad field leaves the others; values cut by runes.
- [ ] **Step 4: Run** `go test -race ./pkg/...`, `go vet ./pkg/...`. Expected: PASS, earlier tests unchanged.
- [ ] **Step 5: Commit** `Read and parse event payloads in the Bugsink client` and push.

### Task 2: The event resource route

**Files:**
- Modify: `pkg/plugin/resources.go`, `pkg/plugin/resources_test.go`, `pkg/plugin/messages.go`, `pkg/plugin/messages_test.go`, `pkg/plugin/query_test.go` (fake Bugsink serves `events/{id}/`)

**Interfaces:**
- Consumes: Task 1's `GetEvent`, `EventDetail`, `ErrTooLarge`, the `Max*` constants; `d.projectNames`, `writeBugsinkError`.
- Produces: `get("/events/{id}", d.handleEvent)`; `func eventJSON(e bugsink.EventDetail, project, browserURL string) eventBody` (M6 adds to it); `func eventNotices(e bugsink.EventDetail) []string`; the event resource contract.

- [ ] **Step 1: Write failing tests:**
  - `TestEventResource`: from the `python-unhandled` fixture, 200 with exactly the contract's keys; `bugsinkUrl` `http://localhost:8800/issues/issue/<issue>/event/<id>/`; `project` `alpha`; `lineno` of a frame a number, `colno` null; `stacktraceMd` null; `tags` `[]` and `breadcrumbs` `[]` (not null) for `message-only`.
  - `TestEventNotices`: an `EventDetail` with every cut flag and two exceptions with omitted frames gives the five texts in contract order, naming each exception's type.
  - `TestEventRouteRejectsBadIDs`: `events/nosuch`, `events/X-1` answer 400 `"nosuch" is not an event id.`; `events/..` and `events/../projects` never answer 200; no Bugsink request.
  - `TestEventRouteErrors`: Bugsink 404 `No Event matches the given query.` gives 502 `Bugsink refused the request: No Event matches the given query.`; 403 naming `events:read` gives 502 `The API token lacks events:read.`; an answer over the limit gives 502 `The answer from Bugsink is larger than 10 MB.`; `POST events/<uuid>` 405; invalid settings 400; no body contains the token.
  - `messages_test.go`: `ErrTooLarge` (wrapped) gives that text and status 502.
- [ ] **Step 2: Run** `go test -race ./pkg/plugin/...`. Expected: FAIL.
- [ ] **Step 3: Implement.** `userMessage` checks `ErrTooLarge` before `ErrNotBugsink`.
- [ ] **Step 4: Run** `go test -race ./pkg/...`, `go vet ./pkg/...`. Expected: PASS.
- [ ] **Step 5: Check in the stack:** `npm run build:backend`, `docker compose up -d --wait`, `docker compose restart grafana`; `GET http://localhost:3300/api/datasources/uid/bugsink/resources/events/7b5d5499-1cf8-4734-8381-14cfb03da60e` lists 26 frames and is under 40 000 bytes (Bugsink's answer is 74 511); the same on uid `bugsink-project` for `97adbabe-2dcd-4773-b24f-5088549ae235` answers 502 `Bugsink refused the request: This token is not allowed to access this project.`
- [ ] **Step 6: Commit** `Serve a normalised event from the data source` and push.

### Task 3: Event types, `getEvent` and the stack trace view

**Files:**
- Create: `src/pages/event/displayOrder.ts`, `displayOrder.test.ts`, `src/pages/event/StackTrace.tsx`, `StackTrace.test.tsx`
- Modify: `src/datasource/types.ts`, `src/datasource/datasource.ts`, `src/datasource/datasource.test.ts`

**Interfaces:**
- Consumes: the event resource contract.
- Produces:

```ts
export interface KeyValue { key: string; value: string }
export interface StackFrame { filename: string; function: string; module: string; lineno: number | null; colno: number | null; inApp: boolean; preContext: string[]; contextLine: string | null; postContext: string[] }
export interface EventException { type: string; value: string; module: string; mechanism: string; handled: boolean | null; frames: StackFrame[]; framesOmitted: number }
export interface Breadcrumb { timestamp: string | null; type: string; category: string; level: string; message: string; data: KeyValue[] }
export interface EventContext { name: string; fields: KeyValue[] }
export interface EventDetail { id: string; eventId: string; issueId: string; projectId: number; project: string; timestamp: string; ingestedAt: string; platform: string; level: string; message: string; transaction: string; environment: string; release: string; traceId: string; tags: KeyValue[]; contexts: EventContext[]; exceptions: EventException[]; breadcrumbs: Breadcrumb[]; stacktraceMd: string | null; notices: string[]; bugsinkUrl: string }
// BugsinkDataSource
getEvent(id: string): Promise<EventDetail> // getResource(`events/${encodeURIComponent(id)}`, undefined, QUIET)
// displayOrder.ts
export type FrameGroup = { kind: 'frame'; frame: StackFrame } | { kind: 'folded'; frames: StackFrame[] };
export function displayExceptions(exceptions: EventException[], platform: string): EventException[] // decision 5, frames reordered too
export function chainSeparator(platform: string): string
export function frameGroups(frames: StackFrame[]): FrameGroup[] // decision 6
export function frameTitle(frame: StackFrame): string // `<filename>:<lineno>:<colno> in <function>`
// StackTrace.tsx
export function ExceptionChain({ exceptions, platform }: { exceptions: EventException[]; platform: string }): React.JSX.Element
```

- [ ] **Step 1: Write failing tests.**
  - `datasource.test.ts`: `getEvent('a/b')` calls `getResource('events/a%2Fb', undefined, { showErrorAlert: false })`.
  - `displayOrder.test.ts`: `python` keeps exceptions and frames; `javascript` and `''` reverse both and do not mutate the input; separators per decision 5; `frameGroups` of `[ext, ext, app, ext]` is `folded(2), frame, folded(1)`; with no in-app frame every frame is a `frame` group; `frameTitle` gives `app.py:60 in _fail`, `index.js:10:5 in onClick`, `app.py` (no line, no function).
  - `StackTrace.test.tsx`: a Python chain of two shows h4 `JSONDecodeError` before `RuntimeError` with the separator between; `Unhandled` badge only when `handled` is false; an in-app frame shows its header and a `<pre>` whose `<mark>` holds the context line with line numbers from `lineno - preContext.length`; two external frames show a button `2 other frames` and not their filenames until it is clicked; `1 other frame` singular; an exception without frames shows `No frames.`
- [ ] **Step 2: Run** `npm run test:ci`. Expected: FAIL.
- [ ] **Step 3: Implement** with `ControlledCollapse` (closed at first; its toggle is a button named by the label), `Badge` and `useStyles2` (theme spacing and monospace font; no inline pixel styles).
- [ ] **Step 4: Run** `npm run test:ci`, `npm run typecheck`, `npm run lint`. Expected: PASS, Jest exits by itself.
- [ ] **Step 5: Commit** `Show an event's exception chain and stack trace` and push.

### Task 4: Breadcrumbs, tags, contexts and the fallback; theme spacing on the issue tables

**Files:**
- Create: `src/pages/event/Breadcrumbs.tsx`, `Breadcrumbs.test.tsx`, `src/pages/event/EventDetailView.tsx`, `EventDetailView.test.tsx`
- Modify: `src/pages/issue/IssueTags.tsx`, `src/pages/issue/IssueHistory.tsx` (tests unchanged and passing)

**Interfaces:**
- Consumes: Task 3's types and `ExceptionChain`.
- Produces: `Breadcrumbs({ breadcrumbs, timeZone }: { breadcrumbs: Breadcrumb[]; timeZone: TimeZone })`; `EventDetailView({ event, timeZone }: { event: EventDetail; timeZone: TimeZone })` rendering everything of the event section contract's Loaded row except the h2.

- `IssueTags` becomes an `InteractiveTable` with columns `Tag`, `Values` (deferred from M4); `IssueHistory` lays out entries with `Stack direction="column" gap={1}`; neither keeps an inline `style`.

- [ ] **Step 1: Write failing tests.**
  - `Breadcrumbs.test.tsx`: headers exactly `Time`, `Type`, `Category`, `Level`, `Message`; rows in the given order; `2026-11-15 07:52:59.387` in `utc`; a null time gives an empty cell; a breadcrumb without message shows `order=42, user=ada`; `No breadcrumbs.`
  - `EventDetailView.test.tsx`: rows `Event ID`, `Timestamp`, `Environment` `development`, `Release`, `Trace ID`; `Level` absent when empty; notices as warnings in order; exceptions give h3 `Exception` and no `Stack trace`; no exceptions and a message give h3 `Message` with the text and h3 `Stack trace` with `stacktraceMd` inside a `pre`; no exceptions and no `stacktraceMd` give `No stack trace.`; `Event tags` rows `service` / `sample-python`; `Contexts` rows `trace` / `trace_id` / the id; `No tags.`, `No contexts.`
  - `EventDetailView.test.tsx` `renders payload text as text`: an event whose exception value, a context line, a breadcrumb message, a tag value, a context value and `stacktraceMd` are `<img src=x onerror=alert(1)><script>alert(1)</script>[x](javascript:alert(1)) __call__`: the string is found verbatim in each place, and the view contains no `img`, `script`, `iframe` or `a` element.
- [ ] **Step 2: Run** `npm run test:ci`. Expected: FAIL.
- [ ] **Step 3: Implement.** Key/value tables are `InteractiveTable`s; `stacktraceMd` goes into `<pre>{event.stacktraceMd}</pre>`.
- [ ] **Step 4: Run** `npm run test:ci`, `npm run typecheck`, `npm run lint`. Expected: PASS.
- [ ] **Step 5: Commit** `Show breadcrumbs, tags, contexts and Bugsink's stack trace text` and push.

### Task 5: The Event section on the issue page

**Files:**
- Modify: `src/pages/IssuePage.tsx`, `src/pages/IssuePage.test.tsx`, `src/pages/routes.ts`, `src/pages/routes.test.ts` (new), `src/pages/issue/EventTable.tsx`, `src/pages/issue/EventTable.test.tsx`

**Interfaces:**
- Consumes: Task 3's `getEvent`, Task 4's `EventDetailView`; `useSection`, `useDataSource`, `listPath`.
- Produces:

```ts
export function withEvent(search: string, eventId?: string): string // sets or removes `event`, other segments verbatim; '' when nothing is left
export function EventTable(props: { rows: EventRow[]; timeZone: TimeZone; selectedId?: string; eventHref: (row: EventRow) => string }): React.JSX.Element
// IssuePage: useSection(source, key: string | undefined, refresh, read, fallback); no read while key is undefined
```

- The shown event id is the `event` parameter, else `events.value?.rows[0]?.id`; its key is `${issueId}|${eventId}`, so a new selection drops an earlier answer. `Back to issues` navigates to `listPath(withEvent(search))`. The sections render only when `source` is set (deferred from M4). `EventTable`: the Event ID cell is a `Link` to `eventHref(row)` (`issuePath(issueId, withEvent(search, row.id))`), plain bold text for `selectedId`, then an icon `LinkButton` named `Open event <eventId> in Bugsink` (`row.url`, new tab, `rel="noreferrer"`, absent without url); no rows shows `<p>No stored events.</p>` like Tags and History (deferred from M4).

- [ ] **Step 1: Write failing tests.**
  - `routes.test.ts`: `withEvent('?orgId=1&kiosk&ds=eu', 'e-1')` is `?orgId=1&kiosk&ds=eu&event=e-1`; replacing an existing `event` keeps its position; `withEvent('?kiosk&event=e-1')` is `?kiosk`; `withEvent('?event=e-1')` is `''`.
  - `EventTable.test.tsx`: the M4 tests pass with the new link names; the selected row's id is not a link; another row's id links to `eventHref`'s value without `target`; the icon link has the row url and `target="_blank"`; `No stored events.` is a paragraph.
  - `IssuePage.test.tsx`: without `event`, `getEvent` runs with the first row's id after `listEvents` resolves and the Event section follows Summary and precedes Events; with `?event=e-2`, `getEvent('e-2')` runs without waiting for the list; clicking another row's Event ID calls `getEvent` with it and the URL gains `event=`; first `getEvent` resolving after the second shows the second; a rejection `"nosuch" is not an event id.` shows under `Event` while Tags, History and Events render; no events gives no `Event` heading; `Back to issues` from `?orgId=1&kiosk&ds=eu&event=e-2` goes to `/a/ahmedismail-bugsink-app?orgId=1&kiosk&ds=eu`; Refresh reruns `getEvent`; `?ds=nosuch` shows no `Tags`, `History`, `Events` or `Event` heading.
- [ ] **Step 2: Run** `npm run test:ci`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `npm run test:ci`, `npm run typecheck`, `npm run lint`. Expected: PASS.
- [ ] **Step 5: Check in the stack:** `npm run build`, restart Grafana; `GET http://localhost:3300/public/plugins/ahmedismail-bugsink-app/module.js` contains `Loading event...`.
- [ ] **Step 6: Commit** `Show the latest or chosen event on the issue page` and push.

### Task 6: Sample app events and end-to-end tests of the demo

**Files:**
- Create: `tests/report-event-detail.setup.ts`, `tests/event-detail.spec.ts`
- Modify: `sample-apps/python/app.py`, `tests/trace-event.ts` (`git mv` to `tests/reported.ts`), `tests/report-trace.setup.ts`, `tests/events-query.spec.ts`, `tests/issue-detail.spec.ts`, `tests/fixtures.ts`, `tests/seeded.ts`, `.gitignore` (`/playwright/.event-detail.json`), `docker/README.md`

**Interfaces:**
- Consumes: everything above, built into `dist/`.
- Produces: `GET /chained?variant=` (adds breadcrumb `category="app", message="parsing the order", data={"order": 42}`, raises `RuntimeError(_message("chained failure", variant)) from` the `JSONDecodeError` of `json.loads("{not json")`, captures it, returns `{trace_id, event_id}`); `GET /message?variant=` (`capture_message(_message("message without exception", variant), level="warning")`, returns `{event_id}`); `attach_stacktrace=True` in `sentry_sdk.init`. `tests/reported.ts`: `writeReported(name: string, value: unknown)`, `readReported<T>(name: string): T` on `playwright/.<name>.json` (the trace keeps `playwright/.trace.json`). `tests/seeded.ts`: `CHAINED_MARKER = 'm5-chained'`, `MESSAGE_MARKER = 'm5-message'`.

- [ ] **Step 1: Check the unverified facts first:** `docker compose up -d --build --wait sample-python`; `curl 'http://localhost:8810/chained?variant=m5-plan'` and `curl 'http://localhost:8810/handled?variant=m5-plan'`; by `events/lookup/` (retry for the digest): the `RuntimeError` frame `app.py` has `in_app: true`, the `json/decoder.py` frames `false`; the `/handled` event has no `threads`. If not, stop and report.
- [ ] **Step 2: Write the setup and tests** (read `.config/AGENTS/e2e-testing.md` first; digest waits retry with `toPass()`):
  - `report-event-detail.setup.ts`: `/chained?variant=m5-chained` twice in sequence, then `/message?variant=m5-message`; writes `event-detail` `{chained: [first, second], message}`.
  - `section()` and `openFirstIssue()` move from `issue-detail.spec.ts` into `tests/fixtures.ts` as exports; `section()` matches a string heading exactly. In `issue-detail.spec.ts` the Events row link check uses the link named `Open event … in Bugsink`.
  - `event-detail.spec.ts` `the latest event's stack trace and breadcrumbs`: open the issue found by `q=m5-chained` as `openFirstIssue` does; section `Event` shows the dashed id of the newest row of section `Events`; headings `JSONDecodeError` and `RuntimeError`; `app.py` and `chained` visible; `json/decoder.py` hidden until the `other frames` button is clicked; `Breadcrumbs` contains `parsing the order`; `Event tags` contains `sample-python`; `Contexts` contains `trace_id`.
  - `switch to an older event`: on the same issue, click the Event ID link of the second row of `Events`; the URL contains `event=`; section `Event` shows that row's event id.
  - `an event without structured frames shows Bugsink's text`: the issue found by `q=m5-message`; `Message` contains `message without exception (variant vm5-message)`; the `pre` under `Stack trace` contains `app.py`.
- [ ] **Step 3: Run** `npm run build && npm run build:backend && docker compose restart grafana && npm run e2e`. Expected: every test passes, twice in a row.
- [ ] **Step 4: Run** `npm run lint`, `npm run typecheck`, `scripts/check_spdx.sh`, `python3 scripts/verify_stack.py`. Expected: PASS.
- [ ] **Step 5:** In `docker/README.md`, list `/chained` and `/message` with what each produces.
- [ ] **Step 6: Commit** `Add chained and message events to the sample app and test event detail end to end` and push.

### Task 7: Documentation and changelog

**Files:**
- Modify: `src/README.md`, `CHANGELOG.md`

- [ ] **Step 1: Write** in `src/README.md` under `## Issue detail page`: the Event section (the newest event by default, how to pick another from Events and that the address keeps it, the rows, the exception chain and its order per platform, in-app frames open and the others folded, breadcrumbs, event tags, contexts); Bugsink's stack trace text shown as plain text for events whose frames are not in an exception; what is not shown (locals, request, user); the limits and their notices (decision 2) and the 10 MB answer limit with Bugsink's `MAX_EVENT_SIZE` default of 1 MiB; Event ID now opens the event in the page and the icon opens Bugsink. In `CHANGELOG.md` under 0.1.0: `Event detail on the issue page: exception chain and stack trace with in-app frames first, breadcrumbs, tags and contexts; the newest event by default, any other from the event list.`
- [ ] **Step 2: Run** `scripts/check_spdx.sh`, `scripts/check_licenses.sh`, `npm run lint`. Expected: PASS.
- [ ] **Step 3: Commit** `Document the event detail` and push.

## Not in this milestone

Open trace and Open logs buttons and data links on event frames (M6, which reads `traceId`, `timestamp`, `tags`, `environment`, `release` and `project` from `EventDetail`); the JavaScript sample app and source-mapped frames (Bugsink maps them in `stacktrace_md` but `data` holds minified frames; M6 checks); frame locals, request, user, extra, modules and SDK; thread stacks as structure; Newer/Older event buttons; caching event reads; event content in dashboard frames.

## After the last task

* Whole-branch review against this plan and spec §4.C and §4.D, then one fix wave.
* From the worktree: `npm ci`, `npm run build`, `npm run build:backend`, `go test -race ./pkg/...`, `npm run test:ci`, `npm run e2e`, `python3 scripts/verify_stack.py`.
* Update spec §4.C (event detail parsed in the backend, the bounds, the fallback rule and plain-text display) and §4.D (the Event section, the `event` parameter, the newest event by default, Event ID opening it in the page), the §10 decisions table, and the roadmap's M5 entry, and add an `## As built` section here.
* The merge into `main` happens only when the owner asks for it.

## As built

1. The backend sends the event with all payload parsing done; the browser never receives `data`.
2. Ruling: the fallback is shown as preformatted plain text, not rendered Markdown (spec §4.C said Markdown), because Markdown from application text could load remote images.
3. Ruling: the fallback is Bugsink's verbatim text and can include local variables; the structured view does not show locals (decision 7 amended).
4. Ruling: the fallback shows whenever no exception has frames (decision 4), so a frameless exception shows `Exception` and `Stack trace`, and shows `No stack trace.` when Bugsink sent no text; the Event section contract row that said "when there are no exceptions" is superseded.
5. `Message` shows only when the event has no exceptions.
6. The README's data source setup section warns that the fallback text can include local variables and that everyone who can query the data source can read it.
7. A switch to hide the fallback text per data source is proposed for M7.
8. Notice texts: `Only the last 10 exceptions of the chain are shown.`, `Only the 250 frames nearest the error are shown for <type>.`, `Only the newest 100 breadcrumbs are shown.`, `Only the first 200 tags, contexts or fields of each list are shown.`, `Values longer than 1024 characters are cut.` and `The stack trace text is cut at 100 KB.`
9. Tags, contexts, context fields and breadcrumb data entries keep the first 200 by key, chosen before their values are read, so a dropped value never sets the values notice.
10. The stack trace text is cut at 100 000 bytes on a character boundary, and the notice says 100 KB.
11. Source lines are sliced to the 5 nearest the frame before they are read, and a line that is not a string stays as an empty line so the numbering holds.
12. A frame title shows the column only when it shows a line.
13. The cross-issue case is built: an `event` that belongs to another issue shows `This event belongs to another issue.` under Event, and the event is not shown; it was listed under "Not in this milestone".
14. The check compares the event's issue id with the address's id and with the loaded issue's id and short id, and the event shows once the issue summary has loaded.
15. Notices and the rows of the event tags and contexts tables are keyed by position, so repeated texts and repeated tag keys all show.
16. A long message wraps inside its column.
17. The e2e setup projects do not retry, so a run reports its events once; the message-event test waits for this run's event.
18. The test fixtures under `pkg/bugsink/testdata/events` hold no local variables, request, server name, extra data or host addresses.
19. Left for later milestones:
    * M6: a Bugsink link for an event chosen by `event=` that is not on the visible Events page (`bugsinkUrl` is unused), the exception message hidden when a logged exception also has a log message, an exception with an empty type, breadcrumb timestamps without a zone, and the check of source-mapped JavaScript frames against the second SDK.
    * M7: Refresh unmounts the Event section until the Events list reloads when the address names no event, and the per-data-source switch for the fallback text.
    * Any milestone: `integer()` accepts values up to 2^31, which overflows `int` on 32-bit builds.
