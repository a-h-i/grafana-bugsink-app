# Phase 0 findings

Milestone M0 (the dev stack and verification spike) answered eleven questions about Bugsink, the Sentry
and OpenTelemetry SDKs, Tempo, Loki and Go client generation. Each section gives the answer, then the
evidence. Everything was observed on the stack in `docker-compose.yaml` against **Bugsink 2.6.3**
(versions at the end), on 2026-11-13. Nothing was tested against 2.6.2.

Evidence comes from `scripts/verify_stack.py` (Q1, Q2), `scripts/probe_api.py --out <file>` (Q3 to Q9,
"section N" below is a section of its report), the pinned schema `bugsink-api/openapi.yaml` (`openapi.yaml:NNN`
is a line of it) and a few hand-run requests, marked as such. Seeded tokens: `GLOBAL_READ`, `GLOBAL_TRIAGE`,
`PROJECT_READ` (bound to project 1), `NO_CAPS` (see `docker/README.md`).

## Q1. Is the `trace` tag of a Bugsink event the OpenTelemetry trace id that Tempo has?

**Yes, with two conditions.** With the Sentry Python SDK's `OTLPIntegration(setup_otlp_traces_exporter=False)`,
the event's `data.contexts.trace.trace_id` equals the OpenTelemetry trace id, and Tempo returns that trace.
The second condition: in a FastAPI/Starlette app with Sentry's default ASGI integration the OpenTelemetry
middleware must be the outermost wrapper; otherwise unhandled-error events carry a trace id Tempo does not have
(observed by hand, Task 3; not reproduced by a script).

Evidence, `uv run scripts/verify_stack.py` (all six checks):

```text
ok   sample /handled returned trace_id=704573ad99b507a714efe5b3e5934dfe event_id=4fdfb70cb0ac426e8be1f2aeb3e6f1ef
ok   bugsink event 4fdfb70cb0ac426e8be1f2aeb3e6f1ef has trace id 704573ad99b507a714efe5b3e5934dfe
ok   bugsink search trace:704573ad99b507a714efe5b3e5934dfe returned 1 issue(s) ['5983e609-fcd6-4957-ba76-20a598dd9d8b'], expected [5983e609-fcd6-4957-ba76-20a598dd9d8b]
ok   tempo trace 704573ad99b507a714efe5b3e5934dfe has 7 span(s)
ok   loki has 1 line(s) with trace_id 704573ad99b507a714efe5b3e5934dfe
ok   no-trace event: trace id daaefeddeb9e4a5295e76d9089a62a16 present in Bugsink, absent from Tempo
```

Check 2 reads `data.contexts.trace.trace_id` from `GET events/lookup/?project=1&event_id=<id>`; check 4 reads the
trace through Grafana's Tempo data source proxy. The `trace` tag is Bugsink's index of that same field (probe
section 6 lists the tag keys: `trace`, `trace.span`, `trace.ctx` among them).

The middleware order was found by hand while building the sample app (Task 3; not reproduced by a script, and
not reproducible with the committed app, which uses the working order): with `FastAPIInstrumentor.instrument_app(app)`, `/handled` matched Tempo but the
unhandled `/error` did not (event trace id `c07f5748...`, Tempo trace `19311112...` for the same request).
Sentry's ASGI middleware sat outside the OpenTelemetry middleware, so Sentry captured the exception after the
request span ended and fell back to a random trace id from its propagation context. The sample app builds a
plain `FastAPI()` and exports `OpenTelemetryMiddleware(api)` (`opentelemetry-instrumentation-asgi`) as the ASGI
app; `/error` events then match Tempo. The cost is that FastAPI-specific span attributes such as `http.route`
are absent; span names are still `GET /error`.

## Q2. Does the default LogQL template of spec §5 find the event's log lines in Loki?

**Yes.** Logs reach Loki through the collector's OTLP/HTTP export. `service_name` is an index label and
`trace_id` (32 lowercase hex characters) is structured metadata, so `{service_name="${service}"} | trace_id="${trace_id}"`
matches. Other resource attributes can be labels as well (see below).

