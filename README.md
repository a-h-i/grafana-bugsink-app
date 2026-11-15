# Bugsink App Plugin for Grafana

View and triage [Bugsink](https://www.bugsink.com/) issues inside Grafana, and jump from an error event to its Tempo trace and Loki logs.

Status: version 0.1.0 is in development and connects only. The Bugsink data source takes a Bugsink URL and an API token and checks them; there is no issue browsing, triage or trace linking yet. The user guide is [`src/README.md`](src/README.md). See the [spec](design/bugsink_grafana_plugin_spec.md) and the [implementation roadmap](design/implementation_roadmap.md) for what comes next.

Local development stack (Bugsink, Grafana, Tempo, Loki, sample app): see [`docker/README.md`](docker/README.md), and the Development section below.

## Development

Prerequisites: Node.js as in `.nvmrc`, Go as in `go.mod`, and Docker with Compose.

Build the frontend and the backend (Grafana mounts `dist/`):

```sh
npm ci
npm run build
npm run build:backend
```

Start the stack (Grafana on <http://localhost:3300>, Bugsink on <http://localhost:8800>) and check it:

```sh
docker compose up -d --wait
python3 scripts/verify_stack.py
```

See [`docker/README.md`](docker/README.md) for the stack's services, ports and seeded tokens.

Unit tests and checks:

```sh
npm run typecheck
npm run lint
npm run test:ci
go vet ./pkg/...
go test ./pkg/...
scripts/check_spdx.sh
scripts/check_licenses.sh
```

End-to-end tests run in a browser against the running stack. Install the browser once with `npx playwright install chromium`, then:

```sh
GRAFANA_URL=http://localhost:3300 npm run e2e
```

Regenerate the Bugsink API client in `pkg/bugsink/internal/gen/` from `bugsink-api/openapi.yaml`:

```sh
go generate ./pkg/bugsink/...
```

## License

Copyright (C) 2026 Ahmed Ismail

This program is free software: you can redistribute it and/or modify it under the terms of the GNU General Public License as published by the Free Software Foundation, either version 3 of the License, or (at your option) any later version.

This program is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See [LICENSE](LICENSE) for details.

`bugsink-api/openapi.yaml` is Bugsink's OpenAPI schema under its own licence, not this one; see [`bugsink-api/NOTICE.md`](bugsink-api/NOTICE.md).
