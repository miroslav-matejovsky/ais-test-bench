// Package simulation is the AIS traffic engine of the test bench, importable by
// other Go programs as
//
//	import "github.com/miroslav-matejovsky/ais-test-bench/simulation"
//
// It owns synthetic vessels, their latest AIS reports, a bounded in-memory AIS
// history, and a virtual clock. A seeded random source creates cargo vessels in
// the North Sea that move at their reported speed and course.
//
// # Configuration
//
// New takes an explicit Config: run identity, start instant, seed, initial
// vessel count, and speed. The engine never reads a clock, sleeps, starts
// goroutines, or uses global randomness. The same implementation, Config, and
// ordered calls produce the same sentences, timestamps, MMSIs, and sequences.
//
// # Virtual time
//
// All timestamps are virtual UTC instants counted from Config.StartTime. A report
// tick falls on every TickInterval after the start. Advance adds a virtual
// duration; Elapse adds a real duration multiplied by the speed. Both process
// every due tick, so one 10-second call equals ten 1-second calls or any split
// reaching the same instants. Speed changes only how fast virtual time passes
// for Elapse, never the knots vessels report. Speed 0 pauses Elapse without later
// catch-up, while Advance still steps. One call covers at most MaxAdvance, so a
// call returns at most MaxAdvance/TickInterval reports per vessel. Virtual dates
// stay within years 1 through 9999.
//
// Commands apply at the current virtual instant in call order. A count change at
// a tick boundary happens after that tick. New vessels report immediately and
// then move only from their birth time. Sequence is the total order; equal
// timestamps are valid.
//
// # Reports and history
//
// Every generated sentence gets the next run-local sequence, starting at 1,
// becomes its vessel's latest report, and is appended to history. SetCount,
// Advance, and Elapse return every report they emit in sequence order, including
// reports already evicted from the MessageLimit history. Latest reports describe
// exactly the active fleet. Sentences are checksummed type 1 !AIVDM lines
// including CRLF; their UTC second field is the second of the report timestamp.
//
// # Atomicity and concurrency
//
// Each mutation stages all state, including the random source, navigation,
// sequence, clock, and scaling remainder, and commits only after every report
// encoded and every context check passed. A failed call returns no reports and
// changes no future result. Errors wrap ErrInvalid for rejected input and
// ErrLimit for exceeded limits. One mutex makes all methods safe for concurrent
// use, but reproducible output requires callers to order their mutations.
// Fleet, History, Metadata, and returned reports are detached copies.
package simulation
