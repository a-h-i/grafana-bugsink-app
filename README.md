# Bugsink App Plugin for Grafana

View and triage [Bugsink](https://www.bugsink.com/) issues inside Grafana, and jump from an error event to its Tempo trace and Loki logs.

Status: in design. See the [spec](design/bugsink_grafana_plugin_spec.md) and the [implementation roadmap](design/implementation_roadmap.md).

Local development stack (Bugsink, Grafana, Tempo, Loki, sample app): see [`docker/README.md`](docker/README.md).

## License

Copyright (C) 2026 Ahmed Ismail

This program is free software: you can redistribute it and/or modify it under the terms of the GNU General Public License as published by the Free Software Foundation, either version 3 of the License, or (at your option) any later version.

This program is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See [LICENSE](LICENSE) for details.

`bugsink-api/openapi.yaml` is Bugsink's OpenAPI schema under its own licence, not this one; see [`bugsink-api/NOTICE.md`](bugsink-api/NOTICE.md).
