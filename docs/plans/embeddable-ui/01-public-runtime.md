---
title: "01 - Public runtime and source contracts"
dependencies: []
effort: "L"
complexity: "high"
---

## Status

Implemented and verified on 2026-09-15.

- Public `simulator`, `simulatorapi`, and `display` packages replace their internal
  counterparts. Driver mechanics and the deterministic clock seam remain internal.
- `simulator.New(Config)` owns an engine and exposes commands, snapshots, `API()`,
  and single-use `Run(ctx)`. Terminal runtimes reject writes; snapshots remain readable.
- `display.New(source)` borrows a two-method `Source`. `Simulator` implements it
  locally; `display.NewHTTPSource(HTTPConfig)` implements it remotely.
  `display.NewClient(origin)` is the convenience constructor owning an HTTP source.
- Both paths share semantic validation and AIS decoding. HTTP additionally checks
  framing, required field presence, canonical decimal strings, and body limits.
- Existing combined-mode HTTP composition remains until step 05. Fixed UI/API
  routes remain until step 03. The integration scenario and display benchmark now
  live in `simulator/scenario_test.go` alongside their private deterministic seam.
- A regression fix rejects canceled count/speed commands even while paused or
  when no real time has elapsed. Cancellation between catch-up chunks still keeps
  delivered chunks and leaves the requested command unapplied.

Verification:

- `task all`: passed, including 617 tests, format, vet, deadcode, architecture, and lint.
- `go test -race ./simulator ./display ./internal/simdriver ./internal/app`: passed.
- `simulator/example_test.go` compiles and runs as an external test package using
  only public library imports. Local/remote parity, malformed local responses,
  source errors, cancellation, ownership, duplicate Run, and terminal writes have
  regression coverage.

Go and lint caches used task-specific directories under the system temporary
directory because the sandbox cannot write the default user cache locations.

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
