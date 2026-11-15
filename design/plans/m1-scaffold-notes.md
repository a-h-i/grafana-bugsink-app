# M1 scaffold notes

`create-plugin` 9.0.3 generated every file in the plan's list; no file is missing or named differently.

Differences from the plan's assumptions:

- The Go SDK's build needs no `tools.go`: `github.com/magefile/mage` v1.17.2 is already in `go.mod` through the SDK, and `go run github.com/magefile/mage -v build:linux` works.
- Grafana starts the nested data source's backend from `dist/datasource/gpx_bugsink_linux_amd64` with `executable: gpx_bugsink`; the `../gpx_bugsink` fallback was not needed.
- The webpack config copies only the app's logos, so the nested data source's `info.logos` (`img/logo.svg`) resolves to a 404 under `public/plugins/ahmedismail-bugsink-datasource/`.
- End-to-end authentication: the scaffold's `auth` project cannot log in against the Compose Grafana (`GF_AUTH_DISABLE_LOGIN_FORM`, "auth.client.notConfigured"). The root `playwright.config.ts` turns the `auth` project into an empty one and drops the stored session, so the tests rely on anonymous Admin; `createDataSourceConfigPage` creates and deletes data sources with it, so `docker-compose.yaml` is unchanged. Each test still logs a non-fatal "Could not login" warning from the plugin-e2e fixture.
- A provisioned data source is read-only: its page has a `Test` button instead of `Save & test`, so the provisioned-data-source test clicks `Test` and checks the health response.
