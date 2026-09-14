package service

import (
	"context"
	"time"

	targetdomain "github.com/miroslav-matejovsky/ais-test-bench/internal/targets/domain"
)

// TargetReader is the viewer's read-only port into live simulation target state.
type TargetReader interface {
	Snapshot(ctx context.Context) (Snapshot, error)
}

// Snapshot is a consistent, detached target set at one simulation timestamp.
type Snapshot struct {
	At      time.Time
	Vessels []targetdomain.Vessel
}

// Viewer projects domain snapshots for display and target selection.
type Viewer interface {
	Targets(ctx context.Context) (TargetView, error)
	Target(ctx context.Context, id string) (Target, error)
}

// TargetView is a consistent projected snapshot suitable for polling.
type TargetView struct {
	At      time.Time
	Targets []Target
}

// Target is a viewer-specific projection in WGS84 geographic coordinates.
// Screen projection, selection state, and label placement belong to the frontend.
type Target struct {
	ID             string
	Label          string
	Latitude       float64
	Longitude      float64
	HeadingDegrees float64
	SpeedKnots     float64
}

// ChartCatalog abstracts chart metadata without choosing a browser map library.
type ChartCatalog interface {
	List(ctx context.Context) ([]Chart, error)
}

// Chart describes a local chart source. URL is a same-origin resource location;
// assets must be packaged or supplied locally for offline operation.
type Chart struct {
	ID          string
	Name        string
	URL         string
	Attribution string
}
