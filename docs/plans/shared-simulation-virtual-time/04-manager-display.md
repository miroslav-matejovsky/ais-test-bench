---
title: "04 - Add Manager controls and Display time"
dependencies: ["03-runtime-api.md"]
effort: "M"
complexity: "medium"
---

## Outcome

Manager changes simulation speed. Both pages periodically show the current
virtual UTC time and effective speed from the simulation API.

## Implementation work

1. Add time values to `internal/display/types.go` and projection logic. Extend
   upstream metadata validation for required time, nonnegative elapsed, valid
   speed, and paused/speed agreement. Validate presence separately from numeric
   zero. Preserve existing full-fleet failure behavior and restart checks.
2. Reuse `Client.Fleet`'s existing concurrent metadata fetch. Forward the validated
   time snapshot in `/display/api/vessels`; do not add another browser origin,
   background poller, or independent display clock.
3. Add a clock/status area to `display.tmpl` and render it in `display.js` from
   successful responses. Show full UTC date/time, multiplier, and pause status.
   Continue real polling at speed 0. Keep real receipt time for connection
   freshness and retain the last view with a stale indication after failure.
4. Add a separate labeled speed form and applied time status to `manager.tmpl`.
   Use a number input with metadata-supplied range/step, an Apply button, and
   concise help explaining that 0 pauses. Include convenient 0.5x, 1x, and 2x
   choices only if they simplify use without extra state machinery.
5. Update `manager.js` to poll metadata each cycle, validate run IDs before
   combining responses, and refresh applied speed/count from server state.
   Preserve dirty or focused form values; show actual effective speed separately
   from the user's draft. Disable the relevant submit button during a write and
   show server-confirmed success or a contextual error.
6. Protect write responses against stale polls with a small request generation
   counter or discarded pre-write poll result. Do not let initialization or a
   poll re-enable a button while its write is pending. On run change, invalidate
   old requests and reset draft/initialization state before rendering the new run.
7. Update CSS only as needed for readable controls and status. Use accessible
   labels and live status text. Update `internal/ui/doc.go` and display API docs
   for mutable metadata and real polling versus virtual report time.

## Verification

- Display projection tests cover custom dates, fractional speed, pause, missing
  time fields, invalid time/speed, and matching/mismatched run IDs. Same-run
  fleet/metadata from adjacent ticks must remain valid.
- API tests prove time forwarding uses metadata and navigation still comes from
  NMEA, including unavailable navigation fields and exact existing error codes.
- Template tests verify labeled controls, disabled startup state, script assets,
  and status elements. Do not mistake HTML presence checks for behavior coverage.
- Browser checks with controlled responses cover 0.5x/2x/0x, unsaved edits, failed
  writes, delayed polls after successful writes, multiple tabs, restart during a
  refresh, and reconnect. Reproduce B3 by returning different simulation IDs for
  fleet/history/metadata and assert the page does not render a mixed run.
- Use an available browser automation harness with mocked fetch responses and
  fake timers for deterministic interaction tests. If none exists, retain Go
  contract/template tests and record these browser cases as an explicit manual
  acceptance checklist; do not add a Node build just for this change.
- Verify full UTC date/time across midnight, a paused but connected clock, and
  stale clock text when upstream fetch fails. Verify two tabs observe applied
  speed within the next successful real poll cycle.

## Acceptance criteria

- Manager can slow, accelerate, pause, and resume via the simulation API.
- Display receives time through its existing server-side simulator client.
- Both pages report effective shared state without overwriting unsaved edits.
- Stale responses and simulator restarts do not combine runs or undo a displayed
  successful command with an older response. B3 is covered by verification.
- Connection freshness and virtual time are visibly distinct.

## Estimate

Approximately 1-2 days. Main risks: asynchronous response ordering and preserving
user input while applying shared state updates.
