# Development stack

A local Docker Compose stack for developing the plugin: Bugsink 2.6.3 (with PostgreSQL), Grafana, Tempo,
Loki, an OpenTelemetry Collector and two sample apps, Python and JavaScript, that report errors to Bugsink and spans and logs to
the collector under one trace id. Findings from building it are in
[`design/phase0_findings.md`](../design/phase0_findings.md).

**Everything below, every password, token and key, is a public development-only value. The stack has no
real authentication (Grafana lets anyone in as Admin). Do not expose it beyond localhost and do not reuse any
of these values elsewhere.**

## Start and stop

Run from the repository root. Build the plugin first, since Grafana mounts `dist/`:

```sh
npm ci
npm run build
npm run build:backend
```

Running `docker compose up` before the first build makes Docker create `./dist` owned by root, and the build then fails to write into it, so build first.

```sh
docker compose up -d --wait --build   # start (builds the sample app), returns when everything is healthy
docker compose down                   # stop, keep the data
docker compose down -v                # stop and wipe the data; the next `up` seeds it again
```

After a rebuild of the backend, run `docker compose restart grafana`.

`bugsink-seed` runs once per `up` and exits; it creates the projects and tokens below and is safe to repeat.

## URLs and ports

| Service | URL | Notes |
| :--- | :--- | :--- |
| Grafana | <http://localhost:3300> | anonymous, role Admin, no login form |
| Bugsink | <http://localhost:8800> | API at `/api/canonical/0/`, schema at `/api/canonical/0/schema/` |
| Python sample app | <http://localhost:8810> | see below |
| JavaScript sample app | <http://localhost:8820> | see below |

Ports are bound to `127.0.0.1` only. PostgreSQL, Tempo, Loki and the collector publish nothing; reach Tempo and
Loki through Grafana (`/api/datasources/proxy/uid/tempo/...`, `/api/datasources/proxy/uid/loki/...`).
`3000` and `8000` are deliberately not used, since other local stacks use them.

To move a port, copy `.env.example` to `.env` (git-ignored) and edit it, then `docker compose up -d --wait`:

```text
BUGSINK_PORT=8800
GRAFANA_PORT=3300
SAMPLE_PYTHON_PORT=8810
SAMPLE_JS_PORT=8820
BUGSINK_VERSION=2.6.3
```

The host scripts do not read `.env`; when a port is moved, export `BUGSINK_URL`, `GRAFANA_URL` and `SAMPLE_PYTHON_URL`, `SAMPLE_JS_URL` so they follow it.

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
| Grafana data source UIDs | `tempo`, `loki`, `bugsink`, `bugsink-project`, `bugsink-triage` |

Use a token as `Authorization: Bearer <token>`, for example
`curl -H "Authorization: Bearer $(printf 'a%.0s' {1..40})" http://localhost:8800/api/canonical/0/projects/`.

## Grafana

Anonymous access with role Admin; the login form is disabled. The built-in Grafana user `admin` keeps the password `admin`, a
public development-only value; the end-to-end setup uses it with basic authentication to create users. The data sources `Tempo` (uid `tempo`), `Loki` (uid `loki`) and
`Bugsink` (uid `bugsink`, the default data source, type `ahmedismail-bugsink-datasource`) are provisioned from
`docker/grafana/provisioning/datasources/datasources.yaml`. `Bugsink` points at the Compose Bugsink and uses the
seeded `GLOBAL_READ` token, a public development-only value, and `Browser URL` `http://localhost:8800`, so
issue links open the Bugsink on the host. `Bugsink project-bound` (uid `bugsink-project`) is the same instance with the
`PROJECT_READ` token, which sees only `sample-python`.

`Bugsink triage` (uid `bugsink-triage`) is the same instance with the `GLOBAL_TRIAGE` token and `Triage` switched on, so editors
and admins get `Resolve`, `Resolve in latest release`, `Resolve in next release`, `Reopen`, `Mute`, `Mute for a period`,
`Mute until a threshold` and `Unmute` on it. Every action runs with the token, and Bugsink's history records it without a user.
Attribution comments are on, so each action also writes a comment in Bugsink naming the Grafana login, or `an anonymous user`
for the anonymous Admin. The other data sources have triage off. Grafana's anonymous user is an Admin, so anyone who can open
this Grafana can triage. A triage call needs the header `X-Bugsink-Triage: 1`, which the app sends, so a page on another web
site cannot triage through your browser. A page that uses DNS rebinding to reach Grafana under its own name can add the header;
it carries no Grafana session, but on this stack anonymous access is Admin, so such a page can triage.

