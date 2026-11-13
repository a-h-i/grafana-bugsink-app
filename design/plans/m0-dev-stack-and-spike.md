# M0: Dev Stack & Verification Spike Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Compose stack (Bugsink, Grafana, Tempo, Loki, OpenTelemetry Collector, a Python sample app) that produces an error linked to a trace and logs, plus written answers to the questions the later milestones are built on.

**Architecture:** One `docker-compose.yaml` at the repo root, with per-service config under `docker/`. Bugsink is seeded by a one-shot service that runs a Django shell script, so projects, DSNs and API tokens are the same on every machine. Two host-side Python scripts (standard library only) turn the demo and the API questions into repeatable checks.

**Tech Stack:** Docker Compose v2+, Bugsink 2.6.3, PostgreSQL 17, Grafana 13.3.1, Tempo 3.1.1, Loki 3.7.10, OpenTelemetry Collector contrib 0.165.0, Python 3.12+ with FastAPI, `sentry-sdk[opentelemetry-otlp]` 2.75+, `uv`.

**Spec:** [`../bugsink_grafana_plugin_spec.md`](../bugsink_grafana_plugin_spec.md) (§7, §8 Phase 0). **Roadmap:** [`../implementation_roadmap.md`](../implementation_roadmap.md) (M0).

## Global Constraints

* No plugin code in this milestone: no `src/`, no Go module, no `package.json`. M1 owns the scaffold.
* Bugsink is 2.6.2 or later (spec §2); this plan pins `bugsink/bugsink:2.6.3`.
* Every image is pinned to an exact version tag, never `latest`.
* Host ports are `3300` (Grafana), `8800` (Bugsink) and `8810` (sample app), each overridable from `.env`. Never publish `3000` or `8000`: other local stacks on the development machine use them.
* Tempo, Loki, the collector and PostgreSQL publish no host ports. Host-side scripts reach Tempo and Loki through Grafana's data source proxy.
* The seeded tokens and keys are fixed, public, development-only values. They appear only in `docker/` files and scripts, each with a comment saying so.
* Bugsink's OpenAPI schema is committed unmodified under Bugsink's license (PolyForm Shield 1.0.0), in its own directory with a notice (roadmap, Licensing).
* New source files start with `SPDX-License-Identifier: GPL-3.0-or-later` in a comment, except the schema.
* Work on branch `m0/dev-stack`; push after every commit.

## Facts already verified (2026-11-13, against the `bugsink/bugsink:2.6.3` image)

The tasks rely on these; they were read from the image, not from documentation.

* The image runs as user `bugsink`, serves on `$PORT` (default 8000), and has `GET /health/ready`.
* `ALLOWED_HOSTS` is `['*']`: a client inside the Compose network may use `http://bugsink:8000` while `BASE_URL` is `http://localhost:8800`.
* A DSN is `http://<sentry_key hex>@<host>/<project id>`. `Project.sentry_key` is a settable UUID field; `Project.id` is an auto-increment integer.
* `bsmain.models.AuthToken` has `token` (exactly 40 characters of `[a-f0-9]`), `description`, `is_project_bound`, `project`, `is_user_bound`, `user`, and one boolean per capability: the capability name with `:` and `-` replaced by `_` (`issues:read` → `issues_read`). `clean()` rejects a project-bound token with `projects:manage`, `teams:read` or `teams:manage`.
* The only token management command, `create_auth_token`, makes full-access tokens. Scoped tokens need the model.
* Routes under `/api/canonical/0/`: `issues/` with `resolve`, `resolve-latest`, `resolve-next`, `reopen`, `mute`, `mute-for`, `mute-until`, `unmute`, `tags`, `event-counts`, `grouping`; `events/`, `events/lookup/`, `events/{id}/stacktrace/`; `issue-comments/`, `issue-history/`; `projects/` with `event-counts`; `releases/`; `teams/`; `schema/`. No route reports the calling token's capabilities.
* The schema generator adds an `x-bugsink-required-capability` extension to operations.
* `tags/utils.py` derives the tag `trace` from `contexts.trace.trace_id`, and `release`, `environment`, `server_name`, `transaction` from top-level event fields (read from source, not yet from a running event).

## Questions M0 answers

Each gets a section in `design/phase0_findings.md` (Task 7) with the evidence: the command and the relevant output.

