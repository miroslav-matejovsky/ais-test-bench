// Package display is the display component backend. It consumes the simulator
// only through its public HTTP API and never reads engine state, decides
// whether a report was received, or recalculates RF values.
//
// Client reads one configured simulator origin. For every browser request it
// performs exactly one simulator read under a five-second deadline derived from
// the request context: GET /api/observations for Client.Observations, or
// GET /api/stations/{id}/receptions for Client.ReceptionHistory. It bounds the
// body before decoding, 8 MiB for observations and 2 MiB for history pages,
// and keeps no background poller, cache, clock, or history. It never reads the
// truth endpoints /api/vessels, /api/messages, or /api/metadata.
//
// # Validation
//
// The projection validates the whole response before producing anything:
//
//   - Run identity, startedAt, supported AIS type 1, settings ranges, revisions,
//     and a complete clock. The clock is decoded presence-aware; now is not
//     before startedAt, elapsedMs is not negative, speed is 0 or a valid step,
//     and paused is true exactly at speed 0. snapshotAt equals time.now.
//   - Decimal uint64 strings are canonical: no sign, leading zero, or overflow.
//   - Stations are unique and in ID order, with definitions, lifecycle times,
//     counters that partition opportunities, recent windows, history bounds,
//     and one coverage MultiPolygon per channel and threshold with closed,
//     bounded rings. The selection equals the requested station set.
//   - Every reception belongs to a known selected station, with sequences within
//     station and run bounds, revisions not newer than the station's, finite
//     link estimates, and a timestamp between startedAt and time.now. Its NMEA
//     passes framing and checksum, is type 1 on channel A or B, and matches the
//     reception MMSI, channel, and UTC second.
//   - Target MMSIs are unique; ages and statuses match the virtual age; the
//     chosen report's station is in provenance, and provenance marks chosen
//     exactly the stations that received the chosen transmission. Status
//     counts match targets and selected station provenance.
//   - All references to one transmission carry the same MMSI, channel,
//     timestamp, and sentence, and one reception identity names one
//     transmission. Each distinct sentence is decoded once per request.
//   - A history page matches its run, station, and cursor; its sequences are
//     contiguous within the retained bounds; nextAfter, hasMore, and gap
//     follow from the cursor and bounds.
//
// Navigation comes only from the chosen received sentence and is null when AIS
// marks a value unavailable. Scenario names and category IDs are labelled
// separately from AIS data; a category missing from the catalogue is added to
// scenarioCategories with its ID as its name. No AIS static data is invented.
// Estimated signal values use explicit names such as estimatedPowerDbm, and
// every reception carries its immutable reception-time receiver configuration.
//
// # Errors
//
// NewAPI serves GET /display/api/observations and
// GET /display/api/stations/{id}/receptions. Malformed display queries return
// 400. A simulator 400, 404 for an unknown station, or 409 for a history
// request from another run keeps its status and message, so the browser can
// reset a selection or cursor. A connection failure, timeout, or cancellation
// returns 503. Any other status or invalid JSON, AIS, geometry, reference, or
// contract returns 502 and a server log entry. A failed request never returns
// partial data, so the browser keeps its last complete view.
//
// # Examples
//
// A standalone display reading a simulator with one station and one received
// target, abbreviated:
//
//	curl http://localhost:8081/display/api/observations?stations=all
//	{
//	  "simulationId": "IJENAMFPYI57PPVSEAOORYM4Z6",
//	  "time": {"now": "2026-09-14T04:42:10.0590619Z", "elapsedMs": 1400, "speed": 1, "paused": false},
//	  "stateRevision": "12", "stationSetRevision": "1",
//	  "selection": ["st-1"],
//	  "stations": [{"id": "st-1", "definition": {...}, "coverage": [...], "counters": {...}, "selected": true, ...}],
//	  "targets": [{
//	    "mmsi": 200000000, "ageMs": 400, "status": "fresh",
//	    "report": {
//	      "stationId": "st-1", "sequence": "2", "transmissionSequence": "2", "mmsi": 200000000,
//	      "messageType": 1, "channel": "B", "receivedAt": "2026-09-14T04:42:09.6590619Z",
//	      "sentence": "!AIVDM,1,1,,B,...*hh\r\n",
//	      "navigation": {"latitude": 52.006843, "longitude": 3.957708, "speed": 7.3, "course": 321, "heading": 321, "utcSecond": 9},
//	      "scenario": {"name": "Vessel 1", "categoryId": "cargo"},
//	      "receiver": {...}, "signal": {"estimatedPowerDbm": -71.2, "marginDb": 38.8, "probability": 1, ...}
//	    },
//	    "stations": [{"stationId": "st-1", "receivedAt": "...", "estimatedPowerDbm": -71.2, "chosen": true, "currentRfRevision": true, ...}]
//	  }],
//	  "recentReceptions": [...], "recentIsSample": true, ...
//	}
//
// Before the simulator is reachable, the same request returns
// "503 simulator unavailable, retrying" as text/plain.
package display