The end-to-end tests make two local Grafana users through the admin API (`tests/users.setup.ts`): `m7-editor` (role Editor,
password `m7-editor-e2e`) and `m7-viewer` (role Viewer, password `m7-viewer-e2e`). Both passwords are public development-only
values; the login form is disabled, so use them with basic authentication (`curl -u m7-editor:m7-editor-e2e`).

`Bugsink` has the link settings `tempoDatasourceUid` `tempo` and `lokiDatasourceUid` `loki`, so its events offer `Open trace` and `Open logs`. `Bugsink project-bound` has no targets, so it shows no buttons and no links. The plugin app is enabled by
`docker/grafana/provisioning/plugins/apps.yaml`; its issue list page opens at
<http://localhost:3300/a/ahmedismail-bugsink-app>.

### Dashboards

`docker/grafana/provisioning/dashboards/` is provisioned (read-only in the UI):

* `Bugsink issues` (uid `bugsink-issues`, last 24 hours): a table `Unresolved issues in sample-python` with the
  unresolved issues of `sample-python` seen in `development`, newest first. Each issue is a link that opens it in
  Bugsink. The table needs a recent event; create one with `curl http://localhost:8810/handled`.
* `Bugsink trace` (uid `bugsink-trace`, last 24 hours): the issues and the events of one trace. Paste a trace id from
  `curl http://localhost:8810/handled` into `Trace ID`.
* `Bugsink event volume` (uid `bugsink-volume`, last 7 days): a time-series panel `Events per hour in sample-python`, drawn as
  bars, from the `Event volume` query of project 1 with a minimum interval of `1h`, so it opens without notices. The last bar
  is the hour in progress.
* `Bugsink e2e: refused query` (uid `bugsink-e2e-refused`): a panel for a project that does not exist, used by the
  end-to-end tests to check that a refused query shows Bugsink's message.

After changing a provisioning file, run `docker compose restart grafana`.

## Sample apps

### Python

The Python app (`sample-apps/python`, FastAPI) sends errors to Bugsink with the Sentry SDK and spans and logs to
the collector with OpenTelemetry, sharing the trace context.

| Endpoint | Effect |
| :--- | :--- |
| `GET /healthz` | `{"status": "ok"}` |
| `GET /handled[?variant=<v>]` | captures a handled error; returns `{trace_id, event_id}` |
| `GET /chained[?variant=<v>]` | adds the breadcrumb `parsing the order` and captures a `RuntimeError` raised from a `JSONDecodeError` (two chained exceptions with frames); returns `{trace_id, event_id}` |
| `GET /message[?variant=<v>]` | captures a warning message with the stack trace of the call (`attach_stacktrace`) and no exception, outside any span, so its trace id is not in Tempo; the plugin shows Bugsink's stack trace text for it; returns `{event_id}` |
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

### JavaScript

The JavaScript app (`sample-apps/javascript`) is a Node server on port 8820. It sends errors to project `sample-js` with `@sentry/node` and `Sentry.openTelemetryIntegration()`, and spans and logs to the collector with OpenTelemetry. Its Sentry setup drops the default Dedupe integration, so repeated demo errors are all reported. Its dependency tree includes `sentry@0.49.0` (FSL-1.1-Apache-2.0) through `@sentry/node`; the image is for local development and is not published.

| Endpoint | Effect |
| :--- | :--- |
| `GET /healthz` | `{"status": "ok"}` |
| `GET /handled[?variant=<v>]` | captures an `Error` with a `cause` inside a span, honouring an incoming `traceparent`; returns `{trace_id, event_id}` |

```sh
curl -s 'http://localhost:8820/handled?variant=1'
```

## Scripts

Both need only `uv` and Python 3 on the host (standard library otherwise), and the stack running.

* `uv run scripts/verify_stack.py` calls the sample apps and prints eleven lines, `ok` or `FAIL`. For the Python app: its trace id, the Bugsink event with that id, the Bugsink search `trace:<id>`, the trace in Tempo, the log lines in
  Loki, the no-trace event (in Bugsink, absent from Tempo) and an incoming `traceparent` kept as the trace id. For the JavaScript app: an incoming `traceparent` kept as its trace id, the Bugsink event in project `sample-js`, the trace in Tempo and the log lines in Loki. Exit 0 only if all eleven pass.
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
