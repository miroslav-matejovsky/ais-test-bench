// Command display runs the standalone display component: the live map page and
// its backend, which reads a simulator over HTTP.
//
// Usage:
//
//	display [-addr localhost:8081] [-simulator-url http://localhost:8000]
//
// -simulator-url is the standalone simulator's base URL: an http(s) origin with
// an optional path prefix. The display reads {simulator-url}/api/ and links
// {simulator-url}/manager. Both flags are validated before listening. It serves
// the display until
// SIGINT or SIGTERM and exits non-zero on startup or shutdown failure. An
// unreachable simulator does not stop the display; the page reports it and
// retries.
package main
