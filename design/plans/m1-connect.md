# M1: Connect Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A user adds a Bugsink data source in Grafana and "Save & test" either passes, naming the projects the token can see, or says exactly what is wrong.

**Architecture:** One app plugin (`ahmedismail-bugsink-app`) scaffolded with `create-plugin` at the repository root, with a nested data source (`ahmedismail-bugsink-datasource`) under `src/datasource/` that owns the only Go backend. The backend talks to Bugsink through one small package, `pkg/bugsink`, which is the only importer of the generated client; the health check is a pure function over that package's interface. The M0 Compose stack mounts `dist/` into Grafana and provisions the app and one data source.

**Tech Stack:** `@grafana/create-plugin` 9.0.3, Node 24, npm 11, Grafana 13.3.1, React with `@grafana/ui`, Go 1.26 with `grafana-plugin-sdk-go` v0.299.0, `oapi-codegen` v2.8.2, Jest, `@grafana/plugin-e2e` (Playwright), GitHub Actions.

**Spec:** [`../bugsink_grafana_plugin_spec.md`](../bugsink_grafana_plugin_spec.md) (§3, §4.A, §9, §10). **Roadmap:** [`../implementation_roadmap.md`](../implementation_roadmap.md) (M1, Definition of done, Licensing). **Findings:** [`../phase0_findings.md`](../phase0_findings.md) (Q4, Q5, Q11).

## Global Constraints

* Plugin ids are `ahmedismail-bugsink-app` and `ahmedismail-bugsink-datasource`; the grafana.com organization is `ahmedismail`.
* The Go module path is `github.com/a-h-i/grafana-bugsink-app` (the scaffold writes `github.com/ahmedismail/bugsink`; Task 1 renames it).
* The app has no backend of its own; every Bugsink call goes through a data source instance (spec §3).
* The API token is kept in `secureJsonData` under the key `apiToken` and never reaches the browser, a log line or an error message.
* The Bugsink base URL is the data source's standard `url` field. The backend appends `/api/canonical/0/`.
* The health check makes one read per capability the configured features need, and no others (spec §4.A). In M1 those are `projects:read`, `issues:read` and `events:read`; nothing is checked or said about `issues:triage` and `issues:comment` until M7 adds a triage setting.
* The config editor has no project id field (spec §4.A).
* `bugsink-api/openapi.yaml` is the generator's input, unmodified. Generated code lives only in `pkg/bugsink/internal/gen/` and is imported only by `pkg/bugsink`.
* **Merge gate:** this branch does not merge while it contains generated code unless `bugsink-api/NOTICE.md` records the Bugsink maintainer's written permission. Without permission, Task 2B replaces Task 2's generated code before the merge (spec §10).
* Licence `GPL-3.0-or-later`: `package.json` carries the SPDX identifier, and every source file written by hand starts with `SPDX-License-Identifier: GPL-3.0-or-later` in a comment. Exempt: `.config/` (owned by `create-plugin`), `pkg/bugsink/internal/gen/`, JSON files, lock files.
* Host ports stay `3300` (Grafana), `8800` (Bugsink), `8810` (sample app). Never publish `3000` or `8000`.
* `python3 scripts/verify_stack.py` still prints six `ok` lines after every task that touches `docker-compose.yaml`.
* End-to-end tests in this milestone produce no Bugsink events.
* Work on branch `m1/connect` in a worktree made with `git worktree add .claude/worktrees/m1-connect -b m1/connect main`; push after every commit; never amend.
* Run every test command in the foreground.

## Facts already verified (2026-11-15)

Against the running M0 stack (Bugsink 2.6.3) unless a source is named.

| Request | Token has the capability | Token lacks it | Capability |
| :--- | :--- | :--- | :--- |
| `GET projects/` | 200, `{next, previous, results:[{id, name, …}]}` | 403 | `projects:read` |
| `GET issues/00000000-0000-0000-0000-000000000000/` | 404 `{"detail":"No Issue matches the given query."}` | 403 | `issues:read` |
| `GET events/` (no `issue`) | 400 `{"issue":["This field is required."]}` | 403 | `events:read` |

