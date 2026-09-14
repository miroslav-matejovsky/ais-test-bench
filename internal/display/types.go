package display

import (
	"time"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/simulatorapi"
)

// Fleet is the GET /display/api/vessels response: one complete browser
// projection of the simulator's active fleet.
type Fleet struct {
	// SimulationID identifies the simulator run; a change means a restart.
	SimulationID string `json:"simulationId"`
	// UpdatedAt is the simulator fleet update time.
	UpdatedAt time.Time `json:"updatedAt"`
	// SpawnBounds is the simulator area where vessels start, for map framing.
	SpawnBounds simulatorapi.SpawnBounds `json:"spawnBounds"`
	// Vessels is the complete active set in simulator order; [] when empty.
	Vessels []Vessel `json:"vessels"`
}

// Vessel is one active vessel. Navigation values are decoded from its latest
// NMEA report and are null when the report marks them unavailable. Coordinates
// are decimal degrees, speed is knots, course and heading are degrees.
type Vessel struct {
	MMSI      uint32   `json:"mmsi"`
	Name      string   `json:"name"`
	TypeID    string   `json:"typeId"`
	TypeName  string   `json:"typeName"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
	Speed     *float64 `json:"speed"`
	Course    *float64 `json:"course"`
	Heading   *int     `json:"heading"`
	// UpdatedAt is the full report generation time from the report envelope.
	UpdatedAt time.Time `json:"updatedAt"`
}
