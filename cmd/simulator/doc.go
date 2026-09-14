// Command simulator runs the standalone simulator component: one simulation
// engine, its HTTP API, and the manager page.
//
// Usage:
//
//	simulator [-addr localhost:8000]
//
// It listens on the given host:port, runs internal/simulator until SIGINT or
// SIGTERM, and exits non-zero on startup or shutdown failure. The host is
// required.
package main