* A capability 403 body is `{"detail":"This token does not have the required capability: <name>."}`.
* A bad token is 401 `{"detail":"Invalid Bearer token."}`; no token is 401.
* A project-bound token gets 200 from `GET projects/` with only its own project, and 404 and 400 from the other two probes, like a global token.
* A server that is not Bugsink can answer JSON too: Grafana returns `404 application/json` for `/api/canonical/0/projects/`.
* Seeded tokens (public, development only, `docker/bugsink/seed.py`): global read `a`×40, global triage `b`×40, project-bound read `c`×40 (project `sample-python`), no capabilities `d`×40. Seeded projects: `sample-python` (1), `sample-js` (2).
* `create-plugin` runs without prompts as `npx @grafana/create-plugin@9.0.3 --plugin-type=app --plugin-name=bugsink --org-name=ahmedismail --backend` (Grafana docs, CLI reference).
* Grafana docs: nested plugins live inside the app's `src/` with their own `plugin.json` and no `package.json`; "each nested data source can have its own backend, independent of the app plugin's backend"; Grafana runs the file `<executable>_<GOOS>_<GOARCH>`.
* `@grafana/plugin-e2e`: `createDataSourceConfigPage({ type })`, `gotoDataSourceConfigPage(uid)`, `await expect(configPage.saveAndTest()).toBeOK()`, `await expect(configPage).toHaveAlert('error', { hasText })`.
* The development machine has Node 26, npm 11 and Go 1.26, and no `mage` binary: use `go run github.com/magefile/mage`.
* `create-plugin` 9.0.3 was run in a scratch directory with the command above. It generates the directory `ahmedismail-bugsink-app/` with:
  * `package.json` (scripts `build`, `dev`, `test`, `test:ci`, `typecheck`, `lint`, `lint:fix`, `e2e`, `server`, `sign`; `license: Apache-2.0`; `author: Ahmedismail`; `packageManager: npm@11.20.0`; `engines.node >=24.5`), `.nvmrc` (`24`), `.npmrc`, `.cprc.json`, `tsconfig.json`, `eslint.config.mjs`, `.prettierrc.js`, `jest.config.js`, `jest-setup.js`, `playwright.config.ts`, and `.config/` (webpack, jest, eslint, Playwright base config, a Dockerfile and `docker-compose-base.yaml`);
  * `go.mod` (`module github.com/ahmedismail/bugsink`, `go 1.26.5`, SDK v0.299.0), `Magefile.go` (`Default = build.BuildAll`), `.golangci.yml`, `pkg/main.go` (`app.Manage`), `pkg/plugin/{app.go,resources.go,resources_test.go}`;
  * `src/plugin.json` (`backend: true`, `executable: gpx_bugsink`, `grafanaDependency >=13.2.0`, four sample pages and a configuration page), `src/module.tsx`, `src/constants.ts`, `src/components/{App,AppConfig,testIds.ts}`, `src/pages/Page{One,Two,Three,Four}.tsx`, `src/utils/utils.routing.ts`, `src/img/logo.svg`, `src/README.md`;
  * `tests/{appConfig.spec.ts,appNavigation.spec.ts,fixtures.ts}`, `provisioning/plugins/apps.yaml`, `docker-compose.yaml`, `CHANGELOG.md`, `LICENSE`, `README.md`, `.gitignore`;
  * `.github/dependabot.yml` and the workflows `ci.yml`, `release.yml`, `is-compatible.yml`, `bundle-stats.yml`, `cp-update.yml`;
  * agent guidance: `AGENTS.md`, `CLAUDE.md`, `GEMINI.md` (three lines each, pointing at `.config/AGENTS/instructions.md`), `.claude/skills/`, `.codex/skills/`.
* The scaffold's webpack config copies every `**/*.json` under `src/` to `dist/`, so `src/datasource/plugin.json` lands at `dist/datasource/plugin.json`.
* The SDK's build (`build/common.go`, v0.299.0) handles a nested data source itself: when `src/plugin.json` has no `executable`, it reads `src/datasource/plugin.json` and writes the binary to `dist/datasource/<executable>_<os>_<arch>`. No `Magefile.go` change is needed.
* `.config/playwright.config.ts` sets `baseURL: process.env.GRAFANA_URL || 'http://localhost:3000'` and has an `auth` project that logs in as `admin` before the `chromium` project.

**Not verified:** that Grafana starts the nested data source's backend from `dist/datasource/` (Task 1, Step 6 proves it), and whether `@grafana/plugin-e2e` logs in against a Grafana that has `GF_AUTH_DISABLE_LOGIN_FORM` (Task 6, Step 1).

## Deviation from the roadmap, for review

The roadmap's M1 says the health check reports the "token kind". No endpoint reports it, and a global token on a one-project installation looks the same as a project-bound token. This plan reports the projects the token can see instead, which is what the user needs to know and is always true. Task 6 updates the roadmap line.

## Health check contract

Task 3 implements this table; Task 6 asserts its messages in the browser; Task 7 documents them. `<url>` is the configured URL as typed.

| Condition | Status | Message |
| :--- | :--- | :--- |
| URL empty | Error | `Enter the Bugsink URL.` |
| URL without `http://` or `https://`, or unparsable | Error | `The Bugsink URL must start with http:// or https://.` |
| Token empty | Error | `Enter an API token.` |
| Connection refused, DNS failure, TLS failure | Error | `Cannot reach Bugsink at <url>: <cause>.` |
| No answer within 10 seconds | Error | `Bugsink at <url> did not answer within 10 seconds.` |
| 401 from the first probe | Error | `Bugsink rejected the API token. Check that it was copied in full and has not been deleted.` |
| First probe answers anything but 200, 401 or a capability 403, or a 200 whose body has no `results` array | Error | `<url> did not answer like a Bugsink API (HTTP <status>). Check the URL: it is the address of Bugsink itself, without /api.` |
| One or more capability 403s | Error | `The API token lacks: <names, comma-separated, in the order projects:read, issues:read, events:read>.` |
| A later probe answers something unexpected (not the "has" or "lacks" answer in the table above) | Error | `Bugsink answered HTTP <status> to a check for <capability>.` |
| All three capabilities present | OK | `Connected to Bugsink. Projects visible to this token: <names, comma-separated>.` With no projects: `Connected to Bugsink. This token sees no projects.` With more than ten: the first ten names, then `and <n> more`. |

All three probes run even when the first reports a missing capability, so one "Save & test" names every missing capability. They stop at the first connection error or 401.

## Review Focus

* **A URL pasted with a trailing slash or with `/api/canonical/0/` on the end** works the same as the bare address. Pinned in Task 2, Step 1.
* **A token pasted with a trailing newline or spaces** is trimmed before use. Pinned in Task 2, Step 1.
* **A URL that points at some other web server** (Grafana itself, a proxy login page returning HTML with 200) gets the "did not answer like a Bugsink API" message, not a JSON parse error. Pinned in Task 3, Step 1.
* **A Bugsink that hangs** ends in the 10-second message; "Save & test" never spins forever. Pinned in Task 3, Step 1.
* **The token in an error path:** no message, log line or `CheckHealthResult` field contains the token, including when the URL is wrong and the transport error echoes the request. Pinned in Task 3, Step 1.
* **Editing the URL of a saved data source** keeps the stored token: the editor shows it as configured and does not send an empty one. Pinned in Task 4, Step 1.

