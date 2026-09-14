// Package simulation is the AIS traffic engine of the test bench, importable by
// other Go programs as
//
//	import "github.com/miroslav-matejovsky/ais-test-bench/simulation"
//
// It owns synthetic vessels, their latest AIS reports, and a bounded in-memory
// AIS history. A seeded random source creates cargo vessels in the North Sea.
// Advance moves them at their reported speed and course and emits a report per
// vessel.
//
// New takes a run identity, start time, and seed, and creates
// InitialVesselCount vessels. SetCount manages 0 to MaxVessels vessels; existing
// vessels keep their identity, type, speed, and course. The engine never reads a
// clock, sleeps, or starts goroutines. Callers supply every timestamp, so a run
// with the same identity, start time, seed, and ordered calls produces the same
// sentences.
//
// Every generated NMEA sentence gets the next run-local sequence, starting at 1,
// becomes its vessel's latest report, and is appended to history. Latest reports
// describe exactly the active fleet, independent of history retention. Removing
// a vessel keeps its retained history until eviction. Sentences are checksummed
// type 1 !AIVDM lines including CRLF; callers decode them with any AIS decoder.
//
// Mutations encode all reports before publishing, so a failed update leaves the
// observable state unchanged. A single mutex protects all state. Fleet, History,
// and Metadata return detached copies using the value types of this package;
// changing them never changes engine state. Metadata settings come from the
// constants that drive generation.
package simulation