Evidence, `verify_stack.py` check 5, and a hand-run query on the sample app's logs:

```text
ok   loki has 1 line(s) with trace_id 1ad3a2e4cef68690b05238d64b7dce1a
```

Hand-run through the Grafana proxy (`/api/datasources/proxy/uid/loki/loki/api/v1/...`) on the sample app's logs:

```text
GET labels  -> ["deployment_environment", "service_instance_id", "service_name"]
{trace_id=~".+"}  -> no streams (trace_id is not a label)
{service_name="sample-python"} | trace_id!=""  -> stream has trace_id, span_id, severity_text, detected_level ... as structured metadata
```

Loki's OTLP ingestion indexes a fixed set of resource attributes by default; of the sample app's resource
attributes, three become labels (`service_name`, `deployment_environment`, `service_instance_id`). A smoke
test that sent only `service.name` (Task 2) produced `service_name` as the single label. Everything else, including `trace_id` and `span_id`, is
structured metadata, so the template needs the `| trace_id=` filter form, not a label matcher. Which label
holds the service name depends on the application's resource `service.name`; the sample app's tag `service`
and its `service.name` are both `sample-python`.

## Q3. Where do environment, release and the service name live in event `data`?

**`data.environment`, `data.release` and `data.tags.service`.** The same values are issue tag keys.

Evidence, probe section 6 (latest `/handled` event):

```text
data.tags.service = sample-python
data.release = sample-python@0.1.0
data.environment = development
data.server_name = <container id>
Tag keys from issues/<issue>/tags/: ['environment', 'handled', 'release', 'server_name', 'service', 'trace', 'trace.ctx', 'trace.span', 'transaction', 'url']
```

Also observed (Task 5, event detail keys): `data.transaction` (`/handled`), `data.breadcrumbs.values`,
`data.contexts.trace.{trace_id,span_id}`, `data.sdk`, `data.request`, `data.exception`. The list endpoint
returns no `data`; only `events/{id}/` and `events/lookup/` do (`EventDetail`). `data.tags` is a dict. In the
sample app the `service` tag is set by the app; Bugsink does not derive it.

## Q4. How can a client learn what a token may do?

**It cannot ask; it must try and read the refusal.** No endpoint returns the calling token's capabilities.
The schema documents the capability each operation needs (`x-bugsink-required-capability`) and a static
list of capabilities (`x-bugsink-capabilities`, the same for every token). A refusal for a missing capability
is a 403 whose body names it; a refusal for a project the token is not bound to is a 403 that names none;
a bad or missing token is a 401.

Evidence, probe section 2:

```text
401 (GET projects/, no token): {"detail": "Authentication credentials were not provided."}
403 (GET projects/, NO_CAPS): {"detail": "This token does not have the required capability: projects:read."}
403 (GET projects/2/, PROJECT_READ): {"detail": "This token is not allowed to access this project."}   names a capability: no
```

Schema, per operation the plugin calls (Task 5): `projects:read` for `projects/`, `projects/{id}/`,
`projects/{id}/event-counts/`; `issues:read` for `issues/`, `issues/{id}/`, `issues/{id}/event-counts/`,
`issue-history/`; **`events:read`** for `issues/{id}/tags/`, `events/`, `events/{id}/`, `events/lookup/`,
`events/{id}/stacktrace/`; `issues:triage` for the eight triage POSTs; `issues:comment` for `POST issue-comments/`.
The twelve capabilities in the schema also include `issues:delete`, `projects:manage`, `teams:read`,
`teams:manage`, `releases:read`, `releases:create`, `debug-files:upload`.

`GET /api/0/` is a minimal Sentry-like root that exists for sentry-cli login checks (`openapi.yaml:1079-1080`). Its 200 response
has a required `auth.scopes` array of strings (`openapi.yaml:1083-1103`), and the operation requires `debug-files:upload`
(`x-bugsink-required-capability`). A token without it gets a 403 naming that capability: with `GLOBAL_READ` it answered
`{"error": "This token does not have the required capability: debug-files:upload."}` (hand-run, Task 5); its 403 body key is `error`,
canonical endpoints use `detail`. The recommended read token does not hold `debug-files:upload`.