---

## File structure

| Path | Responsibility |
| :--- | :--- |
| `package.json`, `.config/`, `tsconfig.json`, `eslint.config.mjs`, `jest.config.js`, `playwright.config.ts`, `Magefile.go`, `go.mod`, `.golangci.yml`, `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `.claude/skills/`, `.codex/skills/` | Scaffold; edited only where a task says so |
| `src/plugin.json`, `src/module.tsx`, `src/pages/Home.tsx` | The app: one page that lists Bugsink data sources |
| `src/datasource/plugin.json`, `module.ts`, `datasource.ts`, `types.ts` | The nested data source, frontend side |
| `src/datasource/components/ConfigEditor.tsx` | URL and token fields |
| `pkg/main.go` | Starts the data source backend |
| `pkg/plugin/datasource.go` | Instance factory, `CheckHealth` wiring, settings |
| `pkg/plugin/health.go` | The health check contract as a pure function |
| `pkg/bugsink/client.go`, `errors.go` | The plugin's own view of the Bugsink API |
| `pkg/bugsink/internal/gen/` | `oapi-codegen` output and its config |
| `docker-compose.yaml`, `docker/grafana/provisioning/` | Mount and provision the plugin |
| `tests/` | End-to-end tests |
| `.github/workflows/ci.yml`, `scripts/check_licenses.sh` | CI |

---

### Task 1: Scaffold, nested data source and a backend that Grafana loads

The risky part of the skeleton. It ends with Grafana in the Compose stack running the nested data source's Go backend, proven by a health call. No Bugsink logic yet.

**Files:**
- Create: the scaffold at the repository root; `src/datasource/{plugin.json,module.ts,datasource.ts,types.ts}`; `docker/grafana/provisioning/plugins/apps.yaml`; `design/plans/m1-scaffold-notes.md`
- Modify: `src/plugin.json`, `package.json`, `go.mod`, `pkg/main.go`, `docker-compose.yaml`, `docker/grafana/provisioning/datasources/datasources.yaml`, `.gitignore`, `README.md`, `docker/README.md`
- Delete: the scaffold's own `docker-compose.yaml`, `provisioning/` and `LICENSE`, its app backend and its sample pages

**Interfaces:**
- Produces: `npm run build` (frontend into `dist/`); `npm run build:backend` (Go binary for Linux into `dist/datasource/`); `npm run typecheck`, `npm run lint`, `npm run test:ci`, `npm run e2e`; data source type `ahmedismail-bugsink-datasource`; provisioned data source uid `bugsink`; `pkg/plugin.NewDatasource(ctx context.Context, settings backend.DataSourceInstanceSettings) (instancemgmt.Instance, error)`; TypeScript types `BugsinkOptions` (empty `jsonData` for now) and `BugsinkSecureJsonData { apiToken?: string }` in `src/datasource/types.ts`.

- [ ] **Step 1: Scaffold into a scratch directory outside the repository and move it in**

Run `npx @grafana/create-plugin@9.0.3 --plugin-type=app --plugin-name=bugsink --org-name=ahmedismail --backend` in an empty scratch directory. Move the generated plugin's contents to the repository root. Keep the repository's `LICENSE`, `README.md`, `docker-compose.yaml`, `docker/`; merge the two `.gitignore` files; drop the scaffold's `docker-compose.yaml`, `provisioning/`, `LICENSE` and `README.md`, and every workflow but `ci.yml`. Set `"license": "GPL-3.0-or-later"` and `"author": "Ahmed Ismail"` in `package.json`, the author name `Ahmed Ismail` in `src/plugin.json`, and the module path `github.com/a-h-i/grafana-bugsink-app` in `go.mod` and the Go imports.

- [ ] **Step 2: Record what was generated**

Compare the generated files with the list under "Facts already verified". Write `design/plans/m1-scaffold-notes.md` with every difference (a file this plan names that is missing, or named differently) and the real path that stands in; later tasks use the notes where they differ from this plan. With no difference, the file says so in one line.

- [ ] **Step 3: Make the app frontend-only and add the nested data source**

  * `src/plugin.json`: id `ahmedismail-bugsink-app`, name `Bugsink`, no `backend` and no `executable`; `includes` has one page (`Bugsink`, path `/a/%PLUGIN_ID%`, `addToNav: true`, `defaultNav: true`) and `{ "type": "datasource", "name": "Bugsink" }`; `dependencies.grafanaDependency` stays `>=13.2.0`.
  * `src/datasource/plugin.json`: `type: datasource`, id `ahmedismail-bugsink-datasource`, name `Bugsink`, `backend: true`, `executable: gpx_bugsink`, `metrics: true`, the same `info` block as the app.
  * `src/datasource/module.ts` exports `plugin = new DataSourcePlugin(BugsinkDataSource).setConfigEditor(ConfigEditor)`; `datasource.ts` is `class BugsinkDataSource extends DataSourceWithBackend<BugsinkQuery, BugsinkOptions>` with nothing overridden; `ConfigEditor` is a placeholder that renders nothing (Task 4 replaces it); `BugsinkQuery extends DataQuery` with no fields.
  * Remove `src/pages/Page{One,Two,Three,Four}.tsx`, `src/components/`, `src/utils/`, `src/constants.ts`, `tests/appConfig.spec.ts`, `tests/appNavigation.spec.ts`, and the app backend (`pkg/plugin/app.go`, `resources.go`, `resources_test.go`). In `tests/fixtures.ts` keep only `gotoPage`. `src/module.tsx` exports an `AppPlugin` whose root page is `src/pages/Home.tsx`, for now the text `Bugsink` (Task 5 fills it).
  * `pkg/main.go` calls `datasource.Manage("ahmedismail-bugsink-datasource", plugin.NewDatasource, datasource.ManageOpts{})`. `NewDatasource` returns a struct whose `CheckHealth` answers `HealthStatusOk` with message `Bugsink backend is running.`

- [ ] **Step 4: Build the backend into the nested plugin's directory**

Add the script `"build:backend": "go run github.com/magefile/mage -v build:linux"`, with the `mage` version pinned in `go.mod` through a `tools.go` file. `Magefile.go` stays as generated.

Run: `npm run build && npm run build:backend && ls dist/datasource/`
Expected: `gpx_bugsink_linux_amd64` and `plugin.json` are listed, and `dist/` itself holds no `gpx_*` file.

If Step 6 then shows Grafana not starting the backend (the plugin is registered but the health call answers 500 or "plugin unavailable"), read Grafana's log for the path it tried, change `executable` to match (the SDK also accepts `../gpx_bugsink`, which puts the binary in `dist/`), and record what worked in the scaffold notes.

- [ ] **Step 5: Mount and provision the plugin in the Compose stack**

  * `docker-compose.yaml`, service `grafana`: volume `./dist:/var/lib/grafana/plugins/ahmedismail-bugsink-app:ro`; environment `GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS: ahmedismail-bugsink-app,ahmedismail-bugsink-datasource`.
  * `docker/grafana/provisioning/plugins/apps.yaml`: `apiVersion: 1`, one app with `type: ahmedismail-bugsink-app`, `org_id: 1`, `disabled: false`, and no `jsonData` or `secureJsonData`.
  * `docker/grafana/provisioning/datasources/datasources.yaml`: add `name: Bugsink`, `type: ahmedismail-bugsink-datasource`, `uid: bugsink`, `access: proxy`, `url: http://bugsink:8000`, `secureJsonData.apiToken` the seeded global read token (`a`×40), with the comment `Public development-only token from docker/bugsink/seed.py.`
  * `docker/README.md`: the plugin must be built (`npm install`, `npm run build`, `npm run build:backend`) before `docker compose up -d --wait`; after a rebuild of the backend, `docker compose restart grafana`.

