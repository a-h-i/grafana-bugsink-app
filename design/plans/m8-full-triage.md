# M8: Full Triage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** An Editor or Admin resolves an issue in the latest or the next release and mutes it for a period or until a volume threshold, from the issue page, and Bugsink records a comment naming the Grafana user for every action.

**Architecture:** `pkg/bugsink.Triage` takes four more actions, two of them with validated mute terms, and the client gains `AddComment`. The data source serves four more POST routes with the M7 check order plus a body check, writes the attribution comment after a successful action (switch `attributionComments`, on by default), and reports a failed comment as a warning on the 200 answer. The health check probes `issues:comment` without a side effect. The issue page gets the new buttons and two small forms; history shows what Bugsink's `metadata` says.

**Tech Stack:** Go 1.26 with `grafana-plugin-sdk-go` v0.299.0, React with `@grafana/ui` and `@grafana/runtime` 13.2.4, Jest, `@grafana/plugin-e2e` 4.0.0 on Playwright 1.65.1, the Compose stack (Bugsink 2.6.3, Grafana 13.3.1 OSS).

**Spec:** [`../bugsink_grafana_plugin_spec.md`](../bugsink_grafana_plugin_spec.md) (§4.A, §4.D, §4.E, §10). **Roadmap:** [`../implementation_roadmap.md`](../implementation_roadmap.md) (M8, Definition of done). **Findings:** [`../phase0_findings.md`](../phase0_findings.md) (Q4, Q10). **Previous:** [`m7-basic-triage.md`](m7-basic-triage.md) (As built).

## Global Constraints

* The API token never reaches the browser, a log line, an error message, a frame or a resource answer.
* The backend pages with the `cursor` from `next` only; it never builds a cursor, requests a URL taken from a response or follows a redirect.
* Generated code stays in `pkg/bugsink/internal/gen/`, imported only by `pkg/bugsink`, not regenerated. Nothing in `.config/` changes. No plugin id, type or `plugin.json` change (so no restart for it); provisioning or backend changes need `docker compose restart grafana`.
* SPDX header `GPL-3.0-or-later` on every hand-written source file; `scripts/check_spdx.sh` passes. Comments are one line and describe the code; tests assert behaviour, never code structure.
* Go commands run on `./pkg/...`. Host ports stay 3300 (Grafana), 8800 (Bugsink), 8810 and 8820 (sample apps).
* Resources never answer 401: Bugsink failures 502, invalid settings or input 400, a refused caller 403. Frontend resource calls pass `showErrorAlert: false` and show the message inline. Untrusted Bugsink text is plain text; lists from a payload are bounded.
* Every write route keeps the M7 check order (path segments 400, method 405, caller 403, settings 400, switch 403, header 403, id 400), reads nothing from the request it does not validate, answers `Cache-Control: no-store`, clears the issue cache through its generation, and writes one log line without the token.
* Reuse `userMessage`, `validIssueID`, `issueJSON`, `d.projectNames`, `writeMessage`, `writeJSON`, `callAs`, the instance caches and the e2e helpers (`tests/fixtures.ts`, `tests/reported.ts`, `tests/seeded.ts`); no parallel copies.
* E2E: at most 5 Bugsink events per run, reported only from `tests/*.setup.ts`; this milestone adds none (decision 13). Digest-dependent assertions retry with `toPass()`.
* Branch `m8/full-triage` in `.claude/worktrees/m8-full-triage` (from `main` at e223c2e). One implementer at a time; push after every commit; never amend. Run every test command in the foreground. Before the first stack check (Task 7 only) run `docker compose up -d --build --wait` from the worktree; never `docker compose down`.

## Facts already verified (2026-11-15)

