---
title: "04 - Simulator HTTP contract and manager controls"
dependencies: ["03-engine-observations.md"]
effort: "L"
complexity: "high"
---

# Simulator HTTP contract and manager controls

## Outcome

Expose a consistent observation snapshot and bounded station history over HTTP.
Provide station management through the simulator component's existing manager.

## Implementation work

Extend `internal/simulatorapi` types/docs, `internal/simulator` handlers/conversion,
and `internal/simdriver` serialized commands. Extend the existing manager template,
JavaScript, and CSS. Update app startup to install station definitions before
creating initial vessels, so creation reports have the intended receivers.

### Proposed routes

| Method and route | Contract |
| --- | --- |
| `GET /api/stations` | Atomic configuration snapshot: simulation ID, station-set revision, virtual time, effective model/reference transmitter, limits, and ordered stations. |
| `POST /api/stations` | Create one site from a complete definition, expected simulation ID, and station-set revision; return 201 with assigned ID and resulting configuration snapshot. |
| `PUT /api/stations/{id}` | Replace editable configuration using expected simulation ID and station-set revision; return the resulting configuration snapshot. ID is immutable. |
| `DELETE /api/stations/{id}` | Require expected simulation ID and station-set revision in the query; return resulting configuration snapshot. |
| `GET /api/observations?stations=all` | Atomic current observation snapshot. Also accept a comma-separated set of station IDs for a union view. Missing selector means all. |
| `GET /api/stations/{id}/receptions?simulationId=...&after=...&limit=...` | Bounded ascending page of successful station receptions, with explicit cursor and gap information. |

Keep `/api/vessels`, `/api/messages`, `/api/metadata`, and `/api/time` with their
truth/clock responsibilities. Update metadata docs: station configuration is now
mutable and has its own revision. The observation API contains everything the
display needs without a second metadata or truth-fleet request.

### Observation snapshot

| Field group | Required content |
| --- | --- |
| Identity | `simulationId`, `stateRevision`, `stationSetRevision`, `snapshotAt` equal to committed virtual `time.now`. |
| Clock and contract | `startedAt`, complete existing `time` shape, model version, reference transmitter, age/history limits, scenario type catalogue. |
| Selection | Canonical sorted station IDs; `all` resolves to existing stations including disabled ones. Reject an explicit empty selector. |
| Stations | All current stations with labels, RF config/revisions, effective channel status, coverage geometry, counters, recent rate windows, and history bounds. |
| Targets | Up to 1,000 distinct observed MMSIs in the selected union; chosen received report and scenario metadata; age/status; compact last reception provenance for every selected observing station. |
| Recent receptions | Newest 50 matching successful receptions sorted by virtual timestamp, transmission sequence, station ID; display reverses for newest-first presentation. |
| Retention | Limits, target-capacity eviction count, and per-station sequence bounds. Explicitly indicate the recent list is a sample, not a continuous feed. |

A chosen report includes exact NMEA, transmission sequence, channel, receive time,
and chosen reception/station identity. Compact provenance includes last reception
time, sequence, channel, estimated power, RF revision, and whether it refers to
the chosen report. Selecting a different station fetches that station's actual last report.
Do not repeat all per-station NMEA payloads in the aggregate target object.

Capture the selected data under one engine read lock. Encode JSON after releasing
the lock, using detached values and immutable geometry/configuration snapshots.
Never build the response from independent `Fleet`, `Metadata`, and station reads.
Stable ordering makes output reproducible: station ID, MMSI, and explicit event
sort keys. Empty collections are `[]`; optional measurements and absent bounds
are null, never fabricated zeroes.

Use decimal strings for new uint64 sequences and revisions in JSON, to avoid
JavaScript integer precision loss. Define comparison/parsing once in the display
client. Existing transmission endpoints can keep their current wire representation;
conversion tests must prove identity is preserved across the two contracts.

### History polling and loss detection

Default `limit=100`, maximum 200. For an initial request without `after`, return
the most recent `limit` events in ascending order, mark it as a tail sample, and
set `nextAfter` to its last reception sequence. For cursor requests return the
earliest retained entries strictly after `after`, at most `limit`.

Return `oldestAvailable`, `latestAvailable`, `nextAfter`, `hasMore`, `gap`, and
`truncatedBefore`. `gap` is true when the requested cursor precedes the retained
window by at least one event (`after + 1 < oldestAvailable`, using overflow-safe
comparison). Return retained records with the gap flag; a history gap is not an
upstream failure. With no new records, `nextAfter` remains the request cursor.
For a station that has never received, bounds are null and the cursor starts at
zero. Reject a future cursor with 400. Reject a mismatched `simulationId` with
409 and the current run identity. An unknown/deleted station returns 404.

Do not compare reception sequences across stations. A transmission-sequence gap
inside station history is normal missed reception, not necessarily polling loss.
GET never consumes history or creates a server-side cursor. Multiple tabs can
read independently. Current target recovery always uses `/api/observations`.

### Writes and errors

Continue strict JSON handling: required fields, no unknown fields, exactly one
object, finite values, correct content type, bounded strings and body. Use a
separate maximum 64 KiB station-write body, preserving the current small limit
for count/speed writes. Return 413 for oversized new-route bodies, 415 for wrong
media type, 400 for invalid configuration/query, 404 for missing station, 409 for
stale configuration revision or simulation identity, 405 with Allow for invalid
methods, and 500 with useful server-side error context for application failures.
JSON is `no-store`.

Driver commands validate before settling time, acquire the serialization lock,
check expected simulation identity and station-set revision, settle at the previous
speed, apply the edit, and return the exact resulting configuration snapshot before
releasing the lock.
A stale edit does not advance time. Settlement can commit elapsed chunks before
a later failure, matching current driver behavior; document this separately from
engine mutation atomicity. Never claim an HTTP write rolls back already settled
virtual time. GET remains read-only.

### Manager behavior

Add a stations table with name/ID, location, enabled channels, capability summary,
and create/edit/disable/delete controls. An expandable form groups general site
settings, channel A/B capability, and optional shadow sectors with unit labels.
Prefill concrete values from the default definition, but validate in the engine.

Show field errors beside inputs. Preserve unsaved edits while polling refreshes
other state. On 409, show that another edit changed the station set, fetch the
current configuration, and let the user reconcile; do not silently overwrite.
The display remains read-only and links to the manager for configuration.
Apply preset changes through the same endpoints as manual edits.

## Verification

Use handler tests for every route, status, required/unknown field, body limit,
query selector, duplicated selector ID, invalid cursor, empty history, and no-op
write. Test cursor rollover, run restart, deletion, history gaps, and two manager
clients with stale revisions. Check snapshots do not mix clocks/configuration
under concurrent ticking, and commands return their own resulting revision.

Serialize maximum-size observation and history fixtures with worst-case permitted
strings/configuration. Assert they fit the [resource limits](assessment.md).
Verify manager behavior with existing Go HTML/handler tests and a documented
browser checklist; do not introduce a frontend build solely for these forms.

## Acceptance criteria

- Display consumers can obtain all observation data with one consistent HTTP read.
- Histories distinguish genuine reception gaps from missing retained events.
- Station edits use existing simulation-time settlement and serialization rules.
- Manager controls expose all implemented capabilities and surface write conflicts.
- Wire docs contain complete field, unit, lifecycle, selection, and error semantics.