- [ ] **Step 6: Verify the skeleton end to end**

Run: `npm install && npm run build && npm run build:backend && docker compose up -d --wait && python3 scripts/verify_stack.py`
Expected: rc 0 and six `ok` lines.

Run: `curl -s http://127.0.0.1:3300/api/datasources/uid/bugsink/health`
Expected: `{"message":"Bugsink backend is running.","status":"OK"}` (key order may differ).

Run: `curl -s http://127.0.0.1:3300/api/plugins/ahmedismail-bugsink-app/settings | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d["enabled"], d["id"])'`
Expected: `True ahmedismail-bugsink-app`

- [ ] **Step 7: Run the scaffold's checks, then commit and push**

Run: `npm run typecheck && npm run lint && npm run test:ci && go test ./...`
Expected: all pass (delete scaffold tests of code removed in Step 3).

Commit `Scaffold the app plugin with a nested backend data source`.

---

### Task 2: The Bugsink client package

**Files:**
- Create: `pkg/bugsink/client.go`, `pkg/bugsink/errors.go`, `pkg/bugsink/client_test.go`, `pkg/bugsink/internal/gen/config.yaml`, `pkg/bugsink/internal/gen/generate.go`, `pkg/bugsink/internal/gen/client.gen.go`
- Modify: `go.mod`

**Interfaces:**
- Produces, in package `bugsink`:

```go
type Project struct {
	ID   int
	Name string
}

// Capability is a Bugsink token capability name, such as "issues:read".
type Capability string

const (
	ProjectsRead Capability = "projects:read"
	IssuesRead   Capability = "issues:read"
	EventsRead   Capability = "events:read"
)

// APIError is any Bugsink answer that is not the success the call expects.
type APIError struct {
	Status     int
	Capability Capability // set when a 403 body names a missing capability
	Detail     string     // Bugsink's "detail" text, empty when the body is not Bugsink JSON
}

func (e *APIError) Error() string

// New builds a client for the Bugsink at baseURL using httpClient for transport.
func New(baseURL, token string, httpClient *http.Client) (*Client, error)

func (c *Client) ListProjects(ctx context.Context) ([]Project, error)
func (c *Client) GetIssue(ctx context.Context, id string) error
func (c *Client) ListEventsWithoutIssue(ctx context.Context) error

var ErrInvalidURL = errors.New("invalid Bugsink URL")
var ErrNoToken = errors.New("no API token")
var ErrNotBugsink = errors.New("not a Bugsink API response")
```

  `ListProjects` follows `next` until it is null and returns `ErrNotBugsink` (wrapped, with the status in an `*APIError` reachable by `errors.As`) for a 200 without a `results` array. `GetIssue` and `ListEventsWithoutIssue` return an `*APIError` for every non-2xx answer, including the 404 and 400 the health check expects; they exist for the capability probes and M4 replaces them with real reads.

