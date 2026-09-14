# Impact, feasibility, and risks

## Assessment

Feasible within the current Go engine, JSON/HTTP boundary, and Leaflet display.
The largest change is the meaning of a displayed target: its position must be the
last successfully received report, even when simulation truth has moved on.
Reception storage, transaction atomicity, and snapshot consistency need more work
than station markers or coverage drawing.

## Impact by area

| Area | Impact | Planned treatment |
| --- | --- | --- |
| Public `simulation` package | High | Explicit station config, channel-aware reports, reception state, snapshot reads, and mutation examples. |
| `internal/ais` | Small | Encode either A or B and expose validated channel; retain type 1 navigation semantics. |
| `internal/simdriver` | Medium | Serialize station edits and return the exact post-command snapshot. |
| `internal/simulatorapi` and `internal/simulator` | High | Add consistent observation and bounded history contracts; manager station controls. |
| `internal/display` | Medium | Replace truth-fleet projection with received-report projection. Retain stateless HTTP operation and explicit failures. |
| Embedded UI | High | Station layers, capabilities, received-message inspector, target provenance, and filter states. |
| Combined/standalone composition | Small | Keep loopback/public route parity; no new runtime service. |
| Documentation/examples | Medium | Explain generated versus received data and revised public/wire APIs. |

## Risks and mitigations

| Risk | Consequence | Mitigation and evidence required |
| --- | --- | --- |
| Uncalibrated coverage looks authoritative | Users mistake test output for real site performance | Label model and reference transmitter; cite evidence; publish formulas, parameters, and limitations. |
| Hidden ground-truth navigation reaches the display | Stations appear to track missed reports | Project only retained receptions; regression case with a received old position and a missed new position. |
| Shared randomness depends on station count/order | Adding a station changes movement or another receiver | Key reception draws by seed, stable station ID, per-station RF revision, and transmission sequence. Test additions and reordering. |
| Partial engine commit | Failed advance changes later results | Stage reception maps, counters, report ordinals, and histories with the existing transaction; inject cancellation before commit. |
| Snapshot/history truncation | Missing targets or silent event loss | Separate full current observation snapshot from bounded event history; expose cursors and gaps. |
| Polling or maps dominate at high speed | Driver backlog or unresponsive UI | Bounded state, one snapshot per poll, precomputed contours, recent-message limits, size tests, benchmarks. |
| History of removed entities has broken references | Inspectors show incorrect or missing attribution | Immutable station IDs, compact config provenance on events, retained metadata, explicit removed selection behavior. |
| Independent API reads mix configuration and targets | Coverage contradicts reception | One atomic observation snapshot with simulation identity, revision, clock, and effective station configuration. |
| Loss percentage or dedup is mislabelled | Incorrect comparison between stations | Define opportunities, successful receptions, unique transmissions, and unique targets separately. |
| UI color is the only station identifier | Comparison is inaccessible | Stable labels, IDs, selection outlines, text tables, and keyboard controls. |

## Initial resource budget

These are implementation limits and measurement targets, not observed benchmarks.

- 16 stations * 100 vessels * 100x = 160,000 station/report evaluations per real
  second. One maximum 60-second virtual engine batch evaluates up to 96,000 pairs.
- At most 1,000 retained reception events per station: 16,000 total, plus existing
  1,000 transmission reports. No unbounded list of all failed reception attempts.
- Keep at most 1,000 distinct recently observed MMSIs across the run's current
  observation store, each with at most 16 station observations. Deterministic
  eviction and a visible capacity-eviction counter cover rapid fleet churn.
- Observation snapshots contain at most 1,000 report records for the selected
  view, compact station provenance, and at most 50 recent receptions. Do not send
  all 16,000 raw per-station reports in the aggregate view.
- At 100 vessels all successfully received, 1,000 station events cover 10 virtual
  seconds, or 0.1 real seconds at 100x. Cursor gaps are expected; live snapshots
  still recover. Lossless event export is outside this delivery.
- Start with a separate 8 MiB bound for observation responses, 2 MiB for bounded
  reception pages, 64 KiB for configuration writes, and the existing five-second
  upstream deadline. Verify worst-case encoded fixtures fit, including strings,
  geometry, JSON escaping, and provenance. Reduce payload before raising limits.
- Generate coverage only when RF settings or reference transmitter change, not
  per vessel, tick, or browser. Maximum 8 sectors per station; sample contours at
  five-degree bearings plus sector edges with a fixed vertex cap.
- Benchmark 1, 3, and 16 stations at 100 vessels, and repeat at maximum retained
  target/history occupancy. Record CPU, allocation rate, lock duration, response
  sizes, and whether the real-time driver sustains 100x on the development machine.
  If it cannot, reduce allocations and copying first; disclose a measured lower
  supported bound instead of silently skipping transmissions.

## Scope and delivery control

Keep a single concrete model. Use plain structures and focused functions in the
existing packages. Avoid a propagation-plugin abstraction until multiple models
actually exist. A station editor and a finite snapshot API are sufficient for the
first delivery; streaming, persistence, and terrain acquisition are separate work.

Ship after all seven steps pass their acceptance criteria. Keep intermediate
changes reviewable, with documentation updated as contracts become implemented.
No commits are part of this plan or its execution instructions.
