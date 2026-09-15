package simulator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/miroslav-matejovsky/ais-testbench/internal/httpserver"
	"github.com/miroslav-matejovsky/ais-testbench/internal/simdriver"
)

// Run starts a new simulation run and serves the standalone simulator (see
// NewStandaloneHandler) on ln until ctx is cancelled. A nil logger means
// slog.Default(). Run takes ownership of ln and closes it, also when
// construction fails.
func Run(ctx context.Context, logger *slog.Logger, ln net.Listener) error {
	sim, err := New(Config{Simulation: simdriver.NewConfig(), Logger: logger})
	if err != nil {
		return errors.Join(fmt.Errorf("create simulation: %w", err), ln.Close())
	}
	handler, err := NewStandaloneHandler(sim)
	if err != nil {
		return errors.Join(fmt.Errorf("create simulator handler: %w", err), ln.Close())
	}
	return Serve(ctx, ln, sim, handler)
}

// Serve runs the pacing loop of sim and serves handler on ln, logging lifecycle
// and HTTP server errors to sim's logger. It stops when ctx is cancelled, the
// pacing loop fails, or serving fails. HTTP shuts down first, within
// httpserver.ShutdownTimeout, then the pacing loop is stopped and joined. Serve
// takes ownership of ln and closes it. It returns nil after a clean shutdown.
func Serve(ctx context.Context, ln net.Listener, sim *Simulator, handler http.Handler) error {
	logger := sim.logger
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
