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
package simulatorapi