* All with the triage token on the all-zeros issue `Z`: `resolve-latest/`, `resolve-next/`, `mute-for/`, `mute-until/` answer 404 `{"detail":"No Issue matches the given query."}`; read, project-bound and no-capability tokens 403 naming `issues:triage`. `POST issue-comments/` with `{"issue":Z,"comment":""}` is 400 `{"issue":["Issue not found."],"comment":["This field may not be blank."]}` with the triage token (it holds `issues:comment`) and 403 `{"detail":"This token does not have the required capability: issues:comment."}` with the other three, also with no body; no token 401. `GET` on both paths is 405.
* Bodies on the throwaway issue `SAMPLE-PYTHON-8` (found open, unmuted, `fixed_at: []`, one history entry; put back by hand, see last line): `mute-for {}` 400 `{"period_name":["This field is required."],"nr_of_periods":["This field is required."]}`; `"fortnight"` or `"Day"` 400 `"…" is not a valid choice.`; `nr_of_periods` 0 or -1 400 `Ensure this value is greater than or equal to 1.`; 1.5 400 `A valid integer is required.`; `"2"` (a string) and an unknown key are accepted; `mute-until` without `gte_threshold` 400 `This field is required.`, 0 400 `Ensure this value is greater than or equal to 1.`. Year 10000 or 99999999999 minutes: 500 with a text body (`year 12026 is out of range`), issue unchanged. `mute-until` with 10000 years is accepted. Bugsink validates the body before the state.
* Transitions (each 200 is the whole issue): open → `mute-for` muted with `unmute_after` now + period; open → `mute-until` muted with `unmute_on_volume_based_conditions: [{"period":"hour","nr_of_periods":1,"volume":5}]`; muted → either mute 400 `Issue is already muted.`; open or muted → `resolve-latest` resolved, `fixed_at: ["m5-probe@0"]` (the project's latest release), unmuted; open or muted → `resolve-next` resolved, `is_resolved_by_next_release: true`, unmuted; resolved → every resolve and mute 400 `Issue is already resolved.`; `reopen` does not clear `fixed_at`. Bugsink 2.6.3 source (`issues/api_views.py:309-315`): `resolve-latest` on a project without releases is 400 `Project has no releases.` (not observed).
* History `metadata`: `{"mute_for":{"period_name":"day","nr_of_periods":2,"unmute_after":"…+00:00"}}`, `{"mute_until":{"period_name":"hour","nr_of_periods":1,"gte_threshold":"5"}}` (threshold as a string), `{"resolved_release":"m5-probe@0"}`, `{"resolve_by_next":true}`, `{"resolved_unconditionally":true}`, `{"mute_unconditionally":true}`, `{}` for reopen and manual unmute. Automatic unmutes (`mute_for {unmute_after}`, `mute_until {period, nr_of_periods, volume}`) and `next_materialized {actual_release}` come from the schema text only.
* A comment: 201 `{"id":373,"issue":"<uuid>","project":1,"timestamp":…,"comment":"…","user":null}`; read-only keys sent (`user`, `project`) and unknown keys are ignored; the short id works for `issue`; a blank or whitespace comment 400 `This field may not be blank.`. It shows in `issue-history` as `kind: "manual_annotation"`, `user_display_name: null`, `metadata: {}`. Bugsink renders comments as escaped text with `linebreaksbr` (`issues/templates/issues/history.html:94`), not Markdown. The API has no comment delete; I removed the probe's history entries and `fixed_at` with `bugsink-manage shell`.
* Grafana 13.3.1 passes an anonymous caller with an empty login: an anonymous resolve on `Z` through `bugsink-triage` logged `user= uname=` (502 for the 404). Service accounts carry the login `sa-<org>-<name>` (M7).
* RBAC: SDK v0.299.0 `backend.User` is only `{Login, Name, Email, Role}` and the SDK has no permission API, only `GrafanaCfg.PluginAppClientSecret()` (env `GF_PLUGIN_APP_CLIENT_SECRET`). The plugin-tools guide's backend check needs `github.com/grafana/authlib/authz` (not a dependency), a managed service account from an `iam` block, the `externalServiceAccounts` feature toggle (single org only) and the `X-Grafana-Id` token (SDK: "Requires idForwarded feature toggle"). The stack's enabled toggles (`/api/frontend/settings`) include neither; `GET /api/access-control/roles` is 404; Grafana's docs mark RBAC for app plugins "Available in Grafana Enterprise and Grafana Cloud".
* `@grafana/ui` 13.2.4 exports `Modal` (with `Modal.ButtonRow`), `Input`, `Field`, `RadioButtonGroup`. `@grafana/runtime` `postResource<T>(path, data?, options?)`.
* Not verified, checked first by Task 7: that Grafana forwards a resource POST body unchanged to the plugin; a token with `issues:triage` but not `issues:comment` (no seeded token has that; unit tests cover it).

## Decisions in this plan, for review

1. **RBAC plugin actions are not built.** A backend plugin on OSS cannot check an action for the calling user (evidence above); a frontend-only check would not be the authority. The M7 rule stays: org role exactly `Editor` or `Admin`.
2. **New routes:** `POST issues/{id}/resolve-latest`, `…/resolve-next`, `…/mute-for`, `…/mute-until`, beside the M7 four, with the same refusals; the two mute routes add a body check after the id check (body 400), then Bugsink.
3. **Body of the mute routes:** at most 1024 bytes; one JSON object and nothing after it; `mute-for` takes exactly `period` and `periods`, `mute-until` also `threshold`; unknown fields, a missing field, `null`, a string number, a fraction or a second value are refused. `period` is one of `minute`, `hour`, `day`, `week`, `month`, `year`; `periods` a whole number from 1 to 100 (100 years stays inside Bugsink's date range, 10000 years is a 500); `threshold` from 1 to 1000000. The query string is never read; the bodiless routes still never read the body.
4. **What reaches Bugsink:** only the validated id, the action from the route pattern, and for the mute routes `{"period_name", "nr_of_periods"}` (plus `"gte_threshold"`) built from the validated values; for the comment, the issue's uuid from Bugsink's answer and the text of decision 6.
5. **The comment names the Grafana login**, read in the backend from `backend.UserFromContext(ctx).Login`, never from the request: it is unique and always set for a signed-in user, while the name can be empty or shared and the email is personal data that every Bugsink user who can see the issue would read. Some sign-in methods use the email as the login; the README says so. An empty login (anonymous access) is written `an anonymous user`; a service account is written by its login (`sa-1-…`).
6. **Comment text:** `<what> from Grafana by <who>.`, with `<what>` one of `Resolved`, `Resolved in the latest release`, `Resolved in the next release`, `Reopened`, `Muted`, `Muted for <n> <unit>`, `Muted until <t> or more events in <n> <unit>`, `Unmuted` (`<unit>` singular for 1, else plural: `1 hour`, `2 days`). `<who>` is the login with every control, format, line and paragraph separator character replaced by the replacement character (U+FFFD), cut to 100 runes with `…` appended when cut. Hostile logins: `m7` newline `editor`, 150 × `x`, `<b>x</b>` (kept, Bugsink escapes it).
7. **Comment after the action:** sent only after Bugsink accepted the action. If it fails, the action stands (no rollback) and the answer is still 200 with the issue plus `warning: "The action succeeded, but the attribution comment was not written: <userMessage>"`, for example `… not written: The API token lacks issues:comment.`. The list shows it as a warning prefixed with the short id, the issue page under the buttons.
8. **Attribution switch** `jsonData.attributionComments`, absent or `null` means on, `false` off, any other type invalid settings. Off: no comment request, no comment probe.
9. **Health check**, with triage and attribution on, after the triage probe: `POST issue-comments/` with `{"issue":"<all zeros>","comment":""}`, which two independent refusals keep from writing; 400 means the token has `issues:comment` and the success message ends ` Actions are attributed by a comment.`; a 403 naming it keeps status OK and ends ` It lacks issues:comment, so actions are not attributed.` (attribution is optional); any other answer is `Bugsink answered HTTP <n> to a check for issues:comment.`
10. **Actions by state:** open offers Resolve, Resolve in latest release, Resolve in next release, Mute, Mute for a period, Mute until a threshold; muted offers the three resolves and Unmute; resolved offers Reopen. The list keeps only Resolve and Mute (spec §4.E). Resolve in latest release stays offered without knowing the project's releases; Bugsink's `Project has no releases.` explains a refusal.
11. **Forms:** a `Modal` per mute action with `Number of periods` (default 1), `Period` (radio, default Days) and, for the threshold, `Events` (default 10); the frontend runs decision 3's rules and messages live and disables `Mute` while one fails; the backend is the authority.
12. **History details:** the backend parses the documented `metadata` keys into a typed `details` object (`release` cut at 1024 characters, `nextRelease`, `period` only from the six names, `periods`, `threshold` from a number or a numeric string, `unmuteAfter`); a key of the wrong type is left empty; nothing else from `metadata` reaches the browser.
13. **E2E budget and issues:** no new event. The demo mutes the `m5-message` issue (as M7) and resolves the `m5-chained` issue (its only other reader, `event-detail.spec.ts`, lists with `state=all`). `beforeAll` and `afterAll` put both back to open and unmuted through Bugsink; `fixed_at` and the history entries stay, which nothing reads.

## Contracts

| Condition (new routes) | Status | Message |
| :--- | :--- | :--- |
| Body over 1024 bytes | 400 | `The request body is larger than 1024 bytes.` |
| Body not one JSON object of the route's fields | 400 | `The request body must be a JSON object with period and periods.` / `… with period, periods and threshold.` |
| `period` not one of the six | 400 | `The period must be minute, hour, day, week, month or year.` |
| `periods` out of range | 400 | `The number of periods must be a whole number from 1 to 100.` |
| `threshold` out of range | 400 | `The threshold must be a whole number from 1 to 1000000.` |
| Action done, comment failed | 200 | issue body plus `warning` (decision 7) |

* Every M7 refusal (405, 403 caller, 400 settings, 403 switch, 403 header, 400 id, 502) applies unchanged to the new routes.
* Log line: `Info("Triage", "action", …, "issue", …, "user", login, "role", role, "status", n, "comment", n)`, `comment` being Bugsink's status for the comment (0 without an answer), omitted when no comment was sent.
* Config editor: field `Attribution comments`, switch id `bugsink-attribution`, under the `Triage` heading, description `After each action, add a comment to the issue in Bugsink naming the Grafana login and the action. Everyone who can see the issue in Bugsink can read it. The API token needs issues:comment.`
* UI labels: `Resolve in latest release`, `Resolve in next release`, `Mute for a period`, `Mute until a threshold` (buttons and modal titles), `Mute` and `Cancel` in the modal; period radio labels `Minutes`, `Hours`, `Days`, `Weeks`, `Months`, `Years`.

## Review Focus

1. A hostile body on a mute route (string number, fraction, `null`, unknown field, trailing value, 1025 bytes, 101 periods): 400 and no Bugsink request (Task 3 `TestMuteRejectsBodies`).
2. The comment fails after the action (token without `issues:comment`, Bugsink down): the issue is shown in its new state with the warning, and the action is not retried or undone (Task 3 `TestAttributionFailureWarns`; Task 5 and Task 6 warning tests).
3. A login that tries to forge a second line or a long one, and an anonymous caller: the comment holds one line naming exactly that caller (Task 3 `TestAttributionText`).
4. A failure that arrives after the user searched again on the list: shown, not dropped, and cleared by the next action or load (Task 6).
5. A click while the list reloads: the buttons are disabled, so no answer lands on a list that is being replaced (Task 6).

## File structure

| File | Responsibility |
| :--- | :--- |
| `pkg/bugsink/triage.go`, `details.go`, `errors.go` | new actions and terms, `AddComment`, `IssuesComment`, history details |
| `pkg/plugin/datasource.go`, `health.go` | attribution switch, comment probe |
| `pkg/plugin/triage.go`, `resources.go` | new routes, body check, attribution, warning, history details |
| `src/datasource/types.ts`, `datasource.ts`, `components/ConfigEditor.tsx` | types, `muteTermsError`, `triage` with terms, switch |
| `src/pages/MuteForm.tsx` (new), `TriageButtons.tsx`, `format.ts`, `IssuePage.tsx`, `issue/IssueHistory.tsx` | issue page actions, forms, history |
| `src/pages/IssuesPage.tsx`, `issues/IssueTable.tsx` | list warnings and the M7 leftovers |
| `tests/triage.spec.ts`, `tests/datasource-config.spec.ts` | demo e2e |

---

### Task 1: New actions, comments and history details in the Bugsink client

**Files:**
- Modify: `pkg/bugsink/triage.go`, `pkg/bugsink/triage_test.go`, `pkg/bugsink/errors.go`, `pkg/bugsink/details.go`, `pkg/bugsink/details_test.go`; for the new `Triage` signature only: `pkg/plugin/triage.go`, `pkg/plugin/health.go` (`prober`), `pkg/plugin/health_test.go` (`fakeProber`)

Authorization: yes

**Interfaces:**
- Consumes: `gen.Client.IssuesResolveLatestCreate`, `IssuesResolveNextCreate` (`(ctx, id, …)`), `IssuesMuteForCreate(ctx, id, gen.IssueMuteFor)`, `IssuesMuteUntilCreate(ctx, id, gen.IssueMuteUntil)`, `IssueCommentsCreate(ctx, gen.IssueComment)`, `gen.PeriodNameEnum`, `gen.TurningPoint.Metadata`, `decodeJSON`, `issueOf`.
- Produces:

```go
const IssuesComment Capability = "issues:comment"
// TriageAction gains ResolveLatest "resolve-latest", ResolveNext "resolve-next", MuteFor "mute-for", MuteUntil "mute-until".
type Period string // PeriodMinute "minute", PeriodHour "hour", PeriodDay "day", PeriodWeek "week", PeriodMonth "month", PeriodYear "year"
type MuteTerms struct { Period Period; Periods, Threshold int } // Threshold is sent by MuteUntil only
// Triage runs action on the issue; MuteFor and MuteUntil send terms, the others send no body.
func (c *Client) Triage(ctx context.Context, issueID string, action TriageAction, terms MuteTerms) (Issue, error)
// AddComment adds a comment to the issue.
func (c *Client) AddComment(ctx context.Context, issueID, text string) error
type TurningPointDetails struct { Release string; NextRelease bool; Period Period; Periods, Threshold int; UnmuteAfter *time.Time }
// TurningPoint gains Details TurningPointDetails.
```

- [ ] **Step 1: Write failing tests** against an `httptest.Server`:
  - `TestTriageSendsBodilessPost` gains `resolve-latest` and `resolve-next`: empty body, no query.
  - `TestTriageSendsMuteTerms`: `MuteFor` with `{PeriodDay, 2, 9}` sends exactly `{"period_name":"day","nr_of_periods":2}` to `issues/<id>/mute-for/`; `MuteUntil` with `{PeriodHour, 1, 10}` sends `{"period_name":"hour","nr_of_periods":1,"gte_threshold":10}`; both `Content-Type: application/json`.
  - `TestTriageUnknownPeriod`: `MuteFor` with `Period("fortnight")` errors without a request; `TestTriageUnknownAction` stays.
  - `TestAddComment`: sends `{"issue":"<uuid>","comment":"Resolved from Grafana by a."}` to `issue-comments/`; a 201 is nil; a 403 naming the capability gives `APIError{Capability: IssuesComment}`; 400 field errors become `Detail`.
  - `details_test.go` `TestHistoryDetails`: the seven verified `metadata` objects give `Release "m5-probe@0"`, `NextRelease true`, `{PeriodDay, 2, UnmuteAfter set}`, `{PeriodHour, 1, Threshold 5}` from `"5"`, and the automatic-unmute shape `{"mute_until":{"period":"hour","nr_of_periods":1,"volume":7}}` gives Threshold 7; `{"resolved_release": 5}`, `{"mute_for":"x"}`, `{"mute_until":{"period_name":"fortnight"}}` and `null` give zero values; a 2000-character release is cut to 1024.
- [ ] **Step 2: Run** `go test ./pkg/bugsink/...`. Expected: FAIL.
- [ ] **Step 3: Implement.** `prober.Triage` takes the new signature and callers in `pkg/plugin` pass `bugsink.MuteTerms{}` (no behaviour change).
- [ ] **Step 4: Run** `go test -race ./pkg/...`, `go vet ./pkg/...`. Expected: PASS.
- [ ] **Step 5: Commit** `Add release resolves, timed mutes and comments to the Bugsink client` and push.

### Task 2: The attribution switch and the comment health check

**Files:**
- Modify: `pkg/plugin/datasource.go`, `pkg/plugin/datasource_test.go`, `pkg/plugin/health.go`, `pkg/plugin/health_test.go`

Authorization: yes

**Interfaces:**
- Consumes: Task 1's `AddComment`, `IssuesComment`.
- Produces: `Datasource.attribution bool` (from `jsonData.attributionComments`, decision 8); `prober` gains `AddComment(ctx context.Context, issueID, text string) error`; `func checkHealth(ctx context.Context, url string, timeout time.Duration, p prober, triage, attribution bool) *backend.CheckHealthResult`; `probe` gains `optional bool` (a missing optional capability does not join the lacking list).

- [ ] **Step 1: Write failing tests.**
  - `datasource_test.go`: absent, `null` and `true` give `attribution` true; `false` false; `"no"` gives `The data source settings are invalid.`
  - `health_test.go`: triage off or attribution off never calls `AddComment`; both on call it once with `absentIssueID` and `""`; 400 gives OK ending ` The token can triage issues. Actions are attributed by a comment.`; a 403 naming `issues:comment` gives OK ending ` The token can triage issues. It lacks issues:comment, so actions are not attributed.`; with `issues:triage` also missing, the error is `The API token lacks: issues:triage.`; 201 gives `Bugsink answered HTTP 201 to a check for issues:comment.`; 500 `Bugsink answered HTTP 500 to a check for issues:comment.`
- [ ] **Step 2: Run** `go test -race ./pkg/plugin/...`. Expected: FAIL.
- [ ] **Step 3: Implement.** The comment probe follows the triage probe and is added only when both switches are on; a success is reported as `APIError{Status: 201}`.
- [ ] **Step 4: Run** `go test -race ./pkg/...`, `go vet ./pkg/...`. Expected: PASS.
- [ ] **Step 5: Commit** `Add the attribution switch and the issues:comment health check` and push.

### Task 3: The new triage routes, the body check and the attribution comment

**Files:**
- Modify: `pkg/plugin/triage.go`, `pkg/plugin/triage_test.go`, `pkg/plugin/resources.go`, `pkg/plugin/resources_test.go`

Authorization: yes

**Interfaces:**
- Consumes: Tasks 1 and 2; `callAs`, `callWithHeaders`, `issueJSON`, `userMessage`.
- Produces:

```go
const maxTriageBody, maxPeriods, maxThreshold, maxLoginRunes = 1024, 100, 1000000, 100
// readMuteTerms decodes and validates the body of a mute route, or returns the refusal message.
func readMuteTerms(r *http.Request, action bugsink.TriageAction) (bugsink.MuteTerms, string)
// attributionText is the comment for action by login (decision 6).
func attributionText(action bugsink.TriageAction, terms bugsink.MuteTerms, login string) string
type triageBody struct { issueBody; Warning string `json:"warning,omitempty"` }
// historyEntry gains Details historyDetails `json:"details"`: release, nextRelease, period, periods, threshold, unmuteAfter, each omitempty.
```

- `routes` registers all eight actions. `handleTriage` reads the body only for `MuteFor` and `MuteUntil`, after the id; after a 2xx from Bugsink and with `d.attribution`, calls `AddComment(ctx, issue.ID, attributionText(…))`; logs once with `role` and `comment` (contract).

- [ ] **Step 1: Write failing tests** in `triage_test.go` with the recording Bugsink server:
  - `TestTriageRoutes` gains the four actions (mute ones with a valid body): 200, Bugsink saw the right path and body.
  - `TestTriageRefusesCaller`, `TestTriageMethods`, `TestTriageOff`, `TestTriageSettingsInvalid`, `TestTriageNeedsHeader`, `TestTriageRejectsIDs`, `TestTriageNoStore`, `TestTriageClearsIssueCache` run over all eight routes, with a valid body on the mute routes; zero Bugsink requests on every refusal; a refused caller with a hostile body still gets 403, and an invalid id with a hostile body 400 for the id.
  - `TestMuteRejectsBodies` (Editor, both mute routes): empty, `[1]`, `"x"`, `{"period":"day","periods":2}{"x":1}`, `{"period":"day","periods":2,"threshold":5}` on mute-for, `{"period":"day","periods":2,"__proto__":{}}`, `{"period":"day","periods":"2"}`, `{"period":"day","periods":2.5}`, `{"period":"day","periods":null}`, `{"period":"day"}`: the shape message; `{"period":"Day","periods":2}`, `{"period":"fortnight","periods":2}`: the period message; periods 0, -1, 101: the periods message; mute-until threshold 0, 1000001, `"5"` (shape): their messages; 1025 bytes of a valid object padded with spaces: the size message. Zero Bugsink requests each.
  - `TestMuteSendsOnlyTerms`: body `{"period":"week","periods":100}` and query `?nr_of_periods=0&gte_threshold=0`: Bugsink got exactly `{"period_name":"week","nr_of_periods":100}`; mute-until with threshold 1000000 sends it. `TestTriageSendsOnlyTheAction` stays for the six bodiless routes.
  - `TestAttributionComment`: Editor `m7-editor`, `mute-for` `{day, 2}`: Bugsink got the action, then `issue-comments/` with the answer's uuid and `Muted for 2 days from Grafana by m7-editor.`; switch off: no comment request; Bugsink refuses the action (400): no comment request.
  - `TestAttributionText`: each action's text, `1 hour`, `Muted until 10 or more events in 1 hour from Grafana by a.`; empty login gives `by an anonymous user.`; decision 6's hostile logins give one line, 100 runes plus `…`, and `<b>x</b>` unchanged.
  - `TestAttributionFailureWarns`: comment answered 403 naming the capability: 200, issue status from the action, `warning` `The action succeeded, but the attribution comment was not written: The API token lacks issues:comment.`; comment server closed: warning starting `… not written: Cannot reach Bugsink:`; no warning on success; the token is in no answer.
  - Logging, with the existing `captureLogs` and `triageLines`: `TestTriageRoutes`' exact line gains `role` (`Editor`, `Admin`) and, with attribution on, `comment: 201`; attribution off or a refused action has no `comment` key; `TestAttributionFailureWarns` logs `comment: 403` and `comment: 0`; `assertNoToken` holds in each.
  - `resources_test.go` `TestHistoryDetails`: the history route answers `details` from Task 1's parsed values (`{"period":"day","periods":2,"unmuteAfter":…}`) and `{}` for an entry without.
- [ ] **Step 2: Run** `go test -race ./pkg/plugin/...`. Expected: FAIL.
- [ ] **Step 3: Implement** with `http.MaxBytesReader` (or a length check of the buffered body), `json.Decoder.DisallowUnknownFields`, and a second `Decode` that must give `io.EOF`.
- [ ] **Step 4: Run** `go test -race ./pkg/...`, `go vet ./pkg/...`. Expected: PASS.
- [ ] **Step 5: Commit** `Add release resolves, timed mutes and attribution comments to the triage routes` and push.

### Task 4: The data source client and the config editor

**Files:**
- Modify: `src/datasource/types.ts`, `src/datasource/datasource.ts`, `src/datasource/datasource.test.ts`, `src/datasource/components/ConfigEditor.tsx`, `src/datasource/components/ConfigEditor.test.tsx`

Authorization: yes

**Interfaces:**
- Consumes: Task 3's routes and contract.
- Produces:

```ts
export type TriageAction = 'resolve' | 'reopen' | 'mute' | 'unmute' | 'resolve-latest' | 'resolve-next' | 'mute-for' | 'mute-until';
export type MuteAction = 'mute-for' | 'mute-until';
export type Period = 'minute' | 'hour' | 'day' | 'week' | 'month' | 'year';
export interface MuteTerms { period: Period; periods: number; threshold?: number }
export interface TriageResult extends IssueDetail { warning?: string }
export interface HistoryDetails { release?: string; nextRelease?: boolean; period?: Period; periods?: number; threshold?: number; unmuteAfter?: string }
// HistoryEntry gains details: HistoryDetails; BugsinkOptions gains attributionComments?: boolean
export const PERIODS: Period[];
export function muteTermsError(action: MuteAction, terms: MuteTerms): string | undefined; // decision 3's messages
// BugsinkDataSource.triage(id: string, action: TriageAction, terms?: MuteTerms): Promise<TriageResult>
```

- [ ] **Step 1: Write failing tests.**
  - `datasource.test.ts`: `muteTermsError` gives the period, periods and threshold messages for `fortnight`, 0, 101, 2.5, `NaN`, threshold 0 and 1000001, and `undefined` for `{day, 2}` and `{hour, 1, 10}`; `triage(uuid, 'mute-for', {period: 'day', periods: 2})` posts `issues/<uuid>/mute-for` with exactly `{period: 'day', periods: 2}`, the triage header and `showErrorAlert: false`; `mute-until` sends `threshold`; `resolve-next` sends no data; `mute-for` without terms or with `periods: 0` rejects with the message and no request; the answer's `warning` is kept.
  - `ConfigEditor.test.tsx`: `Attribution comments` renders on when the key is absent; toggling calls `onOptionsChange` with `attributionComments: false` and the rest unchanged; the description is the contract's.
- [ ] **Step 2: Run** `npm run test:ci`. Expected: FAIL.
- [ ] **Step 3: Implement.** The switch sits under `Triage`, after the `Triage` switch.
- [ ] **Step 4: Run** `npm run test:ci`, `npm run typecheck`, `npm run lint`. Expected: PASS.
- [ ] **Step 5: Commit** `Add the new triage actions to the data source client and the attribution switch` and push.

### Task 5: Release resolves, mute forms and history details on the issue page

**Files:**
- Create: `src/pages/MuteForm.tsx`, `src/pages/MuteForm.test.tsx`
- Modify: `src/pages/TriageButtons.tsx`, `src/pages/TriageButtons.test.tsx`, `src/pages/format.ts`, `src/pages/format.test.ts`, `src/pages/IssuePage.tsx`, `src/pages/IssuePage.test.tsx`, `src/pages/issue/IssueHistory.tsx`, `src/pages/issue/IssueHistory.test.tsx`

**Interfaces:**
- Consumes: Task 4's types, `PERIODS`, `muteTermsError`, `triage`.
- Produces:

```ts
export function periodText(periods: number, period: Period): string; // '1 hour', '2 days'
export function MuteForm(props: { action: MuteAction; onSubmit: (terms: MuteTerms) => void; onDismiss: () => void }): React.JSX.Element;
// ACTION_LABELS and triageActions follow decision 10; IssueHistory's describe becomes entryTitle(entry: HistoryEntry, timeZone: TimeZone): string
```

- The page keeps one notice `{ source, issueId, severity: 'error' | 'warning', message }`, shown only when both match (the M7 leftover). A mute button opens its `MuteForm`; submitting closes it and runs the action with the terms. A `warning` in the answer shows as a warning `Alert` under the buttons after the summary is replaced.

- [ ] **Step 1: Write failing tests.**
  - `format.test.ts`: `periodText(1, 'hour')` `1 hour`, `periodText(2, 'day')` `2 days`.
  - `TriageButtons.test.tsx`: `triageActions` per decision 10, in that order.
  - `MuteForm.test.tsx`: `mute-for` shows `Number of periods` (1) and the period radio (`Days`) and no `Events`; `fireEvent.change` to `2`, choosing `Hours`, then `Mute` calls `onSubmit({period: 'hour', periods: 2})`; `0`, `101` and `2.5` show `The number of periods must be a whole number from 1 to 100.` and disable `Mute`; `mute-until` adds `Events` (10) and `1000001` shows the threshold message; `Cancel` calls `onDismiss`.
  - `IssuePage.test.tsx`: `triage buttons follow the state` uses the new labels; `a timed mute sends its terms`: click `Mute for a period`, type 2, `Mute`: `triage('uuid-1', 'mute-for', {period: 'day', periods: 2})`, the dialog closes, Status `Muted`; `a release resolve`: `Resolve in next release` calls `triage('uuid-1', 'resolve-next', undefined)`; `a warning shows under the buttons`: answer `{...ISSUE, status: 'resolved', warning: 'W'}` shows Status `Resolved` and the warning; `the notice belongs to its issue and data source`: a failure on X-1 shows, is hidden after navigating to X-2, and is hidden after switching the address to `ds=other` for X-1; `a read that lands after the answer does not overwrite it` and its sibling first assert `spinning()` is `true` after the Refresh click (in `beforeEach`), then `false` at the end.
  - `IssueHistory.test.tsx`: `muted` with `{period: 'day', periods: 2, unmuteAfter}` reads `Muted for 2 days, until <time>`; with `threshold 5, period 'hour', periods 1` `Muted until 5 or more events in 1 hour`; `resolved` with `release 'r1'` `Resolved in release r1`, with `nextRelease` `Resolved in the next release`; `unmuted` with `threshold` `Unmuted after 5 or more events in 1 hour`; `next_materialized` with `release` `Resolved in the next release: r1`; `by <user>` still appended; an unknown kind keeps its raw kind.
- [ ] **Step 2: Run** `npm run test:ci`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `npm run test:ci`, `npm run typecheck`, `npm run lint`. Expected: PASS.
- [ ] **Step 5: Commit** `Add release resolves, mute forms and history details to the issue page` and push.

### Task 6: The issue list: warnings and the M7 leftovers

**Files:**
- Modify: `src/pages/IssuesPage.tsx`, `src/pages/IssuesPage.test.tsx`, `src/pages/issues/IssueTable.tsx`, `src/pages/issues/IssueTable.test.tsx`

**Interfaces:**
- Consumes: Task 4's `TriageResult`.
- Produces: `IssueTable`'s `triage` gains `disabled?: boolean` (every button disabled). `IssuesPage` keeps `{ key, severity, message }`, keyed by the list key current when the answer arrives (a ref updated on render); a failure in scope triggers one reload and is keyed by that reload's key, a failure after a new search is keyed by the current key without a reload; a `warning` shows as a warning prefixed with the short id. `disabled` is true while the list loads (`current.key !== key`).

- [ ] **Step 1: Write failing tests.**
  - `IssueTable.test.tsx`: `disabled` disables every row's buttons.
  - `IssuesPage.test.tsx`: `actions wait for the reload`: after Refresh with the list read pending, `Resolve SAMPLE-PYTHON-2` is disabled, and enabled when it lands; `a failure after a new search is shown`: rejection after typing a new search shows `SAMPLE-PYTHON-2: <message>` with no extra `listIssues` call for it; `the failure clears on the next action`, `… on the next load` (Refresh, then the read lands); `a warning shows with the short id` with Status from the answer and no reload.
- [ ] **Step 2: Run** `npm run test:ci`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `npm run test:ci`, `npm run typecheck`, `npm run lint`. Expected: PASS.
- [ ] **Step 5: Commit** `Show triage warnings on the issue list and keep its actions in step with reloads` and push.

### Task 7: End-to-end tests of the demo

**Files:**
- Modify: `tests/triage.spec.ts`, `tests/datasource-config.spec.ts`

**Interfaces:**
- Consumes: everything above, built into `dist/`; `basicAuth`, `EDITOR`, `VIEWER`, `TRIAGE`, `GLOBAL_READ`, `BUGSINK_API`, `CHAINED_MARKER`, `readReported('event-detail')`.
- Produces: no provisioning change: `Bugsink triage` leaves `attributionComments` unset, so it runs the default (on). `findIssue(bugsink, marker, newestEventId)` and `latestComment(bugsink, id): Promise<string>` (newest `manual_annotation` of `issue-history/?issue=<id>&order=desc`) inside `triage.spec.ts`.

- [ ] **Step 1: First contact with the running Grafana.** `docker compose up -d --build --wait`, `npm run build && npm run build:backend`, `docker compose restart grafana`; with `Z` the all-zeros id, `R=http://localhost:3300/api/datasources/uid/bugsink-triage`, header `X-Bugsink-Triage: 1`:
  - `GET $R/health` is OK and ends ` The token can triage issues. Actions are attributed by a comment.`
  - `POST $R/resources/issues/$Z/mute-for` with `{"period":"day","periods":2}` (anonymous Admin): 502 `Bugsink refused the request: No Issue matches the given query.` (the body passed through Grafana); with `{"period":"fortnight","periods":2}` 400 period message; with an extra key 400 shape message; with 2000 bytes 400 size message. `resolve-latest` on `$Z`: 502 with the 404 sentence. As the Viewer: 403 `Only editors and admins can triage issues.`
  - The Grafana log line of those calls shows `role=Admin` and no `comment` (no action succeeded).
  If any differs, stop and report.
- [ ] **Step 2: Write the tests** (read `.config/AGENTS/e2e-testing.md` first). In `triage.spec.ts`, `beforeAll` finds the `m5-message` issue (newest event `message`) and the `m5-chained` issue (newest event `chained[1]`) and puts both back; `afterAll` puts both back (decision 13). As the Editor:
  - `the demo: mute for two days, resolve in the next release, attributed`: on the `m5-message` issue page (`ds=bugsink-triage`) click `Mute for a period`, type `2`, keep `Days`, `Mute`: Status `Muted`, `Unmute after` shown; Bugsink: `is_muted`, `unmute_after` between now + 47 h and + 49 h, `latestComment` is `Muted for 2 days from Grafana by m7-editor.`; the History section shows `Muted for 2 days`. On the `m5-chained` issue page click `Resolve in next release`: Status `Resolved`, `Resolved by next release` `Yes`; Bugsink: `is_resolved_by_next_release`, comment `Resolved in the next release from Grafana by m7-editor.`
  - `mute until a threshold`: `Mute until a threshold` with 1, `Hours`, 10: Status `Muted`; Bugsink `unmute_on_volume_based_conditions` `[{"period":"hour","nr_of_periods":1,"volume":10}]`; comment `Muted until 10 or more events in 1 hour from Grafana by m7-editor.`; then `Unmute`.
  - `resolve in the latest release`: on the `m5-chained` issue, Status `Resolved`, Bugsink `is_resolved` and not by next release; then `Reopen`.
  - `a bad body is refused`: `request.post` to `…/mute-for` with the header and `{"period":"day","periods":"2"}`: 400 shape message; Bugsink still open.
  - M7's tests stay; their actions now also add comments. As the Viewer, `no controls and a refusal` also posts `mute-for` with a valid body: 403.
  - `datasource-config.spec.ts`: `triage on with the triage token` expects the success to contain `Actions are attributed by a comment.`
- [ ] **Step 3: Run** `npm run build && npm run build:backend && docker compose restart grafana && npm run e2e`. Expected: every test passes twice in a row; the run reports exactly the five events m2-e2e, m5-chained twice, m5-message, m6-js; `python3 scripts/verify_stack.py` prints eleven `ok` lines.
- [ ] **Step 4: Run** `npm run lint`, `npm run typecheck`, `scripts/check_spdx.sh`. Expected: PASS.
- [ ] **Step 5: Commit** `Test full triage end to end` and push.

### Task 8: Documentation and changelog

**Files:**
- Modify: `src/README.md`, `CHANGELOG.md`, `docker/README.md`

- [ ] **Step 1: Write** in `src/README.md`, section Triage: the new actions per state and their forms with the bounds; a subsection "Attribution comments": the switch (on by default), `issues:comment`, the exact text of decision 6 with an example, that it names the Grafana login (an email address for some sign-in methods), `an anonymous user` and `sa-…` service accounts, that everyone who can see the issue in Bugsink can read it, the warning when it fails and that the action stands, the health check lines; under Who may triage, that the org role is the rule and Grafana RBAC roles do not apply (an OSS backend plugin cannot check them). Line 32: the read probes are one read per capability, and the triage and comment checks are writes Bugsink refuses before any change. Line 140: the row and page show Bugsink's answer at once and keep it until the next load, which reads Bugsink again. Add a note on Grafana's query caching (Enterprise and Cloud): it can serve a dashboard's issue table from Grafana's own cache after an action, which the plugin cannot clear. Line 126 and `docker/README.md:90`: add that a page using DNS rebinding to reach Grafana under its own name can add the header but carries no Grafana session, so it only gets what anonymous access allows. `CHANGELOG.md` under 0.1.0: release resolves, timed and threshold mutes, attribution comments and their health check, history details, the list fixes.
- [ ] **Step 2: Run** `scripts/check_spdx.sh`, `scripts/check_licenses.sh`, `npm run lint`. Expected: PASS.
- [ ] **Step 3: Commit** `Document full triage` and push.

## Not in this milestone

Grafana RBAC plugin actions (decision 1; revisit when a backend plugin can check an action on OSS without extra feature toggles); a release picker (Bugsink resolves only in the latest or the next release); the mute-until condition in the summary (History shows it); several issues at once; the event-count series (M9) and alerting (M10).

## After the last task

* Whole-branch review against this plan and spec §4.E, then one fix wave.
* From the worktree: `npm ci`, `npm run build`, `npm run build:backend`, `go test -race ./pkg/...`, `npm run test:ci`, `npm run e2e`, `python3 scripts/verify_stack.py`.
* Update spec §4.A (the attribution switch; the `issues:comment` probe replaces "is not probed"), §4.D (history details; the new page actions), §4.E (the eight routes; the check order with the path-segment, method and body checks; the body rules and bounds; attribution field, text, failure; who may triage stays the org role, with the RBAC evidence; the DNS rebinding clause beside the header sentence) and §10 (RBAC, the comment's login, the switch default), the roadmap's M8 entry; add `## As built` here.
* The merge into `main` happens only when the owner asks for it.

## As built

1. The thirteen decisions were built as planned, with the changes below.
2. Grafana RBAC plugin actions are not built, and the org role (exactly Editor or Admin) stays the rule: SDK v0.299.0 hands the plugin only `Login`, `Name`, `Email` and `Role`, and the plugin-tools RBAC guide's backend check needs `github.com/grafana/authlib/authz`, a managed service account from an `iam` block, the `externalServiceAccounts` toggle ("The `externalServiceAccounts` feature only supports single organization setups."), the `X-Grafana-Id` token (`idForwarded` toggle) and Grafana 11.6.0 or later, none of which the stack has.
3. The guide's licensing sentence is "Assigning roles to specific users requires either Grafana Cloud or a Grafana Enterprise license.", so on OSS plugin roles could at most go to basic roles, which the org-role check already covers; whether Grafana OSS 13.3.1 registers plugin roles granted to basic roles was not verified.
4. The attribution comment reads `<what> from Grafana by "<login>".`: the login sits in double quotes, every Unicode quotation mark in it (the double quote included) becomes an apostrophe, control, format, line and paragraph separator characters become the replacement character, and a login over 100 characters is cut with an ellipsis inside the quotes.
5. An empty login (anonymous access) is written `an anonymous user` without quotes, and a service account by its `sa-...` login.
6. The comment names the Grafana login, never the name or the email field, but some sign-in methods (single sign-on among them) make the login an email address, and then the comment holds it for every Bugsink user who can see the issue.
7. Attribution comments are on by default (`attributionComments` absent or `null`), and the README's "Switch it on" steps name `issues:comment`, the comment and the provisioning key `attributionComments: false`.
8. With the switch on and Triage on, the plugin always tries the comment after Bugsink accepted the action, whatever the token's capabilities, and a token without `issues:comment` gets a warning after each action.
9. A failed comment leaves the action standing, with no retry and no undo: the answer is 200 with the issue and `warning`, shown with the short id on the list and under the buttons on the issue page.
10. The health check probes `issues:comment` with a blank comment on the all-zeros issue, which Bugsink refuses twice over, so both health probes are free of side effects.
11. An unexpected answer to the `issues:comment` probe (a 404 or 405 from an older Bugsink, a 500, a 403 naming no capability or another one) keeps the status OK and ends the message with ` Bugsink answered HTTP <n> to a check for issues:comment, so actions may not be attributed.`.
12. A 2xx answer to the `issues:comment` probe stays an error with Bugsink's real status, because Bugsink then accepted something the probe promises not to write.
13. `AddComment` returns Bugsink's status, and the triage log line carries that status as `comment` (0 without an answer) beside the org role.
14. The mute body check gives an integer too large for the platform the range message and `2.0` or `1e1` the shape message, and a request with no body gets the shape message.
15. `MaxPeriods`, `MaxThreshold` and `Period.Valid` are exported from `pkg/bugsink`, so the route and the client share one set of bounds, and the client refuses out-of-range terms again before any request.
16. The JSON key order of the mute bodies sent to Bugsink follows the generated structs, which is accepted because the keys and values are the planned ones.
17. History details also take `actual_release` as the release, accept period counts up to the largest 32-bit integer, and keep no period for a `mute_until` entry without a valid threshold, so it reads `Muted`.
18. `muteTermsError` lives in `src/datasource/muteTerms.ts` with its own tests and no re-export, and it gives a fraction or `NaN` the range message where the backend gives the shape message, which the user never sees because the form refuses it first.
19. The config editor always renders `Attribution comments` under the `Triage` heading, and a stored `null` shows as on.
20. On the issue page, an open mute form closes when the page moves to another issue or data source, and the notice under the buttons belongs to its issue and data source.
21. On the issue list, every button is disabled while the list loads, and the one message above the table is keyed to its list load and dropped when the page leaves that load, so it does not come back when the same search comes back.
22. On the issue list, a failure in the current list ends that list's scope and reloads once, so two failures at once reload once and show the last, and a failure or a warning that arrives after a new search shows on the new list.
23. Grafana 13.3.1 forwards a resource POST body to the plugin unchanged, checked live.
24. Resolve in the latest release on a project without a release was not seen live, because both seeded projects have releases; a unit test pins Bugsink's 400 `Project has no releases.` as a 502 `Bugsink refused the request: Project has no releases.` with no comment.
25. The end-to-end tests add no event, read the newest History entries (the comment first, the action second), check that no attribution warning shows, and leave both issues open and unmuted.
26. Each end-to-end run leaves history entries and comments on the two m5 issues and the `m5-chained` issue keeps its `fixed_at`, because Bugsink's API cannot delete a comment or clear `fixed_at`; on a long-lived stack the History section shows only the newest 5000 entries.
27. Signing in as an end-to-end user before `tests/users.setup.ts` has created it triggers Grafana's five-minute lock-out.
28. Every refusal is tested in Go over all eight routes; the end-to-end tests check a representative subset.
29. The README now says that Bugsink's history records an action without a user, that the health check makes at least one read per capability, and that RBAC needs the two feature toggles.
30. Left for later milestones: M11, read history `metadata` numbers with `UseNumber` so a number too large for a float does not fail the page, run Prettier on `Breadcrumbs*.tsx`, `IssueHistory.tsx` and `src/README.md`, focus the mute form's first input, wrap long release names, key the issue page's busy state by data source and issue id, and give the issue list more than one message slot.
