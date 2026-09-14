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
package display
