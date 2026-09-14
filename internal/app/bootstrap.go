package app

import (
	"context"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/app/config"
)

// ConfigLoader reads, defaults, and validates configuration before resource acquisition.
type ConfigLoader interface {
	Load(ctx context.Context, path string) (config.Config, error)
}

// Bootstrapper builds one process after configuration and logging are ready.
// Build acquires resources in dependency order and rolls back on any failure.
// A successful result transfers ownership of all acquired resources to Runtime.
type Bootstrapper interface {
	Build(ctx context.Context, cfg config.Config) (Runtime, error)
}
