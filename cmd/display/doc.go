// Command display runs the standalone display component: the live map page and
// its backend, which reads a simulator over HTTP.
//
// Usage:
//
//	display [-addr localhost:8081] [-simulator-url http://localhost:8000]
//
// Both flags are validated before listening. It serves display until
// SIGINT or SIGTERM and exits non-zero on startup or shutdown failure. An
// unreachable simulator does not stop the display; the page reports it and
// retries.
package main
