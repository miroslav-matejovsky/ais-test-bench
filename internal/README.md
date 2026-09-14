# Internal packages

The running test bench uses `app` for lifecycle and composition, `simulation`
for the shared fleet, latest reports, in-memory messages, and metadata,
`simulatorapi` for the simulator API wire types, `ais` for position report
encoding, and `ui` for HTML, static assets, and the JSON API.

The domain/application subpackages and the targets, networking, management, and
visualization folders hold the original design contracts. Runtime services use
the concrete packages directly. Package documentation lives in `doc.go`.

See the [project README](../README.md) for the running architecture and API.
