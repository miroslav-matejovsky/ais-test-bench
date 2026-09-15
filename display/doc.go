// Package display validates received-traffic snapshots from a local or remote
// Source and serves the display API and page. It never reads the truth fleet,
// decides whether a report was received, or recalculates RF values.
//
// New borrows a Source and constructs a Client without reading or starting work.
// The public simulator.Simulator satisfies Source; NewHTTPSource provides the
// HTTP implementation. NewClient is a convenience constructor owning an HTTP
// source for one simulator API base URL. Source signatures use public
// simulatorapi DTOs.
//
// HTTPConfig.APIBase is the simulator API base URL including any path prefix,
// for example https://example.test/tools/ais/api/. Its path is absolute, has no
// empty, dot, or percent-encoded segment, and ends with "/"; relative endpoint
// paths are appended below it. User info, query, and fragment are rejected. A
// supplied HTTP client adds host authentication, transports, and TLS. Browser
// credentials are never forwarded.
//
// Each Client request makes exactly one source read under a five-second deadline
// derived from the caller context, then validates and decodes the whole snapshot.
// Local and remote snapshots pass through the same semantic and AIS checks.
// HTTPSource additionally validates JSON framing, required clock field presence,
// canonical decimal strings, and content type. It bounds bodies to 8 MiB for
// observations and 2 MiB for history before decoding. It reads only
// {APIBase}observations and {APIBase}stations/{id}/receptions, never the truth
// fleet, history, or metadata. Clients hold no poller, cache, clock, or history.
//
// Callers own sources supplied to New and clients supplied in HTTPConfig. Neither
// is closed or mutated by the library. CloseIdleConnections releases transports
// owned by HTTPSource or by the NewClient convenience constructor after requests
// finish. Serve owns its listener and closes only the client's owned connections.
// All reads are concurrency-safe. Source methods must honor cancellation and
// return owned snapshots that remain valid after subsequent reads. Cancellation
// is checked before and after reading and projection; CPU-bound conversion may
// finish before cancellation is observed, but no canceled result is returned.
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
// Source errors wrap simulatorapi.ErrInvalidRequest, ErrNotFound, ErrConflict,
// ErrUnavailable, or ErrInvalidResponse. Use errors.Is to inspect categories and
// wrapped causes. Unclassified source failures are HTTP 500. Errors return no
// partial value. Transport failures preserve their underlying network errors.
//
// NewHandler serves the local routes GET /observations and
// GET /stations/{id}/receptions. Mount it below a public API base with
// http.StripPrefix; it sets no CORS or authentication policy. Malformed display queries return
// 400. A simulator 400, 404 for an unknown station, or 409 for a history
// request from another run keeps its status and diagnostic, so the browser can
// reset a selection or cursor. A connection failure, timeout, or cancellation
// returns 503, as does an upstream 503. Other unexpected statuses or invalid JSON,
// AIS, geometry, reference, or
// contract returns 502 and a server log entry. A failed request never returns
// partial data, so the browser keeps its last complete view.
//
// NewStandaloneHandler serves the display below an optional path prefix: the API
// at {base}/display/api/, the page, assets at {base}/assets/, and a {base}/
// redirect to the page that keeps the query. StandaloneConfig.ManagerURL is an
// explicit, optional navigation link and is never inferred from the source. Serve
// runs that handler on a caller-bound listener until cancellation or a serving
// failure, drains requests within five seconds, and closes only owned idle
// connections.
//
// # Logging
//
// Client, HTTPSource, and New neither take nor emit logs; they return errors.
// Config and StandaloneConfig accept a logger; nil means slog.Default(), resolved
// once per call without changing the process default. Each derives one logger
// with component=display, preserving the caller's attributes, groups, and levels,
// so independently constructed handlers never share records.
//
// Normal operation is quiet: successful reads and 4xx responses emit nothing, and
// neither does a request whose client went away. A consumed 5xx source failure
// logs one Warn record with the operation, path, status, and error; a failed
// response write logs at Error. Both use the request context. Serve logs start
// and stop at Info and HTTP server errors at Error.
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
