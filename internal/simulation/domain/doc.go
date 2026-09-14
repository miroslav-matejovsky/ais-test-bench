// Package domain describes scenarios, playback state, and lifecycle events.
// Scenario owns simulation timing and vessel references, not vessel entities.
// State transitions are idle -> running -> paused -> running or stopped;
// a new start selects a scenario from idle or stopped. Dependencies: time only.
package domain
