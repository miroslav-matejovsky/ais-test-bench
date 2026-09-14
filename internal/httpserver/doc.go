// Package httpserver runs HTTP servers with the settings shared by every
// component: a read header timeout, error logging through slog, and a bounded
// graceful shutdown.
//
// Serve starts serving a caller-bound listener in a goroutine and owns that
// listener. Done reports when serving ends, so a runtime can react to an
// unexpected failure. Shutdown drains in-flight requests until its context
// ends, force-closes what remains, waits for the serving goroutine, and returns
// every actionable error. Runtimes create one context with ShutdownTimeout and
// shut their servers down in dependency order.
package httpserver
