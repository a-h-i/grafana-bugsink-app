# Development stack

A local Docker Compose stack for developing the plugin: Bugsink 2.6.3 (with PostgreSQL), Grafana, Tempo,
Loki, an OpenTelemetry Collector and a Python sample app that reports errors to Bugsink and spans and logs to
the collector under one trace id. Findings from building it are in
[`design/phase0_findings.md`](../design/phase0_findings.md).

**Everything below, every password, token and key, is a public development-only value. The stack has no
real authentication (Grafana lets anyone in as Admin). Do not expose it beyond localhost and do not reuse any
of these values elsewhere.**

## Start and stop

Run from the repository root.

```sh
docker compose up -d --wait --build   # start (builds the sample app), returns when everything is healthy
docker compose down                   # stop, keep the data
docker compose down -v                # stop and wipe the data; the next `up` seeds it again
```

`bugsink-seed` runs once per `up` and exits; it creates the projects and tokens below and is safe to repeat.

## URLs and ports

| Service | URL | Notes |
| :--- | :--- | :--- |
| Grafana | <http://localhost:3300> | anonymous, role Admin, no login form |
| Bugsink | <http://localhost:8800> | API at `/api/canonical/0/`, schema at `/api/canonical/0/schema/` |
| Sample app | <http://localhost:8810> | see below |

Ports are bound to `127.0.0.1` only. PostgreSQL, Tempo, Loki and the collector publish nothing; reach Tempo and
Loki through Grafana (`/api/datasources/proxy/uid/tempo/...`, `/api/datasources/proxy/uid/loki/...`).
`3000` and `8000` are deliberately not used, since other local stacks use them.

To move a port, copy `.env.example` to `.env` (git-ignored) and edit it, then `docker compose up -d --wait`:

```text
BUGSINK_PORT=8800
GRAFANA_PORT=3300
SAMPLE_PYTHON_PORT=8810
BUGSINK_VERSION=2.6.3
```

The host scripts do not read `.env`; when a port is moved, export `BUGSINK_URL`, `GRAFANA_URL` and `SAMPLE_PYTHON_URL` so they follow it.

## Seeded values (public, development only)

| Thing | Value |
| :--- | :--- |
| Bugsink superuser | `admin@example.com` / `admin` |
| Team | `Demo` |
| Project 1 | `sample-python`, id `1`, DSN key `11111111-1111-1111-1111-111111111111` |
| Project 2 | `sample-js`, id `2`, DSN key `22222222-2222-2222-2222-222222222222` |
| Token `GLOBAL_READ` | `a` repeated 40 times; `projects:read`, `issues:read`, `events:read` |
| Token `GLOBAL_TRIAGE` | `b` repeated 40 times; the three above plus `issues:triage`, `issues:comment` |
| Token `PROJECT_READ` | `c` repeated 40 times; bound to project 1; `projects:read`, `issues:read`, `events:read` |
| Token `NO_CAPS` | `d` repeated 40 times; global, no capabilities |
| DSN inside Compose | `http://11111111111111111111111111111111@bugsink:8000/1` |
| Grafana data source UIDs | `tempo`, `loki` |

Use a token as `Authorization: Bearer <token>`, for example
`curl -H "Authorization: Bearer $(printf 'a%.0s' {1..40})" http://localhost:8800/api/canonical/0/projects/`.

## Grafana

Anonymous access with role Admin; no login. The data sources `Tempo` (uid `tempo`) and `Loki` (uid `loki`) are
provisioned from `docker/grafana/provisioning/datasources/datasources.yaml`.

## Sample app

The Python app (`sample-apps/python`, FastAPI) sends errors to Bugsink with the Sentry SDK and spans and logs to
the collector with OpenTelemetry, sharing the trace context.

| Endpoint | Effect |
| :--- | :--- |
| `GET /healthz` | `{"status": "ok"}` |
| `GET /handled[?variant=<v>]` | captures a handled error; returns `{trace_id, event_id}` |
| `GET /error[?variant=<v>]` | unhandled error, HTTP 500; captured by Sentry under the request's trace id |
| `POST /no-trace` | captures an error outside any span; returns `{event_id}`; its trace id is not in Tempo |

`variant` goes into the message as `(variant v<v>)`, so each value is a separate Bugsink issue; without it the
message is constant and an endpoint is one issue. Bugsink replaces integers in messages when it groups, which
is why the text is `v7`, not `7`.

```sh
curl -s http://localhost:8810/handled
curl -s 'http://localhost:8810/error?variant=3'
curl -s -X POST http://localhost:8810/no-trace
```

## Scripts

Both need only `uv` and Python 3 on the host (standard library otherwise), and the stack running.

* `uv run scripts/verify_stack.py` calls `/handled` and `/no-trace` and prints six lines, `ok` or `FAIL`: the sample
  app's trace id, the Bugsink event with that id, the Bugsink search `trace:<id>`, the trace in Tempo, the log lines in
  Loki, and the no-trace event (in Bugsink, absent from Tempo). Exit 0 only if all six pass.
* `uv run scripts/probe_api.py [--out report.md]` sends events (264 on an empty stack, 10 afterwards), calls
  the Bugsink API with all four tokens and prints a Markdown report in eight sections: token matrix, refusals,
  issues without a project, events by trace, paging and order, event data, event counts, triage response. It mutes and
  unmutes one issue. Without `--out` it prints to the terminal. A section that fails shows `SECTION FAILED` in its place; exit 1 if Bugsink is unreachable or the probe cannot produce its data.
* `scripts/fetch_schema.sh` refreshes `bugsink-api/openapi.yaml` from the running Bugsink (no token needed). Run it
  after changing `BUGSINK_VERSION`, and review the diff; the file is committed unmodified.

## Known limits

* Bugsink throttles ingestion at about 1000 events per 5 minutes and answers HTTP 429 (observed by hand). Repeated event-heavy runs can reach it (it was hit during development); `verify_stack.py` then fails until the window passes.
* Bugsink digests events asynchronously: an event is not found (404) for a few seconds after it is sent.
* Loki and the collector images have no health check; Grafana's health check waits for Loki, so `up --wait` implies it is ready.
