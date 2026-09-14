// Package simulation owns live synthetic vessels and a bounded in-memory AIS
// history. A seeded random source creates vessels in the North Sea. One-second
// ticks advance their positions at the reported speed and course. All state
// access is synchronized; snapshots are copies. Restarting discards all state.
package simulation
