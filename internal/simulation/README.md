# Simulation

The root package owns the active vessel fleet, seeded random initialization,
one-second movement ticks, and the latest 1,000 AIS sentences in memory. A single
mutex protects mutations and copied snapshots used by the HTTP API.

`New` creates one vessel. `SetCount` manages 0-100 vessels. `Run` advances positions
until context cancellation; `Advance` accepts explicit time for deterministic
tests. Existing vessels keep their identity, speed, and course when count changes.

The application/domain/infrastructure subpackages hold earlier design contracts
for scenarios and playback. Each Go package documents its API in `doc.go`.
