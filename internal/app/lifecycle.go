package app

import "context"

// Worker is a long-lived task supervised by the runtime's errgroup.
// Run blocks until cancellation or failure and joins any child goroutines.
type Worker interface {
	Run(ctx context.Context) error
}

// Runtime owns all workers, listeners, and repositories in one process.
// Run supervises workers with errgroup, initiates Shutdown on cancellation or
// failure, and joins workers before returning. Normal cancellation returns nil
// only when cleanup succeeds; failures preserve both worker and cleanup errors.
type Runtime interface {
	Worker
	// Shutdown stops admission, cancels producers, drains bounded queues, and
	// closes resources. It is safe to call once concurrently with Run, even if
	// Run has already returned. Its context must have a finite deadline.
	Shutdown(ctx context.Context) error
}
