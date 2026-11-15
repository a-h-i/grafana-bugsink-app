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
