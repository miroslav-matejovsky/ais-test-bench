# Shared simulation package and virtual time

Status: Proposed implementation plan. Created 2026-09-14.

## Goal and value

Expose the existing AIS traffic engine as
`github.com/miroslav-matejovsky/ais-test-bench/simulation` so other Go programs
can generate reproducible traffic without starting a server. Make the internal
simulator use this same implementation. Introduce virtual time with a custom
start instant, explicit advancement, and adjustable speed. Expose its status
through the simulator API, show it on the display, and control speed in Manager.

This is one coordinated plan. The numbered files are implementation steps, not
separate proposals. Only this plan and the plans index are changed during planning.
Implementation and `task all` are deferred as requested.

## Current implementation and constraints

- `internal/simulation/simulator.go` owns random generation, movement, reports,
  history, metadata, and a real one-second ticker. Its exported results are
  `internal/simulatorapi` types, which external modules cannot import directly.
- `New(id, now, seed)` already makes initialization reproducible. `Advance(now)`
  accepts an absolute timestamp but emits only one report per vessel regardless
  of how many seconds elapsed. This is not a fixed virtual reporting schedule.
- `internal/simulator/api.go` supplies `time.Now()` to fleet mutations. Combined
  and standalone composition also choose wall start time and a random seed.
- `internal/display/client.go` already fetches fleet and metadata concurrently
  on every display request. The browser polls `/display/api/vessels` using a
  one-second real timer. Extend that existing path for time metadata.
- Manager fetches metadata only during initialization. It does not check run
  identity when combining fleet and history responses.
- Existing defaults are one vessel, at most 100 vessels, AIS type 1, a one-second
  reporting cadence, and 1,000 retained reports. Preserve these application
  defaults, spawn bounds, movement rules, and exact NMEA framing.
- The older domain/application/infrastructure packages are design scaffolding.
  Their scenario, seek, event bus, and clock interfaces are not runtime services.
  Do not adopt those abstractions merely because they already exist.
- `taskfile/deadcode.ps1` analyzes command entry points. Public library methods
  used only by external programs need deliberate treatment in that check.

## Scope and decisions

| Area | Decision |
| --- | --- |
| Packaging | One public root package, `simulation`, in the existing Go module. No nested module or new dependency. |
| Core | Fleet generation, movement, encoding, report ordering, bounded history, virtual time, and copied snapshots. |
| Runtime | Keep `internal/simulation` as a thin real-time driver. It owns wall pacing and lifecycle and delegates all simulation behavior to the public package. |
| Transport | HTTP API, manager, display, and process/listener lifecycle stay internal. The public package starts no HTTP, TCP, or UDP server and creates no background goroutine. |
| Codec | Reuse `internal/ais` inside the module. Public package signatures expose only public package or standard library types. Go callers do not need to import the codec. |
| Time origin | Callers supply a nonzero UTC-normalized start instant. Application startup supplies the current real instant. |
| Advancement | Explicit virtual-duration advancement plus elapsed-real-duration advancement at the configured speed. Both are synchronous and deterministic. |
| Speed | Default application speed 1x. Support 0x for pause and 0.01x through 100x in 0.01 increments. Validate the same limits in Go and HTTP. |
| Reports | One report per active vessel on every virtual one-second boundary, plus immediate creation reports. Speed scales virtual time, not reported knots. |
| UI | Manager controls speed. Manager and Display show virtual UTC date/time, effective multiplier, and paused state. Real polling continues while paused. |
| History | Keep 1,000 reports. Return all reports from each explicit generation operation so callers need not rely on polling bounded history. |
| Reset | Construct a new engine with a new run identity. Rewind, seek, saved scenarios, persistence, and replay storage are outside this change. |

Limits are proposed initial product choices, not measurements of supported
throughput. Verify 100 vessels at 100x during implementation and lower the
advertised maximum if it cannot run reliably on the development machine.

## Public package contract

Use a concrete `Simulator` and a small `Config`. Proposed signatures:

