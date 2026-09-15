---
title: "02 - Application logger injection"
dependencies: ["01-public-runtime.md"]
effort: "M"
complexity: "medium"
---

## Status

Implemented on 2026-09-15.

- `simulation.Config.Logger` added. The engine emits no records; logger choice
  never changes results.
- `simulator.Config.Logger` takes precedence over `Simulation.Logger` and replaces
  it for the owned engine; nil falls back to it, then `slog.Default()`. The
  simulator derives `component=simulator` once in `New`. `NewAPI` is replaced by
  `Simulator.API`; `NewHandler(sim)` and `Serve(ctx, ln, sim, handler)` use the
  simulator's logger, so instances cannot share loggers by mistake.
- `display.NewAPI`, `NewHandler`, and `Run` resolve nil to `slog.Default()` and
  derive `component=display` once per call.
- Handler error sites use contextual slog methods. Audit found no swallowed errors
  or duplicate records; the `httpserver` bridge already preserves handler
  attributes, groups, and levels, and net/http provides no request context there.
- `internal/logtest` provides the capturing handler used by tests.

Verification: `task all` passed (629 tests, vet, fmt, deadcode, arch-lint, lint).
`go test -race` passed for `simulation`, `simulator`, `display`, and `internal/...`.

## Implementation

- Add the README's logger contract to `simulation.Config` and new public runtime,
  source, handler, and UI configs as those are introduced. Keep DTO-only packages
  free of logging configuration.
- Normalize nil once per constructor; pass derived loggers to existing internal
  handlers. Keep process logger creation in `cmd`.
- Audit existing error sites and `httpserver`'s slog bridge for caller attributes,
  groups, contextual logging, duplicate records, and swallowed errors.
- Document precedence, defaults, levels, and quiet normal operation in package docs.
  Avoid a generic logging framework or artificial engine log events.

## Verification and acceptance

- Capture records with a test slog handler. Trigger rendering/API failures and
  verify the supplied logger receives caller fields, component, error, and context.
- Two independently configured instances do not send records to each other's logger.
- Nil works through the documented default; constructing instances never changes it.
- Existing deterministic engine scenarios produce identical results with different
  loggers. Disabled log levels do not change behavior.
- No consumer logger callback executes under an engine/driver state lock.
