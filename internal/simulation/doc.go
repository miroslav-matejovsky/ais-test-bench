// Package simulation is the real-time driver of the application's simulation
// engine. The engine itself, with fleet generation, movement, AIS encoding,
// history, metadata, and the virtual clock, is the public package
// github.com/miroslav-matejovsky/ais-test-bench/simulation; callers importing
// both conventionally name this package simdriver.
//
// NewConfig returns the application's engine configuration: a fresh random
// identity and seed, the current real instant as virtual start, one vessel, and
// speed 1. NewDriver wraps one engine. Driver.Run wakes every
// simulation.TickInterval, measures the real time elapsed since its previous
// wake, and passes it to Elapse in chunks that fit simulation.MaxAdvance at
// simulation.MaxSpeed, so late wakes lose no virtual time. It may run once per
// driver. Driver.SetCount applies fleet changes at the engine's committed
// virtual time, which trails real time by at most one tick. Fleet, History, and
// Metadata delegate to the engine and return its detached copies. The driver
// keeps no simulation state of its own.
//
// The application, domain, and infrastructure subpackages hold earlier design
// contracts for scenarios and playback.
package simulation
