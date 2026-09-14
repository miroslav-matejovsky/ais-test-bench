---
title: "03 - Integrate real-time pacing and management API"
dependencies: ["02-virtual-time.md"]
effort: "L"
complexity: "high"
---

## Outcome

Both application modes pace the shared engine with real elapsed time and expose
coherent virtual time metadata and speed control through HTTP.

## Implementation work

1. Finish the thin driver in `internal/simulation`, with a fixed real heartbeat,
   monotonic elapsed-time accounting, serialized management commands, explicit
   single-run lifecycle, and the bounded catch-up policy in the plan README.
2. Add only the internal clock/timer seam needed for fake elapsed-time and wakeup
   tests. The seam must support prompt context cancellation. Do not reuse the
   old scenario application's broader interfaces or publish a clock framework.
3. Settle elapsed time under the old speed before a valid speed/count change.
   Sample time after acquiring the lock, so an earlier sampled request timestamp
   cannot become a stale command. Validate input before settlement. Return both
   mutation and settlement failures with context; propagate fatal runtime errors.
4. Update `internal/simulator/run.go` and `internal/app/app.go` to create public
   config with real startup UTC, fresh ID/seed, one vessel, and speed 1, then
   create one driver. Keep one driver shared by combined mode's public/private
   listeners. Retain the existing display HTTP boundary and shutdown order.
5. Update `internal/simulatorapi/types.go` and `doc.go` for `TimeState`, speed
   request/limits, mutable metadata, timestamp meanings, and real/virtual units.
   Keep public-package-to-wire conversion explicit in the simulator layer.
6. Implement `PUT /api/time` and update count mutation to call the driver's
   serialized control path. Return metadata after a speed change. All GET routes
   remain read-only and return copies of committed state.
7. Update lifecycle and HTTP fixtures for the new config and clock semantics.
   Existing tests that advance one hour expecting a single report must instead
   drive bounded virtual durations and assert the documented fixed cadence.
8. Update simulator and app package documentation in the same step, including
   virtual `startedAt`, real timeout ownership, catch-up failure policy, and
   metadata consistency across separate reads.

## Verification

- Fake-clock runner tests for regular and late heartbeats, coalesced wakeups,
  count/speed changes between heartbeats, a change at a deadline, pause/resume,
  no-op speed, and real clock regression handling. A regressed injected sample
  must not rewind the baseline or double-count later elapsed time.
- Compare driver-generated output against a manually driven public engine with
  identical elapsed chunks and commands. Use channels/barriers, not sleeps.
- Verify large catch-up chunking preserves all scheduled ticks; over-limit
  backlog fails before catch-up; cancellation stops between complete chunks.
- Verify duplicate Run rejection, cancellation before first wake, cancellation
  while paused, encoding failure propagation, and no leaked timer/goroutine.
- API table tests: valid fractional/zero speed; omitted/null/wrong-type values;
  unknown fields; multiple JSON objects; trailing data; oversized body;
  media type; precision/range; methods; no-store headers; unchanged state on
  invalid input. Compare time and all previous outputs before/after rejection
  with a frozen fake clock.
- Confirm count creation uses current virtual time for custom past/future dates.
  Verify AIS UTC second and envelope timestamp agree after a rate change.
- Test metadata's required fields and explicit zero speed in JSON. Retest empty
  arrays, bounds, CRLF preservation, and error classification.
- Exercise speed updates through both combined listeners and the standalone API.
  Verify they control the same shared engine in combined mode.
- Run affected tests with race detection; verify HTTP timeout and shutdown still
  use real time while simulation speed is zero or extreme.

## Acceptance criteria

- Wall pacing is isolated to the driver; all generation uses the public core.
- Speed changes preserve elapsed time at the old rate up to application time.
- Fleet changes and ticks cannot introduce out-of-order virtual reports.
- HTTP metadata exposes committed virtual time and validated effective speed.
- Real lifecycle remains responsive at pause and supported maximum speed.

## Estimate

Approximately 2-3 days. Main risks: clock sampling order, delayed tick processing,
and propagation of driver errors through existing shutdown paths.
