# Internal packages

The simulator component uses the public root package `simulation` for the
authoritative fleet, latest reports, in-memory messages, and metadata.
`simdriver` is the real-time driver that paces that engine. `simulator` holds
the HTTP API, engine-to-wire conversion, manager composition, and runtime.
`simulatorapi` holds the simulator API wire types shared with its consumers.

The display component is `display`: a simulator HTTP client, the NMEA-derived
map projection, the display API, and its runtime. It never imports simulator
state or runtime packages.

Shared packages: `ais` encodes and decodes position reports, `ui` renders
stateless HTML pages and serves static assets, `httpserver` runs HTTP servers
with bounded shutdown, and `cli` validates listen addresses. `app` composes both
components for the combined executable.

Package documentation lives in `doc.go`. Allowed package dependencies are
defined in [`.go-arch-lint.yml`](../.go-arch-lint.yml).

See the [project README](../README.md) for the running architecture and API.
