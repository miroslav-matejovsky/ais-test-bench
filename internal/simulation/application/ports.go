package application

import (
	"context"
	"time"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/simulation/domain"
	targetdomain "github.com/miroslav-matejovsky/ais-test-bench/internal/targets/domain"
)

// Simulator serializes commands with tick processing and returns copied snapshots.
// Invalid transitions return errors; commands acknowledge an applied change.
type Simulator interface {
	Start(ctx context.Context, scenarioID string) error
	Pause(ctx context.Context) error
	Resume(ctx context.Context) error
	Stop(ctx context.Context) error
	// Seek rebuilds deterministic state while paused, without publishing history.
	Seek(ctx context.Context, elapsed time.Duration) error
	// SetSpeed accepts only finite positive multipliers.
	SetSpeed(ctx context.Context, multiplier float64) error
	Status(ctx context.Context) (domain.Status, error)
}

// ScenarioRepository stores complete scenario definitions, detached from caller memory.
// Save upserts atomically; missing Get/Delete returns a contextual error.
// List returns a stable ID order. Concurrent access must be safe.
type ScenarioRepository interface {
	Get(ctx context.Context, id string) (domain.Scenario, error)
	List(ctx context.Context) ([]domain.Scenario, error)
	Save(ctx context.Context, scenario domain.Scenario) error
	Delete(ctx context.Context, id string) error
}

// Clock supplies interruptible wall-time pacing. Virtual time belongs to Simulator.
// Tests supply a manually advanced clock.
type Clock interface {
	Wait(ctx context.Context, duration time.Duration) error
}

// TargetStepper advances navigation at explicit virtual times.
// Reset freezes initial vessel definitions and seed for replay.
type TargetStepper interface {
	Reset(ctx context.Context, vesselIDs []string, seed int64) error
	Step(ctx context.Context, elapsed time.Duration) ([]targetdomain.Vessel, error)
}

// TrafficGenerator receives a snapshot once per simulation tick.
// Its adapter maps target domain values into AIS reports and publishes due messages.
type TrafficGenerator interface {
	Generate(ctx context.Context, at time.Time, vessels []targetdomain.Vessel) error
}

// EventBus delivers immutable lifecycle events to bounded, in-process subscriptions.
// Publish preserves per-run order and returns errors instead of silently dropping.
// Simulator owns publication; app wires subscribers before starting workers.
type EventBus interface {
	Publish(ctx context.Context, event domain.Event) error
	// Subscribe releases its resources and closes the channel when ctx ends.
	Subscribe(ctx context.Context) (<-chan domain.Event, error)
}
