// Package simulator owns a simulation engine, serialized real-time commands,
// its HTTP API, local received-traffic source, and standalone manager runtime.
//
// New validates Config and creates one engine and its sole mutator, an internal
// driver. It starts no goroutines, listeners, or signal handlers. The caller
// supervises Simulator.Run
// alongside its HTTP server, drains requests, then cancels and joins Run. Run
// paces measured real time from construction; it may be called once, returns nil
// on cancellation, and reports catch-up or delivery failures. A backlog exceeding
// one virtual hour fails before delivery. Commands serialize with pacing and
// settle elapsed time before applying a valid change. A failed settlement keeps
// already delivered chunks, unlike the engine's atomic single-call operations.
//
// Simulator methods are concurrency-safe. Commands before Run are permitted;
// commands after Run returns fail with simulatorapi.ErrUnavailable. Snapshots
// remain readable after termination and never advance time. Simulation errors
// retain their original identity. No context is stored or engine pointer exposed.
//
// Simulator.Observations and ReceptionHistory implement display.Source through
// public simulatorapi types without importing display. Local reads copy committed
// received traffic and preserve NMEA, revisions, and independent station cursors.
// Errors wrap simulatorapi source categories and original causes. Cancellation is
// checked before and after snapshot conversion. Results belong to the caller and
// can be modified independently of the engine. Display validates local and remote
// source values through the same semantic and AIS projection.
//
// # Logging
//
// Config.Logger takes precedence over Config.Simulation.Logger and replaces it
// for the owned engine. Nil falls back to Simulation.Logger, then slog.Default(),
// resolved once in New; construction never changes the process default. The
// simulator derives one logger with component=simulator, preserving the caller's
// attributes, groups, and handler levels, and uses it for its API, standalone
// pages, and Serve. Independent simulators never share records.
//
// Normal operation is quiet: reads, ticks, and successful commands emit nothing.
// API handlers log failures they consume and turn into 5xx responses, and failed
// response writes, at Error with path, input, and error, through the request
// context. Serve logs start and stop at Info and HTTP server errors at Error.
// Run returns failures to its supervising caller instead of logging them. Records
// are emitted after driver and engine locks are released, and logger choice never
// affects simulation state or emitted bytes.
//
// # HTTP API
//
// Simulator.API builds the /api/* routes defined by package simulatorapi over one
// simulation driver. It maps public engine values to wire types with explicit
// conversions, adding the driver's real pacing interval to metadata, and passes
// NMEA sentences through exactly as generated, including CRLF. Reads return
// committed engine copies and never advance virtual time. Count and speed
// requests are validated with one strict JSON reader before the driver is
// called; the driver then settles elapsed real time at the previous speed and
// applies the change. A valid change the driver cannot apply, such as a backlog
// beyond the catch-up limit, is logged and returns 500; a stopped runtime returns
// 503. The handler holds no
// state besides the simulator, so combined composition can mount one API handler
// on several listeners.
//
// Station routes add, replace, and remove receiving sites with expected run and
// station-set revisions. They use a separate 64 KiB JSON reader, require every
// definition field, and return field-addressed validation errors. Station and
// observation responses include atomically captured clock/settings; conversions
// encode uint64 receiving identities as decimal strings and coverage as GeoJSON
// MultiPolygon cut at the antimeridian. Reception pages have independent station
// cursors and explicit eviction gaps. These routes expose received NMEA without
// decoding navigation or changing the display component. All routes are available
// through combined composition's public and private listeners.
//
// NewHandler composes the standalone simulator: the API, the manager page,
// status, static assets, and a root redirect to /manager. Its navigation lists
// only the manager.
//
// Package-level Run creates the demonstration configuration on the system clock
// with the given logger and serves NewHandler. Serve runs Simulator.Run next to an HTTP server on a
// caller-supplied listener and owns that listener. On
// cancellation or a serving or pacing failure, HTTP drains within
// a five-second shutdown timeout, then the pacing loop is stopped and joined.
// Failures are returned with their context. HTTP deadlines and shutdown use real
// time at every simulation speed, including pause.
package simulator
