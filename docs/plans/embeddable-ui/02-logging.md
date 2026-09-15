---
title: "02 - Application logger injection"
dependencies: ["01-public-runtime.md"]
effort: "M"
complexity: "medium"
---

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
