# Bugsink

Connects Grafana to a self-hosted [Bugsink](https://www.bugsink.com/) error tracker.

It adds a Bugsink data source. The data source checks the Bugsink address and the API token you give it, and tells you what is wrong when the check fails. It also queries issues and events for dashboards and Explore, and the app has a page that lists issues and a page for one issue, with the content of its events.

## Requirements

- Grafana 13.2.0 or later.
- A Bugsink instance that Grafana's server can reach over HTTP or HTTPS.
- A Bugsink API token with the capabilities `projects:read`, `issues:read` and `events:read`. `events:read` is needed for the tags and events of an issue, for the event detail and for the events query.
- To triage issues from Grafana, the token also needs `issues:triage` (see [Triage](#triage)).

## Create an API token

Create the token in Bugsink. A token has capabilities, and it can be bound to one project. Give it the three capabilities above. A token bound to a project lets the plugin see that project only. The Bugsink documentation, <https://www.bugsink.com/docs/>, has the exact steps for your Bugsink version.

## Add the data source

1. Enable the app if it is not enabled yet: in Grafana, open Administration > Plugins and data > Plugins, choose Bugsink and enable it.
2. Open the Bugsink app. It shows the issue list (see below). With no Bugsink data source, it shows `No Bugsink data source yet.` and the link `Add a Bugsink data source`.
3. Follow that link, or open Connections > Data sources > Add new data source, and choose **Bugsink**.
4. Fill in the fields:
   - **Bugsink URL**: the address of Bugsink itself, for example `https://bugsink.example.com`. Do not add `/api`.
   - **Browser URL** (optional): the address of Bugsink as your browser reaches it. The plugin uses it for the links to issues. Leave it empty when it is the same as the Bugsink URL. It differs when Grafana and your browser reach Bugsink by different addresses. In the development stack, Grafana reaches Bugsink at `http://bugsink:8000` and the browser at `http://localhost:8800`, so the Bugsink URL is `http://bugsink:8000` and the Browser URL is `http://localhost:8800`.
   - **API token**: the token you created. Grafana stores it encrypted and does not show it again.
   - **Trace and log links** (optional): the Tempo and Loki data sources and the link settings. See [Trace and log links](#trace-and-log-links).
   - **Triage** (optional, off by default): lets editors and admins resolve, reopen, mute and unmute issues. See [Triage](#triage). If anonymous users have the Editor or Admin role, anyone who can open Grafana can triage.
   - **Attribution comments** (optional, on by default, under Triage): writes a comment on the issue in Bugsink after each action. See [Attribution comments](#attribution-comments).
   - **Hide stack trace text** (optional, off by default): see below.
5. Click `Save & test`. A data source provisioned from a file has a `Test` button instead.

The test makes at least one read request per capability and reports the result. With Triage on, it also checks `issues:triage`, and with Attribution comments on, `issues:comment`. Those two checks are writes that Bugsink refuses before it changes anything (see [Triage](#triage)). It stops at the first answer that ends the check: a connection error or timeout, a rejected token, an answer that is not Bugsink's, or an unexpected answer to a capability check. An unexpected answer to the `issues:comment` check fails the test only when Bugsink accepts the check (see [Attribution comments](#attribution-comments)).

For an event whose stack frames are not inside an exception, the plugin shows Bugsink's own stack trace text, which can include local variables. Everyone who can query this data source in Grafana can read it. Switch on `Hide stack trace text` to remove that text: the plugin's backend drops it, so it never reaches the browser. The event then shows the notice `Bugsink's stack trace text is hidden for this data source.`

## Browsing issues in the app

The Bugsink app opens on a page that lists issues. You need at least one Bugsink data source.

- **Data source**: the picker at the top lists your Bugsink data sources. The default data source is selected first; without a default, the first one by name. Changing the data source clears the Project filter and, if Sort by is `Arrival order`, resets it to `Last seen`.
- **Filters**: Project, State, Sort by, Order and Search are the controls of the query editor, described under [Query controls](#query-controls). They apply when you change them; Search applies when you leave the field or press Enter.
- **Time range**: the picker at the top right. The default is the last 7 days. The page shows issues last seen within the range. The refresh button runs the query again; within 15 seconds of the same query it shows the same answer (see [Limits](#limits)).
- **Table**: 25 issues per page, with the columns Issue, Type, Message, Transaction, Project, Events, First seen, Last seen and Status, and Actions when triage is on for the data source and you may triage. In the Issue column, the short id opens the issue page in the app, and the icon beside it opens the issue in Bugsink in a new tab (see [Issue detail page](#issue-detail-page) and [Link to Bugsink](#link-to-bugsink)).
- **States**: on the first load and after you change the data source, the page shows `Loading issues...`. A later load keeps the table on screen and shows a loading bar above it. When nothing matches, it shows `No issues match these filters in this time range.` A failed read shows the error message.

The address bar holds the whole state: data source, filters, time range, sort and order. Copy the address to reopen the same list. The page keeps the address parameters it does not use, such as `orgId` and `kiosk`, as they are.

The page has the limits of [Limits](#limits): at most 5000 issues are read and results can be up to 15 seconds old.

## Issue detail page

Click the short id of an issue in the list to open its page. The page has five sections that load at the same time, except that Event waits for the Events list when the address names no event. When the summary cannot be read, its message shows alone, because the page has no issue to describe. Otherwise Event, Events, Tags and History each show their own error under their heading. For example, a token without `events:read` still shows the summary and the history.

- **Summary**: the type and message, then Status (`Open`, `Muted` or `Resolved`), Project, Transaction, Events, Stored events, First seen and Last seen. Unmute after shows only when it is set, and Resolved by next release only when it is `Yes`.
- **Event**: the content of one event (see [Event](#event)). Needs `events:read`.
- **Events**: the issue's events with Event ID, Timestamp and Ingested at, in the order Bugsink returns them (newest first), 25 per page. Event ID opens the event in the Event section, and the icon beside it opens the event in Bugsink in a new tab. The event on show is in bold. At most 1000 events are read; when it stops there it shows `Only the newest 1000 events were read.`
- **Tags**: one row per tag key, with its values and how many events carry each, for example `development (26), staging (3)`. A row shows the 5 values with the most events, then `and <n> more values`. The page reads at most 5000 tag values; when it stops there it shows `Only the first 5000 tag values were read.` Needs `events:read`.
- **History**: what happened to the issue, newest first: the time, what happened, who did it when Bugsink knows, and the comment text. The entry names the details Bugsink holds, for example `Muted for 2 days, until <time>`, `Muted until 100 or more events in 1 hour`, `Unmuted after 100 or more events in 1 hour`, `Resolved in release <release>` or `Resolved in the next release`. An entry without details shows its kind: `First seen`, `Resolved`, `Muted`, `Regressed`, `Unmuted`, `Reopened` or `Comment`. At most 5000 entries are read; when it stops there it shows `Only the newest 5000 history entries were read.`

With triage on and a role that may triage, the page also shows the actions for the issue's state (see [Triage](#triage)).

The buttons at the top: `Back to issues` returns to the list as you left it, with the same filters, data source and time range. `Open in Bugsink` opens the issue in Bugsink. The refresh button reads every section again, and retries a data source that failed to load. The list of events can be up to 15 seconds old (see [Limits](#limits)).

The address holds the data source (`ds`), the list's parameters and the event on show (`event`), so a copied address opens the same page.

### Event

The Event section shows the newest event of the issue, the first row of Events. Click another Event ID in the Events section to show that event. The address keeps the choice in `event=<event id>`, where the id is Bugsink's id for the event. An `event` that belongs to another issue shows the error `This event belongs to another issue.`

The section has these parts, in this order:

- **Rows**: Event ID, Timestamp, Level, Environment, Release, Transaction and Trace ID. A row the event does not have is left out, except Event ID.
- **Actions**: `Open trace` and `Open logs` when the data source has the target and the event has a trace ID of 32 lowercase hexadecimal characters (see [Trace and log links](#trace-and-log-links)), and `Open event in Bugsink`, which opens the event in Bugsink in a new tab.
- **Notices**: a warning for each bound the event reached (see [Event bounds](#event-bounds)).
- **Message**: the event's message, when it has one, above the exceptions or the stack trace.
- **Exception**: the exceptions of the event, each with its type (`Unknown exception` when it has none), its value and the badge `Unhandled` when the SDK marked it so. For `python` events the exceptions are in the order Bugsink holds them, the first cause first, with the frames from the oldest call to the newest, and `During handling of the above exception, another exception occurred:` between two exceptions. For every other platform the last exception comes first, the frames run from the newest call to the oldest, and `Caused by:` stands between two exceptions. A frame shows `file:line:column in function`. A frame the SDK marked as part of your application (`in_app`) shows in bold with up to 5 lines of source code before and after its line, when the SDK sent them. Other frames are folded, in groups of consecutive frames, under `<n> other frames`, without source lines. When no frame of an exception is part of your application, every frame shows.
- **Stack trace**: for an event without exceptions. It is Bugsink's own text of the stack trace, shown as plain text, when the event has frames outside its exceptions, for example the stack trace of a message captured with `attach_stacktrace`. The same text shows when exceptions exist but none has frames. Bugsink writes this text, not the plugin, and it can include local variables. An event without frames shows `No stack trace.` With `Hide stack trace text` on, the section shows `No stack trace.` and the notice `Bugsink's stack trace text is hidden for this data source.` shows among the Notices.
- **Breadcrumbs**: the trail of actions before the event, oldest first, with Time, Type, Category, Level and Message. The message column shows the breadcrumb's data when it has no message.
- **Event tags**: the tags of this one event, as Tag and Value.
- **Contexts**: the contexts the SDK sent, such as `runtime` or `trace`, as Context, Key and Value.

The structured stack trace does not show local variables. The Event section does not show the request, the user, the extra data, the modules or the SDK of the event. Open the event in Bugsink for them.

### Event bounds

The plugin reads an event from Bugsink and cuts it before it shows it. Each cut adds a warning in the Event section:

| Bound | Notice |
| :--- | :--- |
| The last 10 exceptions of a chain | `Only the last 10 exceptions of the chain are shown.` |
| The 250 frames nearest the error of an exception | `Only the 250 frames nearest the error are shown for <type>.` |
| The newest 100 breadcrumbs | `Only the newest 100 breadcrumbs are shown.` |
| The first 200 tags, contexts or fields of each list, and breadcrumb data entries, by key | `Only the first 200 tags, contexts or fields of each list are shown.` |
| Values of 1024 characters | `Values longer than 1024 characters are cut.` |
| Bugsink's stack trace text, 100 KB | `The stack trace text is cut at 100 KB.` |

The plugin reads at most 10 MB of Bugsink's answer for one event. A larger answer shows `The answer from Bugsink is larger than 10 MB.` in the Event section. The Events row of the event still links to Bugsink. Bugsink's own limit for an event is `MAX_EVENT_SIZE`, 1 MiB by default, so the plugin's limit is reached only when Bugsink is set to accept much larger events.

An event id that is not valid shows `"<id>" is not an event id.`

## Triage

Triage lets people resolve, reopen, mute and unmute issues without leaving Grafana. It is off by default. It works on one issue at a time.

### Switch it on

1. Give the API token `issues:triage`, and `issues:comment` for attribution comments, in addition to the read capabilities.
2. In the data source settings, switch on `Triage` and click `Save & test`.
3. Attribution comments is on by default: after each action the plugin writes a comment in Bugsink with the Grafana login (see [Attribution comments](#attribution-comments)). Switch it off in the settings, or with `attributionComments: false` under `jsonData` in a provisioning file.

The switch is per data source. With it off, nobody sees triage controls, and the data source answers a triage call from an editor or admin with `Triage is switched off for this data source.`

The test with Triage on asks Bugsink whether the token may triage. It calls resolve on an issue id that cannot exist, which changes nothing. The result is one of:

- `The token can triage issues.`, added to the success message.
- `The API token lacks: issues:triage.`
- `Bugsink answered HTTP <n> to a check for issues:triage.`

### Who may triage

Users whose Grafana org role is Editor or Admin. A Viewer sees no controls, and a direct call is refused with `Only editors and admins can triage issues.` Grafana RBAC roles and plugin actions do not apply: a backend plugin on Grafana OSS cannot check them for the caller without the `externalServiceAccounts` and `idForwarded` feature toggles, which are off by default and support one organization only.

An anonymous session and a service account count by their org role.

A triage call must carry the header `X-Bugsink-Triage: 1`. The app sends it; a script that calls the data source's resources must send it too. Without it the call is refused with `Triage calls need the header X-Bugsink-Triage: 1.`

**Warning:** when Grafana allows anonymous access with the role Editor or Admin, everyone who can open Grafana can triage. The required header keeps a page on another web site from triaging through a visitor's browser, because such a page cannot add the header to its request. It does not stop anyone who can open Grafana with the Editor or Admin role, anonymous visitors included. A page that uses DNS rebinding to reach Grafana under Grafana's own name can add the header, but its request carries no Grafana session, so it gets only what anonymous access allows.

The two mute routes read a request body of at most 1024 bytes and refuse one that holds anything but the expected fields.

Every action runs in Bugsink with the API token, and Bugsink's history records it without a user, so it never names the Grafana user. To name the Grafana user, use [Attribution comments](#attribution-comments).

### Actions

| State | Issue page | Issue list |
| :--- | :--- | :--- |
| Open | Resolve, Resolve in latest release, Resolve in next release, Mute, Mute for a period, Mute until a threshold | Resolve, Mute |
| Muted | Resolve, Resolve in latest release, Resolve in next release, Unmute | Resolve |
| Resolved | Reopen | none |

The list shows the buttons in the Actions column of each row. The issue page shows them under the summary. While an action runs, the buttons of that issue are disabled.

- **Resolve** resolves the issue now.
- **Resolve in latest release** resolves it in the latest release Bugsink knows. **Resolve in next release** resolves it in the next release. Bugsink has no release picker in its API, so the plugin has none either.
- **Mute** mutes the issue without an end.
- **Mute for a period** asks for a number of periods from 1 to 100 and a period: minutes, hours, days, weeks, months or years. The issue is unmuted when the time has passed.
- **Mute until a threshold** asks for a number of events from 1 to 1000000, a number of periods from 1 to 100 and a period. The issue is unmuted when it gets that many events within that time.

After an action, the list row and the issue page show Bugsink's answer at once and keep it until the next load, which reads Bugsink again. Another data source that points at the same Bugsink can show the old state for up to 15 seconds (see [Limits](#limits)).

Triage is available in the app only. Dashboard tables and Explore do not offer it. Grafana's query caching (Enterprise and Cloud) can serve a dashboard's issue table from Grafana's own cache after an action. The plugin cannot clear that cache.

### Attribution comments

After a successful action, the plugin writes a comment on the issue in Bugsink that names the action and the Grafana login of the person who took it. For example:

- `Muted for 2 days from Grafana by "m7-editor".`
- `Resolved in the next release from Grafana by "m7-editor".`
- `Muted until 100 or more events in 1 hour from Grafana by "m7-editor".`

An anonymous session is written as `by an anonymous user.`, without quotes. A service account shows as its login, `sa-...`. Some sign-in methods make the login an email address, and then the comment holds that address. A login longer than 100 characters is cut.

Everyone who can see the issue in Bugsink can read the comment. It holds the Grafana login, not the email address of the user. Bugsink's own history entry for the action names no user.

- **Switch**: `Attribution comments` in the data source settings, under Triage. It has an effect only when Triage is on. It is on by default, and a data source saved before the switch existed keeps it on.
- **Token**: the API token needs `issues:comment`. Without it, comments fail.
- **When the comment fails**: the action stands. The app shows a warning with the reason, on the list and on the issue page.
- **Health check**: with Triage and Attribution comments on, the test message ends with one of these lines:
  - `Actions are attributed by a comment.`
  - `It lacks issues:comment, so actions are not attributed.` The check stays green.
  - `Bugsink answered HTTP <n> to a check for issues:comment, so actions may not be attributed.` The check stays green.

  If Bugsink accepts the check, the test fails with `Bugsink answered HTTP <n> to a check for issues:comment.`

  The check sends an empty comment for an issue id that cannot exist, which changes nothing.

## Querying issues

Add a panel, or open Explore, choose the Bugsink data source and set the query. Every change to a control runs the query again. Search is applied when you leave the field and runs when you press Enter, click Run query in Explore or refresh the panel. The result is a table with one row per issue.

### Query controls

| Control | What it does | Default |
| :--- | :--- | :--- |
| Project | Reads the issues of one project. `All projects` reads every project the token sees. | All projects |
| State | Keeps only issues in that state: `All`, `Open`, `Unresolved`, `Resolved` or `Muted`. | Unresolved |
| Sort by | `Last seen`, `Event count` or `Arrival order`. `Arrival order` needs a project: it is not offered with `All projects`. | Last seen |
| Order | `Descending` or `Ascending`. | Descending |
| Search | Bugsink's search text, for example `environment:production release:1.4.2`. It is applied when you leave the field and sent when you press Enter or run the query. It accepts dashboard variables, for example `environment:$environment`. The other controls do not. | empty |

### Columns

| Column | Content |
| :--- | :--- |
| Issue | The issue's short id. It links to the issue in Bugsink (see below). |
| id | The issue's Bugsink id. The link uses it. Hide it with a field override or the Organize fields transformation if you do not want it. |
| Type | The error type. |
| Message | The error message. |
| Transaction | The transaction the error happened in. |
| Project | The project name. The project id shows instead when the project list cannot be read or does not contain the project. |
| Status | `resolved`, `muted` or `open`. |
| Events | The number of events Bugsink counted for the issue. |
| Stored events | The number of events Bugsink still stores for the issue. |
| First seen, Last seen | When the first and the last event arrived. |
| Resolved, Resolved by next release, Muted | Flags of the issue. |
| Unmute after | When a muted issue is unmuted, if it is set. |
| Project ID | The project id. |

### Link to Bugsink

In a dashboard or Explore table, the Issue column carries the link `Open in Bugsink`. In the app's list, the icon beside the short id does the same. It opens the issue's last event in a new tab, on the address in the data source's `Browser URL`, or on the `Bugsink URL` when that is empty.

### Limits

- A row is an issue whose last event arrived within the dashboard time range, between its start and its end.
- A query reads at most 5000 issues. When it stops there, the table carries the warning `Only the first 5000 issues were read. Narrow the search or the time range.`
- Results can be up to 15 seconds old. Bugsink is asked again after that.

## Querying events

In the query editor of a panel or Explore, the `Query type` picker chooses `Issues` or `Events`. The app's list page has no picker: it queries issues only.

An events query has one of three modes. It uses the first one that applies, in this order:

1. **Event ID** is set: it looks that one event up. It needs a Project, and takes the id with or without dashes.
2. **Issue** is set: it reads the events of that issue, narrowed by Search when Search is not empty. Issue is a short id such as `SAMPLE-PYTHON-2`, or Bugsink's issue id. Project is not used in this mode.
3. Search holds `trace:<id>`: it finds the issues that match, narrowed by Project when one is chosen, then reads the events of each one that match too. The rows are grouped by issue, last seen issue first, and the newest event first within an issue. Sort a table panel by Timestamp to mix them.

Search, Issue and Event ID accept dashboard variables, for example `trace:$trace`. With none of the three, the table is empty and carries the notice `An events query needs an issue, an event ID or trace:<id> in Search.` Error messages: `"<issue>" is not an issue id.`, `The event ID must be a UUID.` and `Looking up an event ID needs a project.`

The result is a table with the columns Timestamp, Event ID, Issue, Project, Ingested at, Trace ID, id, issue_id and Project ID. The Event ID cell links to the event in Bugsink, in a new tab. The id and issue_id columns are what the link uses. Trace ID carries the trace links (see [Trace and log links](#trace-and-log-links)).

The dashboard time range does not apply to events: an issue, an event ID or a trace already limits the list.

### Bounds

- One issue: at most 1000 events, the newest ones. Otherwise the table carries `Only the newest 1000 events were read.`
- One trace: at most 10 issues, the last seen ones, and at most 250 events of each. Otherwise the table carries `This trace has events in more than 10 issues. Only the 10 last seen were read.` or `Only the newest 250 events of <short id> in this trace were read.`
- Results can be up to 15 seconds old.

### Example

The development stack has the dashboard `Bugsink trace`. Type a trace id in `Trace ID`, and it shows two tables: the issues of the trace and its events.

## Trace and log links

An event can open its trace in Tempo and its log lines in Loki.

### Settings

The data source editor has these fields under `Trace and log links`:

| Field | What it does | Default |
| :--- | :--- | :--- |
| Tempo data source | `Open trace` runs a trace ID query on this data source. | none |
| Loki data source | `Open logs` runs the logs query on this data source. | none |
| Time window | Links open this many minutes before and after the event. Whole minutes, 1 to 1440. | 15 (when empty) |
| Logs query | The LogQL of `Open logs`, with variables (see below). | `{service_name="${service}"} \| trace_id="${trace_id}"` |
| Service tag | The event tag that holds the service name for `${service}`. | `service` |

A link is offered only for a data source you set. With neither Tempo nor Loki set, there are no buttons and no links. `${service}` is the Bugsink project name when the event does not have the service tag or its value is empty, and always when `Service tag` is empty. A window outside 1 to 1440 is refused as an invalid setting.

### Logs query template

The template is LogQL with these variables: `${trace_id}`, `${service}`, `${project}`, `${environment}`, `${release}` and `${tag:<name>}`, the value of the event tag `<name>`. An unknown `${...}` stays as written.

- Put every variable inside a double-quoted string, for example `trace_id="${trace_id}"`. A template with a variable inside backticks, outside quotes, inside a comment (`#`, `//` or `/* ... */`) or inside a string cut by a line break is refused as an invalid setting.
- Values are escaped for a double-quoted LogQL string. With `=~` or `!~` the value is not escaped for a regular expression.
- In a dashboard table, `${environment}`, `${release}` and `${tag:<name>}` render empty, except for an event ID query, which has the event. In the other rows, `${service}` is the project name.

### Event section

With a trace ID of 32 lowercase hexadecimal characters on the event and a target set, the Event section shows `Open trace` and `Open logs`. Both open Explore in the same tab, on the event's time plus and minus the window. The buttons are shown without asking Tempo. A trace Tempo does not have opens Explore with Tempo's own not-found message. Back from Explore can need more than one press: Explore adds its own history entries.

### Dashboards and Explore

An events query has a `Trace ID` column. Its cells carry the links `Open trace` and `Open logs`, in the cell menu. The links open Explore on the event's time plus and minus the window; for a table of several events, from the first to the last timestamp, plus and minus the window. Inside Explore, Grafana keeps Explore's own time range.

The column is filled for a query with `trace:<id>` in Search and for an event ID query. An issue-only query carries no trace ID, so it has no links. Without a Tempo or a Loki data source, the column has no links. A row whose trace ID is not 32 lowercase hexadecimal characters has no links. The Logs query is rendered per row in a hidden `logs_query` field, which the Loki link reads. Do not remove it from a panel.

## Linking errors to traces

The links need Bugsink events that carry the trace ID of your OpenTelemetry trace. This happens when the Sentry SDK shares the OpenTelemetry trace context.

Python:

- Add `OTLPIntegration(setup_otlp_traces_exporter=False, setup_propagator=False)` to `sentry_sdk.init`. Your own OpenTelemetry setup exports the spans.
- Put the OpenTelemetry ASGI middleware outermost, so the request span is still active when Sentry captures an unhandled error.

Node.js:

- Run your own OpenTelemetry tracer provider, and add `Sentry.openTelemetryIntegration()` to the Sentry integrations.

An event captured outside a span, for example in a new thread, carries a trace ID that Tempo does not have. Tempo then says the trace is not found.

An incoming `traceparent` header is honoured with `setup_propagator=False`. The default of the integration ignores it and starts a new trace.

To go the other way, from a trace to its errors, use the dashboard `Bugsink trace` of the development stack, or an events query with `trace:<id>` in Search. Type the trace ID in lower case, as the SDKs and Tempo show it: Bugsink matches `trace:` terms case-sensitively.

## Test results

A successful test says:

- `Connected to Bugsink. Projects visible to this token: <names>.` Project names are listed in the order Bugsink returns them. With more than ten projects, the first ten are listed, followed by `and <n> more`.
- `Connected to Bugsink. This token sees no projects.` The connection and capabilities are fine, but the token (or its project binding) gives access to no project.

A failed test says one of the following. `<url>` is the URL as you typed it.

| Message                                                                                                                      | What to do                                                                                                                                                                                                                                                    |
| :--------------------------------------------------------------------------------------------------------------------------- | :------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `Enter the Bugsink URL.`                                                                                                     | Fill in the Bugsink URL field.                                                                                                                                                                                                                                |
| `The Bugsink URL must start with http:// or https://.`                                                                       | Add the scheme to the URL.                                                                                                                                                                                                                                    |
| `The browser URL must start with http:// or https://.`                                                                       | Fix the Browser URL. It must be an http or https address without credentials, a query string or a fragment, or empty.                                                                                                                                       |
| `Enter an API token.`                                                                                                        | Fill in the API token field.                                                                                                                                                                                                                                  |
| `The data source settings are invalid.` | Grafana could not build an HTTP client from a setting supplied through provisioning or the API, for example a TLS certificate setting, or a link setting has the wrong type, for example a number where a data source UID belongs. Correct that setting. |
| `The link time window must be a whole number of minutes from 1 to 1440.` | Fix the Time window: a whole number from 1 to 1440, or empty for 15. |
| `In the logs query, put every variable inside double quotes.` | Edit the Logs query so that every variable sits inside a double-quoted string on one line, and not in a comment. |
| `Cannot reach Bugsink at <url>: <cause>.`                                                                                    | Grafana's server could not connect. The cause says why: a refused connection, a DNS failure or a TLS error. Check the address, the network path from the Grafana server to Bugsink and the certificate.                                                       |
| `Bugsink at <url> did not answer within 10 seconds.`                                                                         | Check that Bugsink is running and not overloaded, and that nothing between Grafana and Bugsink drops the traffic.                                                                                                                                             |
| `Bugsink rejected the API token. Check that it was copied in full and has not been deleted.`                                 | Paste the token again, or create a new one in Bugsink.                                                                                                                                                                                                        |
| `<url> did not answer like a Bugsink API (HTTP <status>). Check the URL: it is the address of Bugsink itself, without /api.` | The address answers, but not as Bugsink's API does. Check the URL for a wrong host, port or path prefix, or a proxy in front of Bugsink. The plugin does not follow redirects, so an address that redirects (HTTP 301, 302, 303, 307 or 308) fails here: enter the address it redirects to.                                                                                                                      |
| `The API token lacks: <capabilities>.`                                                                                       | Add the named capabilities to the token in Bugsink, or use a token that has them. The test names every missing capability at once.                                                                                                                            |
| `Bugsink answered HTTP <status> to a check for <capability>.`                                                                | Bugsink answered a capability check in a way the plugin does not expect. Check the Bugsink version and its server log.                                                                                                                                        |

## Get help

Report problems at <https://github.com/a-h-i/grafana-bugsink-app/issues>.
