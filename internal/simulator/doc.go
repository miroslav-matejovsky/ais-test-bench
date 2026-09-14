// Package simulator is the simulator component: the HTTP API over one
// simulation engine, the manager page, and the runtime that serves them.
//
// NewAPI builds the /api/* routes defined by package simulatorapi. It maps
// engine copies to wire types, validates count requests before mutating the
// engine, and passes NMEA sentences through exactly as generated, including
// CRLF. It holds no state besides the engine, so combined composition can mount
// one API handler on several listeners.
//
// NewHandler composes the standalone simulator: the API, the manager page,
// status, static assets, and a root redirect to /manager. Its navigation lists
// only the manager.
//
// Run creates a new simulation run and serves NewHandler. Serve runs an
// engine's tick loop next to an httpserver.Server on a caller-supplied listener
// and owns that listener. On cancellation or a serving or tick-loop failure,
// HTTP drains within httpserver.ShutdownTimeout, then the tick loop is stopped
// and joined. Failures are returned with their context.
package simulator
