# M7: Basic Triage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** An Editor or Admin resolves, reopens, mutes and unmutes Bugsink issues from the App's issue list and issue page, on a data source with triage switched on; anyone else sees no controls and the backend refuses them.

**Architecture:** `pkg/bugsink` gains `Triage`, one bodiless POST per action. The data source reads two switches (`triageEnabled`, `hideStacktraceText`), probes `issues:triage` in the health check without a side effect, and serves four POST resource routes that check the caller's org role from the request's plugin context before anything else. The frontend shows an action only where the backend would allow it and shows the state from the issue Bugsink answers with.

**Tech Stack:** Go 1.26 with `grafana-plugin-sdk-go` v0.299.0, React with `@grafana/ui` and `@grafana/runtime` 13.2.4, Jest, `@grafana/plugin-e2e` 4.0.0 on Playwright 1.65.1, the Compose stack (Bugsink 2.6.3, Grafana 13.3.1).

**Spec:** [`../bugsink_grafana_plugin_spec.md`](../bugsink_grafana_plugin_spec.md) (§4.A, §4.C stack trace text, §4.D, §4.E). **Roadmap:** [`../implementation_roadmap.md`](../implementation_roadmap.md) (M7, Definition of done). **Findings:** [`../phase0_findings.md`](../phase0_findings.md) (Q4, Q5, Q10).

## Global Constraints

* The API token never reaches the browser, a log line, an error message, a frame or a resource answer.
* The backend pages with the `cursor` from `next` only; it never builds a cursor or requests a URL taken from a response.
* Generated code stays in `pkg/bugsink/internal/gen/`, imported only by `pkg/bugsink`, not regenerated. Nothing in `.config/` changes. No plugin id, type or `plugin.json` change. Provisioning changes need `docker compose restart grafana`.
* SPDX header `GPL-3.0-or-later` on every hand-written source file; `scripts/check_spdx.sh` passes. Comments are one line and describe the code; tests assert behaviour, never code structure.
* Go commands run on `./pkg/...`. Host ports stay 3300 (Grafana), 8800 (Bugsink), 8810 and 8820 (sample apps).
* Resources never answer 401: Bugsink failures 502, invalid settings or input 400, a refused caller 403. Resource reads and writes from the frontend pass `showErrorAlert: false` and the page shows the message inline. Untrusted Bugsink text is plain text.
* Reuse `userMessage`, `validIssueID`, `issueJSON`, `d.projectNames`, the instance caches, the query model and the e2e helpers (`tests/fixtures.ts`, `tests/reported.ts`, `tests/seeded.ts`); no parallel copies.
* E2E: at most 5 Bugsink events per run, reported only from `tests/*.setup.ts` (project `report-event`, `setup.describe.configure({ retries: 0 })`); this milestone adds none (decision 17). Digest-dependent assertions retry with `toPass()`.
* Branch `m7/basic-triage` in the worktree `.claude/worktrees/m7-basic-triage` (from `main` at fb842fe). One implementer at a time; push after every commit; never amend. Run every test command in the foreground.
* The running stack may mount another checkout. Before the first stack check (Task 7 only), run `docker compose up -d --build --wait` from the worktree. Never `docker compose down`.

## Facts already verified (2026-11-15)

* Triage POSTs on an absent issue (`issues/00000000-0000-0000-0000-000000000000/<action>/`, all four actions): triage token 404 `{"detail":"No Issue matches the given query."}`; read, project-bound and no-capability tokens 403 `{"detail":"This token does not have the required capability: issues:triage."}`; no token 401. Bugsink checks the capability before it looks up the issue.
* `GET issues/<id>/resolve/` is 405 `{"detail":"Method \"GET\" not allowed."}`; `POST issues/not-a-uuid/resolve/` is 400 `["Invalid issue identifier."]`; the short id works in the path (`issues/SAMPLE-PYTHON-7/resolve/` 200).
* Transitions, on the throwaway issue `SAMPLE-PYTHON-7` (found open and unmuted, left so):

  | State | `resolve/` | `reopen/` | `mute/` | `unmute/` |
  | :--- | :--- | :--- | :--- | :--- |
  | open | 200, resolved | 400 `Issue is not resolved.` | 200, muted | 400 `Issue is not muted.` |
  | muted | 200, resolved and unmuted | 400 `Issue is not resolved.` | 400 `Issue is already muted.` | 200, open |
  | resolved | 400 `Issue is already resolved.` | 200, open | 400 `Issue is already resolved.` | 400 `Issue is already resolved.` |