- [ ] **Step 1: Write the failing tests** in `pkg/bugsink/client_test.go`, each against an `httptest.Server`:

  * `TestNewNormalisesBaseURL`: for each of `http://h`, `http://h/`, `http://h/api/canonical/0`, `http://h/api/canonical/0/`, a `ListProjects` call requests the path `/api/canonical/0/projects/`. A base URL with a path prefix (`http://h/bugsink/`) requests `/bugsink/api/canonical/0/projects/`.
  * `TestNewRejectsBadInput`: `New("", "t", nil)`, `New("ftp://h", "t", nil)` and `New("h:8000", "t", nil)` return `ErrInvalidURL`; `New("http://h", "  \n", nil)` returns `ErrNoToken`.
  * `TestTokenIsTrimmedAndSentAsBearer`: `New(url, " abc\n", nil)` sends the header `Authorization: Bearer abc`.
  * `TestListProjectsFollowsNext`: two pages, the first with `next` pointing at the second; the result has the projects of both, in order, with `ID` and `Name`.
  * `TestListProjectsOnHTMLIsNotBugsink`: a 200 with `<html>` returns an error for which `errors.Is(err, ErrNotBugsink)` is true.
  * `TestCapabilityRefusal`: a 403 with `{"detail":"This token does not have the required capability: issues:read."}` gives an `*APIError` with `Status` 403 and `Capability` `IssuesRead`; a 403 with `{"detail":"This token is not allowed to access this project."}` gives `Capability` `""`.
  * `TestUnauthorized`: a 401 gives an `*APIError` with `Status` 401 and `Detail` `Invalid Bearer token.`
  * `TestProbeAnswers`: `GetIssue` on a 404 and `ListEventsWithoutIssue` on a 400 each give an `*APIError` with that status and an empty `Capability`.

- [ ] **Step 2: Run them and see them fail**

Run: `go test ./pkg/bugsink/...`
Expected: FAIL, the package does not compile.

- [ ] **Step 3: Generate the client**

`pkg/bugsink/internal/gen/config.yaml`: `package: gen`, `output: client.gen.go`, `generate: { models: true, client: true }`. `generate.go` holds `//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.2 -config config.yaml ../../../../bugsink-api/openapi.yaml`. Run `go generate ./pkg/bugsink/...` and commit the output.

- [ ] **Step 4: Implement `pkg/bugsink`** over the generated client

The token goes on through a request editor. A nil `httpClient` means `http.DefaultClient`. The capability is parsed from the `detail` text with the fixed prefix `This token does not have the required capability: ` and the trailing `.` removed. No other package imports `internal/gen`.

- [ ] **Step 5: Run the tests and see them pass**

Run: `go test ./pkg/bugsink/... && go vet ./...`
Expected: PASS, vet clean.

- [ ] **Step 6: Commit and push** as `Add the Bugsink client package`.

### Task 2B: Hand-written client (only if permission for generated code is refused or not given)

**Files:**
- Delete: `pkg/bugsink/internal/gen/`
- Modify: `pkg/bugsink/client.go`, `go.mod`, `design/implementation_roadmap.md` (M1 scope and Open decisions), `design/bugsink_grafana_plugin_spec.md` (§3 "Typed client generated from the pinned OpenAPI schema", §10)

- [ ] **Step 1:** Reimplement the three calls of Task 2 with `net/http` and `encoding/json`, decoding only the fields `Project` needs. The Interfaces block and the tests of Task 2 stay as they are.
- [ ] **Step 2:** Run `go test ./pkg/bugsink/... && go mod tidy && go build ./...`. Expected: PASS, and `go.mod` no longer names `oapi-codegen` or its runtime.
- [ ] **Step 3:** Commit and push as `Replace the generated Bugsink client with a hand-written one`.

---

### Task 3: The health check

**Files:**
- Create: `pkg/plugin/health.go`, `pkg/plugin/health_test.go`
- Modify: `pkg/plugin/datasource.go`, `pkg/plugin/datasource_test.go`

**Interfaces:**
- Consumes: everything Task 2 produces.
- Produces, in package `plugin`:

```go
// prober is the part of the Bugsink client the health check needs.
type prober interface {
	ListProjects(ctx context.Context) ([]bugsink.Project, error)
	GetIssue(ctx context.Context, id string) error
	ListEventsWithoutIssue(ctx context.Context) error
}

// checkHealth runs the capability probes and renders the contract's status and message.
func checkHealth(ctx context.Context, url string, p prober) *backend.CheckHealthResult

const defaultHealthTimeout = 10 * time.Second
const absentIssueID = "00000000-0000-0000-0000-000000000000"
```

  `Datasource.CheckHealth` reads `settings.URL` and `settings.DecryptedSecureJSONData["apiToken"]`, builds the HTTP client from `settings.HTTPClientOptions(ctx)` with the SDK's `httpclient.New`, maps `ErrInvalidURL` and `ErrNoToken` from `bugsink.New` to their messages, and calls `checkHealth` under a context that ends after the data source's `healthTimeout` field, which `NewDatasource` sets to `defaultHealthTimeout`. It always returns a nil error.

- [ ] **Step 1: Write the failing tests** in `pkg/plugin/health_test.go` with a fake `prober`, one table-driven test `TestCheckHealthMessages` with a case per row of the health check contract, asserting the exact status and message. Cases that need more than the table says:

  * `all present, two projects` → OK, `Connected to Bugsink. Projects visible to this token: sample-python, sample-js.`
  * `all present, twelve projects p1…p12` → OK, message ends `p9, p10 and 2 more.`
  * `lacks issues:read and events:read` → Error, `The API token lacks: issues:read, events:read.`
  * `lacks projects:read only` → Error, `The API token lacks: projects:read.`, and the fake records that all three probes ran.
  * `401 on the first probe` → the token message, and the fake records that the other two probes did not run.
  * `first probe 404 JSON` and `first probe ErrNotBugsink on 200` → the "did not answer like a Bugsink API" message with `HTTP 404` and `HTTP 200`.
  * `second probe 500` → `Bugsink answered HTTP 500 to a check for issues:read.`

  And in `pkg/plugin/datasource_test.go`, against an `httptest.Server`:

  * `TestCheckHealthEndToEnd`: a server that answers like the verified table → OK.
  * `TestCheckHealthSettingsErrors`: empty URL, `ftp://x`, and an empty token give the three settings messages, and the server receives no request.
  * `TestCheckHealthUnreachable`: a closed port → message starts `Cannot reach Bugsink at http://127.0.0.1:`.
  * `TestCheckHealthTimeout`: a handler that blocks, with the data source's `healthTimeout` set to 50 milliseconds, → `did not answer within`.
  * `TestCheckHealthNeverLeaksToken`: for every case above, with the token `secret-token-value`, neither `Message` nor `JSONDetails` contains it.
  * `TestCheckHealthOnHTML`: a server answering 200 `text/html` → the "did not answer like a Bugsink API (HTTP 200)" message.

