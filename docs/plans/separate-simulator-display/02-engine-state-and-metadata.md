---
title: "02 - Add current AIS report state and simulation metadata"
dependencies: ["01-api-contracts.md"]
effort: "M"
complexity: "medium"
---

## Outcome

The engine can supply the new fleet, history, and metadata responses through
consistent snapshots. Movement and count behavior remain the current simple
scenario.

## Implementation work

1. Read `internal/simulation/simulator.go` and its tests before editing. Retain
   the existing seeded random source, explicit tick times, mutex, and bounded
   history. Introduce only the state required by the contract.
2. Give each engine start a unique opaque `simulationId` and a UTC `startedAt`.
   Production composition supplies or generates the identity once; tests can
   provide a fixed identity. Do not derive identity only from a random movement
   seed, since two starts using one seed must be distinguishable.
3. Assign a supported `typeId` to each newly created vessel. Start with the single
   metadata category defined in step 01. Keep existing MMSI/name behavior.
4. Generate each NMEA sentence once. Give it the next report sequence, store it
   as that vessel's latest report, and append that same report to history.
   Snapshot reads must not generate messages or consume sequence numbers.
5. Preserve latest reports separately from the bounded history. Removing a
   vessel removes its latest-report state and membership, while its historical
   messages remain until eviction. A newly added vessel immediately gets a
   latest report. Setting count to zero yields an empty active snapshot.
6. Preserve the invariant that vessel movement state, latest report, history,
   and snapshot timestamp describe a successful update. Prepare and validate a
   mutation before publishing it; an encoding failure must not expose a moved
   vessel with its previous report or half of a requested fleet change.
7. Centralize currently scattered settings only as far as needed for metadata
   and generation to read the same values. Do not add flags or a settings editor
   for every metadata field. Keep message cadence tied to the current tick.
8. Provide copied snapshot methods for active reports, retained history, and
   metadata. Build response data under the state lock, then release it before
   HTTP serialization. Keep wire mappings in the simulator HTTP package if
   needed to avoid making the engine depend on transport types.
9. Update `internal/simulation/doc.go` and its README with the actual state and
   invariants. Existing methods used by the old combined path can remain only
   until their callers are migrated in step 06.

## Verification

Use deterministic time, seeds, and identities. Extend unit tests to cover:

- Initial one-vessel snapshot has one valid NMEA report and sequence 1.
- Movement updates the latest report and history with the same emitted sentence.
- Increasing count preserves existing identities and immediately initializes
  new reports. Reducing count removes only active/latest-report state.
- Zero vessels and no-op count changes have the timestamp semantics in step 01.
- History eviction preserves capacity, ordering, sequence bounds, and all
  currently active latest reports.
- Mutating returned snapshots does not mutate engine state.
- Metadata values match the generation bounds, type assignment, and limits.
- Separate starts have distinct run identities, including when seeds match.
- Failed mutation leaves the previously observable state intact.
- Concurrent reads, ticks, and fleet changes pass the race detector.

## Acceptance criteria

- Current reports describe exactly the active fleet without scanning history.
- Sequence numbers identify emission, not reads or snapshots.
- All exposed metadata describes actual current implementation behavior.
- Engine methods own no HTTP objects and store no contexts.
- Existing navigation accuracy and count-limit tests continue to pass.

## Effort and complexity

Medium effort and complexity. Most existing movement code stays in place.
Atomic publication of generated reports is the main area requiring care.
