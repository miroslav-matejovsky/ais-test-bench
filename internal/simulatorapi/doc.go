// Package simulatorapi defines the JSON wire types of the simulator HTTP API.
// It has no dependency on the simulation engine, HTTP handlers, or UI, so the
// simulator, the display backend, and tests can share one contract.
//
// # Routes
//
//	GET /api/vessels   Fleet: the complete active fleet with latest reports
//	PUT /api/vessels   CountRequest in, Fleet out
//	GET /api/messages  History: retained reports, oldest first
//	GET /api/metadata  Metadata: run identity, catalogs, effective settings
//
// Successful responses are JSON with Cache-Control: no-store. Reads never
// mutate engine state. PUT requires application/json, one object, a required
// integer count from 0 to Settings.MaxVessels, no unknown fields, and a body of
// at most 1 KiB.
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
// The API is a finite snapshot API, not a stream. History is bounded; clients
// polling slower than retention allows miss reports and can detect that from
// sequence gaps. There is no cursor or replay guarantee. Fleet always contains
// exactly the active vessels, so a live view recovers from any gap by replacing
// its state with the latest Fleet.
//
// # Examples
//
// Responses captured from one standalone simulator run. Metadata describes the
// run and the settings the engine actually uses:
//
//	curl http://localhost:8000/api/metadata
//	{
//	  "simulationId": "IJENAMFPYI57PPVSEAOORYM4Z6",
//	  "startedAt": "2026-09-14T04:42:08.6590619Z",
//	  "vesselTypes": [{"id": "cargo", "name": "Cargo vessel"}],
//	  "supportedMessageTypes": [1],
//	  "settings": {
//	    "initialVesselCount": 1,
//	    "maxVessels": 100,
//	    "tickIntervalMs": 1000,
//	    "messageIntervalMs": 1000,
//	    "messageHistoryLimit": 1000,
//	    "speedKnots": {"min": 6, "max": 15.9},
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
//	  "updatedAt": "2026-09-14T04:42:09.6601754Z",
//	  "messageCount": 2,
//	  "messageLimit": 1000,
//	  "vessels": [
//	    {
//	      "mmsi": 200000000,
//	      "name": "Vessel 1",
//	      "typeId": "cargo",
//	      "report": {
//	        "sequence": 2,
//	        "timestamp": "2026-09-14T04:42:09.6601754Z",
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
//	    {"sequence": 2, "mmsi": 200000000, "timestamp": "2026-09-14T04:42:09.6601754Z", "sentence": "!AIVDM,1,1,,A,12vg200P190B7O2MhS2dRb2B0000,0*06\r\n"},
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
// Rejected counts leave the fleet unchanged and return a plain text error:
//
//	-d '{"count":101}'                  400 count must be between 0 and 100
//	-H "Content-Type: text/plain"       415 Content-Type must be application/json
package simulatorapi
