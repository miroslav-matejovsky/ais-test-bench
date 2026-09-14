// Package app is the composition root of the combined executable.
//
// Run creates one simulation engine, its real-time driver, and one simulator API
// handler. It mounts that handler on the public listener and on a private
// 127.0.0.1 listener with an OS-assigned port, which serves API routes only, so
// count and speed changes through either listener control the same engine. The
// display gets a simulator HTTP client for the private listener, so combined
// mode uses the same HTTP data path as separate processes and never hands engine
// state to the display. The public listener also serves the home, manager,
// display, and status pages and static assets, with navigation to both pages.
//
// Any server or pacing-loop exit stops everything, including a driver failure
// such as an exceeded catch-up limit. Shutdown drains public requests first
// while the private API still answers their simulator reads, then closes idle
// client connections, stops the private server, and joins the pacing loop, all
// within one httpserver.ShutdownTimeout budget of real time.
package app