```go
type Config struct {
    ID                 string
    StartTime          time.Time
    Seed               uint64
    InitialVesselCount int
    Speed              float64
}

func New(config Config) (*Simulator, error)
func (s *Simulator) Advance(ctx context.Context, virtualDelta time.Duration) ([]Message, error)
func (s *Simulator) Elapse(ctx context.Context, realDelta time.Duration) ([]Message, error)
func (s *Simulator) SetCount(count int) ([]Message, error)
func (s *Simulator) SetSpeed(speed float64) error
func (s *Simulator) Fleet() Fleet
func (s *Simulator) History() History
func (s *Simulator) Metadata() Metadata
```

These signatures specify the intended API, not implementation code. Public
types include `Fleet`, `Vessel`, `Report`, `Message`, `History`, `Metadata`,
`TimeState`, and their settings/catalog value types. Keep HTTP request bodies
and JSON-specific conversion in `internal/simulatorapi` and `internal/simulator`.
Keep public structs free of HTTP concerns; use explicit wire conversion rather
than aliases that couple API evolution to library types.

All configuration fields are explicit. Seed 0, initial count 0, and speed 0 are
valid values, not requests for defaults. Production composition supplies count
1 and speed 1. The package performs no hidden random ID or time selection.
Keep production ID generation in internal composition. Reject an empty ID,
invalid count/speed, zero start instant, and dates outside the JSON-compatible
year range 1-9999. Check the same date range on advancement.

Returned slices and pointer values are detached from engine memory. Methods are
safe for concurrent access, but reproducibility requires callers to choose a
stable order for mutations. Scheduling competing goroutines is not deterministic.
Do not store caller contexts. An already cancelled context makes an advance fail
without change. Check cancellation between virtual ticks during longer advances.

`New` generates initial reports available through `Fleet` and `History`.
`SetCount` returns just the newly generated creation reports. `Advance` and
`Elapse` return every report emitted by that call in sequence order, including
reports already evicted from retained history. Successful calls that emit
nothing return an empty slice. Failed calls return no reports and leave state
unchanged. Returned reports can be fed directly into another program's decoder
or test input; no subscription framework or callback is needed.

Example scenario to turn into a compiling external-package example:

1. Construct an engine with ID `test-run`, a fixed UTC date, seed 42, count 2,
   and speed 1. Capture the two initial messages from history.
2. Advance five virtual seconds. Receive ten additional ordered reports.
3. Set speed to 0.5 and supply two elapsed real seconds with `Elapse`. Receive
   the reports for one additional virtual second without sleeping.
4. Set speed to 0. `Elapse` emits no reports and leaves virtual time unchanged.
5. `Advance` still permits an explicit virtual step while paused. Its duration
   is never multiplied by speed. A caller wanting rewind constructs a new run.

## Virtual time semantics

1. The public package never calls `time.Now`, sleeps, starts timers, or reads
   global randomness. `StartTime`, seed, and the ordered commands determine
   generated bytes, timestamps, MMSIs, and sequences for the same implementation.
2. Store authoritative virtual `now`, elapsed time, the next report boundary,
   speed, and a fractional scaling remainder. `Elapse(d)` adds `d * speed`;
   `Advance(d)` adds exactly `d`. Negative durations are errors. Zero duration
   is a no-op. Explicit advancement preserves the scaling remainder.
3. Normalize accepted speed to an integer number of hundredths. Use checked
   integer scaling with a carried sub-nanosecond remainder. Validate float inputs
   before conversion, including NaN, infinities, range, and supported precision.
   Handle ordinary binary representations of valid hundredths without rejecting
   them. Repeated small elapsed durations must equal their combined duration.
4. Report boundaries are `StartTime + n * time.Second`, for positive integer n.
   Process every boundary up to and including the target time. At each boundary,
   move vessels from their previous navigation time and emit reports in creation
   order. A vessel born between boundaries moves only from its birth instant.
5. A target between boundaries advances the clock without publishing a partial
   tick. Latest vessel reports remain at their last report times. Fleet update
   time records the last report tick or effective fleet mutation; `time.now`
   records committed clock progress and may be later.
6. An empty fleet still advances the clock and report boundary schedule. Time
   does not pause just because vessel count is zero. At speed 0, elapsed real
   time is discarded, so resuming has no paused-time catch-up.
