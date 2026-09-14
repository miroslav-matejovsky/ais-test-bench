# Simulation

The root package owns the active vessel fleet, seeded random initialization,
one-second movement ticks, each vessel's latest AIS report, and the latest 1,000
AIS sentences in memory. A single mutex protects mutations and copied snapshots.

`New` takes a run identity (`NewID` in production), start time, and seed, and
creates one vessel. `SetCount` manages 0-100 vessels. `Run` advances positions
until context cancellation; `Advance` accepts explicit time for deterministic
tests. Existing vessels keep their identity, type, speed, and course when count
changes.

Each emitted sentence has a run-local sequence starting at 1. The same message
is stored as the vessel's latest report and in history. Mutations are prepared
and encoded before any state is published. `Fleet`, `History`, and `Metadata`
return `simulatorapi` responses; metadata settings come from the constants
that drive generation. `Snapshot` returns decoded positions for the current
display page.

The application/domain/infrastructure subpackages hold earlier design contracts
for scenarios and playback. Each Go package documents its API in `doc.go`.