| # | Question | Decides | Answered by |
| :--- | :--- | :--- | :--- |
| Q1 | Is the `trace` tag of a Bugsink event the OpenTelemetry trace id that Tempo has? | spec §5 as a whole | Task 4 |
| Q2 | Does the default LogQL template of spec §5 find the event's log lines in Loki (`service_name` label, `trace_id` structured metadata)? | §5 default template | Task 4 |
| Q3 | Where do environment, release and the service name live in event `data`? | §4.C, `${service}` source | Task 6 |
| Q4 | How can a client learn what a token may do? | M1 health check | Tasks 5, 6 |
| Q5 | What does a project-bound token get from each endpoint the plugin uses? | §4.A | Task 6 |
| Q6 | Does `GET /issues/` work without `project`, as spec §4.B says? (Bugsink's docs say it is required.) | M2 project picker | Task 6 |
| Q7 | Can events be searched by `q=trace:<id>` without an `issue` filter? If not, does `GET /issues/?q=trace:<id>` find the issue? | §4.C reverse link, M4 | Task 6 |
| Q8 | What does a page look like: the `next` field, the page size, the order with `sort=last_seen`? | M2 pagination | Task 6 |
| Q9 | `event-counts`: parameter names, bucket shape, behaviour off hour boundaries and beyond 90 days. | §6, M9 | Task 6 |
| Q10 | Does the schema contain every endpoint, parameter and field named in spec §4.B, §4.C, §4.E and §6? | spec corrections | Task 5 |
| Q11 | Does a Go client generate cleanly from the schema, and with which generator? | M1 | Task 5 |

## Review Focus

* **Second `docker compose up` on an existing database:** the seed must succeed and change nothing. Pinned in Task 1, Step 5.
* **Bugsink digests events asynchronously:** an event is not queryable the instant the SDK call returns. Every check polls with a deadline and never sleeps a fixed time. Pinned in Task 4, Step 1.
* **A host port already in use:** the stack must be movable without editing tracked files. Pinned in Task 1, Step 6.
* **An event without a trace** (an error outside any span): it must still arrive, with no `trace` tag, so M6 can test "no buttons". Pinned in Task 3, Step 4.
* **A probe that gets an unexpected status must report it, not crash:** the probe's output is evidence, and a 403 or 404 is an answer. Pinned in Task 6, Step 1.

## File Structure

| File | Responsibility |
| :--- | :--- |
| `docker-compose.yaml` | All services, pinned images, health checks, start order. |
| `.env.example` | Host ports and image tags that may be overridden. |
| `docker/bugsink/seed.py` | Team, projects, DSN keys and API tokens (Django shell script). |
| `docker/otel-collector/config.yaml` | OTLP in; traces to Tempo, logs to Loki. |
| `docker/tempo/tempo.yaml`, `docker/loki/loki.yaml` | Minimal single-binary configs, local filesystem storage. |
| `docker/grafana/provisioning/datasources/datasources.yaml` | Tempo and Loki data sources with fixed UIDs. |
| `docker/README.md` | How to run the stack; the seeded values; the two scripts. |
| `sample-apps/python/` (`pyproject.toml`, `uv.lock`, `app.py`, `Dockerfile`) | Error-producing service with shared trace context. |
| `scripts/verify_stack.py` | The M0 demo as a check (Q1, Q2). |
| `scripts/probe_api.py` | Endpoint × token matrix and the API questions (Q3 to Q9). |
| `scripts/fetch_schema.sh` | Downloads the schema from the running Bugsink. |
| `bugsink-api/openapi.yaml`, `bugsink-api/NOTICE.md` | The pinned schema and its source, version and license. |
| `design/phase0_findings.md` | Answers Q1 to Q11 with evidence. |

## Seeded values

Used by several tasks. All are development-only.

| Thing | Value |
| :--- | :--- |
| Superuser | `admin@example.com` / `admin` (`CREATE_SUPERUSER`) |
| Team | `Demo` |
| Project 1 | slug `sample-python`, id `1`, `sentry_key` `11111111-1111-1111-1111-111111111111` |
| Project 2 | slug `sample-js`, id `2`, `sentry_key` `22222222-2222-2222-2222-222222222222` (used from M6; here it is the "other project" for Q5) |
| Token `GLOBAL_READ` | `a` × 40; `projects:read`, `issues:read`, `events:read` |
| Token `GLOBAL_TRIAGE` | `b` × 40; the three above plus `issues:triage`, `issues:comment` |
| Token `PROJECT_READ` | `c` × 40; bound to project 1; `projects:read`, `issues:read`, `events:read` |
| Token `NO_CAPS` | `d` × 40; global, no capabilities |
| DSN inside Compose | `http://11111111111111111111111111111111@bugsink:8000/1` |
| Grafana data source UIDs | `tempo`, `loki` |

---

### Task 1: Bugsink, PostgreSQL and the seed

**Files:**
- Create: `docker-compose.yaml`, `.env.example`, `docker/bugsink/seed.py`

**Interfaces:**
- Produces: service `bugsink` reachable at `http://bugsink:8000` in the Compose network and `http://localhost:${BUGSINK_PORT:-8800}` on the host; the seeded values above.

- [ ] **Step 1: Write `docker-compose.yaml` with `postgres`, `bugsink` and `bugsink-seed`**

  * `postgres`: `postgres:17-alpine`, a named volume, health check `pg_isready`.
  * `bugsink`: `bugsink/bugsink:${BUGSINK_VERSION:-2.6.3}`; environment `SECRET_KEY` (a fixed 50+ character dev string), `CREATE_SUPERUSER=admin@example.com:admin`, `PORT=8000`, `DATABASE_URL` pointing at `postgres`, `BASE_URL=http://localhost:${BUGSINK_PORT:-8800}`, `BEHIND_HTTPS_PROXY=false`; port `${BUGSINK_PORT:-8800}:8000`; `depends_on` postgres healthy; health check that requests `http://localhost:8000/health/ready` with the image's Python.
  * `bugsink-seed`: same image and database environment, `restart: "no"`, `depends_on` bugsink healthy, `docker/bugsink/seed.py` mounted read-only, command `bugsink-manage shell -c "exec(open('/seed.py').read())"`.

- [ ] **Step 2: Write `docker/bugsink/seed.py`**

```python
# SPDX-License-Identifier: GPL-3.0-or-later
# Development-only seed: every key and token below is public.
import uuid

from bsmain.models import AuthToken
from projects.models import Project
from teams.models import Team

PROJECTS = [("sample-python", "1" * 32), ("sample-js", "2" * 32)]
READ = ["projects_read", "issues_read", "events_read"]
TOKENS = [
    ("a" * 40, "GLOBAL_READ", None, READ),
    ("b" * 40, "GLOBAL_TRIAGE", None, READ + ["issues_triage", "issues_comment"]),
    ("c" * 40, "PROJECT_READ", "sample-python", READ),
    ("d" * 40, "NO_CAPS", None, []),
]

team, _ = Team.objects.get_or_create(name="Demo")
projects = {}
for expected_id, (slug, key) in enumerate(PROJECTS, start=1):
    project, _ = Project.objects.get_or_create(
        slug=slug, defaults={"name": slug, "team": team, "sentry_key": uuid.UUID(key)})
    assert project.id == expected_id, f"{slug} has id {project.id}, expected {expected_id}"
    projects[slug] = project

for value, description, project_slug, capabilities in TOKENS:
    token, _ = AuthToken.objects.update_or_create(token=value, defaults={
        "description": description,
        "is_project_bound": project_slug is not None,
        "project": projects.get(project_slug),
        **{name: True for name in capabilities},
    })
    token.full_clean()
print("seeded", sorted(projects), [t[1] for t in TOKENS])
```

- [ ] **Step 3: Write `.env.example`** with `BUGSINK_PORT=8800`, `GRAFANA_PORT=3300`, `SAMPLE_PYTHON_PORT=8810`, `BUGSINK_VERSION=2.6.3`.

- [ ] **Step 4: Start it and check the seed**

  Run: `docker compose up -d --wait bugsink && docker compose up bugsink-seed`
  Expected: the seed prints `seeded ['sample-js', 'sample-python'] [...]` and exits 0.

  Run: `curl -s -H "Authorization: Bearer $(printf 'a%.0s' {1..40})" http://localhost:8800/api/canonical/0/projects/`
  Expected: JSON listing both projects.

  Then sign in at `http://localhost:8800` as the superuser and confirm both projects are visible. If they are not, the web UI needs a membership row that the API path would have created: add it to the seed and record it in the findings.

- [ ] **Step 5: Run the seed a second time**

  Run: `docker compose up bugsink-seed`
  Expected: exit 0, same output, and the project list from Step 4 unchanged (still two projects, ids 1 and 2).

- [ ] **Step 6: Move the port**

  Run: `BUGSINK_PORT=8899 docker compose up -d --wait bugsink && curl -sf http://localhost:8899/health/ready`
  Expected: success. Then restore with `docker compose up -d --wait bugsink`.

- [ ] **Step 7: Commit** (`Add Bugsink and seeded tokens to the dev stack`) and push.

### Task 2: Tempo, Loki, the collector and Grafana

**Files:**
- Create: `docker/tempo/tempo.yaml`, `docker/loki/loki.yaml`, `docker/otel-collector/config.yaml`, `docker/grafana/provisioning/datasources/datasources.yaml`
- Modify: `docker-compose.yaml`

**Interfaces:**
- Produces: OTLP endpoint `http://otel-collector:4318` (HTTP) and `otel-collector:4317` (gRPC) in the Compose network; Grafana at `http://localhost:${GRAFANA_PORT:-3300}` with anonymous Admin access and data sources `tempo` and `loki`; proxy paths `/api/datasources/proxy/uid/tempo/…` and `/api/datasources/proxy/uid/loki/…`.

- [ ] **Step 1: Add the four services to `docker-compose.yaml`**

  * `tempo`: `grafana/tempo:3.1.1`, monolithic target, OTLP receiver, local storage in a named volume.
  * `loki`: `grafana/loki:3.7.10`, single binary, filesystem storage in a named volume, `allow_structured_metadata: true`.
  * `otel-collector`: `otel/opentelemetry-collector-contrib:0.165.0`.
  * `grafana`: `grafana/grafana:13.3.1`, port `${GRAFANA_PORT:-3300}:3000`, `GF_AUTH_ANONYMOUS_ENABLED=true`, `GF_AUTH_ANONYMOUS_ORG_ROLE=Admin`, `GF_AUTH_DISABLE_LOGIN_FORM=true`, the provisioning directory mounted read-only.
  * Each has a health check where the image allows one; `grafana` depends on `tempo` and `loki`.

  Run `docker compose pull` first: it confirms the four tags exist. If one does not, use the nearest released patch version and note it in the findings.

- [ ] **Step 2: Write the Tempo and Loki configs**

  Start from each project's documented local single-binary example for the pinned major version. Tempo 3.x removed the `ingester`, `compactor`, `ingester_client` and `metrics_generator_client` sections; a config copied from a 2.x example fails at start. Keep only what a local single node needs.

- [ ] **Step 3: Write `docker/otel-collector/config.yaml`**

  Receivers: `otlp` (gRPC 4317, HTTP 4318). Pipelines: `traces` → `otlp` exporter to `tempo:4317` (insecure); `logs` → `otlphttp` exporter to `http://loki:3100/otlp`. A `batch` processor on both.

- [ ] **Step 4: Write the Grafana data source provisioning**

  Two data sources: `Tempo` (type `tempo`, uid `tempo`, url `http://tempo:3200`) and `Loki` (type `loki`, uid `loki`, url `http://loki:3100`).

- [ ] **Step 5: Check the stack is healthy**

  Run: `docker compose up -d --wait`
  Expected: every service healthy or running; none restarting (`docker compose ps`).

  Run: `curl -s http://localhost:3300/api/datasources/uid/tempo/health` and the same for `loki`
  Expected: `"status":"OK"` for both.

- [ ] **Step 6: Commit** (`Add Tempo, Loki, collector and Grafana to the dev stack`) and push.

### Task 3: Python sample app

**Files:**
- Create: `sample-apps/python/pyproject.toml`, `sample-apps/python/uv.lock`, `sample-apps/python/app.py`, `sample-apps/python/Dockerfile`
- Modify: `docker-compose.yaml`

**Interfaces:**
- Consumes: the DSN and the OTLP endpoint from Tasks 1 and 2.
- Produces: service `sample-python` on `http://localhost:${SAMPLE_PYTHON_PORT:-8810}` with:
  * `GET /healthz` → `200 {"status": "ok"}`
  * `GET /handled` → `200 {"trace_id": "<32 hex>", "event_id": "<32 hex>"}`; logs one line, adds a breadcrumb, captures a handled `ValueError`.
  * `GET /error` → `500`; raises an unhandled `RuntimeError` from a function two calls deep, so the stack trace has several in-app frames.
  * `POST /no-trace` → `200 {"event_id": "<32 hex>"}`; captures an exception from a background thread that has no active span.

- [ ] **Step 1: Write `pyproject.toml`**

  Dependencies: `fastapi`, `uvicorn`, `sentry-sdk[opentelemetry-otlp]>=2.75`, `opentelemetry-sdk`, `opentelemetry-exporter-otlp-proto-http`, `opentelemetry-instrumentation-fastapi`, `opentelemetry-instrumentation-logging`. Run `uv lock`.

- [ ] **Step 2: Write `app.py`**

  * OpenTelemetry: a `TracerProvider` and a `LoggerProvider` with resource `service.name=sample-python`, `deployment.environment=development`, `service.version=0.1.0`, exporting over OTLP/HTTP to `OTEL_EXPORTER_OTLP_ENDPOINT`. The standard `logging` root logger gets the OpenTelemetry handler. FastAPI is instrumented.
  * Sentry, initialised after the tracer provider:

    ```python
    sentry_sdk.init(
        dsn=os.environ["SENTRY_DSN"],
        environment="development",
        release="sample-python@0.1.0",
        integrations=[OTLPIntegration(setup_otlp_traces_exporter=False)],
    )
    sentry_sdk.set_tag("service", "sample-python")
    ```

    `traces_sample_rate` stays unset: the OTLP integration requires that, and Bugsink takes no traces.
  * `/handled` returns the trace id of the current OpenTelemetry span, formatted as 32 hex characters, and the id returned by `sentry_sdk.capture_exception`.

- [ ] **Step 3: Write the `Dockerfile` and add the service to Compose**

  `python:3.13-slim` with `uv sync --frozen`; environment `SENTRY_DSN=http://11111111111111111111111111111111@bugsink:8000/1` and `OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318`; port `${SAMPLE_PYTHON_PORT:-8810}:8000`; `depends_on` the seed completed successfully and the collector started; health check on `/healthz`.

- [ ] **Step 4: Trigger each endpoint and look**

  Run: `docker compose up -d --wait --build && curl -s localhost:8810/handled; curl -s -o /dev/null -w '%{http_code}\n' localhost:8810/error; curl -s -X POST localhost:8810/no-trace`
  Expected: two ids, then `500`, then one id. In Bugsink's UI (`http://localhost:8800`, project `sample-python`) three issues appear within a few seconds; the first two events have a `trace` tag, the `/no-trace` event has none.

  If the first two events have no `trace` tag, stop: that is Q1 answered "no", and the finding goes to the user before any more work (it invalidates spec §5 as written).

- [ ] **Step 5: Commit** (`Add Python sample app with shared trace context`) and push.

### Task 4: The demo as a check

**Files:**
- Create: `scripts/verify_stack.py`

**Interfaces:**
- Consumes: `/handled` and `/no-trace` from Task 3; token `GLOBAL_READ`; Grafana's data source proxy.
- Produces: `uv run scripts/verify_stack.py` exits 0 when the stack correlates, non-zero with one line per failed check otherwise. Base URLs come from `BUGSINK_URL`, `GRAFANA_URL` and `SAMPLE_PYTHON_URL`, defaulting to the ports in `.env.example`.

- [ ] **Step 1: Write the script** (standard library only: `urllib`, `json`)

  A helper `poll(check, deadline_s=30)` retries a check every half second until it returns a value or the deadline passes; no check sleeps a fixed time. Checks, in order, each printing `ok` or `FAIL` with what was seen:

  1. `GET {sample}/handled` returns a `trace_id` and an `event_id`.
  2. Bugsink `GET /api/canonical/0/events/lookup/?project=1&event_id=<event_id>` returns an event whose `data.contexts.trace.trace_id` equals `trace_id`.
  3. Bugsink `GET /api/canonical/0/issues/?project=1&q=trace:<trace_id>` returns exactly one issue.
  4. Tempo, through Grafana: `GET {grafana}/api/datasources/proxy/uid/tempo/api/traces/<trace_id>` returns 200 with at least one span.
  5. Loki, through Grafana: `query_range` over the last 15 minutes with the spec's default template, `{service_name="sample-python"} | trace_id="<trace_id>"`, returns at least one line.
  6. `POST {sample}/no-trace`, then the lookup of its `event_id` returns an event with no `trace` tag.

- [ ] **Step 2: Run it against a stack that is up**

  Run: `uv run scripts/verify_stack.py`
  Expected: six `ok` lines, exit 0.

  A `FAIL` on check 2, 3 or 4 answers Q1; on check 5, Q2. Do not change a check to make it pass. Find out why (`docker compose logs otel-collector loki tempo`), and if the cause is the spec's assumption and not the stack's configuration, record what does work for the findings (for example the label Loki actually assigns) and adjust the check to that, with a comment in the findings.

- [ ] **Step 3: Run it against a fresh stack**

  Run: `docker compose down -v && docker compose up -d --wait --build && uv run scripts/verify_stack.py`
  Expected: exit 0 with no manual step in between.

- [ ] **Step 4: Commit** (`Add stack verification script`) and push.

### Task 5: Pin the schema and try client generation

**Files:**
- Create: `scripts/fetch_schema.sh`, `bugsink-api/openapi.yaml`, `bugsink-api/NOTICE.md`

**Interfaces:**
- Produces: `bugsink-api/openapi.yaml`, the input for M1's generated Go client.

- [ ] **Step 1: Write `scripts/fetch_schema.sh`**

  Downloads `${BUGSINK_URL:-http://localhost:8800}/api/canonical/0/schema/` to `bugsink-api/openapi.yaml`, unmodified. Whether the endpoint needs a token and whether it returns YAML or JSON by default are not yet known: try it without a token first, pass `GLOBAL_READ` if it answers 401 or 403, and note both facts in the findings.

- [ ] **Step 2: Fetch and write the notice**

  Run: `scripts/fetch_schema.sh && head -5 bugsink-api/openapi.yaml`
  Expected: an OpenAPI document whose `info.version` is `2.6.3`.

  `bugsink-api/NOTICE.md` states: the file is the OpenAPI schema served by Bugsink 2.6.3 at `/api/canonical/0/schema/`, retrieved on the date of the fetch, unmodified; it belongs to the Bugsink project and is licensed under the PolyForm Shield License 1.0.0 (link to Bugsink's `LICENSE`, and the copyright line copied from that file); it is not covered by this repository's GPL; refresh it with `scripts/fetch_schema.sh`.

- [ ] **Step 3: Check the schema against the spec (Q10)**

  For every endpoint, query parameter, request body field and response field that spec §4.B, §4.C, §4.E and §6 name, find it in the schema. Record a table in the findings: name in the spec, name in the schema, match or difference. Include the `x-bugsink-required-capability` value of each operation the plugin will call (it feeds Q4).

- [ ] **Step 4: Generate a Go client in a scratch directory (Q11)**

  In a temporary directory outside the repository, `go mod init scratch`, then generate a client with `oapi-codegen` v2.8.2 (`-generate types,client`) and run `go build ./...`. If it fails or the output is poor (untyped `interface{}` for the issue and event models), try `ogen` v1.25.0 the same way. Record for each: the command, whether it built, warnings, and how the issue list response and the `mute-for` request body are typed. Nothing generated is committed.

- [ ] **Step 5: Commit** (`Pin Bugsink 2.6.3 OpenAPI schema`) and push.

### Task 6: Probe the API

**Files:**
- Create: `scripts/probe_api.py`

**Interfaces:**
- Consumes: the seeded tokens; issues and events produced by Task 3.
- Produces: `uv run scripts/probe_api.py` prints a Markdown report to standard output; `--out <file>` writes it. Exit code 0 whenever the report was produced, whatever the statuses in it.

- [ ] **Step 1: Write the script** (standard library only)

  A helper `call(token, method, path, body=None) -> (status, parsed body or text)` returns the status for any HTTP response, including 4xx and 5xx, and never raises on them. The script first produces data (calls `/handled` and `/error` on the sample app 60 times each with distinct query strings, so the issue list exceeds one page if the page size allows, then waits by polling for the issue count to settle). Sections of the report:

  1. **Matrix (Q5):** for each of the four tokens and for no token, the status of: `GET projects/`, `GET projects/1/`, `GET projects/2/`, `GET issues/?project=1`, `GET issues/?project=2`, `GET issues/` (no project), `GET issues/<id of project 1>/`, `GET events/?issue=<id>`, `GET events/lookup/?project=1&event_id=<id>`, `GET issues/<id>/tags/`, `GET issue-history/?issue=<id>`, `GET issues/<id>/event-counts/`, `GET projects/1/event-counts/`, `POST issues/<id>/mute/` followed by `POST issues/<id>/unmute/`. One table row per call, one column per token.
  2. **Refusals (Q4):** the full body of one 401 and one 403, and whether a 403 names the missing capability.
  3. **Issues without a project (Q6):** status and the number of projects represented in the result, for `GLOBAL_READ`.
  4. **Events by trace (Q7):** `GET events/?q=trace:<id>` without `issue`; `GET issues/?project=1&q=trace:<id>`; `GET issues/?q=trace:<id>`.
  5. **Paging (Q8):** the top-level keys of an issue list response, the page length, the form of `next`, and whether following `next` to the end with `sort=last_seen&order=desc` yields `last_seen` values that never increase.
  6. **Event data (Q3):** for the latest `/handled` event, the JSON paths and values of environment, release, `contexts.trace`, the `service` tag, and the list of tag keys from `issues/<id>/tags/`.
  7. **Event counts (Q9):** the response for the last 24 hours on hour boundaries; the status and body for `start` off the hour boundary, for `start` 91 days back, and with no parameters.
  8. **Triage response:** the keys of the body returned by `mute/`, to confirm it is the issue object (spec §4.E).

- [ ] **Step 2: Run it**

  Run: `uv run scripts/probe_api.py --out /tmp/probe.md && sed -n '1,40p' /tmp/probe.md`
  Expected: a report with all eight sections and the matrix filled for every cell, exit 0. Cells showing 403 or 404 are results, not errors.

- [ ] **Step 3: Commit** (`Add API probe script`) and push.

### Task 7: Findings, spec corrections and the stack README

**Files:**
- Create: `design/phase0_findings.md`, `docker/README.md`
- Modify: `design/bugsink_grafana_plugin_spec.md`, `design/implementation_roadmap.md`, `README.md`

- [ ] **Step 1: Write `design/phase0_findings.md`**

  One section per question Q1 to Q11: the answer in one or two sentences, then the evidence (the command, and the relevant lines of its output or of the probe report). After the questions, a section "Consequences" listing each place the spec or a later milestone must change, and a section "Versions" with the image tags the findings were made on. Unanswered questions stay in the document marked as open, with what was tried.

- [ ] **Step 2: Correct the spec**

  Edit `design/bugsink_grafana_plugin_spec.md` wherever a finding contradicts it, so the spec describes Bugsink as it behaves. If a finding removes or reshapes a milestone (for example Q7 answered "no" twice removes the reverse link from M4), update `design/implementation_roadmap.md` too. Each edit is listed under "Consequences" in the findings.

- [ ] **Step 3: Write `docker/README.md`**

  How to start and stop the stack; the URLs and ports and how to move them with `.env`; the seeded values table with the warning that they are public; the sample app's endpoints; how to run the two scripts and refresh the schema. Add one line to the root `README.md` pointing to it.

- [ ] **Step 4: Final check from a clean state**

  Run: `docker compose down -v && docker compose up -d --wait --build && uv run scripts/verify_stack.py && uv run scripts/probe_api.py --out /tmp/probe.md && git status --short`
  Expected: verification exits 0, the probe writes its report, and `git status` shows no untracked or modified files (nothing the stack writes lands in the repository).

- [ ] **Step 5: Commit** (`Record Phase 0 findings and correct the spec`) and push.

## Exit

M0 is done when `verify_stack.py` passes on a fresh stack, every question Q1 to Q11 has an answer or is recorded as open with a reason, and the spec agrees with the findings. A "no" on Q1 is a valid outcome of the spike, but it stops the roadmap for a decision before M1.
