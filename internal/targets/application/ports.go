package application

import (
	"context"
	"time"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/targets/domain"
)

// VesselRepository stores vessel definitions, detached from active run state.
// Save upserts atomically. Missing Get/Delete returns a contextual error.
// List has stable ID ordering; implementations must support concurrent access.
type VesselRepository interface {
	Get(ctx context.Context, id string) (domain.Vessel, error)
	List(ctx context.Context) ([]domain.Vessel, error)
	Save(ctx context.Context, vessel domain.Vessel) error
	Delete(ctx context.Context, id string) error
}

// MovementModel derives state from a frozen initial vessel, virtual elapsed time,
// and a seed. Equal inputs must produce equal outputs, regardless of tick order.
type MovementModel interface {
	Position(ctx context.Context, initial domain.Vessel, elapsed time.Duration, seed int64) (domain.NavigationState, error)
}
