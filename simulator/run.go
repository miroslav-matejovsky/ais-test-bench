package simulator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"

	"github.com/miroslav-matejovsky/ais-testbench/internal/httpserver"
	"github.com/miroslav-matejovsky/ais-testbench/simulation"
)

// StandaloneConfig configures Serve.
type StandaloneConfig struct {
	// Simulation is the owned engine configuration, for example DemoConfig().
	// Zero values keep their engine meanings.
	Simulation simulation.Config
	// Logger follows Config.Logger.
	Logger *slog.Logger
	// BasePath is the public prefix of every route, such as "/tools/ais", without
	// a trailing "/". Empty serves at the root. See NewStandaloneHandler.
	BasePath string
}

// Serve creates a simulator and serves NewStandaloneHandler on ln until ctx is
// cancelled, serving fails, or pacing fails. Serve takes ownership of ln and
// closes it, also when construction fails. On any stop, HTTP requests drain
// within a five-second budget of real time while pacing still runs; then pacing
// is cancelled and joined. Serve logs start and stop at Info and HTTP server
// errors at Error, all with component=simulator, and returns failures, such as an
// exceeded catch-up limit, with their context. It returns nil after a clean
// shutdown.
func Serve(ctx context.Context, ln net.Listener, config StandaloneConfig) error {
	sim, err := New(Config{Simulation: config.Simulation, Logger: config.Logger})
	if err != nil {
		return errors.Join(err, ln.Close())
	}
	handler, err := NewStandaloneHandler(sim, config.BasePath)
	if err != nil {
		return errors.Join(fmt.Errorf("create simulator handler: %w", err), ln.Close())
	}
	sim.logger.Info("simulator started", "url", "http://"+ln.Addr().String()+config.BasePath+"/manager")
	if err := httpserver.Run(ctx, sim.logger, ln, handler, sim.pace); err != nil {
		return err
	}
	sim.logger.Info("simulator stopped")
	return nil
}

// pace runs the pacing loop and adds context to its failure.
func (s *Simulator) pace(ctx context.Context) error {
	if err := s.Run(ctx); err != nil {
		return fmt.Errorf("run simulation: %w", err)
	}
	return nil
}
