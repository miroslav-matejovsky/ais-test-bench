package service

import (
	"context"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/app/config"
	simulationdomain "github.com/miroslav-matejovsky/ais-test-bench/internal/simulation/domain"
	targetdomain "github.com/miroslav-matejovsky/ais-test-bench/internal/targets/domain"
)

// ScenarioManager exposes validated scenario CRUD to HTTP consumers.
// Mutations of a scenario used by an active run return a conflict.
type ScenarioManager interface {
	Get(ctx context.Context, id string) (simulationdomain.Scenario, error)
	List(ctx context.Context) ([]simulationdomain.Scenario, error)
	Save(ctx context.Context, scenario simulationdomain.Scenario) error
	Delete(ctx context.Context, id string) error
}

// TargetManager exposes validated vessel CRUD to HTTP consumers.
// It enforces MMSI uniqueness, navigation ranges, and valid tracks.
// Mutations of vessels frozen into an active run return a conflict.
type TargetManager interface {
	Get(ctx context.Context, id string) (targetdomain.Vessel, error)
	List(ctx context.Context) ([]targetdomain.Vessel, error)
	Save(ctx context.Context, vessel targetdomain.Vessel) error
	Delete(ctx context.Context, id string) error
}

// SystemReader exposes effective startup configuration and operational status.
// Returned values must be detached copies, including configuration slices.
type SystemReader interface {
	Configuration(ctx context.Context) (config.Config, error)
	Status(ctx context.Context) (SystemStatus, error)
}

// SystemStatus is the management view of the single process.
type SystemStatus struct {
	Ready      bool
	TCPClients int
	UDPEnabled bool
}