* A 200 is the whole issue, the same 19 keys as `GET issues/<id>/`. A JSON body sent to `unmute/` is ignored. Each action adds a history entry (`user_display_name` null for the seeded token).
* SDK v0.299.0: `backend.User{Login, Name, Email, Role string}` in `PluginContext.User`; `httpadapter` stores it in the request context (`handler.go:37`), read by `backend.UserFromContext(ctx)`, nil when absent; `backend.Logger.FromContext(ctx)`.
* Grafana v13.3.1 source: `adapters.BackendUserFromSignedInUser` sets `Role: string(requester.GetOrgRole())` and the user is nil without a requester (`pkg/services/pluginsintegration/adapters/adapters.go:49-58`, `plugincontext/base_plugincontext.go`); org roles are `None`, `Viewer`, `Editor`, `Admin`; an anonymous user carries `auth.anonymous.org_role` (`pkg/services/anonymous/anonimpl/client.go:119`).
* Grafana routes `/api/datasources/uid/:uid/resources/*` with `.Any` behind `datasources:query` (`pkg/api/api.go:408-409`). In a throwaway Grafana 13.3.1 a Viewer, an Editor and a Viewer service account (login `sa-1-probe-viewer`) got 200 from `resources/projects`, and their POST and PUT reached the plugin (its 405). Grafana does not stop a Viewer; the plugin must.
* API keys are gone (`GET /api/auth/keys` 404). The data source API path `/apis/ahmedismail-bugsink-datasource.datasource.grafana.app/v0alpha1/…/resource/projects` is 404. The data proxy carries no token: `POST /api/datasources/proxy/uid/bugsink/api/canonical/0/issues/<zero>/resolve/` is 400 `Authentication to data source failed` (Bugsink's 401; the read token would give 403).
* Users in the stack: `POST /login` is 400 `auth.client.notConfigured` (login form disabled), so plugin-e2e's `login` and `createUser` fixtures fail. Basic auth works: `admin:admin` creates users (`POST /api/admin/users`; 412 when the login exists, then `GET /api/users/lookup?loginOrEmail=`) and sets roles (`PATCH /api/orgs/1/users/<id>`; a Viewer gets 403 there). A Playwright context with `extraHTTPHeaders: { Authorization: 'Basic …' }` loads pages as that user (`grafanaBootData.user.orgRole` `Viewer` or `Editor`); `httpCredentials` with `send: 'always'` did not reach browser requests. A wrong password falls back to the anonymous Admin with 200; anonymous `GET /api/user` is 401.
* `@grafana/ui` 13.2.4 TableNG hides a field only when `custom.hideFrom.viz === true` (`components/Table/TableNG/utils.mjs`, `getVisibleFields`); `custom.hidden` is read by the legacy table only.
* `@grafana/runtime` 13.2.4: `postResource<T>(path, data?, options?)`; `config.bootData.user.orgRole: OrgRole | ''`. `@grafana/ui` exports `Switch`.
* `pkg/plugin/links.go:80` `logsVarPattern` is `\$\{([^}]*)\}`, which matches across a newline.
* Not verified, checked first by Task 7: that Grafana 13.3.1's own table honours `hideFrom.viz` and the Open logs link still reads the hidden field; the role the plugin receives, observed through the routes for the anonymous Admin, an Editor, a Viewer and a Viewer service account. Not checked and not relied on: the identities of alerting and public dashboards (they only query; decision 3).

## Decisions in this plan, for review

1. **Who may triage:** a caller whose `backend.UserFromContext(ctx).Role` is exactly `Editor` or `Admin`. Fixed in code; the only setting is the per-data-source switch; M8's RBAC actions make it configurable. Nil user, empty role, `None` and `Viewer` are refused. The org role counts, not the server-admin flag.
2. **Anonymous users and service accounts** carry their org role: an anonymous Editor or Admin (the dev stack's anonymous Admin) and an Editor service account can triage. The README says so.
3. **Only `CallResource` writes.** `QueryData` has no write path (an unknown `queryType` is 400 with no Bugsink request), so what Grafana runs as queries (alerting, public dashboards, reports) cannot triage, and the data proxy carries no token.
4. **Routes and order of checks:** `POST issues/{id}/resolve`, `…/reopen`, `…/mute`, `…/unmute`; any other method on them is 405 with `Allow: POST`. Then: caller (403), settings (400), switch (403), id (400), Bugsink. The caller check comes first, so a refused caller learns nothing about the settings.
5. **Nothing user-controlled reaches Bugsink but the id and the action.** The id passes `validIssueID` (`^[A-Za-z0-9_-]{1,100}$`, uuid or short id); the action comes from the route pattern; the request body and query string are never read; the Bugsink request has no body. Hostile ids `..`, `a%2Fb`, `x%20y`, `x%3Fy`, an empty segment and 101 characters are 400 with no Bugsink request.
6. **Answers:** 200 with the `GET issues/{id}` body built from Bugsink's answer; any Bugsink failure 502 with `userMessage`'s text, so a changed issue reads `Bugsink refused the request: Issue is already resolved.` and a token without the capability `The API token lacks issues:triage.`. Every answer of a triage route carries `Cache-Control: no-store`; no action is cached; after any call Bugsink answered, the instance's issue cache is emptied.
7. **Logging:** one info line per call that reached Bugsink, with action, issue id, Grafana login and Bugsink's status; never the token, a header, Bugsink's body or a URL.
8. **Triage switch** `jsonData.triageEnabled`, default off: writes are opt-in and existing data sources stay read-only with nothing new probed. A non-boolean value gives `The data source settings are invalid.`
9. **Health check probes `issues:triage`** when triage is on, with `POST issues/00000000-0000-0000-0000-000000000000/resolve/`: 404 means the token has it, a 403 naming it adds it to `The API token lacks: …`. Spec §4.A says write capabilities cannot be checked without a side effect; Bugsink checks the capability before the issue lookup (verified), so this plan follows the verified behaviour and the as-built step updates the spec.
10. **Actions by state** (the verified transitions): open offers Resolve and Mute; muted offers Resolve and Unmute; resolved offers Reopen. The list offers only Resolve and Mute of those (spec §4.E); a resolved row has none. Actions send the issue's uuid.
11. **Shown state:** the list replaces the row's status from the answer until the list next loads, without a new query; the issue page replaces its summary from the answer and reads History again. After a failed action the message shows inline (list: above the table, prefixed with the short id; page: under the buttons), the list reloads and the page reads summary and History again, so the shown state is Bugsink's.
12. **Frontend gate:** controls show only when the data source has `triageEnabled` and `config.bootData.user.orgRole` is `Editor` or `Admin`; this is convenience, the backend decides. The client checks issue ids with the backend's pattern before every issue resource call and rejects with `"<id>" is not an issue id.` without a request.
13. **`hideStacktraceText`** (default off: shown, as today): the event resource drops Bugsink's text in the backend (`stacktraceMd: null`, never sent) and, when there was text, adds the notice `Bugsink's stack trace text is hidden for this data source.` The switch sits next to the triage switch.
14. **`logs_query` is hidden by the frame:** `Config.Custom` becomes `{"hidden": true, "hideFrom": {"viz": true, "legend": true, "tooltip": true}}`, so every table honours it, including Explore and new panels; no dashboard override.
15. **The logs variable pattern excludes newline:** `\$\{([^}\n]*)\}`, in the quote check and the renderer; a `${…}` across a line is text and stays as written. Refused: `{a="b"} # ${tag:k<newline>} "<newline>| x=${service} | y="z"`.
16. **E2E identities:** `tests/users.setup.ts` makes `m7-editor` (Editor) and `m7-viewer` (Viewer) through the basic-auth admin API; specs send `extraHTTPHeaders` Basic and first assert `GET /api/user` returns that login. New provisioned `Bugsink triage` (uid `bugsink-triage`, triage token, triage on); `Bugsink` stays read-only with triage off and is the switched-off case.
17. **E2E state and budget:** the triage spec acts on the issue of the `m5-message` event of `tests/report-event-detail.setup.ts` (its only other reader lists with `state=all`), so a run still reports the five events m2-e2e, m5-chained twice, m5-message and m6-js. The spec runs serially; `beforeAll` and `afterAll` put the issue back to open through Bugsink with the triage token (reopen when resolved, unmute when muted), so a run that died half-way is repaired by the next one.

## Contracts

Triage routes (`CallResource`, JSON `{"message": …}` on refusal):

| Condition | Status | Message |
| :--- | :--- | :--- |
| Method other than POST | 405, `Allow: POST` | `Method not allowed.` |
| Caller not `Editor` or `Admin`, or no user | 403 | `Only editors and admins can triage issues.` |
| Settings invalid | 400 | the settings message |
| Triage off | 403 | `Triage is switched off for this data source.` |
| Id fails `validIssueID` | 400 | `"<id>" is not an issue id.` |
| Bugsink answers anything but 2xx, or no answer | 502 | `userMessage(err)` |
| Success | 200 | the issue body of `GET issues/{id}` |

* Health: with triage on and the capability present, the success message ends with ` The token can triage issues.`; missing, `issues:triage` joins the lacking list (`The API token lacks: issues:triage.`).
* Config editor: field `Triage`, switch id `bugsink-triage`, description `Let editors and admins resolve, reopen, mute and unmute issues from Grafana. The API token needs issues:triage. Every action runs as the token's Bugsink user.`; field `Hide stack trace text`, switch id `bugsink-hide-stacktrace-text`, description `Do not show Bugsink's own stack trace text, which can include local variables, for events without structured frames.`
* UI: buttons `Resolve`, `Reopen`, `Mute`, `Unmute`; list column `Actions`, buttons named `Resolve <short id>` and `Mute <short id>`.

## Review Focus

1. A caller with no user, an empty role, `None`, `Viewer` or a wrong-case `editor`, on any route and with any method: 403 or 405 and no request reaches Bugsink (Task 3 `TestTriageRefusesCaller`, `TestTriageMethods`).
2. The issue changed meanwhile (a second click, another user): 502 with Bugsink's sentence, and the UI then shows the current state (Task 3 `TestTriageBugsinkErrors`; Task 5 and Task 6 failure tests).
3. Hostile ids on a write route (`..`, encoded slash, space, `?`, empty, too long): 400 and no Bugsink request; the frontend refuses before calling (Task 3 `TestTriageRejectsIDs`; Task 4 test).
4. A body or query string on a write route (`period_name`, `gte_threshold`) never reaches Bugsink (Task 3 `TestTriageSendsOnlyTheAction`).
5. Refresh after an action never shows the 15-second cached old state (Task 3 `TestTriageClearsIssueCache`).

## File structure

| File | Responsibility |
| :--- | :--- |
| `pkg/bugsink/triage.go` (new) | `TriageAction`, `Triage` |
| `pkg/bugsink/errors.go` | `IssuesTriage` capability |
| `pkg/plugin/links.go`, `frames.go` | M6 leftovers: variable pattern, `hideFrom` |
| `pkg/plugin/datasource.go`, `health.go` | the two switches, the triage probe |
| `pkg/plugin/triage.go` (new) | caller check, triage routes |
| `pkg/plugin/resources.go`, `cache.go` | route registration, hidden stack trace text; `clear` |
| `src/datasource/types.ts`, `datasource.ts`, `components/ConfigEditor.tsx` | `TriageAction`, id check, `triage`, `canTriage`, switches |
| `src/pages/format.ts` (new), `src/pages/TriageButtons.tsx` (new) | shared status labels and time format; action buttons |
| `src/pages/IssuesPage.tsx`, `issues/IssueTable.tsx`, `IssuePage.tsx` | inline and detail actions |
| `docker/grafana/provisioning/datasources/datasources.yaml`, `tests/users.setup.ts` (new), `tests/triage.spec.ts` (new) | demo data source, users, e2e |

---

### Task 1: Triage calls in the Bugsink client, and two M6 leftovers

**Files:**
- Create: `pkg/bugsink/triage.go`, `pkg/bugsink/triage_test.go`
- Modify: `pkg/bugsink/errors.go`, `pkg/plugin/links.go:80`, `pkg/plugin/links_test.go:53`, `pkg/plugin/frames.go:127`, `pkg/plugin/frames_test.go:231`

**Interfaces:**
- Consumes: `gen.Client.IssuesResolveCreate`, `IssuesReopenCreate`, `IssuesMuteCreate`, `IssuesUnmuteCreate` (each `(ctx, id string, …) (*http.Response, error)`), `decodeJSON`, `issueOf`.
- Produces:

```go
const IssuesTriage Capability = "issues:triage"
type TriageAction string // Resolve "resolve", Reopen "reopen", Mute "mute", Unmute "unmute"
// Triage runs one bodiless triage action and returns the issue Bugsink answers with.
func (c *Client) Triage(ctx context.Context, issueID string, action TriageAction) (Issue, error)
```

- [ ] **Step 1: Write failing tests** against an `httptest.Server`:
  - `TestTriageSendsBodilessPost`: per action the server sees `POST /api/canonical/0/issues/<uuid>/<action>/`, an empty body, no query, `Authorization: Bearer <token>`; a 200 copied from the verified issue with `is_resolved: true` gives `Issue{IsResolved: true, …}`.
  - `TestTriageShortID`: `SAMPLE-PYTHON-7` reaches the path unchanged.
  - `TestTriageRefusals`: 400 `{"detail":"Issue is already resolved."}` gives `APIError{Status: 400, Detail: "Issue is already resolved."}`; a 403 naming the capability gives `Capability: IssuesTriage`; 404 keeps its detail.
  - `TestTriageUnknownAction`: `TriageAction("delete")` errors and makes no request.
  - `links_test.go`: add `"{a=\"b\"} # ${tag:k\n} \"\n| x=${service} | y=\"z\""` to the refused list; `TestRenderLogsQuery` gains `"\"${tag:a\nb}\""`, which renders unchanged.
  - `frames_test.go:231`: `logs_query` has `Custom["hidden"] == true` and `Custom["hideFrom"]` equal to `map[string]any{"viz": true, "legend": true, "tooltip": true}`.
- [ ] **Step 2: Run** `go test ./pkg/...`. Expected: FAIL.
- [ ] **Step 3: Implement** `Triage` as a switch over the four generated calls; `logsVarPattern` becomes `\$\{([^}\n]*)\}`; the `hideFrom` map on `logs_query`.
- [ ] **Step 4: Run** `go test -race ./pkg/...` and `go vet ./pkg/...`. Expected: PASS.
- [ ] **Step 5: Commit** `Add triage calls to the Bugsink client` and push.

### Task 2: The triage and stack trace switches, and the triage health check

**Files:**
- Modify: `pkg/plugin/datasource.go`, `pkg/plugin/datasource_test.go`, `pkg/plugin/health.go`, `pkg/plugin/health_test.go`, `pkg/plugin/resources.go` (`handleEvent`), `pkg/plugin/resources_test.go`

Authorization: yes

**Interfaces:**
- Consumes: Task 1's `Triage`, `Resolve`, `IssuesTriage`.
- Produces: `Datasource.triageEnabled bool`, `Datasource.hideStacktraceText bool` (from `jsonData.triageEnabled`, `jsonData.hideStacktraceText`); `prober` gains `Triage(ctx context.Context, id string, action bugsink.TriageAction) (bugsink.Issue, error)`; `func checkHealth(ctx context.Context, url string, timeout time.Duration, p prober, triage bool) *backend.CheckHealthResult`.

- [ ] **Step 1: Write failing tests.**
  - `datasource_test.go`: `{"triageEnabled": true}` sets `triageEnabled`; absent is false; `{"triageEnabled": "yes"}` gives `The data source settings are invalid.`; the same three for `hideStacktraceText`.
  - `health_test.go` with the fake prober: triage off never calls `Triage`; triage on calls it once with `absentIssueID` and `bugsink.Resolve`; a 404 gives OK and a message ending ` The token can triage issues.`; a 403 naming the capability gives `The API token lacks: issues:triage.`, and with `events:read` also missing `The API token lacks: events:read, issues:triage.`; a 200 gives `Bugsink answered HTTP 200 to a check for issues:triage.`; a 405 `Bugsink answered HTTP 405 to a check for issues:triage.`; a 401 on the first probe keeps its M1 message.
  - `resources_test.go` `TestEventHidesStacktraceText`: with the switch on, an event with text and no frames answers `stacktraceMd: null`, the body does not contain the text and `notices` holds `Bugsink's stack trace text is hidden for this data source.`; without text, no notice; switch off, unchanged.
- [ ] **Step 2: Run** `go test -race ./pkg/plugin/...`. Expected: FAIL.
- [ ] **Step 3: Implement.** The triage probe is the last entry of `probes`, added only when on, with `has` 404.
- [ ] **Step 4: Run** `go test -race ./pkg/...`, `go vet ./pkg/...`. Expected: PASS.
- [ ] **Step 5: Commit** `Add the triage and stack trace text switches and the triage health check` and push.

### Task 3: The triage routes and the caller check

**Files:**
- Create: `pkg/plugin/triage.go`, `pkg/plugin/triage_test.go`
- Modify: `pkg/plugin/resources.go` (`routes`), `pkg/plugin/cache.go`, `pkg/plugin/cache_test.go`, `pkg/plugin/query_test.go`

Authorization: yes

**Interfaces:**
- Consumes: Task 1's `Triage`; Task 2's `triageEnabled`; `validIssueID`, `notAnIssueID`, `issueJSON`, `d.projectNames`, `userMessage`, `writeMessage`, `writeJSON`.
- Produces:

```go
const msgCallerRefused = "Only editors and admins can triage issues."
const msgTriageOff = "Triage is switched off for this data source."
// callerMayTriage reports whether the user's org role is Editor or Admin.
func callerMayTriage(u *backend.User) bool
func (d *Datasource) handleTriage(action bugsink.TriageAction) http.HandlerFunc
func (c *cache[V]) clear()
```

- In `routes`, a `post(pattern, h)` helper beside `get`: `POST /issues/{id}/<action>` per action, and the bare pattern answering 405 with `Allow: POST`.
- `handleTriage` sets `Cache-Control: no-store`, then runs decision 4's checks in order, calls `d.client.Triage`, clears `d.issueCache` when Bugsink answered (success or `*bugsink.APIError`), logs decision 7's line with `backend.Logger.FromContext(ctx).Info("Triage", "action", …, "issue", …, "user", login, "status", …)`, and answers per the contract.

- [ ] **Step 1: Write failing tests** in `triage_test.go`, with a helper `callAs(t, d, user *backend.User, method, path, body string) resourceReply` that sets `req.PluginContext.User`, and a Bugsink test server that records every request:
  - `TestTriageRoutes`: users `{Role: "Editor"}` and `{Role: "Admin"}`, each action: 200, `status` taken from Bugsink's answer, `project` the project name, Bugsink saw `POST /api/canonical/0/issues/<id>/<action>/`.
  - `TestTriageRefusesCaller`: users nil, `{}`, `{Role: "None"}`, `{Role: "Viewer"}`, `{Role: "editor"}`, `{Login: "admin", Role: "Viewer"}` × four actions: 403 `Only editors and admins can triage issues.`, zero Bugsink requests; the same with triage off and with invalid settings.
  - `TestTriageMethods`: GET, PUT, PATCH, DELETE, HEAD on each route as Admin: 405, `Allow: POST`, zero Bugsink requests.
  - `TestTriageOff` (Editor, switch off): 403 `Triage is switched off for this data source.`; `TestTriageSettingsInvalid`: 400 with the settings message. Zero requests in both.
  - `TestTriageRejectsIDs`: `issues/../resolve`, `issues/a%2Fb/resolve`, `issues/x%20y/mute`, `issues/x%3Fy/unmute`, `issues//reopen`, 101 × `a`: 400, zero requests.
  - `TestTriageSendsOnlyTheAction`: body `{"period_name":"day","nr_of_periods":2}` and `?gte_threshold=5`: Bugsink's request has an empty body and no query.
  - `TestTriageBugsinkErrors`: 400 `Issue is already resolved.` gives 502 `Bugsink refused the request: Issue is already resolved.`; a 403 naming the capability 502 `The API token lacks issues:triage.`; 404 502 `Bugsink refused the request: No Issue matches the given query.`; a closed server 502 starting `Cannot reach Bugsink:`; no answer contains the token.
  - `TestTriageNoStore`: the 200, 403, 400 and 502 answers carry `Cache-Control: no-store`.
  - `TestTriageClearsIssueCache`: an issues query (one list request), a triage call answered 200, the same query: a second list request; again with Bugsink answering 400.
  - `cache_test.go` `TestCacheClear`: after `clear`, `get` misses every key.
  - `query_test.go` `TestQueryNeverWrites`: queries with `queryType` `resolve`, `triage`, `issues` and `events` against a server that records methods: the first two give `Unknown queryType "…".` with 400; no request is anything but GET.
- [ ] **Step 2: Run** `go test -race ./pkg/plugin/...`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test -race ./pkg/...`, `go vet ./pkg/...`. Expected: PASS.
- [ ] **Step 5: Commit** `Add the triage routes with the caller check` and push.

### Task 4: Triage in the data source client and the config editor

**Files:**
- Modify: `src/datasource/types.ts`, `src/datasource/datasource.ts`, `src/datasource/datasource.test.ts`, `src/datasource/components/ConfigEditor.tsx`, `src/datasource/components/ConfigEditor.test.tsx`, `src/datasource/components/IssuesQueryEditor.test.tsx`

**Interfaces:**
- Consumes: Task 3's routes and contract.
- Produces:

```ts
export type TriageAction = 'resolve' | 'reopen' | 'mute' | 'unmute';
export interface BugsinkOptions { /* existing */ triageEnabled?: boolean; hideStacktraceText?: boolean }
export function isIssueId(id: string): boolean // /^[A-Za-z0-9_-]{1,100}$/
// in BugsinkDataSource
readonly triageEnabled: boolean;
canTriage(): boolean; // triageEnabled and config.bootData.user.orgRole is 'Editor' or 'Admin'
triage(id: string, action: TriageAction): Promise<IssueDetail>; // postResource(`issues/${id}/${action}`, undefined, QUIET)
```

- [ ] **Step 1: Write failing tests.**
  - `datasource.test.ts`: `triage('c0d07f26-4cc5-478c-8ae4-7e0fda5bc26c', 'resolve')` posts `issues/c0d07f26-4cc5-478c-8ae4-7e0fda5bc26c/resolve` with no data and `showErrorAlert: false` and resolves to the answer; `triage('a/b', 'mute')`, `getIssue('..')`, `getIssueTags('')` and `getIssueHistory('x y')` reject with `"<id>" is not an issue id.` and make no request; `canTriage()` is true for triage on with `Editor` or `Admin`, false for `Viewer` or `''`, false with triage off.
  - `ConfigEditor.test.tsx`: both switches render off when the keys are absent; toggling `Triage` calls `onOptionsChange` with `jsonData.triageEnabled: true` and the other options unchanged; the same for `Hide stack trace text`; descriptions as in the contract.
  - `IssuesQueryEditor.test.tsx` (restored from M2): with `getProjects` rejecting `Bugsink rejected the API token.`, the text shows and choosing State `Muted` still calls `onChange` with `state: 'muted'`, and Enter in `Search` still calls `onChange` and `onRunQuery`.
- [ ] **Step 2: Run** `npm run test:ci`. Expected: FAIL.
- [ ] **Step 3: Implement.** The two `Field`s with `Switch` follow `Service tag`.
- [ ] **Step 4: Run** `npm run test:ci`, `npm run typecheck`, `npm run lint`. Expected: PASS.
- [ ] **Step 5: Commit** `Add triage to the data source client and the config editor` and push.

### Task 5: Inline Resolve and Mute on the issue list

**Files:**
- Create: `src/pages/format.ts`, `src/pages/format.test.ts`, `src/pages/TriageButtons.tsx`, `src/pages/TriageButtons.test.tsx`
- Modify: `src/pages/issues/IssueTable.tsx`, `src/pages/issues/IssueTable.test.tsx`, `src/pages/IssuesPage.tsx`, `src/pages/IssuesPage.test.tsx`; `src/pages/issue/IssueSummary.tsx`, `src/pages/issue/EventTable.tsx`, `src/pages/issue/IssueHistory.tsx`, `src/pages/event/EventDetailView.tsx` (use `format.ts`)

**Interfaces:**
- Consumes: Task 4's `TriageAction`, `canTriage`, `triage`.
- Produces:

```ts
export const STATUS_LABELS: Record<IssueStatus, string>; // Open, Muted, Resolved
export function formatTime(value: number | string, timeZone: TimeZone): string; // 'YYYY-MM-DD HH:mm:ss'
export const ACTION_LABELS: Record<TriageAction, string>; // Resolve, Reopen, Mute, Unmute
export function triageActions(status: IssueStatus): TriageAction[]; // decision 10
export function TriageButtons(props: { actions: TriageAction[]; busy: boolean; onAction: (action: TriageAction) => void; name?: (action: TriageAction) => string; size?: ComponentSize }): React.JSX.Element;
// IssueTable gains: triage?: { busyId?: string; onAction: (row: IssueRow, action: TriageAction) => void }
```

- `Breadcrumbs.tsx` keeps its millisecond format. `IssuesPage` passes `triage` only when `source.canTriage()`; a row's status from an answer is kept until the list loads again; the failure alert clears on the next action or list load.

- [ ] **Step 1: Write failing tests.**
  - `format.test.ts`: `formatTime(Date.UTC(2026, 10, 13, 10), 'utc')` and `formatTime('2026-11-13T10:00:00Z', 'utc')` give `2026-11-13 10:00:00`.
  - `TriageButtons.test.tsx`: `triageActions` gives `['resolve','mute']`, `['resolve','unmute']`, `['reopen']`; a click calls `onAction`; `busy` disables every button; `name` sets accessible names.
  - `IssueTable.test.tsx`: no `triage`, no `Actions` header; with it, an open row has `Resolve SAMPLE-PYTHON-2` and `Mute SAMPLE-PYTHON-2`, a muted row only `Resolve …`, a resolved row none; a click calls `onAction(row, 'resolve')`.
  - `IssuesPage.test.tsx`: `canTriage` true: clicking `Resolve SAMPLE-PYTHON-2` calls `triage(<row uuid>, 'resolve')`, the row's Status reads `Resolved` and `listIssues` is not called again; a rejection `Bugsink refused the request: Issue is already resolved.` shows `SAMPLE-PYTHON-2: Bugsink refused the request: Issue is already resolved.` and calls `listIssues` again; `canTriage` false shows no `Actions` column.
- [ ] **Step 2: Run** `npm run test:ci`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `npm run test:ci`, `npm run typecheck`, `npm run lint`. Expected: PASS.
- [ ] **Step 5: Commit** `Add inline Resolve and Mute to the issue list` and push.

### Task 6: Triage actions on the issue page

**Files:**
- Modify: `src/pages/IssuePage.tsx`, `src/pages/IssuePage.test.tsx`

**Interfaces:**
- Consumes: Task 4's `canTriage`, `triage`; Task 5's `TriageButtons`, `triageActions`.
- Produces: no new exports. `useSection` keeps a section's last value for the same data source and key while a newer refresh is pending, so Refresh keeps every section, the summary included, and the Event section stays mounted. The buttons sit directly under the summary when `canTriage()`. Success replaces the summary from the answer and reads History again; failure shows the message in an error `Alert` under the buttons and reads summary and History again.

- [ ] **Step 1: Write failing tests** in `IssuePage.test.tsx`:
  - `triage buttons follow the state`: open summary shows `Resolve` and `Mute`; muted `Resolve` and `Unmute`; resolved `Reopen`; `canTriage` false none.
  - `an action shows the issue Bugsink answers`: address `issues/SAMPLE-PYTHON-2`, resolved summary, click `Reopen`: `triage` gets the summary's uuid and `'reopen'`; Status reads `Open`; `getIssue` was called once, `getIssueHistory` twice; buttons are disabled while pending.
  - `a failed action reads the issue again`: rejection `Bugsink refused the request: Issue is not resolved.` shows that text; `getIssue` and `getIssueHistory` are called again.
  - `refresh keeps the page while it reads`: with every second read pending, after Refresh the summary heading, the Event section and the Events table are still shown.
  - `links to the issue in Bugsink in a new tab` gains `rel="noreferrer"`.
  - `goes back to the list without a query string`: address `issues/SAMPLE-PYTHON-2` with no search; Back navigates to `/a/ahmedismail-bugsink-app`.
- [ ] **Step 2: Run** `npm run test:ci`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `npm run test:ci`, `npm run typecheck`, `npm run lint`. Expected: PASS.
- [ ] **Step 5: Commit** `Add triage actions to the issue page` and push.

### Task 7: Triage data source, users and end-to-end tests of the demo

**Files:**
- Create: `tests/users.setup.ts`, `tests/triage.spec.ts`
- Modify: `docker/grafana/provisioning/datasources/datasources.yaml`, `tests/seeded.ts`, `tests/fixtures.ts`, `tests/datasource-config.spec.ts`, `tests/trace-links.spec.ts`, `docker/README.md`

**Interfaces:**
- Consumes: everything above, built into `dist/`.
- Produces: data source `Bugsink triage` (uid `bugsink-triage`, URL `http://bugsink:8000`, `jsonData: {browserUrl: http://localhost:8800, triageEnabled: true}`, token `b`×40 with the seeded-token comment); `seeded.ts` exports `TRIAGE` (`'b'.repeat(40)`), `EDITOR = { user: 'm7-editor', password: 'm7-editor-e2e' }`, `VIEWER = { user: 'm7-viewer', password: 'm7-viewer-e2e' }`, `BUGSINK_API` (`http://localhost:8800/api/canonical/0`); `fixtures.ts` exports `basicAuth(user: { user: string; password: string }): Record<string, string>`.

- [ ] **Step 1: First contact with the running Grafana.** `docker compose up -d --build --wait`, add the data source, `npm run build && npm run build:backend`, `docker compose restart grafana`, then with `Z=00000000-0000-0000-0000-000000000000` and `R=http://localhost:3300/api/datasources/uid`:
  - `GET $R/bugsink-triage/health` is OK and ends ` The token can triage issues.`
  - `POST $R/bugsink-triage/resources/issues/$Z/resolve` anonymously (Admin) and as an Editor (basic auth; make the users with Step 2's setup or by hand): 502 `Bugsink refused the request: No Issue matches the given query.`; as a Viewer, and with a Viewer service account token (delete it afterwards): 403 `Only editors and admins can triage issues.`
  - `GET` and `PUT` on the same path: 405. `POST $R/bugsink/resources/issues/$Z/resolve`: 403 `Triage is switched off for this data source.`
  - `POST /api/datasources/proxy/uid/bugsink-triage/api/canonical/0/issues/$Z/resolve/`: 400 `Authentication to data source failed` (no token on the proxy).
  - The `Bugsink trace` dashboard's events table shows no `logs_query` column and `Open logs` still carries the rendered query.
  If any differs, stop and report.
- [ ] **Step 2: Write the setup** `tests/users.setup.ts` (`setup.describe.configure({ retries: 0 })`): with `basicAuth({user: 'admin', password: 'admin'})`, for `EDITOR` and `VIEWER`: `POST /api/admin/users`, on 412 look the id up, then `PUT /api/admin/users/<id>/password` and `PATCH /api/orgs/1/users/<id>` with the role. No Bugsink event.
- [ ] **Step 3: Write the tests** (read `.config/AGENTS/e2e-testing.md` first). `tests/triage.spec.ts`, `test.describe.configure({ mode: 'serial' })`; a helper finds the `m5-message` issue through `GET ${BUGSINK_API}/issues/?project=1&state=all&q=m5-message&sort=last_seen&order=desc` with `GLOBAL_READ` (with `toPass()`), and `beforeAll`/`afterAll` reopen or unmute it with `TRIAGE` (decision 17). Each describe sets `test.use({ extraHTTPHeaders: basicAuth(…) })` and each test first asserts `GET /api/user` returns that login.
  - Editor, `the demo: resolve from the list, reopen from the issue page`: list `ds=bugsink-triage&project=1&state=open&q=m5-message&from=now-24h&to=now`; click `Resolve SAMPLE-PYTHON-<n>`; the row's Status reads `Resolved`; open the issue; Status `Resolved`, click `Reopen`; Status `Open`; Bugsink says open.
  - Editor, `mute and unmute on the issue page`: Mute gives `Muted` with `Resolve` and `Unmute`; Unmute gives `Open`.
  - Viewer, `no controls and a refusal`: the list on `bugsink-triage` has no `Actions` column, the issue page no triage button; `request.post` to `resources/issues/<uuid>/resolve` is 403 with `Only editors and admins can triage issues.`; Bugsink still says open.
  - Editor, `triage switched off`: on `ds=bugsink` no `Actions` column and no buttons; the direct POST is 403 `Triage is switched off for this data source.`
  - `datasource-config.spec.ts`: `triage on, token without issues:triage` (`GLOBAL_READ`, switch `Triage` on, save and test): error `The API token lacks: issues:triage.`; `triage on with the triage token`: success containing `The token can triage issues.`
  - `trace-links.spec.ts` (dashboard row test): the events panel has no `logs_query` column header.
- [ ] **Step 4: Run** `npm run build && npm run build:backend && docker compose restart grafana && npm run e2e`. Expected: every test passes twice in a row; the run reports exactly decision 17's five events; `python3 scripts/verify_stack.py` prints eleven `ok` lines.
- [ ] **Step 5: Run** `npm run lint`, `npm run typecheck`, `scripts/check_spdx.sh`. Expected: PASS.
- [ ] **Step 6:** In `docker/README.md`, describe `Bugsink triage`, the e2e users and their public development-only passwords.
- [ ] **Step 7: Commit** `Provision a triage data source and test triage end to end` and push.

### Task 8: Documentation and changelog

**Files:**
- Modify: `src/README.md`, `CHANGELOG.md`, `design/bugsink_grafana_plugin_spec.md:139`, `design/implementation_roadmap.md:228`

- [ ] **Step 1: Write** in `src/README.md` a section "Triage": the `Triage` switch and the `issues:triage` capability; who may triage (org role Editor or Admin, including anonymous users and service accounts with that role); which actions show for which state, Resolve and Mute on the list; that every action runs as the token's Bugsink user, so Bugsink's history does not name the Grafana user; the refusals of the contract; the health check line. In the data source setup: the `Hide stack trace text` switch beside the local-variables warning (line 31). Replace the `logs_query` sentence (line 198) with: the Logs query is rendered per row in a hidden `logs_query` field, which the Loki link reads. In `CHANGELOG.md` under 0.1.0: triage from the issue list and the issue page, the triage health check, the stack trace text switch, the hidden `logs_query` field.
- [ ] **Step 2: Wording:** spec §4.F `; In Explore` becomes `; in Explore`; roadmap `All nine decisions are made on 2026-11-15` becomes `All nine decisions were all made on 2026-11-15`.
- [ ] **Step 3: Run** `scripts/check_spdx.sh`, `scripts/check_licenses.sh`, `npm run lint`. Expected: PASS.
- [ ] **Step 4: Commit** `Document triage` and push.

## Not in this milestone

Triage from dashboard tables or Explore (spec §4.E puts it in the App); resolve in a release, timed and threshold mutes, attribution comments and RBAC plugin actions (M8); a configurable minimum role (superseded by M8's RBAC actions); several issues at once; showing a new state before Bugsink answers.

## After the last task

* Whole-branch review against this plan and spec §4.E, then one fix wave.
* From the worktree: `npm ci`, `npm run build`, `npm run build:backend`, `go test -race ./pkg/...`, `npm run test:ci`, `npm run e2e`, `python3 scripts/verify_stack.py`.
* Update spec §4.A (the two switches; the health check probes `issues:triage` on an absent issue, replacing "not verified"), §4.C (the stack trace text switch; `logs_query` hidden through `hideFrom`), §4.D (the Actions column; actions on the issue page; Refresh keeps the page), §4.E (who may triage, the refusals, the transitions, the state from the answer) and §10, and the roadmap's M7 entry, to what was built; add `## As built` here.
* The merge into `main` happens only when the owner asks for it.

## As built

1. Every triage route also requires the header `X-Bugsink-Triage: 1`, checked after the switch and before the id, and answers 403 `Triage calls need the header X-Bugsink-Triage: 1.` without a Bugsink request; the app sends the header, and Grafana 13.3.1 forwards it to the plugin.
2. The Bugsink client never follows a redirect, for reads or writes: a redirect is reported as `Bugsink answered HTTP <status>.`, or in the health check as an answer that is not Bugsink's, and never with its `Location`.
3. Anonymous sessions and service accounts triage by their org role, and the config editor, the README and the docker README warn that anonymous Editor or Admin access lets anyone who can open Grafana triage, while the header only stops other web sites.
4. The roles that may triage stay fixed to exactly Editor and Admin until M8's RBAC plugin actions.
5. The `Triage` and `Hide stack trace text` switches are off by default, and a value that is not a boolean makes the settings invalid.
6. The health check probes `issues:triage` with a resolve on the all-zeros issue id, which Bugsink 2.6.3 refuses for the capability before it looks up the issue; a 200 is reported as an unexpected answer, and spec §4.A and §10 now say so.
7. Hidden stack trace text also drops its 100 KB cut notice, so its length does not show.
8. The issue list cache is emptied after every triage call that was sent, not only after an answer.
9. A list read that started before a triage call is not stored in the cache: the cache keeps a generation that clearing increments, and a read stores only if the generation is unchanged.
10. The triage log line is also written when Bugsink gave no answer, with status 0.
11. A dot or empty path segment on a triage route answers 400 `"<segment>" is not an issue id.` from the plugin's path check before the method and caller checks, reading no settings and sending nothing.
12. The logs query check treats a line break as the end of a double-quoted string and refuses a variable in such a string.
13. The logs query check treats `//` to the end of the line and `/* */` (across lines; an unclosed one runs to the end) like `#` outside strings, refuses a substituted variable inside any comment, and treats a lone `/` as division.
14. Issue reads in the browser validate the id with the backend's pattern and no longer URL-encode it.
15. The config editor shows the `Triage` switch under a `Triage` heading and `Hide stack trace text` under a `Privacy` heading.
16. On the issue list, every issue with an action in flight keeps its buttons disabled, in any list, until its own answer arrives.
17. On the issue list, an answer or a failure that arrives after the list changed is dropped, using a scope made by each list load.
18. The failure alert on the issue list stays through the reload it triggers.
19. On the issue page, an action's answer replaces the summary at the latest version, and a summary read that started before the answer is dropped, so a Refresh during an action neither brings back the old state nor keeps loading.
20. Refresh on the issue page keeps every section's content while the new reads run.
21. The end-to-end setup creates the two users and asserts that each signs in with its password, instead of resetting a password that Grafana refuses to change while the login form is disabled.
22. The triage end-to-end test waits until the issue's newest event is the one this run reported, adds a POST without the header, and leaves the issue open and unmuted.
23. Bugsink's history of an action does not name the Grafana user, because every action runs as the token's Bugsink user.
24. Left for later milestones: M8, disable the actions while the list reloads, test that the failure alert clears on the next action or load, show a failure that arrives after a new search, key the issue page's failure by data source and id, log the role with the login, and add a README note on Grafana's query caching; M11, read the provisioned data source uids instead of hard-coding them in the end-to-end tests.
