# Impact, feasibility, and risks

## Feasibility

The work is feasible within the current module and dependency set. The engine
already centralizes generation, uses a seeded random source, provides copied
snapshots, and accepts explicit timestamps. The display already reads metadata
on every poll. Extraction and virtual time can build on those boundaries.

The main change is replacing an absolute-time, one-report-per-call update with
a canonical virtual reporting schedule and a separate real-time driver. This
requires deliberate changes to tests that currently encode the simplified
large-advance behavior. It does not require a scenario framework, scheduler
library, new network transport, or a second module.

## Impact by area

| Area | Impact |
| --- | --- |
| Public API | New root import path and concrete documented types. Callers can generate traffic synchronously and collect complete batches. |
| Internal engine | Core moves out; remaining package becomes a small pacing/control adapter. Existing internal callers and tests migrate together. |
| Time | All simulation/report timestamps become virtual. Real time remains responsible for pacing, request deadlines, shutdown, uptime, and UI freshness. |
| HTTP | Add speed mutation and mutable clock metadata. Existing fleet/history sentence framing and bounded snapshot semantics remain. |
| Display | Add validated clock metadata to the existing projection and render it; retain the HTTP and NMEA boundaries. |
| Manager | Add controls, periodic metadata refresh, run consistency, and stale-response protection. |
| Tooling | Public methods can be unreachable from command binaries; preserve useful deadcode checking with narrow documented exceptions if necessary. |
| Documentation | Root architecture, package ownership, public examples, time semantics, and API examples all need updates. |

## Risks and mitigations

| Risk | Consequence | Mitigation and verification |
| --- | --- | --- |
| Wire types leak into public API | External programs cannot use a clean library contract. | Public value types, explicit internal mapping, and independent-module compilation. |
| Multiple clocks or generators survive extraction | Commands and reports disagree about time or behavior. | One core, one runner per application run, reference inspection, parity tests. |
| Lost ticker notifications | Reports disappear or simulation runs too slowly. | Measure monotonic elapsed time and process every canonical virtual tick. |
| Float rounding depends on chunk size | Same scenario produces different timestamps or bytes. | Hundredths speed normalization, checked integer scaling, remainder carry, split/combined tests. |
| Count/speed races with heartbeat | Reports receive stale dates or elapsed time uses the wrong speed. | Sample and settle under one driver lock, then apply command; fake-clock boundary tests. |
| Incomplete rollback | Failed operations alter later traffic. | Stage all core state including PCG and remainder; failure/retry control comparison. |
| High speed multiplies work | Lock contention, slow API, short history coverage. | Bounded public batches, fixed upper speed, bounded catch-up, targeted maximum-load check. |
| Long host suspension | Unbounded catch-up delays cancellation and API. | Explicit one-hour virtual catch-up limit and runtime error, documented with maximum-rate implications. |
| Metadata becomes mutable | Consumers cache stale speed or expect snapshot equality. | Updated contract, periodic metadata polling, time validation, separate-read consistency documentation. |
| Browser compares custom date to real clock | Future/past scenarios appear stale or disconnected. | Use real receipt time for freshness; render full virtual UTC separately. |
| Poll/write/restart races | UI shows old speed or combines runs. | Request generation guard, coherent run checks, controlled-response browser verification. |
| Broader external reuse pressure | Plan grows into arbitrary vessels, streaming, playback, and routing. | Deliver existing traffic model plus explicit time and batches; defer extra scenario features. |

## Compatibility and limits

This project is in experimentation. Migrate internal callers and tests directly;
do not preserve the old absolute `Advance(now)` API with a compatibility wrapper.
The public API is new. Preserve useful existing HTTP response fields while
documenting their virtual-time meaning. Update repository consumers in the same
change, including fixtures that assume immutable metadata.

Replay is reproducible for the same engine implementation, config, and ordered
commands. Seed alone is insufficient if start time, count changes, or command
timing differ. No guarantee is made about bytes across future algorithm changes.

The snapshot HTTP API can miss reports at high speed because retention is finite.
That existing tradeoff becomes more visible. The Go package's returned batches
provide complete per-operation output; a lossless HTTP/TCP/UDP stream is separate
future work. Reset requires constructing a new engine; backwards time mutation
is excluded to keep sequence and report histories unambiguous.

## Review confidence

The plan is grounded in the current source, package docs, task scripts, and
existing tests. B1-B3 are code-review findings with specified regressions still
to be executed. No application code was changed, no tests or `task all` were run,
and no runtime performance claim was verified during this planning task.
