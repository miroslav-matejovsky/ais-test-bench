---
title: "01 - Public runtime and source contracts"
dependencies: []
effort: "L"
complexity: "high"
---

## Implementation

- Extract public `simulator`, `simulatorapi`, and `display` from the current internal
  implementations. Keep driver mechanics internal and expose explicit simulator
  construction, serialized commands, snapshots, API handler, and pacing lifecycle.
- Define the display-owned observation source with public wire DTOs, history
  request, and error categories. Implement local simulator and remote HTTP sources.
- Move semantic validation/projection out of the HTTP-only client path. Retain
  remote JSON/framing validation and body limits. Both sources enforce cancellation.
- Define source/client ownership, single-run behavior, writes after termination,
  clean cancellation, and error propagation. Avoid constructor side effects.
- Update imports and architecture rules as packages move. Add `doc.go` files with
  complete ownership, concurrency, source, and error contracts.

## Verification and acceptance

- Public examples compile without internal imports or inaccessible signature types.
- Existing engine, station, driver, and display validation tests pass after moves.
- Test local/remote parity using fixed snapshots, including raw NMEA, station
  selection, sequence strings, missing station, stale run, and malformed source data.
- Test cancellation, duplicate Run calls, post-stop commands, and catch-up failure
  with deterministic clocks and synchronization. Constructors open no listeners.
- Display imports neither `simulation` nor the simulator implementation.