- [ ] **Step 2: Run them and see them fail**

Run: `go test ./pkg/plugin/...`
Expected: FAIL, `checkHealth` undefined.

- [ ] **Step 3: Implement `checkHealth` and wire `Datasource.CheckHealth`**

A probe's answer is classified from its error with `errors.As` to `*bugsink.APIError`: a set `Capability` means "lacks"; the expected status (`nil` for projects, 404 for the issue, 400 for events) means "has"; a context deadline means the timeout row; any other non-API error is the "Cannot reach" row, with the cause taken from the innermost error so that it carries no URL query or header.

- [ ] **Step 4: Run the tests and see them pass**

Run: `go test ./... && go vet ./...`
Expected: PASS.

- [ ] **Step 5: Check it on the stack**

Run: `npm run build:backend && docker compose restart grafana && docker compose up -d --wait && curl -s http://127.0.0.1:3300/api/datasources/uid/bugsink/health`
Expected: status `OK`, message `Connected to Bugsink. Projects visible to this token: sample-python, sample-js.` (project order as Bugsink returns it).

- [ ] **Step 6: Commit and push** as `Add the data source health check`.

---

### Task 4: Config editor

**Files:**
- Create: `src/datasource/components/ConfigEditor.tsx`, `src/datasource/components/ConfigEditor.test.tsx`
- Modify: `src/datasource/module.ts`, `src/datasource/types.ts`

**Interfaces:**
- Consumes: `BugsinkOptions`, `BugsinkSecureJsonData` from Task 1.
- Produces: `export function ConfigEditor(props: DataSourcePluginOptionsEditorProps<BugsinkOptions, BugsinkSecureJsonData>): JSX.Element`. Field labels, used by the end-to-end tests: `Bugsink URL` and `API token`.

- [ ] **Step 1: Write the failing tests** in `ConfigEditor.test.tsx` with React Testing Library:

  * `shows the URL from options`: `options.url = 'http://bugsink:8000'` → the `Bugsink URL` input has that value.
  * `typing a URL calls onOptionsChange with url`: the last call's argument has `url: 'http://x'` and leaves `secureJsonData` and `secureJsonFields` as they were.
  * `typing a token sets secureJsonData.apiToken`: the last call has `secureJsonData: { apiToken: 'abc' }`.
  * `a stored token shows as configured and is not sent again`: with `secureJsonFields: { apiToken: true }`, the `API token` field is disabled and shows `configured`, a `Reset` button is present, and typing in `Bugsink URL` calls `onOptionsChange` with no `apiToken` in `secureJsonData`.
  * `Reset clears the stored token`: clicking `Reset` calls `onOptionsChange` with `secureJsonFields.apiToken === false` and `secureJsonData.apiToken === ''`.
  * `explains the token`: the text `projects:read, issues:read and events:read` is on the page.

- [ ] **Step 2: Run them and see them fail**

Run: `npm run test:ci -- ConfigEditor`
Expected: FAIL.

- [ ] **Step 3: Implement the editor** with `@grafana/ui` `Field`, `Input` and `SecretInput`.

  * `Bugsink URL`: placeholder `https://bugsink.example.com`, description `The address of Bugsink itself, without /api.`
  * `API token`: description `A Bugsink API token with the capabilities projects:read, issues:read and events:read. A project-bound token works: the plugin shows only that token's project.`
  * No other fields.

- [ ] **Step 4: Run the tests and see them pass**

Run: `npm run test:ci && npm run typecheck && npm run lint`
Expected: PASS.

- [ ] **Step 5: Commit and push** as `Add the data source config editor`.

---

### Task 5: App home page

The app needs one honest page: without it the navigation entry leads nowhere, and M3 needs the same list for its data source picker.

**Files:**
- Create: `src/pages/Home.tsx`, `src/pages/Home.test.tsx`
- Modify: `src/module.tsx`

**Interfaces:**
- Consumes: data source type id `ahmedismail-bugsink-datasource`.
- Produces: `export function Home(): JSX.Element`.

- [ ] **Step 1: Write the failing tests** in `Home.test.tsx`, mocking `getDataSourceSrv().getList`:

  * `lists Bugsink data sources`: two instances `Bugsink` (uid `bugsink`) and `Bugsink EU` (uid `eu`) → two links with those names, to `/connections/datasources/edit/bugsink` and `/connections/datasources/edit/eu`; `getList` was called with `{ type: 'ahmedismail-bugsink-datasource' }`.
  * `explains what to do when there are none`: an empty list → the text `No Bugsink data source yet.` and a link `Add a Bugsink data source` to `/connections/datasources/new`.

- [ ] **Step 2: Run them and see them fail**

Run: `npm run test:ci -- Home`
Expected: FAIL.

- [ ] **Step 3: Implement `Home`** inside `PluginPage`, with the page title `Bugsink` and one sentence above the list: `Issue browsing arrives in a later version. For now, connect Grafana to Bugsink with a data source.`

- [ ] **Step 4: Run the tests and see them pass**

Run: `npm run test:ci && npm run typecheck && npm run lint`
Expected: PASS.

