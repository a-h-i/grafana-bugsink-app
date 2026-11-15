# Bugsink

Connects Grafana to a self-hosted [Bugsink](https://www.bugsink.com/) error tracker.

This version only connects: it adds a Bugsink data source that checks the Bugsink address and the API token you give it, and tells you what is wrong when the check fails. Browsing issues and events comes in later versions.

## Requirements

- Grafana 13.2.0 or later.
- A Bugsink instance that Grafana's server can reach over HTTP or HTTPS.
- A Bugsink API token with the capabilities `projects:read`, `issues:read` and `events:read`.

## Create an API token

Create the token in Bugsink. A token has capabilities, and it can be bound to one project. Give it the three capabilities above. A token bound to a project lets the plugin see that project only. The Bugsink documentation, <https://www.bugsink.com/docs/>, has the exact steps for your Bugsink version.

## Add the data source

1. Enable the app if it is not enabled yet: in Grafana, open Administration > Plugins and data > Plugins, choose Bugsink and enable it.
2. Open the Bugsink app. It lists your Bugsink data sources. With none, it shows `No Bugsink data source yet.` and the link `Add a Bugsink data source`.
3. Follow that link, or open Connections > Data sources > Add new data source, and choose **Bugsink**.
4. Fill in the two fields:
   - **Bugsink URL**: the address of Bugsink itself, for example `https://bugsink.example.com`. Do not add `/api`.
   - **API token**: the token you created. Grafana stores it encrypted and does not show it again.
5. Click `Save & test`. A data source provisioned from a file has a `Test` button instead.

The test makes at least one read request per capability and reports the result. It stops at the first answer that ends the check: a connection error or timeout, a rejected token, an answer that is not Bugsink's, or an unexpected answer to a capability check.

## Test results

A successful test says:

- `Connected to Bugsink. Projects visible to this token: <names>.` Project names are listed in the order Bugsink returns them. With more than ten projects, the first ten are listed, followed by `and <n> more`.
- `Connected to Bugsink. This token sees no projects.` The connection and capabilities are fine, but the token (or its project binding) gives access to no project.

A failed test says one of the following. `<url>` is the URL as you typed it.

| Message                                                                                                                      | What to do                                                                                                                                                                                                                                                    |
| :--------------------------------------------------------------------------------------------------------------------------- | :------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `Enter the Bugsink URL.`                                                                                                     | Fill in the Bugsink URL field.                                                                                                                                                                                                                                |
| `The Bugsink URL must start with http:// or https://.`                                                                       | Add the scheme to the URL.                                                                                                                                                                                                                                    |
| `Enter an API token.`                                                                                                        | Fill in the API token field.                                                                                                                                                                                                                                  |
| `The data source settings are invalid.`                                                                                      | Grafana could not build an HTTP client from the data source's settings. The editor has only the URL and the token, so this can only come from settings supplied through provisioning or the API, for example a TLS certificate setting. Correct that setting. |
| `Cannot reach Bugsink at <url>: <cause>.`                                                                                    | Grafana's server could not connect. The cause says why: a refused connection, a DNS failure or a TLS error. Check the address, the network path from the Grafana server to Bugsink and the certificate.                                                       |
| `Bugsink at <url> did not answer within 10 seconds.`                                                                         | Check that Bugsink is running and not overloaded, and that nothing between Grafana and Bugsink drops the traffic.                                                                                                                                             |
| `Bugsink rejected the API token. Check that it was copied in full and has not been deleted.`                                 | Paste the token again, or create a new one in Bugsink.                                                                                                                                                                                                        |
| `<url> did not answer like a Bugsink API (HTTP <status>). Check the URL: it is the address of Bugsink itself, without /api.` | The address answers, but not as Bugsink's API does. Check the URL for a wrong host, port or path prefix, or a proxy in front of Bugsink.                                                                                                                      |
| `The API token lacks: <capabilities>.`                                                                                       | Add the named capabilities to the token in Bugsink, or use a token that has them. The test names every missing capability at once.                                                                                                                            |
| `Bugsink answered HTTP <status> to a check for <capability>.`                                                                | Bugsink answered a capability check in a way the plugin does not expect. Check the Bugsink version and its server log.                                                                                                                                        |

## Get help

Report problems at <https://github.com/a-h-i/grafana-bugsink-app/issues>.
