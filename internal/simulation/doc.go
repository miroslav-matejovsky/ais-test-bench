// Package simulation owns live synthetic vessels, their latest AIS reports, and
// a bounded in-memory AIS history. A seeded random source creates cargo vessels
// in the North Sea. One-second ticks advance their positions at the reported
// speed and course.
//
// New takes a run identity (NewID in production), start time, and seed, and
// creates one vessel. SetCount manages 0-100 vessels; existing vessels keep
// their identity, type, speed, and course. Run advances positions until context
// cancellation; Advance accepts explicit time for deterministic tests.
//
// Each engine start has an opaque simulation identity and a start time. Every
// generated NMEA sentence gets the next run-local sequence, starting at 1,
// becomes its vessel's latest report, and is appended to history. Latest
// reports describe exactly the active fleet, independent of history retention.
// Removing a vessel keeps its retained history until eviction.
//
// Mutations encode all reports before publishing, so a failed update leaves the
// observable state unchanged. A single mutex protects all state; Fleet, History,
// Metadata, and Snapshot return copies built under the lock. Fleet, History,
// and Metadata use simulatorapi wire types, and metadata settings come from the
// constants that drive generation. Snapshot is the decoded fleet view used by
// the current display page. Restarting discards all state.
//
// The application, domain, and infrastructure subpackages hold earlier design
// contracts for scenarios and playback.
package simulation
