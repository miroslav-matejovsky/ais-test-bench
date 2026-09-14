// Package display is the display component backend. It consumes the simulator
// only through its public HTTP API and never reads engine state.
//
// Client reads one configured simulator origin. For every browser request,
// Client.Fleet reads /api/vessels and /api/metadata concurrently under one
// five-second deadline derived from the request context, bounds each body to
// 2 MiB, and waits for both reads before returning. It keeps no background
// poller, cache, or history.
//
// The projection validates both responses before producing anything: matching
// simulation identities, required metadata, unique MMSIs, and every report
// decoded with package ais with its payload MMSI equal to the envelope MMSI.
// Positions, speed, course, and heading come only from the NMEA payloads and
// are null when AIS marks them unavailable. Names and type IDs come from the
// fleet, type names from metadata, and full report times from the envelope.
//
// NewAPI serves the projection at GET /display/api/vessels. A connection
// failure, timeout, or simulator restart between the two reads returns 503; an
// invalid simulator response returns 502. A failed request never returns a
// partial or empty fleet, so the browser keeps its last known markers.
//
// # Examples
//
// Captured from a standalone display reading a simulator with two vessels.
// Coordinates are the decoded AIS values, in 1/600000 degree steps:
//
//	curl http://localhost:8081/display/api/vessels
//	{
//	  "simulationId": "IJENAMFPYI57PPVSEAOORYM4Z6",
//	  "updatedAt": "2026-09-14T04:42:09.9398422Z",
//	  "spawnBounds": {"south": 52, "north": 52.04, "west": 3.94, "east": 4},
//	  "vessels": [
//	    {
//	      "mmsi": 200000000,
//	      "name": "Vessel 1",
//	      "typeId": "cargo",
//	      "typeName": "Cargo vessel",
//	      "latitude": 52.006843333333336,
//	      "longitude": 3.957708333333333,
//	      "speed": 7.3,
//	      "course": 321,
//	      "heading": 321,
//	      "updatedAt": "2026-09-14T04:42:09.6601754Z"
//	    },
//	    {
//	      "mmsi": 200000001,
//	      "name": "Vessel 2",
//	      "typeId": "cargo",
//	      "typeName": "Cargo vessel",
//	      "latitude": 52.00447666666667,
//	      "longitude": 3.9865683333333335,
//	      "speed": 13.9,
//	      "course": 340,
//	      "heading": 340,
//	      "updatedAt": "2026-09-14T04:42:09.9398422Z"
//	    }
//	  ]
//	}
//
// Before the simulator is reachable, the same request returns
// "503 simulator unavailable, retrying" as text/plain.
package display
