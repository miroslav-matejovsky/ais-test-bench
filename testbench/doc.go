// Package testbench composes a complete AIS test bench for Go applications: one
// simulator, its manager, the display, a status page, and embedded assets below
// one public base path.
//
// New validates Config and builds the bench only from the public simulator,
// display, and ui packages; it binds no listener and starts no goroutine. The
// display reads the simulator in process, so the bench opens no private listener
// and makes no HTTP request to itself. It still consumes only received
// observations and NMEA, never the truth fleet.
//
// # Embedding
//
// A host mounts Handler at BasePath on its own mux, without http.StripPrefix,
// and may wrap it in authentication, authorization, CSRF, and request logging
// middleware:
//
//	bench, err := testbench.New(testbench.Config{
//	    Simulation: simulator.DemoConfig(),
//	    Logger:     logger,
//	    BasePath:   "/tools/ais",
//	})
//	if err != nil {
//	    return err
//	}
//	mux.Handle("/tools/ais/", authenticate(bench.Handler()))
//
// The host owns its server and supervises Bench.Run next to it. On shutdown it
// drains requests first, while pacing still runs so draining writes succeed,
// then cancels and joins Run. Run is single-use and returns pacing failures, such
// as an exceeded catch-up limit, which should stop the host's server. After Run
// returns, writes answer 503 while pages and reads keep working. The bench never
// closes the host's server, listener, or clients. UI returns the bench's ui.UI
// for rendering components into host templates with matching API and asset URLs.
//
// For layouts the facade does not cover, such as separately mounted APIs, a
// prefix-stripping proxy, or a display reading a remote simulator, compose
// simulator, display, and ui directly; the examples show both levels.
//
// # Standalone serving
//
// Serve owns a caller-bound listener: it creates the bench, serves Handler, and
// runs pacing until the context is cancelled, serving fails, or pacing fails.
// Construction failures close the listener before returning. Requests drain
// within a five-second budget of real time before pacing is cancelled and
// joined. The ais-testbench command is flags, a logger, signals, a listener, and
// one Serve call.
//
// # Logging
//
// Config.Logger applies to every child, including the engine, and replaces
// Simulation.Logger. Nil falls back to Simulation.Logger, then slog.Default(),
// resolved once in New without changing the process default. Children add
// component=simulator, display, or ui; Serve logs start and stop with
// component=testbench. Normal operation is quiet.
package testbench
