// Package simdriver is the real-time driver of the application's simulation
// engine. The engine itself, with fleet generation, movement, AIS encoding,
// history, metadata, and the virtual clock, is the public package
// github.com/miroslav-matejovsky/ais-test-bench/simulation.
//
// NewConfig returns the application's engine configuration: a fresh random
// identity and seed, the current real instant as virtual start, one vessel, and
// speed 1. NewDriver wraps one engine and takes the current real instant from
// its Clock as baseline.
//
// Driver.Run wakes every Heartbeat and settles: it delivers the real time
// elapsed since the baseline to the engine's Elapse. Elapsed time is measured
// when a wakeup is processed, so late or coalesced wakeups lose no virtual time.
// A backlog is delivered in chunks that fit simulation.MaxAdvance at
// simulation.MaxSpeed, so every report tick is processed. Cancellation stops
// between chunks, and the baseline keeps every delivered chunk. A backlog above
// MaxCatchUp virtual time, for example after a long host suspension, fails
// before any delivery and stops Run. Higher speeds reach that limit after less
// real time: 36 real seconds at 100x. A clock sample before the baseline changes
// nothing, so a regressed clock neither rewinds nor double-counts. Elapsed real
// time is dropped while paused. Run may be called once per driver.
//
// SetCount and SetSpeed validate input first; invalid input changes nothing.
// Then, under the lock shared with pacing, they sample the clock, settle elapsed
// time at the previous speed, and apply the change at the settled virtual
// instant, so heartbeats and commands cannot reorder reports. Unlike the atomic
// engine operations, a settlement failure leaves the change unapplied but keeps
// delivered chunks. Fleet, History, and Metadata return committed engine copies
// without settling, so metadata can trail pacing by one heartbeat plus
// processing time.
//
// Clock is the real-time seam and SystemClock the production clock. Real
// instants keep their monotonic reading until elapsed time is computed; UTC
// normalization applies only to virtual timestamps.
package simdriver