- [ ] **Step 5: Commit and push** as `Add the app home page`.

---

### Task 6: End-to-end tests of the demo

**Files:**
- Create: `tests/datasource-config.spec.ts`, `tests/app-home.spec.ts`, `tests/seeded.ts`
- Modify: `playwright.config.ts`, `package.json`, `design/implementation_roadmap.md`

**Interfaces:**
- Consumes: field labels from Task 4; messages from the health check contract; provisioned uid `bugsink`.
- Produces: `npm run e2e` runs against `GRAFANA_URL`, default `http://localhost:3300`. `tests/seeded.ts` exports `BUGSINK_URL = 'http://bugsink:8000'` and the tokens `GLOBAL_READ`, `PROJECT_READ`, `NO_CAPS` (`a`, `c`, `d` × 40), under the comment `Public development-only values from docker/bugsink/seed.py.`

- [ ] **Step 1: Point Playwright at the Compose stack**

`playwright.config.ts` (the root one, which extends `.config/playwright.config.ts`): `use: { baseURL: process.env.GRAFANA_URL || 'http://localhost:3300' }`. The stack runs Grafana with anonymous Admin and no login form. Run the scaffold's auth setup project once; if it fails to log in, remove the `auth` project and the `storageState` dependency, so that the tests rely on anonymous Admin alone, and record the outcome in `design/plans/m1-scaffold-notes.md`. If `createDataSourceConfigPage` then cannot create a data source, re-enable the login form in `docker-compose.yaml` (keeping anonymous Admin) so the default `admin:admin` API credentials work, and record that instead.

- [ ] **Step 2: Write the tests** in `tests/datasource-config.spec.ts`. Each creates a data source with `createDataSourceConfigPage({ type: 'ahmedismail-bugsink-datasource' })`, fills `Bugsink URL` and `API token`, and calls `saveAndTest()`:

| Test | URL | Token | Assertion |
| :--- | :--- | :--- | :--- |
| `valid global token` | `BUGSINK_URL` | `GLOBAL_READ` | `toBeOK()`; success alert has `Projects visible to this token:` and both `sample-python` and `sample-js` |
| `project-bound token` | `BUGSINK_URL` | `PROJECT_READ` | `toBeOK()`; success alert has `sample-python` and not `sample-js` |
| `URL with trailing path` | `BUGSINK_URL + '/api/canonical/0/'` | `GLOBAL_READ` | `toBeOK()` |
| `wrong URL` | `http://bugsink:9` | `GLOBAL_READ` | `not.toBeOK()`; error alert has `Cannot reach Bugsink at http://bugsink:9` |
| `not a Bugsink` | `http://grafana:3000` | `GLOBAL_READ` | error alert has `did not answer like a Bugsink API` |
| `bad token` | `BUGSINK_URL` | `'0'.repeat(40)` | error alert has `Bugsink rejected the API token` |
| `token without capabilities` | `BUGSINK_URL` | `NO_CAPS` | error alert has `The API token lacks: projects:read, issues:read, events:read.` |
| `no token` | `BUGSINK_URL` | (empty) | error alert has `Enter an API token.` |
| `provisioned data source` | (opened with `gotoDataSourceConfigPage('bugsink')`) | | `API token` shows `configured`; `saveAndTest()` `toBeOK()` |

`tests/app-home.spec.ts`: opening `/a/ahmedismail-bugsink-app` shows the heading `Bugsink` and a link named `Bugsink` to the provisioned data source.

- [ ] **Step 3: Run them**

Run: `npm run build && npm run build:backend && docker compose up -d --wait && docker compose restart grafana && docker compose up -d --wait && npm run e2e`
Expected: 10 passed. A failure here is a defect in Tasks 1 to 5 or in the test; fix the cause in a new commit, never loosen an assertion to a substring that the contract table does not contain.

- [ ] **Step 4: Run twice more to rule out leftovers**

Run: `npm run e2e && npm run e2e`
Expected: 10 passed both times (data sources the tests create are removed by the fixture; if they pile up, delete them in an `afterEach`).

- [ ] **Step 5: Update the roadmap's M1 line**

In `design/implementation_roadmap.md`, M1 scope: replace `for URL, token and token kind, and for the capabilities the token lacks` with `for URL and token, the projects the token can see, and the capabilities it lacks`.

- [ ] **Step 6: Commit and push** as `Add end-to-end tests for connecting a data source`.

---

### Task 7: CI, licence checks and documentation

