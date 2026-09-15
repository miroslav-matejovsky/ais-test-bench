# Internal packages

`simdriver` paces the public `simulation` engine and serializes commands for the
public `simulator` runtime. It owns the internal clock seam and catch-up limits.
Public `simulator`, `simulatorapi`, `display`, `ui`, and `testbench` packages own
runtime APIs, wire contracts, received-traffic projection, embeddable rendering,
and composition.

Shared packages: `ais` encodes and decodes position reports, `httpserver` runs
HTTP servers with bounded shutdown and supervises serving next to pacing,
`urlpath` validates public base paths and prefixes, and `cli` validates listen
addresses. `logtest` is a test-only in-memory slog handler for logger injection
tests.

Package documentation lives in `doc.go`. Allowed package dependencies are
defined in [`.go-arch-lint.yml`](../.go-arch-lint.yml).

See the [project README](../README.md) for the running architecture and API.
