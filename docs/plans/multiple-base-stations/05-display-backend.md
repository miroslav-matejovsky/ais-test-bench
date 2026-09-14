---
title: "05 - HTTP-only display observation projection"
dependencies: ["04-simulator-api-and-manager.md"]
effort: "M"
complexity: "medium"
---

# HTTP-only display observation projection

Status: implemented. The display backend and its page read only received
observations over HTTP. Station, coverage, and inspector presentation remain in step 06.

## Outcome

The display backend reads station observations from the simulator, validates the
contract, and decodes only successfully received AIS reports for presentation.

## Implementation work

Update `internal/display/client.go`, `project.go`, `types.go`, `api.go`, and
`doc.go`. Continue using the configured simulator origin and request-scoped HTTP
client. Add no engine imports, polling goroutine, persistent cache, or RF logic.

Replace the browser's main feed with
`GET /display/api/observations?stations=all`. Add
`GET /display/api/stations/{id}/receptions` as a validated history projection of
the corresponding simulator route. Retire `/display/api/vessels` when its browser
caller and docs migrate. The simulator's truth `/api/vessels` remains available
to its existing manager/debug consumers.

Each observation request performs one upstream observation read under the existing
five-second wall-clock deadline. Apply the new 8 MiB observation body bound and
2 MiB history bound before decoding. Preserve cancellation, connection reuse,
content-type validation, and contextual errors. The browser calls only its own
origin in combined and standalone mode.

### Validate before projecting

- Require simulation identity, model/reference settings, complete clock, revisions,
  and recognized model version. Validate virtual time and age limits without
  extrapolating wall time.
- Require unique station IDs and target MMSIs, valid station/channel configuration,
  bounded collections, and known station references in current observations.
- Check sequence/revision strings parse as uint64; reject overflow, negative values,
  invalid lexical forms, impossible bounds, and duplicate reception identities.
- Require every target's chosen reception to belong to the requested selection.
  Check provenance indicates exactly which stations received the chosen transmission.
- Validate NMEA framing/checksum, supported type 1, channel A/B, MMSI equality,
  timestamp presence, and payload UTC second consistency when available. Within
  this atomic snapshot, no reception timestamp may exceed `time.now`.
- Decode latitude, longitude, speed, course, and heading solely from the chosen
  received sentence. Preserve existing AIS unavailable values as null. A target
  without position stays in the table but gets no map marker.
- Keep scenario-provided name/category separately identified from received AIS
  navigation. An unknown category uses its ID as a label; do not invent AIS static
  fields such as call sign, dimensions, or destination.
- Require finite estimated power/margin/distance, probability in [0, 1], valid
  age/status, and consistent counts/windows. Do not recalculate RF to validate it.
- Validate coverage type, coordinates, ring closure, bounds, vertex limits,
  channel, threshold, model version, and matching current RF revision. Geometry
  is simulator-owned presentation data; render it unchanged.

Repeated references to the same transmission within one response must have the
same sentence, channel, MMSI, and timestamp. Decode each distinct sentence once
per request where it appears in targets and the recent-message sample. Do not
merge unrelated reports because they share MMSI or a one-second timestamp.

### Projection and failures

Return decoded targets, station configuration/coverage/counters, provenance,
received-message summaries, raw NMEA for inspection, and the validated committed
clock. Preserve RF values and receipt times with explicit field names such as
`estimatedPowerDbm` and `receivedAt`. Full message details include the immutable
reception-time configuration so later station changes cannot mislabel an old signal.

Network failure or timeout returns 503. Invalid upstream JSON, AIS, geometry,
references, or contract returns 502. No partial observation response is returned.
Unknown station selection is a user-visible 404, not a generic outage. A run-ID
conflict on history is 409 so the browser clears its cursor and reloads. Other
unexpected upstream status codes receive the existing invalid/upstream-failure
treatment with actionable server logs.

A new run's complete snapshot replaces the browser state. For detail requests
running concurrently with the main poll, the browser checks simulation identity
and active selection before showing the response. Historical RF revisions are
valid but labelled; current coverage must always match the current station revision.

Keep `.go-arch-lint.yml` dependencies unchanged except responsibility comments if
needed. The combined application's loopback API must mount every new read route.

## Verification

Use `httptest` fixtures for received-old/missed-new navigation, two stations with
different last receptions, overlap deduplication, null navigation, both channels,
bad checksum, MMSI/channel mismatch, future timestamps, malformed geometry,
unknown references, overflow, excessive bodies, and selector forwarding.

Test failure status mapping, cancellation, history gaps and conflicts, empty
observations with nonempty truth fleet, and no dependence on `/api/vessels` or
`/api/metadata`. An upstream fixture should reject any unexpected route access.
Check combined and standalone composition with the same fixture contract.

## Acceptance criteria

- The default feed is derived exclusively from received reports.
- One bad upstream snapshot preserves the browser's previous complete view.
- Shared types remain data-only; display never imports simulation or simulator.
- HTTP observations support both process arrangements without behavior differences.

## Implementation and verification record

- `/display/api/vessels`, `Client.Fleet`, and the fleet types are removed. The page
  polls `/display/api/observations?stations=all`, draws only received targets with
  a position, mutes stale targets, hides lost ones, and keeps its last view on 502/503.
- `Client.Observations` and `Client.ReceptionHistory` perform one bounded read each
  (8 MiB / 2 MiB, five-second deadline). Simulator 400/404/409 API errors keep their
  status; other statuses and contract violations return 502; transport failures 503.
- `validate.go` checks settings, clock, station definitions, counter partitions,
  history bounds, and coverage geometry. `project.go` checks references, ages,
  provenance, transmission and reception identity consistency, and decodes each
  distinct sentence once. A token scan rejects noncanonical decimal uint64 strings,
  which `encoding/json` alone accepts with leading zeros.
- Scenario categories missing from the catalogue are appended with their ID as name,
  so history pages, which carry no catalogue, use the same labels.
- No model version exists in the wire contract and coverage carries no revision of
  its own; validation checks station RF revisions and contour shape instead.
- Detail and history presentation, including identity/selection checks of
  concurrent responses, is implemented in step 06.
- `httptest` fixtures derive snapshots and history pages from one consistent
  scenario and fail on any other route. `task all` passed with 594 tests. A smoke run
  of the combined binary at 100 vessels and 100x returned 25 of 25 valid snapshots
  (267 KB), a truncated history tail, a gap page, and a paused station selection.
