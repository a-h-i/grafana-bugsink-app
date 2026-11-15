# Bugsink

Connects Grafana to a self-hosted [Bugsink](https://www.bugsink.com/) error tracker.

It adds a Bugsink data source. The data source checks the Bugsink address and the API token you give it, and tells you what is wrong when the check fails. It also queries issues for dashboards and Explore, and the app has a page that lists issues. Events are not available yet.

## Requirements

- Grafana 13.2.0 or later.
- A Bugsink instance that Grafana's server can reach over HTTP or HTTPS.
- A Bugsink API token with the capabilities `projects:read`, `issues:read` and `events:read`.

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
5. Click `Save & test`. A data source provisioned from a file has a `Test` button instead.

The test makes at least one read request per capability and reports the result. It stops at the first answer that ends the check: a connection error or timeout, a rejected token, an answer that is not Bugsink's, or an unexpected answer to a capability check.

## Browsing issues in the app

The Bugsink app opens on a page that lists issues. You need at least one Bugsink data source.

- **Data source**: the picker at the top lists your Bugsink data sources. The default data source is selected first; without a default, the first one by name. Changing the data source clears the Project filter and, if Sort by is `Arrival order`, resets it to `Last seen`.
- **Filters**: Project, State, Sort by, Order and Search are the controls of the query editor, described under [Query controls](#query-controls). They apply when you change them; Search applies when you leave the field or press Enter.
- **Time range**: the picker at the top right. The default is the last 7 days. The page shows issues last seen within the range. The refresh button runs the query again; within 15 seconds of the same query it shows the same answer (see [Limits](#limits)).
- **Table**: 25 issues per page, with the columns Issue, Type, Message, Transaction, Project, Events, First seen, Last seen and Status. The Issue column links to the issue in Bugsink, in a new tab, as described under [Link to Bugsink](#link-to-bugsink).
- **States**: on the first load and after you change the data source, the page shows `Loading issues...`. A later load keeps the table on screen and shows a loading bar above it. When nothing matches, it shows `No issues match these filters in this time range.` A failed read shows the error message.

The address bar holds the whole state: data source, filters, time range, sort and order. Copy the address to reopen the same list. The page keeps the address parameters it does not use, such as `orgId` and `kiosk`, as they are.

The page has the limits of [Limits](#limits): at most 5000 issues are read and results can be up to 15 seconds old.

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

The Issue column carries the link `Open in Bugsink`. It opens the issue's last event in a new tab, on the address in the data source's `Browser URL`, or on the `Bugsink URL` when that is empty.

### Limits

- A row is an issue whose last event arrived within the dashboard time range, between its start and its end.
- A query reads at most 5000 issues. When it stops there, the table carries the warning `Only the first 5000 issues were read. Narrow the search or the time range.`
- Results can be up to 15 seconds old. Bugsink is asked again after that.

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
| `The data source settings are invalid.`                                                                                      | Grafana could not build an HTTP client from the data source's settings. The editor has only the URL, the browser URL and the token, so this can only come from settings supplied through provisioning or the API, for example a TLS certificate setting. Correct that setting. |
| `Cannot reach Bugsink at <url>: <cause>.`                                                                                    | Grafana's server could not connect. The cause says why: a refused connection, a DNS failure or a TLS error. Check the address, the network path from the Grafana server to Bugsink and the certificate.                                                       |
| `Bugsink at <url> did not answer within 10 seconds.`                                                                         | Check that Bugsink is running and not overloaded, and that nothing between Grafana and Bugsink drops the traffic.                                                                                                                                             |
| `Bugsink rejected the API token. Check that it was copied in full and has not been deleted.`                                 | Paste the token again, or create a new one in Bugsink.                                                                                                                                                                                                        |
| `<url> did not answer like a Bugsink API (HTTP <status>). Check the URL: it is the address of Bugsink itself, without /api.` | The address answers, but not as Bugsink's API does. Check the URL for a wrong host, port or path prefix, or a proxy in front of Bugsink.                                                                                                                      |
| `The API token lacks: <capabilities>.`                                                                                       | Add the named capabilities to the token in Bugsink, or use a token that has them. The test names every missing capability at once.                                                                                                                            |
| `Bugsink answered HTTP <status> to a check for <capability>.`                                                                | Bugsink answered a capability check in a way the plugin does not expect. Check the Bugsink version and its server log.                                                                                                                                        |

## Get help

Report problems at <https://github.com/a-h-i/grafana-bugsink-app/issues>.
