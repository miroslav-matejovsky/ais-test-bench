---
title: "03 - Engine reception history and observed targets"
dependencies: ["01-station-configuration.md", "02-reception-and-coverage.md"]
effort: "L"
complexity: "high"
---

# Engine reception history and observed targets

## Outcome

Make station observations authoritative simulation state, committed atomically
with transmissions and recoverable without replaying retained message history.

## Implementation work

### Separate the three identities

| Record | Identity | Meaning |
| --- | --- | --- |
| Transmission | `(simulationId, transmissionSequence)` | One generated AIS sentence, whether anybody receives it or not. |
| Reception | `(simulationId, stationId, receptionSequence)` | One station successfully decoded that transmission. Reception sequences are contiguous per station and survive RF edits. |
| Observed target | `(simulationId, MMSI)` plus per-station observations | Last successfully received reports and their station provenance. |

Keep current `Message.Sequence` as transmission identity. Add a reception record
with station ID, config/RF revision, transmission sequence, per-station reception
sequence, MMSI, channel, full virtual transmission/receive timestamp, exact NMEA,
estimated receive power, margin, probability, and distance at reception. Include
a compact immutable RF configuration snapshot sufficient to explain the result,
and scenario name/type attribution captured at that time. Share immutable values
internally where helpful; serialize self-contained bounded history records.

No receive latency is modelled: transmission and reception timestamps are equal.
Browser fetch time stays a separate wall-clock field. A record can outlive the
original transmission-history entry, so reception inspection must not require a
lookup in the 1,000-entry transmission ring.

### Atomic engine processing

For creation reports and every virtual tick: move vessel, increment its report
ordinal, encode once, allocate transmission sequence, evaluate all station
opportunities, stage successful receptions, update target observations and counters,
then commit with existing state only after all encoding and context checks succeed.

Stage all mutated nested maps/slices, counters, reception sequences, station IDs,
and revisions. Avoid shallow-copy aliasing into committed state. Check context
between bounded chunks of station evaluation and immediately before commit. A
failed call returns no transmissions and changes no future result, including
RF decisions, observation age, and sequence allocation.

Preserve existing `Advance`, `Elapse`, and `SetCount` return values as complete
transmission batches. Add an explicit atomic `Observations(selection)` read and
bounded `ReceptionHistory(stationID, cursor, limit)` read. Document that the new
reception history is finite; do not imply the complete-batch guarantee of the
transmission-returning APIs applies to reception polling.

### Latest observed targets

Maintain at most one latest successful reception per station/MMSI independently
of event history. A newer generated report that is missed changes none of its
last-known navigation, channel, power, or last-received time. Do not remove a
target just because it left the active truth fleet.

Use these initial run-level virtual-time age settings, published in metadata:

- Fresh: age <= 10 seconds since last successful reception.
- Stale: age > 10 seconds and <= 60 seconds.
- Lost: age > 60 seconds and < 600 seconds, still available as last known.
- Expired: age >= 600 seconds; remove from the current observation store.

Age is `snapshot.time.now - receivedAt`. It advances with committed virtual time,
including with an empty fleet, and freezes while paused. These are application
choices for the one-second test cadence, not standard AIS target-aging rules.
Reads calculate status without mutating state; clock mutations perform expiry.

Cap the store at 1,000 distinct MMSIs. On capacity pressure evict the target with
oldest last reception across all stations, breaking equal times by MMSI, and
increment a visible capacity-eviction counter. Remove all station observations
for that MMSI together. Expiry and capacity eviction do not erase bounded event
history. This keeps high-churn runs finite despite unlimited new MMSIs over time.

For an aggregate view, choose the highest transmission sequence among selected
stations' retained observations for each MMSI. Decode only that report for the
aggregate position. Include compact last-seen metadata for every receiving station;
mark precisely which stations received the chosen transmission, rather than
claiming all previously observing sites saw the latest position. Break equal
reception ties by stable station ID. One MMSI creates one aggregate marker.

Filter semantics: selected stations are a union (received by any selected site).
All stations is the default. A station-specific target uses that station's report.
Disabled stations' retained observations stay in the union and age normally;
their provenance shows the station is now disabled. Deleted stations no longer
contribute. Current-target counts include fresh and stale; lost counts are separate.

### History and counters

Retain the newest 1,000 successful receptions per station. Use bounded rings or
equivalent append/eviction storage with detached read results. Transmissions
received at three stations count as three receptions but one unique transmission.

Keep cumulative opportunity/outcome counters since station creation, and 60
virtual seconds of per-second count buckets for recent rates. An opportunity is
every generated transmission while the station exists, even if disabled. The
mutually exclusive reason counts sum to opportunities. Include per-channel
opportunities and successes. Capacity/TTL cleanup never reduces cumulative counts.
RF edits retain counters and show their since-time; recent windows may span RF
revisions, which the UI must disclose.

Report receive ratio as successes/opportunities, or null when the denominator is
zero. Call it a simulated opportunity ratio, not measured packet error rate. Show
the actual rate-window duration as `min(60s, now-createdAt)`, using null for rates
at a zero-duration creation instant. No zero-delay wall-clock division.

Expose a monotonically increasing state revision for all effective commits and a
separate station-set revision for configuration writes. Reads do not increment
either. One observation snapshot includes its clock, stations, counts, selected
targets, recent receptions, and sequence bounds from one committed state.

## Verification

Test zero receivers, overlapping receivers, all-drop/no-drop fixtures, creation
reports, station edits between ticks, disable/re-enable, remove/re-add IDs, and
fleet reduction. Verify old received positions survive newer missed reports.

Prove equal outputs for combined versus split advances and equivalent speed-scaled
elapsed calls. Test cancellation during multi-station processing, encode failure,
sequence overflow, detached return values, bounded history rollover, exact age
thresholds, pause, large virtual advances, and deterministic capacity eviction.
Use controlled fixtures or pure outcome inputs, never sleeps.

## Acceptance criteria

- Transmission and reception totals have distinct tested identities and meaning.
- Failed commands leave all reception state and future outcomes unchanged.
- Latest observations survive event-history eviction and reconstruct the live map.
- Aggregate and per-station targets use only successfully received navigation.
- Every growing structure has a documented bound and observable truncation behavior.