Hand-run with temporary tokens (created and deleted with `bugsink-manage shell`, not seeded): a token holding only `debug-files:upload`
and a token holding `debug-files:upload` plus `issues:read` both got `200 {"version": "0", "auth": {"scopes": ["org:ci"]}, "user": null}`.
So `auth.scopes` was the same fixed list for both tokens and does not list the token's capabilities. Otherwise a client learns a
capability only by calling an operation that needs it and reading the refusal. Not tried: any endpoint outside the pinned schema.

## Q5. What does a project-bound token get from each endpoint the plugin uses?

**Everything inside its project, nothing outside it, and it can list projects, getting only its own.** The
spec's statement that a project-bound token cannot list projects is wrong. A token with no capabilities gets
403 everywhere; no token gets 401.

Evidence, probe section 1 (cells are HTTP statuses):

| Request | GLOBAL_READ | GLOBAL_TRIAGE | PROJECT_READ | NO_CAPS | (no token) |
| --- | --- | --- | --- | --- | --- |
| GET projects/ | 200 (ids [1, 2]) | 200 (ids [1, 2]) | 200 (ids [1]) | 403 | 401 |
| GET projects/1/ | 200 | 200 | 200 | 403 | 401 |
| GET projects/2/ | 200 | 200 | 403 | 403 | 401 |
| GET issues/?project=1 | 200 | 200 | 200 | 403 | 401 |
| GET issues/?project=2 | 200 | 200 | 403 | 403 | 401 |
| GET issues/ (no project) | 200 | 200 | 200 | 403 | 401 |
| GET issues/?project=1&sort=digest_order | 200 | 200 | 200 | 403 | 401 |
| GET issues/?sort=digest_order (no project) | 400 | 400 | 200 | 403 | 401 |
| GET issues/<issue>/ | 200 | 200 | 200 | 403 | 401 |
| GET events/?issue=<issue> | 200 | 200 | 200 | 403 | 401 |
| GET events/lookup/?project=1&event_id=<event> | 200 | 200 | 200 | 403 | 401 |
| GET issues/<issue>/tags/ | 200 | 200 | 200 | 403 | 401 |
| GET issue-history/?issue=<issue> | 200 | 200 | 200 | 403 | 401 |
| GET issues/<issue>/event-counts/ | 200 | 200 | 200 | 403 | 401 |
| GET projects/1/event-counts/ | 200 | 200 | 200 | 403 | 401 |
| POST issues/<issue>/mute/ then unmute/ | 403 / 403 | 200 / 200 | 403 / 403 | 403 / 403 | 401 / 401 |

A read-only token cannot triage (403); only `GLOBAL_TRIAGE` can. The `PROJECT_READ` token's implicit project
makes `issues/?sort=digest_order` without `project` succeed (200) where global tokens get 400 (see Q6).
Schema text agrees: the issue list says "Project-bound tokens stay within their project".

## Q6. Does `GET /issues/` work without `project`, as spec §4.B says?

**Yes, for the default sort.** A global token gets issues of every project it can see, ordered by `last_seen`
descending. `sort=digest_order` needs a single project: 400 for a global token without `project`.

Evidence, probe sections 3 and 1:

```text
GET issues/ with GLOBAL_READ: status 200
Projects represented in the first page: 2 [1, 2]
Project 1 has 262 issues and project 2 has 1; the first page holds 250 results, `next` is set
GET issues/?sort=digest_order (no project): GLOBAL_READ 400, GLOBAL_TRIAGE 400, PROJECT_READ 200
```

By hand (Task 5): `GET issues/?sort=digest_order` returns 400 `digest_order requires a single project.` The schema
describes `project` as optional: "Omit to list all projects accessible to the token". Defaults depend on scope
(probe section 5): across projects `last_seen` descending; with `project=1`, `digest_order` ascending. A
single-project query therefore needs `sort=last_seen&order=desc` sent explicitly for the plugin's "most recent first" use.

