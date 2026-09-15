// Command simulator runs the standalone simulator component: one simulation
// engine, its HTTP API, and the manager page.
//
// Usage:
//
//	simulator [-addr localhost:8000] [-base-path /sim]
//
// It validates both flags, listens on the given host:port, and serves
// simulator.Serve with simulator.DemoConfig until SIGINT or SIGTERM. -base-path
// prefixes every route and is empty for the root. It exits non-zero on startup,
// pacing, or shutdown failure.
package main
