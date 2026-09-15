// Command ais-testbench runs the combined test bench in one process: one
// simulation engine, its API, the manager, the display, and the status page.
//
// Usage:
//
//	ais-testbench [-addr localhost:8000] [-base-path /tools/ais]
//
// It validates both flags, listens on the given host:port, and serves
// testbench.Serve with simulator.DemoConfig until SIGINT or SIGTERM. The display
// reads the engine in process. -base-path prefixes every route and is empty for
// the root. It exits non-zero on startup, pacing, or shutdown failure.
package main
