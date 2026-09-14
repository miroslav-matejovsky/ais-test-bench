package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/display"
	"github.com/miroslav-matejovsky/ais-test-bench/internal/httpserver"
	"github.com/miroslav-matejovsky/ais-test-bench/internal/simdriver"
	"github.com/miroslav-matejovsky/ais-test-bench/internal/simulator"
	"github.com/miroslav-matejovsky/ais-test-bench/internal/ui"
	"github.com/miroslav-matejovsky/ais-test-bench/simulation"
)

// internalAddr is the private simulator API listener of combined mode. The OS
// assigns the port, independent of the public address.
const internalAddr = "127.0.0.1:0"

// Run serves the combined test bench on the public listener ln until ctx is
// cancelled or any component fails. It runs one simulation engine, serves its
// API publicly and on a private loopback listener, and gives the display a
// simulator HTTP client for that private listener, so the display reads the
// engine over HTTP exactly as in separate mode.
//
// Run takes ownership of ln and closes it, also when construction fails. It
// returns nil after a clean shutdown.
func Run(ctx context.Context, logger *slog.Logger, ln net.Listener) error {
	internalLn, err := net.Listen("tcp", internalAddr)
	if err != nil {
		return errors.Join(fmt.Errorf("listen for internal simulator API: %w", err), ln.Close())
	}
	return serve(ctx, logger, ln, internalLn)
}

// serve runs the combined components with ln as the public listener and
// internalLn as the private simulator API listener. It owns and closes both.
func serve(ctx context.Context, logger *slog.Logger, ln, internalLn net.Listener) error {
	fail := func(err error) error {
		return errors.Join(err, ln.Close(), internalLn.Close())
	}
	engine, err := simulation.New(simdriver.NewConfig())
	if err != nil {
		return fail(fmt.Errorf("create simulation: %w", err))
	}
	sim := simdriver.NewDriver(engine, simdriver.SystemClock{})
	client, err := display.NewClient("http://" + internalLn.Addr().String())
	if err != nil {
		return fail(fmt.Errorf("create display client: %w", err))
	}
	pages, err := ui.NewPages(logger, []ui.Link{{Href: "/manager", Label: "Manager"}, {Href: "/display", Label: "Display"}})
	if err != nil {
		return fail(fmt.Errorf("create pages: %w", err))
	}

	simulatorAPI := simulator.NewAPI(logger, sim)
	internal := http.NewServeMux()
	internal.Handle("/api/", simulatorAPI)
	public := http.NewServeMux()
	public.Handle("/api/", simulatorAPI)
	public.Handle("/display/api/", display.NewAPI(logger, client))
	public.Handle("GET /static/", ui.Static())
	public.HandleFunc("GET /{$}", pages.Home)
	public.HandleFunc("GET /manager", pages.Manager)
	public.HandleFunc("GET /display", pages.Display)
	public.HandleFunc("GET /status", pages.Status)

	// Both listeners are bound, so display requests can reach the internal API
	// as soon as public serving starts.
	internalServer := httpserver.Serve(logger, internalLn, internal)
	publicServer := httpserver.Serve(logger, ln, public)
	logger.Info("ais-test-bench started", "url", "http://"+ln.Addr().String(), "internalAPI", client.Origin())
	simulationCtx, stopSimulation := context.WithCancel(ctx)
	simulationDone := make(chan struct{})
	var simulationErr error // Read only after simulationDone closes.
	go func() {
		simulationErr = sim.Run(simulationCtx)
		close(simulationDone)
	}()

	select {
	case <-publicServer.Done():
	case <-internalServer.Done():
	case <-simulationDone:
	case <-ctx.Done():
	}

	// Public requests drain first while the internal API still answers their
	// simulator reads. One budget bounds the whole shutdown.
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), httpserver.ShutdownTimeout)
	defer cancel()
	err = wrap("public server", publicServer.Shutdown(shutdownCtx))
	client.CloseIdleConnections()
	err = errors.Join(err, wrap("internal simulator API server", internalServer.Shutdown(shutdownCtx)))
	stopSimulation()
	<-simulationDone
	err = errors.Join(err, wrap("run simulation", simulationErr))
	if err != nil {
		return err
	}
	logger.Info("ais-test-bench stopped")
	return nil
}

// wrap adds context to a non-nil error.
func wrap(context string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", context, err)
}