## Q7. Can events be searched by `q=trace:<id>` without an `issue` filter? Does `GET /issues/?q=trace:<id>` find the issue?

**No to the first, yes to the second.** `GET events/` requires `issue` (400 without it), so events cannot be
searched by trace across issues. `GET issues/?q=trace:<id>` finds the issue, with or without `project`, and
`GET events/?issue=<id>&q=trace:<id>` then finds the events. A trace to events link therefore goes trace, issue, events.

Evidence, probe section 4:

| Request | Status | Results | Contains the trace's issue |
| --- | --- | --- | --- |
| `GET events/?q=trace:<id>` (no issue) | 400 | - | `{"issue": ["This field is required."]}` |
| `GET events/?issue=<issue>&q=trace:<id>` | 200 | 1 | yes |
| `GET issues/?project=1&q=trace:<id>` | 200 | 1 | yes |
| `GET issues/?q=trace:<id>` (no project) | 200 | 1 | yes |

A bare id without the `trace:` prefix returned no results (Task 4, hand-run). The schema marks `issue` as a
required query parameter of `GET /events/`.

## Q8. What does a page look like: the `next` field, the page size, the order with `sort=last_seen`?

**`{next, previous, results}`, 250 results per page, no total count.** `next` is an absolute URL with an opaque
`cursor` that keeps `sort` and `order`. With `sort=last_seen&order=desc`, `last_seen` never increases across
pages. The event list is ordered by `digest_order` descending (arrival order); it has no timestamp sort.

Evidence, probe section 5:

```text
Top-level keys: ['next', 'previous', 'results']
Page length: 250
`next` form: http://localhost:8800/api/canonical/0/issues/?cursor=<opaque>&order=desc&sort=last_seen
`previous` on the first page: None
Following `next` to the end: 2 pages, 263 issues, page lengths [13, 250]; `last_seen` never increases: yes
Default order of GET issues/ (all projects): last_seen descending, digest_order mixed
Default order of GET issues/?project=1: last_seen mixed, digest_order ascending
Default order of GET events/?issue=<issue> (at least two events): digest_order descending, timestamp descending, ingested_at descending
```

The probe prints the distinct page lengths in sorted order, so `[13, 250]` means one page of 250 and a last page of 13;
the 263 issues are 262 of project 1 and 1 of project 2. In the probe's data the
event order by `digest_order`, `timestamp` and `ingested_at` agree, which only shows they agree when
events arrive in order; the schema's `order` parameter documents `digest_order` only
(`openapi.yaml:35`: "Sort order of digest_order (default: desc)"), so for late-arriving events
timestamp order is not guaranteed. The schema has no count field on any list (`PaginatedIssueList`, `openapi.yaml:2155-2173`).

## Q9. `event-counts`: parameter names, bucket shape, behaviour off hour boundaries and beyond 90 days

**`start` and `end`, both optional, on UTC hour boundaries, within the last 90 days.** The response is
`{start, end, interval_seconds: 3600, buckets: [{timestamp, count}]}`, buckets ascending, one per hour.
With no parameters, `end` is the end of the current hour and `start` is `end` minus 24 hours (24 buckets). A
`start` off the hour, a range over 90 days, or a `start` older than 90 days is a 400.

Evidence, probe section 7:

```text
start/end on hour boundaries, 24 h: 200, keys ['buckets', 'end', 'interval_seconds', 'start'], bucket keys ['count', 'timestamp'], 24 buckets, interval_seconds 3600
start off the hour boundary: 400 {"start": ["Must be on a UTC hour boundary."]}
start 91 days back: 400 {"start": ["The range cannot exceed 90 days."]}
no parameters: 200, 24 buckets, interval_seconds 3600
end only, on the hour: 200, 24 buckets
Defaults by observation: end is the end of the current hour, start is end minus 24 h; buckets are in ascending order
projects/1/event-counts/ with no parameters: 200, same shape
```

