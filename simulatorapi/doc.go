// Package simulatorapi defines simulator wire DTOs, source error categories,
// and HistoryRequest for local and remote received-traffic reads.
// It has no dependency on the simulation engine, HTTP handlers, or UI, so the
// simulator, the display backend, and tests can share one contract.
//
// Sources return owned snapshots, honor caller cancellation, and wrap ErrInvalidRequest,
// ErrNotFound, ErrConflict, ErrUnavailable, or ErrInvalidResponse while retaining
// their original cause. HistoryRequest and ValidateSelection validate direct reads;
// ErrorStatus maps categories to HTTP 400, 404, 409, 503, and 502 respectively.
// Wire sequences, revisions, and counters remain decimal JSON strings; the same
// Go DTOs are used for local reads without JSON encoding.
//
// # Routes
//
//	GET /api/vessels   Fleet: the complete active fleet with latest reports
//	PUT /api/vessels   CountRequest in, Fleet out
//	GET /api/messages  History: retained reports, oldest first
//	GET /api/metadata  Metadata: run identity, virtual clock, catalogs, settings
//	PUT /api/time      TimeRequest in, Metadata out
//	GET /api/stations  StationSet: atomic configuration, clock, settings
//	POST /api/stations StationRequest in, StationCreated out (201)
//	PUT /api/stations/{id} StationRequest in, StationSet out
//	DELETE /api/stations/{id} simulationId and stationSetRevision query, StationSet out
//	GET /api/observations Observations: selected received targets and station state
//	GET /api/stations/{id}/receptions ReceptionPage: bounded successful receptions
//
// Successful responses are JSON with Cache-Control: no-store. Reads never
// mutate engine state or advance time. PUT requires application/json, one
// object, no unknown fields, and a body of at most 1 KiB. A count is a required
// integer from 0 to Settings.MaxVessels. A speed is a required number: 0 pauses,
// otherwise it is Settings.Speed.Min to Settings.Speed.Max in Settings.Speed.Step
// increments. Invalid requests return 400 and change nothing; other media types
// return 415. A valid change the simulator cannot apply returns 500.
//
// # Virtual time
//
// All simulation timestamps are virtual UTC instants. Metadata.StartedAt is the
// initial instant; the simulator reports every vessel at each TickIntervalMs of
// virtual time after it. The simulator paces virtual time with real elapsed
// time multiplied by the speed. Speed never changes the knots in reports.
// Metadata.Time is the committed clock; it is not extrapolated and can trail
// real time by one PacingIntervalMs plus processing time. While paused,
// time.now stays unchanged and the API keeps answering. Before a count or speed
// change, the simulator settles elapsed time at the previous speed, so the
// change applies at the current virtual instant. HTTP deadlines and shutdown
// always use real time.
//
// Every response is a separate snapshot. Matching simulation IDs identify the
// same run, not the same instant: a report timestamp can be later than a
// time.now read earlier.
//
// # AIS transport
//
// Each report carries the exact checksummed NMEA sentence, including its
// trailing CRLF, in a JSON string. Fleet contains no decoded navigation values;
// consumers decode position, speed, course, and heading from the sentence. The
// payload MMSI always equals the envelope MMSI. The payload holds only the UTC
// second, so the full generation time is in the report timestamp.
//
// # Sequences and polling
//
// Report sequences start at 1 for each simulation run and increase with every
// emitted sentence. Reads never consume sequences. A new engine start has a new
// SimulationID and restarts sequences, so the pair (SimulationID, Sequence)
// identifies one report for deduplication.
//
// The API is a finite snapshot API, not a stream. Transmission History is bounded; clients
// polling slower than retention allows miss reports and can detect that from
// sequence gaps. Higher speeds emit more reports per real second, so they
// shorten the real time history covers. Transmission History has no cursor or replay guarantee.
// Fleet always contains exactly the active vessels, so a live view recovers from
// any gap by replacing its state with the latest Fleet.
//
// # Station configuration
//
// StationSet embeds the Metadata fields alongside stateRevision,
// stationSetRevision, snapshotAt, and stations. All fields are captured under
// one engine lock. snapshotAt equals time.now. JSON encoding happens after the
// lock is released. Station writes return their own resulting snapshot before
// another driver command can change it. StationSetRevision changes only for
// effective station edits; StateRevision also follows clock/fleet changes.
//
// Each Station carries id, definition, configRevision, rfRevision, createdAt,
// rfUpdatedAt, and coverage. Definition fields are name, latitude, longitude,
// enabled, antennaHeightMeters, receiveGainDbi, feederLossDb, channelA, channelB,
// and shadowSectors. Both channels require enabled, sensitivityDbm,
// noisePenaltyDb, and dropProbability. Sectors require startDegrees, endDegrees,
// and lossDb. Every definition field must be present and non-null, including
// false, zero, and empty shadowSectors: []. Public type comments document ranges.
// Lower sensitivity dBm means a more sensitive receiver. Model power and margin
// are estimates, not AIS payload measurements.
//
// POST and PUT require application/json and this complete body:
//
//	{"simulationId":"run-1","stationSetRevision":"1","definition":{
//	 "name":"Coast","latitude":52,"longitude":4,"enabled":true,
//	 "antennaHeightMeters":25,"receiveGainDbi":3,"feederLossDb":2,
//	 "channelA":{"enabled":true,"sensitivityDbm":-110,"noisePenaltyDb":0,"dropProbability":0},
//	 "channelB":{"enabled":true,"sensitivityDbm":-110,"noisePenaltyDb":0,"dropProbability":0},
//	 "shadowSectors":[]}}
//
// Creation returns stationId plus the resulting StationSet, status 201. Deletion
// takes simulationId and stationSetRevision as query parameters. A no-op edit
// preserves station revisions; a name-only edit preserves RF revision. Newly
// created or re-enabled stations receive only later transmissions. Moving a
// station updates coverage while retained receptions keep their original RF
// attribution, including the original name. Deletion removes the station's
// history and current observations; its ID is not reused within the run.
//
// Writes validate before settling elapsed time. A stale run/revision, missing
// station, invalid definition, or full station set is rejected without settling.
// A valid write settles at the old speed and applies at the resulting virtual
// instant. Settlement failures can retain elapsed chunks already delivered;
// the station edit itself remains atomic. Run IDs are immutable per engine.
//
// New routes return APIError JSON with error and optional fields keyed by input
// path (for example channelA.sensitivityDbm). Errors are 400 for invalid input,
// 404 for unknown stations, 409 for run/revision conflicts, 413 for a body over
// 64 KiB, 415 for wrong media type, and 500 for a driver/application failure.
// Conflicts include current simulationId and stationSetRevision. Station
// definitions are limited to 4 KiB after canonical JSON encoding. Unsupported
// methods use standard HTTP 405 and Allow. Unknown/duplicate query parameters
// are rejected, as are unknown JSON fields and extra JSON objects.
//
// # Observation snapshots
//
// GET /api/observations accepts stations=all (also the default) or a comma-separated
// list of distinct station IDs. Empty selectors, duplicates, and mixtures of all
// with IDs return 400; unknown IDs return 404. Selection is a union. Disabled
// stations retain observations until they age out; deleted stations contribute
// nothing. Station and selection lists use ascending opaque ID order. Target
// lists use MMSI order. Equal chosen transmissions retain the engine's first
// receiver in creation order, with Chosen=true on every matching provenance entry.
//
// Observations embeds Metadata and contains stateRevision, stationSetRevision,
// snapshotAt, selection, all stations with counters/history bounds, selected
// targets and counts, recentReceptions, recentIsSample=true, and run totals.
// All values refer to the same committed state. Settings additionally publishes
// maxStations, transmitter, reception model constants,
// and observation age/retention limits. Metadata.Time changes during a run;
// mutable station configuration is carried by station/observation endpoints.
//
// Each target has mmsi, report (a Reception), ageMs, status, and compact station
// provenance. Decode navigation only from report.sentence. Its full timestamp
// is both transmission and reception time. VesselName and VesselTypeID are
// scenario labels; type 1 AIS does not carry them. Missed transmissions never
// update observed navigation. Ages are virtual and freeze while paused: fresh
// through 10 seconds, stale through 60 seconds, lost until expiry at 600 seconds.
// CurrentTargets counts fresh plus stale; LostTargets is separate. Per-station
// counts cover all that station's observations, regardless of the selection.
//
// Lifetime opportunities count every transmission while a station exists,
// including disabled periods. Received plus mutually exclusive loss counters
// equals opportunities. Per-channel totals count the same channel opportunities.
// Recent counters use one-second buckets over at most 60 virtual seconds, with
// durationMs and rates per virtual second. Rates are null at zero duration;
// ratios are null without opportunities. Counters span RF edits; rfUpdatedAt
// identifies a change within the window. They are simulator diagnostics.
//
// Coverage is GeoJSON MultiPolygon, with closed counterclockwise exterior rings
// and [longitude,latitude] coordinates. Antimeridian crossings are split into
// bounded pieces. An empty coverage has coordinates: []. Each contour states
// channel, threshold (0.9 or 0.5), and min/max radius in metres. Its station RF
// revision and Settings reference transmitter/model parameters apply to every contour.
// Geometry is sampled estimated coverage; reception never uses polygon membership.
//
// # Reception history and precision
//
// Reception sequences are independent and contiguous per station. Their identity
// is (simulationId, stationId, sequence); transmissionSequence refers to the
// original report identity. All new uint64 counters, sequences, revisions, and
// cursors encode as decimal strings. Legacy transmission endpoints retain numeric
// sequences. New requests require canonical unsigned decimal strings, without
// signs or leading zeros. Compare identities without float conversion.
//
// History requests require simulationId, optionally after and limit. Limit
// defaults to 100 and is 1-200. Without after, the newest limit records form an
// ascending tail sample. With after, return the earliest retained records strictly
// after it. NextAfter is the final returned sequence, or the unchanged request
// cursor when empty (zero for an empty tail). HasMore means later retained events
// exist. OldestAvailable/LatestAvailable are null before the first reception.
// TruncatedBefore is the first retained sequence when any earlier records were
// evicted, otherwise null. Gap is true only if the requested continuation lost
// events. A transmission-sequence gap can instead be an ordinary missed signal.
// Future cursors return 400; another run returns 409, even for a reused station ID.
//
// Histories retain 1,000 receptions per station. Recent snapshots sample 50;
// neither promises lossless replay. Latest observed targets survive event-history
// eviction. At most 1,000 distinct observed MMSIs are retained, with deterministic
// oldest-target eviction exposed in targetEvictions. Current observations recover
// from history gaps through a full snapshot, never through the truth Fleet.
// Empty collections encode as [], optional unavailable values and bounds as null.
// Consumer body budgets are 8 MiB for observations and 2 MiB for reception pages.
//
// # Examples
//
// Example responses from one standalone simulator run. Metadata describes the
// run, its clock, and the settings the engine actually uses:
//
//	curl http://localhost:8000/api/metadata
//	{
//	  "simulationId": "IJENAMFPYI57PPVSEAOORYM4Z6",
//	  "startedAt": "2026-09-14T04:42:08.6590619Z",
//	  "time": {"now": "2026-09-14T04:42:13.7612053Z", "elapsedMs": 5102, "speed": 1, "paused": false},
//	  "vesselTypes": [{"id": "cargo", "name": "Cargo vessel"}],
//	  "supportedMessageTypes": [1],
//	  "settings": {
//	    "initialVesselCount": 1,
//	    "maxVessels": 100,
//	    "tickIntervalMs": 1000,
//	    "messageIntervalMs": 1000,
//	    "pacingIntervalMs": 100,
//	    "messageHistoryLimit": 1000,
//	    "speedKnots": {"min": 6, "max": 15.9},
//	    "speed": {"min": 0.01, "max": 100, "step": 0.01},
//	    "spawnBounds": {"south": 52, "north": 52.04, "west": 3.94, "east": 4}
//	  }
//	}
//
// The fleet one second after start holds the default vessel with its latest
// report. The sentence string ends with the escaped CRLF:
//
//	curl http://localhost:8000/api/vessels
//	{
//	  "simulationId": "IJENAMFPYI57PPVSEAOORYM4Z6",
//	  "updatedAt": "2026-09-14T04:42:09.6590619Z",
//	  "messageCount": 2,
//	  "messageLimit": 1000,
//	  "vessels": [
//	    {
//	      "mmsi": 200000000,
//	      "name": "Vessel 1",
//	      "typeId": "cargo",
//	      "report": {
//	        "sequence": 2,
//	        "timestamp": "2026-09-14T04:42:09.6590619Z",
//	        "sentence": "!AIVDM,1,1,,A,12vg200P190B7O2MhS2dRb2B0000,0*06\r\n"
//	      }
//	    }
//	  ]
//	}
//
// After a count of 2, history holds both reports of Vessel 1 and the immediate
// report of the added Vessel 2, oldest first:
//
//	curl http://localhost:8000/api/messages
//	{
//	  "simulationId": "IJENAMFPYI57PPVSEAOORYM4Z6",
//	  "messageLimit": 1000,
//	  "oldestSequence": 1,
//	  "latestSequence": 3,
//	  "messages": [
//	    {"sequence": 1, "mmsi": 200000000, "timestamp": "2026-09-14T04:42:08.6590619Z", "sentence": "!AIVDM,1,1,,A,12vg200P190B7OdMhRvdRb2@0000,0*17\r\n"},
//	    {"sequence": 2, "mmsi": 200000000, "timestamp": "2026-09-14T04:42:09.6590619Z", "sentence": "!AIVDM,1,1,,A,12vg200P190B7O2MhS2dRb2B0000,0*06\r\n"},
//	    {"sequence": 3, "mmsi": 200000001, "timestamp": "2026-09-14T04:42:09.9398422Z", "sentence": "!AIVDM,1,1,,A,12vg20@P2;0B?t:MhMOeB:`B0000,0*34\r\n"}
//	  ]
//	}
//
// A count of 0 empties the fleet and keeps the retained history:
//
//	curl -X PUT -H "Content-Type: application/json" -d '{"count":0}' http://localhost:8000/api/vessels
//	{
//	  "simulationId": "IJENAMFPYI57PPVSEAOORYM4Z6",
//	  "updatedAt": "2026-09-14T04:42:10.6418411Z",
//	  "messageCount": 3,
//	  "messageLimit": 1000,
//	  "vessels": []
//	}
//
// A speed of 0 pauses virtual time and returns the full metadata:
//
//	curl -X PUT -H "Content-Type: application/json" -d '{"speed":0}' http://localhost:8000/api/time
//	{
//	  "simulationId": "IJENAMFPYI57PPVSEAOORYM4Z6",
//	  "startedAt": "2026-09-14T04:42:08.6590619Z",
//	  "time": {"now": "2026-09-14T04:42:15.9690619Z", "elapsedMs": 7310, "speed": 0, "paused": true},
//	  ...
//	}
//
// Rejected requests change nothing and return a plain text error:
//
//	-d '{"count":101}'                  400 count must be between 0 and 100
//	-d '{"speed":0.015}'                400 invalid simulation input: speed must be a multiple of 0.01: 0.015
//	-H "Content-Type: text/plain"       415 Content-Type must be application/json
package simulatorapi
