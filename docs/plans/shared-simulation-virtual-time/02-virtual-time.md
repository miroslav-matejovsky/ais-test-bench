---
title: "02 - Implement deterministic virtual time"
dependencies: ["01-public-package.md"]
effort: "L"
complexity: "high"
---

## Outcome

The public engine implements the configuration, advancement, speed, batching,
and timestamp semantics defined in the plan README without wall-time dependencies.

## Implementation work

1. Finalize `Config` and `New`. Validate ID, start date, count, and speed before
   consuming random state or generating reports. Make zero count/seed/speed
   explicit valid values. Normalize simulation timestamps to UTC.
2. Add authoritative virtual clock state and `TimeState`. Keep the latest report
   deadline separate from fleet mutation timestamps. Remove the old absolute
   timestamp argument from `SetCount` and replace `Advance(now)` with duration
   advancement. Update callers and test fixtures as part of this migration.
3. Implement fixed virtual one-second tick iteration, partial-tick accumulation,
   stable vessel ordering, and immediate birth reports. Return complete report
   batches while retaining only the newest 1,000 in history. At 100 vessels a
   60-second advance returns 6,000 reports even though history retains 1,000.
4. Implement speed normalization, exact integer hundredths scaling, remainder
   carry, pause, and `Elapse`. Keep `Advance` independent of multiplier. Reject
   negative and oversized durations, bad precision, overflow, and invalid dates
   before committing. Define contextual errors for validation and limits.
5. Stage random state using an owned PCG value/source that can be copied and
   committed with the rest of the operation. Do not keep a staged `Rand` pointing
   at the original source. Include clock/remainder/cadence state in rollback.
6. Preserve existing movement math at canonical ticks. Do not also redesign
   navigation, coast avoidance, vessel types, or AIS message selection.
7. Document atomicity, explicit command ordering, report cadence, pause behavior,
   maximum batch duration, and full versus retained reports in `doc.go` and API
   comments. Explain that full timestamps are virtual and AIS UTC seconds come
   from those same instants.

## Verification

Use table-driven tests with fixed IDs, UTC instants, seeds, and durations. No
sleep or real timer is needed.

- Initial count 0 and 1, count limits, invalid ID/start date/speed, and seed 0.
- 0x, 0.01x, 0.5x, 1x, 2x, and 100x. Reject negative values, NaN, infinity,
  over-limit rates, and unsupported precision. Exercise float normalization.
- Compare one 10-second advance with ten one-second advances and fractional
  splits. Compare complete output batches and final fleet/history/time.
- Compare split versus combined `Elapse`, including sub-nanosecond remainders
  and a speed change. Test explicit steps interleaved with elapsed-time calls.
- Pause, elapsed time while paused, explicit step while paused, and resume.
  Verify no paused-duration catch-up and no report emitted just by setting speed.
- Spawn and remove vessels between ticks and at exact boundaries. Confirm
  existing ships do not move extra time when a new vessel is created.
- Clock advancement with an empty fleet, then creation at the current instant.
- Minute/day/year rollover, UTC normalization, zero/negative durations, maximum
  batch size, dates near the accepted limits, and checked duration arithmetic.
- Reproduce B1 with the corresponding ordered virtual commands; all report
  timestamps are nondecreasing and count changes cannot suppress due ticks.
- Extend the forced failure test for B2: fail midway through creation, restore
  the forced fault, then compare successful creation with an untouched control
  engine, including random coordinates, courses, speeds, MMSIs, and reports.
- Force failure during a multi-tick advance and cancellation between ticks;
  verify complete rollback, including time and scaling remainder.
- Check full message batch ordering, sequence overflow handling, bounded history,
  copies of nested values, and AIS second bits matching report timestamps.
- Run public package tests under the race detector with concurrent readers and
  serialized deterministic mutations; separately exercise concurrent mutations
  for safety without asserting their scheduling order.

## Acceptance criteria

- Equivalent ordered scenarios produce identical report bytes and final state.
- The public engine has no wall-clock reads, timers, hidden IDs, or goroutines.
- Speed changes movement per real second but leave AIS speed in knots unchanged.
- B1 and B2 have regression coverage and are fixed in the shared implementation.
- Failed public operations do not change any future observable result.

## Estimate

Approximately 2-3 days. Main risks: rounding, incomplete rollback, and report
ordering at boundaries. Keep one explicit tick loop and one state commit path.
