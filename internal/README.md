# Internal packages

The simulator component uses `simulation` for the authoritative fleet, latest
reports, in-memory messages, and metadata, and `simulator` for its HTTP API,
manager composition, and runtime. `simulatorapi` holds the simulator API wire
types shared with its consumers.

The display component is `display`: a simulator HTTP client, the NMEA-derived
map projection, the display API, and its runtime. It never imports simulator
state or runtime packages.

Shared packages: `ais` encodes and decodes position reports, `ui` renders
stateless HTML pages and serves static assets, `httpserver` runs HTTP servers
with bounded shutdown, and `cli` validates listen addresses. `app` composes both
components for the combined executable.

The domain/application subpackages and the targets, networking, management, and
visualization folders hold the original design contracts. Runtime services use
the concrete packages directly. Package documentation lives in `doc.go`.

See the [project README](../README.md) for the running architecture and API.
