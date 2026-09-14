---
title: "01 - Define simulator and display API contracts"
dependencies: []
effort: "M"
complexity: "medium"
---

## Outcome

Define one small simulator HTTP contract that the manager, display backend, and
external clients can consume. Define a distinct browser-facing display contract.
Create `internal/simulatorapi` for transport types, without engine dependencies,
and document the implemented contract near that package during implementation.

## Simulator endpoints

| Method | Route | Behavior |
| --- | --- | --- |
| GET | `/api/vessels` | Complete active fleet with each vessel's latest NMEA position report |
| PUT | `/api/vessels` | Accept `{ "count": 3 }`; return the resulting complete fleet snapshot |
| GET | `/api/messages` | Retained NMEA history, oldest first, with source identity and sequence bounds |
| GET | `/api/metadata` | Simulation identity, vessel type catalog, supported messages, and effective settings |

All successful responses are JSON and use `Cache-Control: no-store`. AIS is
transported as the exact checksummed NMEA sentence in a JSON string, including
escaped CRLF. A client reads `sentence` to obtain the original NMEA line. The API
is a finite snapshot API; it is not an open-ended NMEA stream.

`PUT` requires `application/json`, one object, a required integer count in
0-100, no unknown fields, and the existing 1 KiB request limit. Preserve clear
400 responses for invalid input, 415 for the wrong content type, 405 for an
unsupported method, and 500 for an internal failure. Error bodies may remain
plain text for this small API. Reads never mutate or drain engine state.

### Active fleet response

The following describes required fields, not a new decoded-position endpoint.

| Field | Meaning |
| --- | --- |
| `simulationId` | Opaque nonempty identity, new for every engine start |
| `updatedAt` | UTC time of the latest engine tick or effective fleet mutation |
| `messageCount` | Number of currently retained history records |
| `messageLimit` | Effective history capacity, initially 1000 |
| `vessels` | Complete active set, ordered by creation; `[]` for zero vessels |
| `vessels[].mmsi` | Stable numeric MMSI within this simulation run |
| `vessels[].name` | Human-readable synthetic name |
| `vessels[].typeId` | Reference to a metadata vessel type |
| `vessels[].report.sequence` | Monotonically increasing report number within this run |
| `vessels[].report.timestamp` | Full UTC generation time |
| `vessels[].report.sentence` | Latest complete type 1 AIVDM sentence with CRLF |

Every active vessel has a report, including immediately after creation. There
are no latitude, longitude, speed, course, or heading fields in this response;
consumers obtain navigation data from its NMEA payload. MMSI in the envelope
must match MMSI in that payload.

`updatedAt` advances on ticks even when the fleet is empty. Effective count
changes also update it; a no-op count request need not change it. Copy the entire
response from one locked engine snapshot. The API marshals the copy after
releasing the lock.

### History response

Use an envelope containing `simulationId`, `messageLimit`, `oldestSequence`,
`latestSequence`, and `messages`. Each message contains `sequence`, `mmsi`,
`timestamp`, and `sentence`. Use `null` sequence bounds for an empty history.
Bounds describe retained records, not a separate global counter.

Sequences start at 1, increase for every emitted sentence, and are not reused
when a vessel is removed. A restarted engine uses a new `simulationId` and
restarts its sequence. The pair identifies a report for clients deduplicating
repeated snapshots. Sequence gaps let a polling client detect records already
evicted from the bounded history.

The initial API has no cursor, pagination, acknowledgement, destructive read,
or replay guarantee. Clients may miss reports if they poll more slowly than
retention permits. Full current fleet snapshots still support live display
recovery. Removed vessels' retained reports remain available in history.

### Metadata response

Example structure, with illustrative runtime identity and timestamp:

```json
{
  "simulationId": "opaque-run-identity",
  "startedAt": "2026-09-14T12:00:00Z",
  "vesselTypes": [
    { "id": "cargo", "name": "Cargo vessel" }
  ],
  "supportedMessageTypes": [1],
  "settings": {
    "initialVesselCount": 1,
    "maxVessels": 100,
    "tickIntervalMs": 1000,
    "messageIntervalMs": 1000,
    "messageHistoryLimit": 1000,
    "speedKnots": { "min": 6, "max": 15.9 },
    "spawnBounds": {
      "south": 52,
      "north": 52.04,
      "west": 3.94,
      "east": 4
    }
  }
}
```

Derive these values from the actual engine configuration and constants. The
initial count is a startup setting; the live count comes from `vessels.length`.
Spawn bounds are decimal degrees; north and east are exclusive limits for the
current random generator. Speed bounds are inclusive, in 0.1-knot increments.

Type IDs are application categories, not numeric AIS ship-type codes. Assigning
the initial category does not add AIS type 5 or change movement. Metadata is
read-only. Adding fields for hypothetical configurable settings is unnecessary.

## Display endpoint

`GET /display/api/vessels` returns a complete decoded fleet, `simulationId`,
source `updatedAt`, and the subset of metadata needed by the browser. Vessel
fields are `mmsi`, `name`, `typeId`, `typeName`, `latitude`, `longitude`, `speed`,
`course`, `heading`, and `updatedAt`. Navigation values retain the current UI
units. AIS unavailable values are JSON `null`, not misleading zeroes.

Include `spawnBounds` so initial map framing can use simulation metadata.
Successful reads return 200 with `Cache-Control: no-store`. Upstream connection
failure, timeout, or a restart between the two upstream reads returns 503 with
a concise retryable message. Invalid upstream JSON, metadata, or AIS returns
502 with useful context. The browser keeps its previous markers on these errors
and displays that updates are unavailable.

The two upstream responses must have matching `simulationId` values. Metadata
settings are immutable within one run in this scope, so no metadata revision
protocol is required. Vessel names and type IDs travel in the atomic fleet
snapshot; metadata does not provide a separately fetched active roster.

## Verification and acceptance

- JSON contract tests cover empty arrays, full UTC timestamps, CRLF round trips,
  report sequence fields, nullable unavailable values, and request validation.
- All simulator transport types compile without importing UI or engine packages.
- Example metadata and response documentation use the exact chosen field names.
- External client instructions explain polling, deduplication, and bounded history.
- No runtime endpoint work is considered complete by merely defining the types;
  serving and state integration are covered by steps 02 and 03.

## Effort and complexity

Medium effort and complexity. The work is schema definition and focused contract
tests. The main decision is separating current active reports from retained
history so consumers have reliable fleet membership.
