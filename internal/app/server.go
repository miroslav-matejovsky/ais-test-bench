package app

import "context"

// HTTPServer is the lifecycle surface consumed by the runtime.
// The adapter owns a listener acquired during bootstrap.
type HTTPServer interface {
	// Run serves until stopped; expected server closure is normalized to nil.
	Run(ctx context.Context) error
	// Shutdown drains requests, then force-closes connections at the deadline.
	Shutdown(ctx context.Context) error
}
