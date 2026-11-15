# Changelog

## 0.1.0 (unreleased)

- Bugsink data source: connect to a Bugsink instance and verify the URL, the API token and its read capabilities.
- Issues query: list the issues last seen within the dashboard time range as a table, in dashboards and Explore, with a link to each issue in Bugsink.
- Query editor with Project, State, Sort by, Order and Search controls.
- `Browser URL` setting: the address of Bugsink as the browser reaches it, used for the issue links.
- Issue list page in the app: pick a Bugsink instance, filter by project, state, search and time range, sort by last seen or event count, link to each issue in Bugsink; the page state is kept in the URL.
- Issue detail page in the app: summary, tag values with counts, history and the issue's events; the list's short id opens it.
- Events query: the events of one issue, one event looked up by its SDK id, or every event of a trace, as a table.
- Event detail on the issue page: exception chain and stack trace with in-app frames open and the other frames folded, breadcrumbs, tags and contexts; the newest event by default, any other from the event list.
- Open trace and Open logs on events, links on the trace ID of event tables, and the Tempo, Loki, time window, logs query and service tag settings. The Event section also has `Open event in Bugsink`, shows an event's message above its exceptions and names an exception without a type `Unknown exception`.
- Triage from the issue list and the issue page: resolve, reopen, mute and unmute for editors and admins, with the `Triage` switch per data source. A triage call needs the header `X-Bugsink-Triage: 1`, which the app sends.
- The test of a data source with Triage on checks that the token has `issues:triage`.
- Full triage on the issue page: resolve in the latest or the next release, mute for a period, and mute until a volume threshold. The issue list keeps Resolve and Mute.
- Attribution comments: after an action the plugin writes a comment in Bugsink with the Grafana login, switched per data source (`Attribution comments`, on by default) and needing `issues:comment`. A failed comment shows a warning and the action stands. The test of a data source checks `issues:comment` and reports it without failing.
- History shows the details Bugsink holds: mute periods, volume thresholds and releases.
- Issue list: a triage warning shows with the issue's short id, and the actions stay in step with a reload.
- `Hide stack trace text` switch: removes Bugsink's own stack trace text in the backend.
- The plugin no longer follows redirects from Bugsink. A Bugsink URL that redirects now fails the test with `did not answer like a Bugsink API (HTTP 30x)`; enter the address it redirects to.
- The `logs_query` field of event tables is hidden; the Loki link still reads it.
- Event volume query: a project's or an issue's events per hour or per whole-hour step as a time series, up to 90 days.
- `Last 24 hours` column in the issue list: one small chart per visible row, loaded at most four at a time.
- The issue list cancels a load that a newer one supersedes.
- Alerting: the data source can be chosen in Grafana's alert rule editor. New query types `Issue count`, `Event count` and `New issues` return current values and series that alert rules can read; a Threshold reads the two counts directly. Numbers are labelled with the project or issue id.
- Request limit: each data source makes at most 8 Bugsink requests at once. An evaluation makes at most 1 call for an event count and 20 list pages for an issue count or new issues, and a count over 5000 matching issues is an error.
- Text from Bugsink shown in errors is cut to 500 characters and never contains the API token.
- The sparkline charts in the issue list are no longer in the tab order.
