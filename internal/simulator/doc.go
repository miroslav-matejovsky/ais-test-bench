// Package simulator is the simulator component: the HTTP API over one
// simulation driver, the manager page, and the runtime that serves them.
//
// NewAPI builds the /api/* routes defined by package simulatorapi over one
// simulation driver. It maps public engine values to wire types with explicit
// conversions, adding the driver's real pacing interval to metadata, and passes
// NMEA sentences through exactly as generated, including CRLF. Reads return
// committed engine copies and never advance virtual time. Count and speed
// requests are validated with one strict JSON reader before the driver is
// called; the driver then settles elapsed real time at the previous speed and
// applies the change. A valid change the driver cannot apply, such as a backlog
// beyond the catch-up limit, is logged and returns 500. The handler holds no
// state besides the driver, so combined composition can mount one API handler
// on several listeners.
//
// NewHandler composes the standalone simulator: the API, the manager page,
// status, static assets, and a root redirect to /manager. Its navigation lists
// only the manager.
//
// Run creates an engine from simdriver.NewConfig and a driver on the system
// clock, and serves NewHandler. Serve runs the driver's pacing loop next to an
// httpserver.Server on a caller-supplied listener and owns that listener. On
// cancellation or a serving or pacing failure, HTTP drains within
// httpserver.ShutdownTimeout, then the pacing loop is stopped and joined.
// Failures are returned with their context. HTTP deadlines and shutdown use real
// time at every simulation speed, including pause.
package simulator
