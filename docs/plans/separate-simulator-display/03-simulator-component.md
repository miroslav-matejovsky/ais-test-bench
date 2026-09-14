---
title: "03 - Extract the simulator API, manager, and standalone command"
dependencies: ["01-api-contracts.md", "02-engine-state-and-metadata.md"]
effort: "M"
complexity: "medium"
---

## Outcome

`cmd/simulator` starts one engine with the manager page and simulator API.
The same API handlers and runtime building blocks can later be used in combined
mode.

## Implementation work

1. Create `internal/simulator` with a small concrete HTTP handler and lifecycle
   implementation. Move simulator endpoint responsibilities out of
   `internal/ui/api.go`. Map engine snapshots to `internal/simulatorapi` types.
2. Implement all four routes in step 01. Preserve bounded request parsing and
   count validation. Keep NMEA framing exactly as generated; do not strip CRLF
   to make the manager output prettier. Browser rendering can trim display text.
3. Separate API route construction from page/static composition. Combined mode
   needs to mount the same API on its public and private listeners without
   constructing another engine. Provide concrete constructors or handler methods
   for that actual reuse, not a generic routing plugin system.
4. Refactor `internal/ui` toward stateless rendering. A manager page handler
   receives page/navigation data and renders assets; it does not own a simulator
   pointer. Preserve the existing renderer and htmx status fragment behavior.
5. Update `manager.js` for the history envelope, latest-report fleet response,
   and metadata limits. Populate count bounds and history capacity from metadata
   while retaining server-side validation as authoritative. Preserve current
   input during background refresh and show actual fleet count separately.
6. Keep recent AIS messages and the raw-JSON history link. Display messages use
   the last ten history entries; do not decode reports just to show raw NMEA.
7. Add `cmd/simulator/main.go`, `doc.go`, and focused CLI tests. Default `-addr`
   to `localhost:8000`; require a valid explicit host and port. Start one engine,
   serve the manager/API, and stop on process cancellation.
8. Serve `/` as a redirect to `/manager`. Keep `/status` useful for the simulator.
   Standalone navigation does not advertise a local display route.
9. Reuse current HTTP header/shutdown timeout behavior. Listener ownership must
   be explicit: construction failure closes owned listeners, runtime failure
   stops the engine, and shutdown waits for it. Do not log a routine failure at
   multiple layers or discard the underlying error.
10. Document the new command and packages in `doc.go`. Preserve the existing
    combined entry point until step 06; remove migration adapters there.

## Verification

- Handler tests exercise all simulator endpoints, invalid input, exact NMEA
  preservation, metadata, HTTP methods, and empty fleet responses.
- A runtime test starts simulator handlers on a bound ephemeral listener and
  retrieves manager HTML and API data without creating a display component.
- Manager tests cover the history envelope and metadata-driven input limits.
- CLI tests cover defaults, invalid/empty hosts, invalid ports, unknown flags,
  and extra positional arguments.
- Cancellation and startup failure tests verify owned listeners close and the
  tick loop exits. Coordinate with channels; do not use wall-clock sleeps.

## Acceptance criteria

- `go run ./cmd/simulator` serves a usable manager and all simulator endpoints.
- A regular HTTP client can retrieve and parse complete AIS sentences and
  metadata without starting the display.
- Simulator mode creates exactly one simulation engine and no display client.
- Shared UI rendering has no direct engine-state dependency in the new path.
- The simulator remains usable while a display client is disconnected or absent.

## Effort and complexity

Medium effort and complexity. Most work is moving existing handlers and updating
their data shapes. Route ownership and resource ownership must remain explicit.