7. Adding vessels uses the engine's current virtual time. Removing vessels
   preserves history. Count changes do not reset cadence. A command at a tick
   boundary occurs after the advance that reached that boundary; repeated
   commands at the same instant follow call order. Equal timestamps are valid;
   sequence is the total order.
8. `SetSpeed` changes only future elapsed-duration scaling. It preserves the
   current instant, next report deadline, reports, sequence, and fractional
   remainder. A repeated speed is a no-op. Setting speed never emits reports.
9. A single public advance may move at most 60 virtual seconds. Validate the
   scaled result, duration/date arithmetic, and sequence/MMSI capacity before
   publishing. Larger requests return an error without partial progress; callers
   can advance in documented chunks. At 100 vessels this bounds one returned
   batch to 6,000 scheduled reports, apart from separately returned creations.
10. Stage a complete public operation, including random source state, navigation,
    history, sequence, clock, cadence, and scaling remainder, then commit only
    after successful encoding and cancellation checks. A failed retry must not
    change subsequent random vessels. Keep staging bounded by the batch limit.
11. Advancing 10 seconds in one call must equal ten one-second calls and suitable
    fractional calls when commands occur at the same virtual instants. Movement
    uses the same canonical ticks in every case. Do not claim arbitrary concurrent
    command schedules or future algorithm versions reproduce the same bytes.

## Real-time driver and clock ownership

The thin internal runner owns an engine, a real monotonic baseline, a mutex
serializing elapsed-time delivery and management commands, and lifecycle state.
It invokes `Elapse` on a fixed 100 ms real heartbeat. It measures actual elapsed
time at processing time rather than assuming a ticker notification means exactly
100 ms. Lost/coalesced notifications therefore do not lose simulated time.

Before applying a valid count or speed command, sample real time under the same
runner lock and settle elapsed time using the old speed. Then apply the command
and retain the new real baseline. Invalid requests are rejected before settlement.
An encoding failure after successful settlement may leave that earlier elapsed
progress committed, but the requested mutation must remain unapplied; document
this distinction from atomic public package operations. Propagate fatal engine
errors through the existing runtime shutdown path.

Use small elapsed chunks whose scaled duration fits the public advance limit.
Do not skip intermediate report ticks during catch-up. Check cancellation between
chunks and retain the last committed baseline if a chunk fails. Bound a single
catch-up to one virtual hour; reject a larger backlog before processing it and
return a contextual runtime error, which stops the component. This explicit
overload policy prevents an unbounded replay after a long machine suspension.
Document that extreme speed shortens the real suspension interval this allows.

Use a small internal clock/timer seam for deterministic runner tests. Keep real
time values carrying their monotonic component until elapsed duration is computed.
UTC conversion belongs to simulation timestamps and API formatting, not the real
baseline. Do not use virtual time for HTTP deadlines, shutdown, polling, or uptime.

Only one `Run` is allowed for a driver lifetime; return a clear error for a second
start. Stop and join the heartbeat on cancellation. Avoid holding a lock during a
timer wait, network operation, or browser response write. Reads return committed
snapshots and never advance the simulation. Time metadata can lag actual pacing
by one heartbeat plus processing delay; it is not an extrapolated wall clock.

## HTTP and display contract

Extend existing `GET /api/metadata` with mutable `time` metadata:

```json
{
  "time": {
    "now": "2030-01-02T03:04:10Z",
    "elapsedMs": 5000,
    "speed": 0.5,
    "paused": false
  }
}
```

The rest of metadata retains run identity, `startedAt`, catalogs, and settings.
`startedAt` now explicitly means the initial virtual instant, not a distinct
process startup timestamp. `time.now` is UTC and authoritative to nanoseconds;
`elapsedMs` is elapsed virtual milliseconds truncated for JSON and presentation.
Expose real poll/pacing settings separately from the existing virtual
`tickIntervalMs` and `messageIntervalMs`, and add speed minimum, maximum, and
step settings. State clearly that 0 is the pause value. Do not describe the
whole metadata response as immutable anymore.