Schema (Task 5): `start` inclusive, `end` exclusive; the current hour is included and incomplete; both
endpoints (`issues/{id}/event-counts/`, `projects/{id}/event-counts/`) have the same parameters and response.
The schema says "Ranges must use UTC hour boundaries and fall within the last 90 days" (`openapi.yaml:345`, also `:822`).
Hand-run request, a one-day range 104 days back:

```text
GET projects/1/event-counts/?start=2026-08-01T00:00:00Z&end=2026-08-02T00:00:00Z  -> 400 {"start":["Hourly counts are retained for 90 days."]}
```

Not probed: an `end` off the hour boundary.

## Q10. Does the schema contain every endpoint, parameter and field named in spec §4.B, §4.C, §4.E and §6?

**Yes, every named endpoint exists, with eight differences from the spec's wording.** The spec says "as of
2.6.2"; the schema is 2.6.3 (`openapi.yaml:4`), so the comparison does not cover 2.6.2.

The eight differences (Task 5):

1. `sort=digest_order` needs a single project (live 400); sort and order defaults depend on scope (Q6).
2. Issue history is `GET /issue-history/?issue=<uuid or friendly id>`, not nested under `issues/{id}/`. Items are
   `TurningPoint {id, issue, project, triggering_event, user, user_display_name, timestamp, kind, comment, metadata}`,
   author name in `user_display_name` (nullable); `kind` is one of first_seen, resolved, muted, regressed,
   unmuted, reopened, next_materialized, manual_annotation, so comments appear in the history too. `metadata` is
   an untyped object that depends on `kind`.
3. Issue comments are `POST /issue-comments/` with body `{issue, comment}` (response 201), capability `issues:comment`, not nested under the issue.
4. `q=trace:<id>` on `/events/` works only within one issue, because `issue` is required (Q7).
5. A project-bound token can list its own project (Q5).
6. No documented way to learn a token's capabilities (Q4).
7. "Newest first" for the event list is `digest_order` order only (`openapi.yaml:35`), no timestamp sort (Q8).
8. "Open-issue count" (spec §6): no list response has a count field (`openapi.yaml:2155-2173`), so a count needs a page-through.

Other findings from the comparison:

* Tag values with counts are `GET issues/{id}/tags/` (`IssueTag {id, key, value, count}`) and need `events:read`, not `issues:read`.
  The token the spec recommends already has `events:read`.
* The issue fields the spec names all exist. `calculated_type`, `calculated_value`, `transaction`, `is_resolved`,
  `is_resolved_by_next_release`, `is_muted` and `unmute_after` are **not in the schema's `required` list** although every
  live response contains them (`Issue.required`, `openapi.yaml:2011-2022`); `unmute_after` is also nullable (`openapi.yaml:2000-2003`)
  and is `null` for issues that are not muted. A generated client must treat them as optional.
* Extra fields not in the spec: `is_resolved_unconditionally`, `fixed_at[]`, `events_at[]`, `digest_order`,
  `unmute_on_volume_based_conditions[] {period, nr_of_periods, volume}`.
