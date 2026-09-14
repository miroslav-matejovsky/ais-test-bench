// Command ais-testbench is the single process entry point.
//
// Usage:
//
//	ais-testbench [-addr localhost:8000]
//
// It listens on the given host:port, runs internal/app until SIGINT or SIGTERM,
// and exits non-zero on startup or shutdown failure. The host is required.
package main
