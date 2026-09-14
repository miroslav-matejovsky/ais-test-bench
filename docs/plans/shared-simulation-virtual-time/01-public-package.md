---
title: "01 - Extract the public simulation core"
dependencies: []
effort: "L"
complexity: "medium"
---

## Outcome

Other Go modules can construct and consume the same engine used by the internal
simulator. This step establishes package ownership before changing time behavior.

## Implementation work

1. Add `simulation/doc.go`, `types.go`, and `simulator.go`. Move the concrete
   fleet, movement, AIS encoding orchestration, history, and metadata behavior
   from `internal/simulation/simulator.go`. Preserve algorithms and defaults
   during extraction. Keep time migration contained to the next step.
2. Introduce package-owned value types corresponding to current fleet/history
   results. Copy catalog/settings values into public types. Public declarations
   must not mention an `internal` type, even through an alias or embedded field.
3. Keep codec implementation in `internal/ais`, imported privately by the public
   engine. Do not move or copy the codec, or expose its `Position` type in the
   library API. Verify external compilation instead of assuming every internal
   dependency must become public.
4. Add explicit conversions in `internal/simulator` from public values to
   `internal/simulatorapi` response types. Preserve CRLF exactly and preserve
   non-null empty arrays and nullable history bounds. Keep display dependent on
   HTTP wire types and codec, not on the engine.
5. Reduce `internal/simulation` to delegation and real-time lifecycle. Remove
   duplicate fleet, history, encoding, and movement state. Update composition,
   handler signatures, imports, and tests together so commands still build.
6. Move core tests into the public package, preferably `simulation_test`.
   Retain narrowly scoped same-package tests for forced failure paths. Preserve
   fleet identity, removal/retention, checksum, timestamp, and snapshot-copy tests.
7. Document the import path and package responsibilities. Update root and internal
   README architecture tables and package docs as the extraction lands. Keep the
   older design scaffolding explicitly separate; no broad scaffolding cleanup.

## Verification

- Compare API response values before and after extraction for a fixed ID, time,
  and seed. Check sequences, MMSIs, history bounds, and exact sentence strings.
- Compile an external-package example importing only the public simulation API
  and standard library types. Final external-module proof is in step 05.
- Run focused public engine, simulator API, app, and display tests. Confirm no
  transport or UI dependency in `go list -deps ./simulation` except dependencies
  already required by the private codec; inspect any unexpected dependency.
- Use `gopls` references for migrated symbols. Verify callers no longer depend on
  the old concrete core and no generation implementation remains duplicated.

## Acceptance criteria

- One concrete core owns generation and history.
- Combined and standalone simulator paths use that core.
- Library callers can name and inspect every public input and result type.
- Snapshot mutation cannot modify simulator-owned state.
- Existing endpoint shapes and AIS bytes remain intact at this extraction step.

## Estimate

Approximately 1-2 days. Main risk: hidden coupling to wire types in tests and
handlers. Keep conversions explicit rather than introducing a mapping framework.
