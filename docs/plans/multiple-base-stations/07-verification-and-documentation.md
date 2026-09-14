---
title: "07 - Integration verification, performance, and documentation"
dependencies: ["01-station-configuration.md", "02-reception-and-coverage.md", "03-engine-observations.md", "04-simulator-api-and-manager.md", "05-display-backend.md", "06-display-interface.md"]
effort: "M"
complexity: "medium"
---

# Integration verification, performance, and documentation

Status: implemented; manual browser verification by the user is pending.

## Outcome

Prove that station differences reach the display through the public HTTP boundary,
that behavior is deterministic, and that bounded storage and polling remain useful
at the supported traffic limits. Synchronize implemented documentation.

This step applies when implementing the plan. Preparing these Markdown documents
does not require `task all`, runtime changes, or tests.

## Implementation work and verification

### Cross-component scenarios

Use a fixed seed and virtual start instant, explicit station settings, and controlled
test positions. Exercise the engine and handlers in-process with `httptest`; use
the existing fake driver clock for timing. Avoid sleeps and external map/network
dependencies in Go tests. Do not create a generic scenario framework for a handful
of fixtures.

| Scenario | Observable result |
| --- | --- |
| Overlap | One transmission has receptions at two sites; aggregate has one MMSI marker; both station views include it. |
| Different capabilities | Same report has different power/margin/probability by station; fixture proves a successful reception at only one site. |
| Outside all coverage | Truth endpoint contains vessel; observation API and display contain no newly observed target. |
| Lost update | Station receives report N and misses N+1; station display remains at N's decoded position. |
| Receiver diversity | A previously marginal target becomes observable through another site; previous site's old timestamp remains accurate. |
| Shadow sector | Equal-range bearings produce different probabilities and contour radii. |
| Channel B failure | B reports stop at one site, A reports continue, and other stations retain both channels. |
| Disable/re-enable | No receptions while disabled; stored targets age; resume only on later transmissions. |
| Create/move/edit/delete | Lifecycle follows step 01; no retroactive receptions or stale geometry; deleted selected site is handled. |
| Empty fleet | No transmissions; virtual clock still ages existing observations to expiry. |
| History pressure | More than 1,000 receptions evict events, flag cursor gaps, and preserve latest observed targets. |
| Target capacity | More than 1,000 distinct observed MMSIs evict deterministically, bound memory, and expose eviction counts. |
| Pause and 100x | Same virtual transmissions and decisions at equal virtual instants; age freezes at pause. |
| Cancellation and invalid commands | No partial engine reception state; later results match an untouched control engine. |
| Upstream failure/restart | Last good view becomes stale on failure; new run clears all old station and target identities. |
| Separate processes | Standalone display uses only simulator HTTP routes and produces equivalent output to combined mode. |

For probabilistic fixtures, choose and record deterministic keys/outcomes rather
than asserting a random packet happens to arrive. Keep separate tests for model
probability and receive/drop sampling. Use `require` and table-driven cases where
they make behavior clearer.

### Performance and retention

Benchmark `Advance` for 1, 3, and 16 stations at 100 vessels, both empty and full
history/target stores. Benchmark snapshot construction/encoding and display
decoding at maximum occupancy. Include high-churn targets and all-received traffic,
which amplify storage and copying more than all-drop traffic.

Measure real allocations and encoded sizes before changing algorithms. Prefer
immutable sentence/config sharing and bounded rings over full history copying per
packet. Preserve transaction atomicity if changing staging. Precompute station
trigonometric constants and immutable coverage only if measurements justify it.

Record the development machine and Go version with results. Check the real-time
driver keeps up at the proposed maximum without unbounded backlog. Browser updates
must remain usable at 100x even though intermediate receptions are intentionally
absent from the sampled UI. Do not silently drop engine opportunities to meet a
performance target. Resolve or document measured limit changes before acceptance.

### Documentation changes during implementation

- Root `README.md`: update architecture, generated versus received data, station
  defaults/controls, display behavior, APIs, and combined/standalone examples.
- `simulation/doc.go`: public station/model types, explicit configuration, receipt
  identities, lifecycle, atomicity, age/retention rules, and deterministic examples.
- `internal/simulatorapi/doc.go`: complete wire shapes, units, revisions, snapshots,
  error semantics, selectors, cursor recovery, and scenario versus AIS metadata.
- `internal/simulator/doc.go` and `internal/simdriver/doc.go`: manager ownership,
  station commands, clock settlement, and driver versus engine atomicity.
- `internal/display/doc.go`: HTTP-only observation projection, validation, body
  limits, failure states, and no truth-based target recovery.
- `internal/ais/doc.go`: explicit channel encoding/decoding and supported payloads.
- UI package/asset docs and `internal/README.md`: responsibilities of new controls,
  coverage presentation, and unchanged component separation.
- `.go-arch-lint.yml`: responsibility comments remain synchronized; do not widen
  display dependencies to access simulation state.
- Keep a concise implemented model explanation close to `simulation` code, with
  primary source links and clear empirical assumptions, before this plan is retired.

Use `go doc` to inspect the resulting public API and examples. Run targeted tests
as behavior is implemented. Before implementation acceptance, run `task all` and
resolve every test, format, lint, deadcode, and architecture failure it reports.
Do not commit changes. If implementation later stops unfinished, document remaining
work in root `.todo` as required by repository instructions.

## Acceptance criteria

