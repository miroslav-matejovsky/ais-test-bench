// Package simulation owns live synthetic vessels, their latest AIS reports, and
// a bounded in-memory AIS history. A seeded random source creates cargo vessels
// in the North Sea. One-second ticks advance their positions at the reported
// speed and course.
//
// Each engine start has an opaque simulation identity and a start time. Every
// generated NMEA sentence gets the next report sequence, becomes its vessel's
// latest report, and is appended to history. Latest reports describe exactly
// the active fleet, independent of history retention. Removing a vessel keeps
// its retained history until eviction.
//
// Mutations encode all reports before publishing, so a failed update leaves the
// observable state unchanged. All state access is synchronized; Fleet, History,
// Metadata, and Snapshot return copies built under the lock. Fleet, History,
// and Metadata use simulatorapi wire types. Snapshot is the decoded fleet view
// used by the current display page. Restarting discards all state.
package simulation
