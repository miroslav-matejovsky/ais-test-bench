// Package app is the composition root of the executable.
//
// Run creates one shared simulator, supplies it to the UI and JSON API, and
// runs its tick loop alongside HTTP on a caller-supplied listener. Cancellation
// stops the simulator and gracefully shuts down the HTTP server.
package app
