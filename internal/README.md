# Internal packages

`simdriver` paces the public `simulation` engine and serializes commands for the
public `simulator` runtime. It owns the internal clock seam, catch-up limits, and
demonstration configuration. Public `simulator`, `simulatorapi`, and `display`
packages own runtime APIs, wire contracts, and received-traffic projection.

Shared packages: `ais` encodes and decodes position reports, `ui` renders
stateless HTML pages and serves static assets, `httpserver` runs HTTP servers
with bounded shutdown, and `cli` validates listen addresses. `app` composes both
components for the combined executable. `logtest` is a test-only in-memory slog
handler for logger injection tests.

Package documentation lives in `doc.go`. Allowed package dependencies are
defined in [`.go-arch-lint.yml`](../.go-arch-lint.yml).

See the [project README](../README.md) for the running architecture and API.
