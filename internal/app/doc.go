// Package app is the composition root of the combined executable.
//
// Run creates one simulation engine and one simulator API handler. It mounts
// that handler on the public listener and on a private 127.0.0.1 listener with
// an OS-assigned port, which serves API routes only. The display gets a
// simulator HTTP client for the private listener, so combined mode uses the
// same HTTP data path as separate processes and never hands engine state to
// the display. The public listener also serves the home, manager, display, and
// status pages and static assets, with navigation to both pages.
//
// Any server or tick-loop exit stops everything. Shutdown drains public
// requests first while the private API still answers their simulator reads,
// then closes idle client connections, stops the private server, and joins the
// tick loop, all within one httpserver.ShutdownTimeout budget.
package app