Add `PUT /api/time` accepting exactly `{ "speed": 2 }`. Return a complete
metadata response after applying the change. Use the existing strict JSON
conventions: required non-null number, one object, no unknown fields, 1 KiB body
limit, `application/json`, and `Cache-Control: no-store`. Invalid JSON, speed,
or precision returns 400; unsupported media type 415; unsupported method 405;
unexpected engine failure 500 with contextual server logging. Speed validation
must also exist in the public package. Reuse parsing code only where it makes
the two write handlers simpler. A required pointer distinguishes missing speed
from valid speed 0.

Extend `/display/api/vessels` with `time` copied from metadata. Preserve the
existing concurrent fleet/metadata fetch, timeout, response size bound, restart
identity checks, and NMEA-derived navigation. Validate time fields, including a
missing speed versus a valid paused speed. Reject invalid upstream time metadata
as 502; retain 503 for reachability/restart failures.

The two upstream reads are separate snapshots. Matching simulation IDs establish
the same run, not the same tick. It is valid for a report timestamp to be slightly
later than the independently fetched clock snapshot. Do not add a cross-response
`report.timestamp <= metadata.time.now` rejection. Document this eventual
consistency; do not introduce another snapshot endpoint or revision protocol.

UI clock text shows the last received authoritative time without local ticking
or browser clock extrapolation. Show a full UTC date, time, and multiplier so
custom dates and midnight crossings are clear. The existing real receipt time
remains the basis for connection freshness. While disconnected, mark time and
markers stale; an unchanged paused clock is not a disconnection.

Manager polls metadata on every refresh. Apply speed/count write responses
immediately and prevent older in-flight poll responses from overwriting them.
Preserve unsaved form edits during polling. Reconcile effective state across
tabs, and reset per-run UI state when simulation identity changes. Require
matching identities before rendering fleet/history/metadata together. If a run
changes during reads, keep the last coherent view and retry.

## Code-review defects included

These are findings from reading the code. No execution-based reproduction or
test run was performed during planning. Add regression tests before fixes.

| ID | Evidence and failure | Planned fix |
| --- | --- | --- |
| B1 | `SetCount(count, now)` accepts timestamps older than `updatedAt`, while `publish` keeps the newer fleet time. After `Advance(t+10s)`, `SetCount(2, t)` emits a higher-sequence report timestamped in the past. A fleet change at a future time also makes `Advance` skip a pending earlier tick through its fleet-wide time guard. | Remove caller timestamps from count changes; use authoritative virtual time and an independent report schedule. Serialize real driver commands with elapsed-time settlement. |
| B2 | `SetCount` draws directly from `s.random` before encoding succeeds. `TestFailedMutationKeepsState` forces an encoding failure but only checks fleet/history/sequence; later random draws have already changed. | Stage and commit the PCG source together with all other state. Add a failure-then-retry comparison against an untouched control engine. |
| B3 | Manager's `initialized` flag permanently disables metadata fetching, and `refresh` combines fleet/history without comparing simulation IDs. Restart during requests can display different runs together and leave settings stale. | Periodically refresh metadata, verify all identities, reset run state, and protect writes from stale polls while adding time controls. |

The current large-advance behavior (one report after any elapsed interval) is a
deliberate simplified cadence documented in the project. Changing it to canonical
virtual ticks is a required behavior change, not a claim of an existing defect.
Similarly, the missing single-run guard becomes an explicit runtime invariant
during extraction rather than an unrelated bug investigation.

## Implementation sequence

| Step | Dependencies | Effort | Complexity |
| --- | --- | --- | --- |
| [01 - Extract the public simulation core](01-public-package.md) | None | L | Medium |
| [02 - Implement deterministic virtual time](02-virtual-time.md) | 01 | L | High |
| [03 - Integrate real-time pacing and management API](03-runtime-api.md) | 02 | L | High |
| [04 - Add Manager controls and Display time](04-manager-display.md) | 03 | M | Medium |
| [05 - Verify external use and complete documentation](05-verification-docs.md) | 01-04 | M | Medium |

Overall estimate: approximately 8-12 focused engineering days including tests
and review, with the largest uncertainty in clock settlement and UI races.
This is a planning estimate, not a delivery commitment. See
[assessment](assessment.md) for impact and risks. Each implementation step updates
nearby documentation and runs focused checks. Final implementation requires
`task all` to pass. Never commit changes.
