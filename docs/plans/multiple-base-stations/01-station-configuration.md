---
title: "01 - Station configuration and lifecycle"
dependencies: []
effort: "M"
complexity: "medium"
---

# Station configuration and lifecycle

## Outcome

Define explicit, validated receiving sites and transmitter assumptions before
adding reception behavior. Establish identity, edits, and reproducible demo data.

## Implementation work

Extend `simulation/types.go` and add focused station/configuration files inside
`simulation`. Update `simulation/doc.go` with all public types and invariants.
Keep JSON wire types in `internal/simulatorapi` when step 04 implements the API.

### Station fields

| Field | Meaning and initial constraint |
| --- | --- |
| ID | Immutable run-local opaque string, assigned by a monotonic engine allocator. Never reused within a run. IDs are independent of slice indices and names. |
| Name | Required display name, at most 80 Unicode characters and 320 UTF-8 bytes. Duplicate names allowed; show ID to disambiguate. |
| Position | WGS84 latitude [-90, 90], longitude [-180, 180); longitude 180 normalized to -180. |
| Enabled | Administrative receiving state; defaults are explicit in app presets. |
| Antenna height | Metres above mean sea level, greater than 0 and at most 500. Include site elevation plus mast height in this value. |
| Receive gain | dBi, -10 through 20. Model a single effective gain, not an antenna pattern catalogue. |
| Feeder loss | dB, 0 through 30, including cable/connectors/splitter. |
| Channels A and B | Each has enabled, sensitivity dBm [-125, -80], noise penalty dB [0, 40], and extra packet-drop probability [0, 1]. |
| Shadow sectors | At most 8 non-overlapping bearing intervals with extra loss dB [0, 60]. Clockwise from true north; start inclusive and end exclusive; allow wrap across north. |
| Config revision | Increases on any effective station edit. Preserve it on a no-op. |
| RF revision | Increases only on changes affecting reception or coverage. Name-only edits preserve random outcomes and contours. |

Both channel entries are required, even when disabled. Validate all numeric fields
for finiteness before range checks. Empty shadow sectors mean omnidirectional
effective coverage. Represent a full-circle loss through the common receiver/link
settings, not an ambiguous sector with equal start and end. Reject overlapping
sectors instead of inventing hidden stacking rules. Impose a 4 KiB encoded limit
per station configuration in addition to the structured limits.

The public engine config receives an explicit station-definition slice; an empty
slice means zero receivers. Startup allocates stable IDs in definition order.
Subsequent additions allocate new IDs without consuming vessel randomness.
Reordering existing stations preserves their IDs and behavior. Detect ID/revision
overflow before mutation and return `ErrLimit`.

### Transmitter and model settings

Add an explicit run-level reference transmitter: power W, height m above sea
level, gain dBi, and feeder loss dB. Start with 12.5 W, 10 m, 2 dBi, and 1 dB.
Validate power in (0, 25], height in (0, 100], gain in [-10, 20], feeder loss in
[0, 30]. These are simulator limits. Current vessels use this same transmitter
profile; per-vessel power/class variation is deferred. Snapshot metadata includes
the profile so every coverage overlay states which transmitter it assumes.

Expose the model version and the explicit parameters from step 02 in effective
settings. Do not randomize capabilities on every tick or generate hidden defaults
inside the public engine. App defaults belong beside `simdriver.NewConfig`.

### Lifecycle

- Create a station at the current virtual instant with empty history and no
  targets. It only receives later transmissions, including future vessel-creation
  reports. It never reprocesses existing fleet reports as new receptions.
- Apply edits after settling time through the driver. Only future transmissions
  use the new RF revision; past records retain the settings attribution they had.
- Name-only edits update current labels. Event details retain a compact name and
  RF provenance captured at reception, so historical attribution is self-contained.
- Disabling preserves observations and history; they age with virtual time.
  Re-enabling starts receiving future reports. All channels disabled is a valid
  degraded configuration, clearly reported as receiving no channels.
- Moving a station preserves old last-known targets and their original reception
  geometry. Rebuild current coverage immediately. Old reports are not relocated.
- Delete a station from current configuration, coverage, and target aggregation.
  Release its station history and observations immediately. Already delivered
  client records remain self-describing. Querying its history then returns 404.
- Deleting the final station leaves vessel generation and truth history running.

### Default demonstration

Provide three explicitly synthetic receiving sites, not claimed real installations:

| Name | Latitude / longitude | Height | Sensitivity A/B | Gain / feeder loss | Shadow |
| --- | --- | --- | --- | --- | --- |
| Rotterdam coast | 51.98 / 4.05 | 25 m | -110 / -110 dBm | 3 / 2 dB | None |
| Northern coast | 52.12 / 4.24 | 40 m | -112 / -112 dBm | 3 / 2 dB | None |
| Harbour receiver | 51.95 / 4.14 | 15 m | -108 / -108 dBm | 2 / 3 dB | 270 to 330 degrees, 15 dB loss |

Start all channels enabled with zero noise penalty and extra drop probability.
Use a manager preset to disable B on the harbour receiver for a clear channel
failure demonstration. Parameters are chosen scenario values, not field measurements.

The current spawn rectangle is only a few kilometres wide. Ordinary VHF coverage
will overlap strongly there. Add fixed in-memory test scenarios with explicit
positions near and beyond the proposed contour, and document a longer simulated
run for movement through the edges. Do not shrink plausible coverage solely to
fit the initial map. A general route/scenario editor is outside this step.

## Verification

Use table-driven validation tests with `require`: absent channel values, zero and
maximum stations, duplicate or overlapping sectors, north wrap, non-finite values,
UTF-8 limits, invalid heights, exact boundaries, no-op edits, and ID exhaustion.
Verify rejected input changes neither engine state nor future vessel randomness.
Check that adding/reordering stations does not change existing vessel reports.

## Acceptance criteria

- The app starts with three documented definitions; library callers can choose
  zero through sixteen sites without implicit random capability differences.
- Identity, metadata revisions, RF revisions, and lifecycle semantics are tested.
- The manager/API can later distinguish validation from missing station and
  stale-edit conflicts using meaningful errors.
- Public configuration and preset assumptions are documented close to code.
