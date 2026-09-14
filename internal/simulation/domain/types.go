package domain

import "time"

// Scenario is a saved simulation definition. ID is stable and Name is nonempty.
// VesselIDs must resolve at startup; a run freezes their initial definitions.
type Scenario struct {
	ID        string
	Name      string
	VesselIDs []string
	StartTime time.Time
	Duration  time.Duration
	Seed      int64
}

// State is a simulation lifecycle state.
type State string

const (
	// StateIdle indicates that no scenario has been started.
	StateIdle State = "idle"
	// StateRunning indicates advancing virtual time.
	StateRunning State = "running"
	// StatePaused indicates a loaded scenario with frozen virtual time.
	StatePaused State = "paused"
	// StateStopped indicates a completed or explicitly stopped run.
	StateStopped State = "stopped"
)

// Status is an immutable snapshot of the current run.
type Status struct {
	ScenarioID string
	State      State
	// Elapsed is relative to Scenario.StartTime, in [0, Scenario.Duration].
	Elapsed time.Duration
	Speed   float64
}

// Event is an immutable lifecycle notification, ordered within a run.
type Event struct {
	RunID    string
	Sequence uint64
	Status   Status
}
