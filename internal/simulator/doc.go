// Package simulator is the simulator component: the HTTP API over one
// simulation engine, the manager page, and the runtime that serves them.
//
// NewAPI builds the /api/* routes defined by package simulatorapi over one
// simulation driver. It maps public engine values to wire types with explicit
// conversions, validates count requests before mutating the engine, and passes
// NMEA sentences through exactly as generated, including CRLF. It holds no
// state besides the driver, so combined composition can mount one API handler
// on several listeners.
//
// NewHandler composes the standalone simulator: the API, the manager page,
// status, static assets, and a root redirect to /manager. Its navigation lists
// only the manager.
//
// Run creates a new engine and driver and serves NewHandler. Serve runs the
// driver's tick loop next to an httpserver.Server on a caller-supplied listener
// and owns that listener. On cancellation or a serving or tick-loop failure,
// HTTP drains within httpserver.ShutdownTimeout, then the tick loop is stopped
// and joined. Failures are returned with their context.
package simulator
