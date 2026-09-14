---
title: "07 - Integration verification, performance, and documentation"
dependencies: ["01-station-configuration.md", "02-reception-and-coverage.md", "03-engine-observations.md", "04-simulator-api-and-manager.md", "05-display-backend.md", "06-display-interface.md"]
effort: "M"
complexity: "medium"
---

# Integration verification, performance, and documentation

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
