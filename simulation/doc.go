// Package simulation is the AIS traffic engine of the test bench, importable by
// other Go programs as
//
//	import "github.com/miroslav-matejovsky/ais-test-bench/simulation"
//
// It owns synthetic vessels, their latest AIS reports, a bounded in-memory AIS
// history, receiving stations with their receptions and observed targets, and a
// virtual clock. A seeded random source creates cargo vessels in the North Sea
// that move at their reported speed and course.
//
// # Configuration
//
// New takes an explicit Config: run identity, start instant, seed, initial
// vessel count, speed, reference transmitter, and receiving stations. The engine
// never reads a clock, sleeps, starts goroutines, or uses global randomness, and
// never fills in defaults: a zero TransmitterProfile or StationDefinition is
// invalid. The same implementation, Config, and ordered calls produce the same
// sentences, timestamps, MMSIs, and sequences.
//
// # Receiving stations
//
// A station is a synthetic shore receiving site: position, antenna height,
// receive gain, feeder loss, channel A and B capability, and up to
// MaxShadowSectors bearing sectors with extra loss. Config.Stations holds 0
// through MaxStations definitions; zero stations is valid. TransmitterProfile is
// the one run-level transmitter every vessel uses and is published in
// Metadata.Settings. Field docs state every range. Validation rejects non-finite
// numbers before range checks. Stored definitions are canonical: longitude 180
// becomes -180 and shadow sectors are sorted by start. Sectors must not overlap.
//
// New assigns IDs station-1, station-2, and so on in definition order.
// AddStation allocates the next number, never reusing one, and never consumes
// vessel randomness, so stations never change vessel reports. Every station has
// a config revision, increased by every effective edit, and an RF revision,
// increased by every edit except a name-only one. A no-op edit keeps both. The
// StationSet revision starts at 1, increases with every effective add, edit, or
// removal, and is the expected revision of the next edit. Station latitude is
// limited to [-85, 85], the web map range.
//
// # Reception model
//
// One deterministic model estimates, per station channel and transmitter
// position, the geodesic distance and bearing, a 4/3-earth radio horizon, the
// received power from transmitter power and gains, free-space loss at the
// channel frequency, a fixed site loss and path exponent, and shadow sector
// loss. The margin against sensitivity plus noise penalty maps to a decode
// probability through fixed knots, multiplied by a horizon taper and by one
// minus the channel drop probability. Disabled stations and channels have
// probability 0. Metadata.Settings.Reception publishes every parameter. The
// values are empirical test-bench choices, not calibrated predictions.
//
// Receive decisions hash the run seed, station ID, RF revision, and
// transmission sequence into a uniform draw, so they never consume vessel
// randomness and do not depend on batching or station order.
//
// Station.Coverage holds 0.9 and 0.5 probability contours per channel for
// Settings.Transmitter, computed with the same function whenever the RF
// revision changes. Rings sample every 5 degrees plus both sides of each shadow
// sector boundary, and bisect each radius over the horizon.
//
// Station errors separate their causes: ErrInvalid for a rejected definition,
// ErrConflict for a stale expected revision, ErrNotFound for an unknown ID, and
// ErrLimit for MaxStations or an exhausted ID or revision range. Validation and
// all checks complete before any change.
//
// # Receptions and observations
//
// Three identities stay separate. A transmission is one generated report,
// identified by Message.Sequence, whether any station receives it or not. A
// Reception is one station's successful decoding of a transmission, identified
// by station ID and a per-station sequence that starts at 1 and stays
// contiguous across RF edits. A target is one MMSI with the latest reception of
// every station that received it. A Reception carries the exact sentence, the
// full virtual timestamp (receive latency is not modelled), model diagnostics,
// the station RF configuration, and scenario attribution, so it stays
// meaningful after its transmission leaves History.
//
// Every generated report, including creation reports, is one opportunity at
// every configured station, including disabled ones. ReceptionCounters count
// opportunities since station creation by exclusive outcome and by channel;
// RecentCounters count the last RateWindow in one-second buckets. Counters
// survive RF edits, and Station.RFUpdatedAt tells whether a window spans RF
// revisions.
//
// Observations is one consistent snapshot: clock, settings, stations with
// counters, selected targets, recent receptions, and totals. A target's
// navigation is the newest report a selected station actually received, so a
// missed report never moves it, and it stays after its vessel leaves the fleet.
// Several selected stations produce one target with the highest transmission
// sequence, from the first station in creation order on ties, and mark which
// stations received that transmission. Ages are virtual: fresh up to FreshAge,
// stale up to StaleAge, then lost. Clock mutations remove observations at
// ExpiryAge, also with an empty fleet; ages freeze while paused. The store
// holds at most TargetLimit MMSIs and evicts the target whose newest reception
// is oldest, lowest MMSI first, counting Observations.TargetEvictions.
// RemoveStation removes the station's counters, history, and observations.
//
// ReceptionHistory pages through the newest ReceptionHistoryLimit receptions
// of one station. Unlike the complete batches SetCount, Advance, and Elapse
// return, it is finite: a cursor that falls behind it reports a Gap. Expiry and
// eviction never remove history, and Observations rebuilds the live view after
// any gap.
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
// Each vessel alternates its reports between AIS channels A and B, starting on
// A for an even MMSI and on B for an odd one.
//
// # Atomicity and concurrency
//
// Each mutation stages all state, including the random source, navigation,
// sequence, clock, scaling remainder, reception counters and sequences, and new
// receptions. It commits only after every report encoded, every station
// evaluated, every limit and context check passed; applying the staged
// receptions cannot fail. A failed call returns no reports and changes no
// future result, including reception decisions, observation ages, and
// sequences. Observations.StateRevision increases with every effective commit.
// Errors wrap ErrInvalid for rejected input and ErrLimit for exceeded limits.
// One mutex makes all methods safe for concurrent use, but reproducible output
// requires callers to order their mutations. Fleet, History, Metadata,
// Stations, Observations, ReceptionHistory, and returned values are detached
// copies.
package simulation
