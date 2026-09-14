package domain

import "time"

// Vessel is a simulated target. ID is stable; MMSI is a nine-digit decimal identity.
// MMSI must be unique within a scenario. MovementModel selects a registered model.
type Vessel struct {
	ID            string
	MMSI          string
	Name          string
	CallSign      string
	Navigation    NavigationState
	Track         Track
	MovementModel string
}

// NavigationState uses finite values; latitude [-90,90], longitude [-180,180],
// nonnegative speed in knots, and course/heading in degrees [0,360).
// Unavailable AIS values are handled explicitly at the AIS mapping boundary.
type NavigationState struct {
	Latitude       float64
	Longitude      float64
	SpeedKnots     float64
	CourseDegrees  float64
	HeadingDegrees float64
}

// Track is an ordered route relative to scenario start.
// Waypoint offsets are nonnegative and strictly increasing.
type Track struct {
	Waypoints []Waypoint
}

// Waypoint is a navigation state at a virtual-time offset.
type Waypoint struct {
	Offset     time.Duration
	Navigation NavigationState
}
