// Command ais-test-bench is the single process entry point.
//
// Usage:
//
//	ais-test-bench [-port 8080]
//
// It listens on localhost at the given port, runs internal/app until SIGINT or
// SIGTERM, and exits non-zero on startup or shutdown failure.
package main
