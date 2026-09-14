// Package simulation is the real-time driver of the application's simulation
// engine. The engine itself, with fleet generation, movement, AIS encoding,
// history, and metadata, is the public package
// github.com/miroslav-matejovsky/ais-test-bench/simulation; callers importing
// both conventionally name this package simdriver.
//
// NewID creates the opaque identity for a new engine run. NewDriver wraps one
// engine. Driver.Run advances it every simulation.TickInterval of wall-clock
// time until context cancellation and may run once per driver. Driver.SetCount
// applies fleet changes at the current real time. Fleet, History, and Metadata
// delegate to the engine and return its detached copies. The driver keeps no
// simulation state of its own.
//
// The application, domain, and infrastructure subpackages hold earlier design
// contracts for scenarios and playback.
package simulation