**Files:**
- Create: `.github/workflows/ci.yml` (from the scaffold's, reworked), `scripts/check_licenses.sh`, `scripts/check_spdx.sh`, `CHANGELOG.md`, `src/README.md`
- Modify: `README.md`, `docker/README.md`, `package.json`

**Interfaces:**
- Consumes: the npm scripts of Task 1, `npm run e2e` of Task 6.
- Produces: one workflow `CI` with jobs `checks` and `e2e`; `scripts/check_licenses.sh` and `scripts/check_spdx.sh`, both exit non-zero on a violation and print the offending package or file.

- [ ] **Step 1: Write `scripts/check_spdx.sh` and see it fail on a planted file**

It lists tracked files matching `*.go`, `*.ts`, `*.tsx`, `*.js`, `*.mjs`, `*.sh`, `*.py`, `*.yaml`, `*.yml` outside `.config/`, `pkg/bugsink/internal/gen/`, `bugsink-api/` and `node_modules/`, and fails when the first five lines of one lack `SPDX-License-Identifier: GPL-3.0-or-later`.

Run: `scripts/check_spdx.sh`
Expected: it names every scaffold file still without a header. Add the headers; run again; expected rc 0.

- [ ] **Step 2: Write `scripts/check_licenses.sh`**

  * npm, production dependencies only, with `license-checker-rseidelsohn` as a dev dependency: allow `MIT`, `Apache-2.0`, `BSD-2-Clause`, `BSD-3-Clause`, `ISC`, `0BSD`, `CC0-1.0`, `Unlicense`, `MPL-2.0`, `BlueOak-1.0.0`, `Python-2.0`, `LGPL-3.0-or-later`, `GPL-3.0-or-later`, `AGPL-3.0-only`, `AGPL-3.0-or-later`.
  * Go, with `go run github.com/google/go-licenses check ./...` pinned in `tools.go`: the same list.
  * A dependency under any other licence fails the script. Add a licence to the list only when it is GPLv3-compatible, and say which package needed it in the commit message.

Run: `scripts/check_licenses.sh`
Expected: rc 0.

- [ ] **Step 3: Write the workflow**

`on: [push, pull_request]`, `permissions: contents: read`, each action at the version the scaffold's `ci.yml` uses. The scaffold's signing, packaging, validator and version-matrix steps are left out; M11 adds release.

  * Job `checks`: checkout; `actions/setup-node` with `node-version-file: .nvmrc` and npm cache; `npm ci`; `npm run typecheck`; `npm run lint`; `npm run test:ci`; `npm run build`; `actions/setup-go` with `go-version-file: go.mod`; `go vet ./...`; `golangci/golangci-lint-action` as in the scaffold's workflow; `go test ./...`; `go generate ./pkg/bugsink/... && git diff --exit-code pkg/bugsink` (dropped by Task 2B); `scripts/check_spdx.sh`; `scripts/check_licenses.sh`.
  * Job `e2e`, `needs: checks`: checkout; node and Go as above; `npm ci`; `npm run build`; `npm run build:backend`; `npx playwright install --with-deps chromium`; `docker compose up -d --wait`; `python3 scripts/verify_stack.py`; `npm run e2e` with `GRAFANA_URL: http://localhost:3300`; on failure, `docker compose logs grafana bugsink` and upload `playwright-report/` with `actions/upload-artifact`.

- [ ] **Step 4: Documentation**

  * `src/README.md` (the plugin's catalog page): what the plugin is, that this version connects only, how to create a Bugsink API token and which three capabilities it needs, the two fields, and every health check message from the contract table with what to do about it.
  * `CHANGELOG.md`: `## 0.1.0 (unreleased)` with `Bugsink data source: connect to a Bugsink instance and verify the URL, the API token and its read capabilities.`
  * `README.md`: replace `Status: in design.` with the current state, and add a `Development` section: prerequisites (Node per `.nvmrc`, Go per `go.mod`, Docker), build, stack, unit tests, end-to-end tests, regenerating the client.

- [ ] **Step 5: Run everything the workflow runs, locally**

Run: `npm run typecheck && npm run lint && npm run test:ci && npm run build && go vet ./... && go test ./... && scripts/check_spdx.sh && scripts/check_licenses.sh && npm run build:backend && docker compose up -d --wait && python3 scripts/verify_stack.py && npm run e2e`
Expected: every command rc 0, six `ok` lines, 10 passed.

- [ ] **Step 6: Commit and push** as `Add CI, licence checks and user documentation`. Do not watch the workflow run; report that it was pushed.

---

## After the last task

* Whole-branch review against this plan, the health check contract and the Review Focus list.
* The merge gate: check `bugsink-api/NOTICE.md` for the recorded permission. If it is absent, stop and ask; Task 2B runs only on the user's word.
* The demo, by hand, on the stack: the five cases of the roadmap's M1 demo.

---

## As built

Differences between this plan and the code on `m1/connect`:

1. A 401 from any probe, not only the first, gives the token message.
2. One message is not in the contract table: `The data source settings are invalid.` (the HTTP client could not be built from the data source's settings).
3. The check stops at the first connection error, timeout, rejected token, answer that is not Bugsink's, or unexpected answer to a later probe. An unexpected later answer is reported ahead of missing capabilities. A 2xx on a later probe counts as having the capability.
4. The timeout message renders the configured timeout; the signature is `checkHealth(ctx, url, timeout, p)`.
5. `ListProjects` pages by passing the `cursor` value of Bugsink's `next` to the generated method. It never requests the `next` URL, so the token is only sent to the configured host. Reading `projects:read` can therefore take more than one request.
6. Go commands run on `./pkg/...`, not `./...`: `node_modules` contains a Go package.
7. There is no `tools.go`: `mage` comes through the SDK, and `go-licenses` runs with `go run …@v1.6.0`.
8. The provisioned-data-source end-to-end test clicks `Test` (a provisioned data source has no `Save & test`) and checks the health response.
9. The home page passes no `pageNav` (it duplicated the breadcrumb) and filters the data source list to the Bugsink type on the client, because Grafana adds its built-in `-- Grafana --` data source to the list.
10. The npm licence check evaluates SPDX expressions with `spdx-satisfies`. The allow-list also has `Zlib` (pako). `get-document@1.0.0` is excluded by name (no licence field; its repository is MIT). Two packages that ship the Unlicense text without a licence field are mapped to `Unlicense` by exact version. The generated-client check in CI uses `git status --porcelain`.
11. Jest: the root `jest-setup.js` closes the ports of the `worker_threads` `MessageChannel`s opened during the tests (the scaffold's setup installs that class), so that Jest exits.
12. End-to-end authentication relies on anonymous Admin; the scaffold's `auth` project is emptied in the root `playwright.config.ts`.
13. The branch was made from `plan/m1-connect`, not `main`.
14. The merge gate was lifted: on 2026-11-15 the owner accepted the risk of shipping the generated client without the Bugsink maintainer's permission. Task 2B did not run.
