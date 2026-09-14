package simulator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"time"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/httpserver"
	"github.com/miroslav-matejovsky/ais-test-bench/internal/simulation"
)

// Run starts a new simulation run and serves the standalone simulator (see
// NewHandler) on ln until ctx is cancelled. Run takes ownership of ln and
// closes it, also when construction fails.
func Run(ctx context.Context, logger *slog.Logger, ln net.Listener) error {
	sim, err := simulation.New(simulation.NewID(), time.Now(), rand.Uint64())
	if err != nil {
		return errors.Join(fmt.Errorf("create simulation: %w", err), ln.Close())
	}
	handler, err := NewHandler(logger, sim)
	if err != nil {
		return errors.Join(fmt.Errorf("create simulator handler: %w", err), ln.Close())
	}
	return Serve(ctx, logger, ln, sim, handler)
}

// Serve runs the tick loop of sim and serves handler on ln. It stops when ctx
// is cancelled, the tick loop fails, or serving fails. HTTP shuts down first,
// within httpserver.ShutdownTimeout, then the tick loop is stopped and joined.
// Serve takes ownership of ln and closes it. It returns nil after a clean
// shutdown.
func Serve(ctx context.Context, logger *slog.Logger, ln net.Listener, sim *simulation.Simulator, handler http.Handler) error {
	server := httpserver.Serve(logger, ln, handler)
	logger.Info("simulator started", "url", "http://"+ln.Addr().String())
	simulationCtx, stopSimulation := context.WithCancel(ctx)
	simulationDone := make(chan struct{})
	var simulationErr error // Read only after simulationDone closes.
	go func() {
		simulationErr = sim.Run(simulationCtx)
		close(simulationDone)
	}()

	select {
	case <-server.Done():
	case <-simulationDone:
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), httpserver.ShutdownTimeout)
	defer cancel()
	err := server.Shutdown(shutdownCtx)
	stopSimulation()
	<-simulationDone
	if simulationErr != nil {
		err = errors.Join(err, fmt.Errorf("run simulation: %w", simulationErr))
	}
	if err != nil {
		return err
	}
	logger.Info("simulator stopped")
	return nil
}