- Every scenario above has automated behavioral coverage or a recorded browser
  check where browser behavior is the subject.
- Performance measurements confirm the published limits; worst-case response
  fixtures fit the display client's bounds and UI memory remains bounded.
- No external network, timing sleeps, or mutable global randomness is required for
  deterministic engine/API tests.
- Both process arrangements pass and architectural dependency rules remain intact.
- Implemented documentation matches final APIs and model assumptions.
- `task all` passes for implementation; user accepts the completed behavior before
  the plan folder is removed under `docs/plans/README.md` guidance.

## Implementation record

### Scenario coverage

| Scenario | Automated coverage |
| --- | --- |
| Overlap, different capabilities, outside all coverage, channel B failure, lost update, receiver diversity, disable/re-enable, deleted selected site | `internal/app/scenario_test.go`: one seeded engine behind the simulator API over HTTP, read by the display API through its HTTP client. Every link has probability 1 or 0, so no assertion depends on a draw. It also asserts the display reads only observation and reception routes. |
| Lost update, aggregate provenance | `simulation` `TestMissedReportKeepsLastReceivedPosition`; display `api_test.go` fixtures |
| Shadow sector | `TestShadowLoss`, `TestCoverageSectorsAndEmptyContours` |
| Create/move/edit/delete | `TestStationLifecycle`, `TestStationEditsBetweenTicks`, `TestStationHTTPCommandsAndObservations` |
| Empty fleet ages to expiry, pause freezes age | `TestObservationAging` |
| History pressure | `TestReceptionHistoryPaging`, `TestReceptionHistoryRollover`, display gap tests |
| Target capacity | `TestTargetCapacityEviction`, `TestExpiryBeforeCapacityIsSplitIndependent` |
| Pause and 100x | `TestSplitCallsMatchCombined` now includes speed 100 in split real-time calls |
| Cancellation and invalid commands | `TestCancellationBeforeCommitRollsBack`, `TestFailedAdvancesChangeNothing`, driver rejection tests |
| Upstream failure/restart | display API status tests; browser checklist from step 06 |
| Separate processes | standalone `internal/display` route tests with the fixture simulator; combined `internal/app` tests with the real engine |

Model probability and receive sampling stay separately tested in
`reception_internal_test.go`. No generic scenario framework was added.

### Performance

Intel Core Ultra 7 265H, 16 logical CPUs, 63 GB RAM, Windows 11, Go 1.27.1,
2026-09-14. One `Advance` operation is one virtual second at 100 vessels; 100x
allows 10 ms.

| Benchmark | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| 1 station, no receptions | 261,245 | 132,115 | 1,004 |
| 1 station, all received, full stores | 362,512 | 174,305 | 1,212 |
| 1 station, 100 new MMSIs per second | 2,588,552 | 406,808 | 2,837 |
| 3 stations, no receptions | 357,396 | 137,701 | 1,204 |
| 3 stations, all received, full stores | 580,974 | 267,221 | 1,814 |
| 3 stations, 100 new MMSIs per second | 3,017,767 | 601,221 | 4,241 |
| 16 stations, no receptions | 696,116 | 171,857 | 2,504 |
| 16 stations, all received, full stores | 2,091,327 | 860,104 | 5,717 |
| 16 stations, 100 new MMSIs per second | 7,887,703 | 1,829,974 | 12,247 |
| Engine `Observations`, 16 stations, 1,000 targets | 2,659,346 | 2,829,088 | 2,226 |
| Display request end to end, same occupancy | 179,886,083 | 105,852,861 | 764,429 |

- The real-time driver sustains 100x at every configuration; the churn case is a
  synthetic worst case, since fleet replacement is a manual command.
- Maximum encoded sizes: observations 5,453,334 bytes (8 MiB bound), 200 receptions
  246,609 bytes (2 MiB bound), definition 1,225 bytes (4 KiB bound). The display
  response at maximum occupancy is 5,477,976 bytes.
- The display request profile is spread over JSON encoding in both hops (26%),
  the decimal-string scan (21%), and decoding (18%). At 180 ms against a one-second
  poll interval and five-second deadline, no algorithm change was made.
- No engine opportunity is skipped; limits are unchanged.

### Documentation

`simulation/doc.go` now lists primary sources for the reference terms and states
the model assumptions, so the plan's research file is not needed to understand
the code. `.go-arch-lint.yml` responsibility comments include stations and station
commands; dependencies are unchanged. The README records the measured limits.
Other package documentation was already synchronized by steps 01-06.

### Manual verification checklist

1. `task run`, open the manager and display. Three stations appear with A/B
   coverage; targets are drawn only where received.
2. Set 100 vessels and speed 100 for at least 10 minutes. The display stays
   responsive, the clock advances, no "stale" state appears, and the simulator log
   has no backlog error.
3. Apply "Disable B" to the harbour receiver: its B counters stop increasing while
   A continues; other stations keep both channels.
4. Disable a station: its targets age to stale and lost; re-enable it and new
   receptions resume without retroactive data.
5. Move a station far offshore: coverage and received targets change; old messages
   in its history keep the original receiver snapshot.
6. Select one station, then delete it in the manager: the display returns to all
   stations with a removal notice.
7. Pause: target ages freeze; resume continues.
8. Stop the simulator while `task run:display` runs separately: the last view stays,
   marked stale; restart the simulator and the display replaces all identities.