* Issue paths accept the UUID or the friendly id (`SAMPLE-PYTHON-2`).
* `EventList` has `id` (Bugsink's own uuid), `event_id` (the SDK id), `timestamp`, `ingested_at`, plus `digested_at`,
  `issue`, `grouping`, `project`, `digest_order`; no `data`. `EventDetail.data` is `readOnly` and untyped; `stacktrace_md` is a string.
  `GET events/{id}/stacktrace/` returns `text/markdown`.
* Triage: all eight POSTs exist; `mute-for/` takes `period_name` (year, month, week, day, hour, minute) and `nr_of_periods` (min 1);
  `mute-until/` adds `gte_threshold` (min 1). Each documents only `200 -> Issue`; no error response is documented
  on any canonical operation. The `mute/` response has the same keys as `GET issues/<issue>/` (probe section 8).
* Every canonical operation lists a second security scheme `bearerAuth` that `components.securitySchemes`
  does not define (first at `openapi.yaml:47`). This is a schema bug in 2.6.3 and breaks `ogen` (Q11).
* The schema is served without a token (`GET /api/canonical/0/schema/`, 200), as YAML by default (JSON with `Accept: application/json`).

## Q11. Does a Go client generate cleanly from the schema, and with which generator?

**`oapi-codegen` v2.8.2 does; `ogen` v1.25.0 does not.** `oapi-codegen` generates and builds from the
unmodified schema. `ogen` fails on four constructs and works only on a patched, canonical-only copy.
Adopted for M1: `oapi-codegen` with `-generate types,client`.

Evidence (Task 5, Go 1.26.10, scratch directory outside the repository):

```text
go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.2 -generate types,client -package client -o client/client.go bugsink-api/openapi.yaml
-> exit 0, no warnings, one file of 11425 lines; after `go mod tidy`, `go build ./...` and `go vet` clean

go run github.com/ogen-go/ogen/cmd/ogen@v1.25.0 --target api --package api --clean bugsink-api/openapi.yaml
-> exit 1: openapi.yaml:47:9 -> unknown security scheme "bearerAuth"
```

What blocks `ogen`, in the order met: (1) the undefined `bearerAuth` scheme on 32 operations; (2) `POST /api/0/organizations/{organization_slug}/chunk-upload/`,
a `multipart/form-data` body with `{type: object, additionalProperties: true}` ("complex form schema" not implemented); (3) `POST /api/{project_pk}/minidump/`,
a similar multipart body, which with `ignore_not_implemented` makes `ogen` panic; (4) `application/csp-report` on `POST /api/{project_pk}/security/`
("unsupported content type"). It succeeded only on a copy with `bearerAuth` renamed and restricted to the 26 `/api/canonical/` paths
(32218 lines in 17 files, including a server side, with OpenTelemetry dependencies).

Costs of `oapi-codegen` to plan for: every property that is not `required`, and every `readOnly` property even
when required (`Issue.Id`, `FriendlyId`, `StoredEventCount`), is a pointer; nullable and absent are not
distinguishable (`UnmuteAfter *time.Time`); `EventDetail.Data` is `interface{}`; the bearer token is added with
a request editor (it ignores the dangling `bearerAuth`); error bodies are untyped; enum constants such as
`PeriodNameEnum` values (`Day`, `Hour`, ...) are unprefixed. `ogen` types are more exact (`OptNilDateTime`, `jx.Raw`
for `data`) and would be the fallback over a script-filtered copy of the schema, which would stop the generator input being the committed file. The
undefined `bearerAuth` could be reported to Bugsink.

## Other findings

* **No-span events still carry a trace id.** An event captured with no active span (`POST /no-trace` runs the
  capture in a new thread) has `data.contexts.trace.trace_id` and `span_id` from the SDK's propagation context;
  Bugsink derives a `trace` tag from it; Tempo does not have the trace. The only difference in `contexts.trace` is
  `parent_span_id: null`; the rest of the event was not compared. So "has a trace id" does not distinguish a real trace from a made-up one, and a link can lead to a
  trace Tempo does not have. Evidence: probe section 6 (`/no-trace` event: `contexts.trace` present: yes; keys only on it:
  `['parent_span_id']`) and `verify_stack.py` check 6:
  ```text
  ok   no-trace event: trace id 5d005bc2d063409a9603fe0314d27a86 present in Bugsink, absent from Tempo
  ```
* **`OTLPIntegration` replaces the global propagator.** In the sample app an incoming W3C `traceparent` header
  was ignored: trace ids were fresh (observed by hand, Task 3; not reproduced by a script). The option `setup_propagator=False` was not tested, so what
  keeps both inbound propagation and the shared trace id is open.
* **Grouping replaces integers.** Bugsink's message normalisation replaces integers with `<int>`, so
  `(variant 1)` and `(variant 2)` landed in one issue. The sample app writes `(variant v1)`; letter-prefixed
  values stay separate issues (checked for v1, v2, v12, v60; the same variant twice gives one issue with two events).
* **Page size is 250** (Q8); a test that needs a second page needs more than 250 issues. The probe makes 130
  `/handled` and 130 `/error` variants, giving 261 issues in project 1 (260 variants plus the `/no-trace` issue); the 262nd is `verify_stack.py`'s variant-less `/handled` issue.
* **Ingestion throttle.** Bugsink throttled ingestion at about 1000 events per 5 minutes and answered HTTP 429; after repeated full
  probe runs `verify_stack.py` then found no new events until the window passed. This was observed by hand during development;
  no probe section tests it and the exact limits were not measured. The probe's first run on an empty stack sends 264 events;
  later runs send 10 (it skips the bulk once project 1 has at least 260 issues).
* **Digest is asynchronous.** `events/lookup/` returns 404 until the event is digested (several seconds), so
  a client or a test must retry.
* **Compose `--wait`.** On Compose v5.7.1 a bare `docker compose up -d --wait` fails when a one-shot
  service exits, even with code 0 (`container ...-bugsink-seed-1 exited (0)`). It succeeds when a long-running service
  depends on the one-shot with `condition: service_completed_successfully` (Grafana and the sample app do).
* **Distroless images.** The Loki and collector images have no shell, `curl` or health flag, so neither has a health
  check; Grafana's health check also waits for Loki's `/ready`, so `up --wait` implies Loki is ready.
* **Collector warning.** The collector warns that the exporter names `otlp` and `otlphttp` are deprecated aliases
  (use `otlp_grpc`, `otlp_http`); they work and the config keeps them.
* **Tempo.** Tempo 3.1.1 runs as one binary with `-target=all` and a minimal config; its log has harmless warnings
  (metrics-generator not configured, "no jobs found").
* **Batch processor.** Rebuilding the sample app container can drop spans still queued in the OpenTelemetry batch processor (seen once).
* **Event `/no-trace` relies on the thread.** `POST /no-trace` assumes a new `threading.Thread` has an empty OpenTelemetry context (true on Python 3.13).

## Consequences

Edits made in this change (S = spec, R = roadmap):

| # | Document | Section | Change | Finding |
| :--- | :--- | :--- | :--- | :--- |
| S1 | Spec | §2 Target Versions | Behaviour checked on 2.6.3 only; 2.6.2 stays as the floor | Q10 |
| S2 | Spec | §4.A Authentication | A project-bound token lists its own project (how the config editor uses this: §10) | Q5 |
| S3 | Spec | §4.A Health check | No endpoint reports capabilities (`GET /api/0/` returns a fixed `auth.scopes` and needs `debug-files:upload`); the two 403 forms tell a client what is missing | Q4 |
| S4 | Spec | §4.B Issue queries | Verified against 2.6.3; `digest_order` needs one project; default sort by scope; page size 250, no count; history path; tag values need `events:read` | Q6, Q8, Q10 |
| S5 | Spec | §4.C Event queries | Event list needs `issue` and is in arrival order; trace search works on issues and within one issue; reverse link restated | Q7, Q8, Q10 |
| S6 | Spec | §4.D Event detail | Note beside "hidden when no `trace_id`": events without a span still carry one | Other findings |
| S7 | Spec | §4.E Attribution | Comments are created with `POST /issue-comments/` | Q10 |
| S8 | Spec | §5 Prerequisite | Working Python setup (hand observations point to this document); ASGI middleware order; no-span events; `traceparent` ignored by default | Q1, Other findings |
| S9 | Spec | §5 Loki template | Which labels exist; `trace_id` is structured metadata | Q2 |
| S10 | Spec | §6 | Opening line (2.6.2 and later, verified on 2.6.3); event-counts defaults and limits (within the last 90 days); open-issue count needs a page-through today | Q9, Q10 |
| S11 | Spec | §7 | Ingestion throttle constrains tests that produce events | Other findings |
| S12 | Spec | §10 Open Questions | Added eight questions (below) | Q4, Q5, Q7, Q8, Other findings, Q10, Q11 |
| R1 | Roadmap | M0 | Scope states the seeded projects and tokens; questions marked answered, link to this document | all |
| R2 | Roadmap | Definition of done | End-to-end tests stay under the ingestion throttle | Other findings |
| R3 | Roadmap | M1 | Client generated with `oapi-codegen`; health check covers capabilities the token lacks (method open); a project-bound token's project can be discovered (choice open); demo: no-capabilities token and project-bound token replace "project-bound token without a project id" | Q4, Q5, Q11 |
| R4 | Roadmap | M2 | Explicit `sort=last_seen&order=desc`; `digest_order` only with a project; page size, no total | Q6, Q8 |
| R5 | Roadmap | M4 | History endpoint; tag values capability; event list order left to spec §10; trace search through the issue list; demo reworded; "Needs first" line | Q7, Q8, Q10 |
| R6 | Roadmap | M6 | Documentation covers ASGI middleware order, no-span traces and `traceparent`; decision needed on the missing-trace case; demo reworded; "Needs first" line | Q1, Other findings |
| R7 | Roadmap | M10 | Open-issue count costs a page-through; "Needs first" line | Q10 |
| R8 | Roadmap | Open decisions | Eight rows for the new spec §10 questions | Q4, Q5, Q7, Q8, Other findings, Q10, Q11 |

Consequences that need a decision, not an edit (all recorded in spec §10):

| Decision | Needed by | Why |
| :--- | :--- | :--- |
| What the UI does when the event's trace is not in Tempo (the "hidden when no `trace_id`" rule will rarely apply) | M6 | Events without a span carry a trace id Tempo lacks |
| Whether the documented setup keeps the OTLP integration's propagator or sets `setup_propagator=False` so inbound `traceparent` works | M6 (documentation) | The default propagator ignored `traceparent`; the option is untested |
| How the health check learns capabilities: one call per capability, or only for configured features | M1 | `GET /api/0/` needs `debug-files:upload` and returned a fixed `auth.scopes` in the hand-run; no other endpoint reports capabilities |
| How the event list is ordered in the UI: arrival order as served, or re-sorted by `timestamp` within what was fetched | M4 | The API orders by `digest_order` only |
| How the open-issue count for alert rules is obtained: page through the list per evaluation, cache it, or ask Bugsink for a count | M10 | List responses carry no count |
| Whether the events query type fans out across issues for `trace:<id>` | M4 | `GET /events/` needs an `issue` |
| Whether the config editor for a project-bound token discovers the project or also accepts an id | M1 | `GET /projects/` returns the token's project |
| Whether code generated from the PolyForm Shield schema may be compiled into the GPL plugin | M1 | M1 adopts `oapi-codegen` output; the licence status is unrecorded |

Not decided here, noted for later: `oapi-codegen` makes optional and `readOnly` fields pointers, so the
backend should treat missing fields as absent rather than assume presence (Q10, Q11); the JavaScript sample app (M6) has not been tried against any of these questions.

## Versions

Taken from `docker-compose.yaml`, `sample-apps/python/Dockerfile`, `sample-apps/python/uv.lock` and the task logs.

| Component | Version |
| :--- | :--- |
| Bugsink | `bugsink/bugsink:2.6.3` (Python 3.12 inside the image) |
| PostgreSQL | `postgres:17.12-alpine` |
| Grafana | `grafana/grafana:13.3.1` |
| Tempo | `grafana/tempo:3.1.1` |
| Loki | `grafana/loki:3.7.10` |
| OpenTelemetry Collector | `otel/opentelemetry-collector-contrib:0.165.0` |
| Python base image (sample app) | `python:3.13.18-slim` |
| uv (in the sample app image) | `ghcr.io/astral-sh/uv:0.13.8` |
| sentry-sdk | 2.75.0 (extra `opentelemetry-otlp`) |
| opentelemetry-sdk | 1.45.2 |
| opentelemetry-instrumentation-asgi | 0.66b2 |
| fastapi / uvicorn | 0.162.7 / 0.54.0 |
| Docker Compose | v5.7.1 (Docker 29.9.3) |
| Go (client trial) | 1.26.10 |
| oapi-codegen | v2.8.2 |
| ogen | v1.25.0 |
