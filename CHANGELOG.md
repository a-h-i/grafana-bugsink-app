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
- `Hide stack trace text` switch: removes Bugsink's own stack trace text in the backend.
- The plugin no longer follows redirects from Bugsink. A Bugsink URL that redirects now fails the test with `did not answer like a Bugsink API (HTTP 30x)`; enter the address it redirects to.
- The `logs_query` field of event tables is hidden; the Loki link still reads it.
