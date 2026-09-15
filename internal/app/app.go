package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/miroslav-matejovsky/ais-testbench/display"
	"github.com/miroslav-matejovsky/ais-testbench/internal/httpserver"
	"github.com/miroslav-matejovsky/ais-testbench/internal/simdriver"
	"github.com/miroslav-matejovsky/ais-testbench/simulator"
	"github.com/miroslav-matejovsky/ais-testbench/ui"
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
	sim, err := simulator.New(simulator.Config{Simulation: simdriver.NewConfig(), Logger: logger})
	if err != nil {
		return fail(fmt.Errorf("create simulation: %w", err))
	}
	client, err := display.NewClient("http://" + internalLn.Addr().String() + "/api/")
	if err != nil {
		return fail(fmt.Errorf("create display client: %w", err))
	}
	displayAPI, err := display.NewHandler(display.Config{Client: client, Logger: logger})
	if err != nil {
		return fail(fmt.Errorf("create display API: %w", err))
	}
	public, err := publicHandler(logger, sim, displayAPI)
	if err != nil {
		return fail(err)
	}
	simulatorAPI := http.StripPrefix("/api", sim.API())
	internal := http.NewServeMux()
	internal.Handle("/api/", simulatorAPI)

	// Both listeners are bound, so display requests can reach the internal API
	// as soon as public serving starts.
	internalServer := httpserver.Serve(logger, internalLn, internal)
	publicServer := httpserver.Serve(logger, ln, public)
	logger.Info("ais-testbench started", "url", "http://"+ln.Addr().String(), "internalAPI", "http://"+internalLn.Addr().String()+"/api/")
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
	logger.Info("ais-testbench stopped")
	return nil
}

// publicHandler composes the public routes at the root path: home, manager,
// display, and status pages, assets, the simulator API, and the display API.
func publicHandler(logger *slog.Logger, sim *simulator.Simulator, displayAPI http.Handler) (http.Handler, error) {
	pages, err := ui.New(ui.Config{
		ManagerAPIBase: "/api/", DisplayAPIBase: "/display/api/", AssetsBase: "/assets/",
		HomeURL: "/", ManagerURL: "/manager", DisplayURL: "/display", StatusURL: "/status", Logger: logger,
	})
	if err != nil {
		return nil, fmt.Errorf("create pages: %w", err)
	}
	managerPage, err := pages.ManagerPage()
	if err != nil {
		return nil, fmt.Errorf("create manager page: %w", err)
	}
	displayPage, err := pages.DisplayPage()
	if err != nil {
		return nil, fmt.Errorf("create display page: %w", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/api/", http.StripPrefix("/api", sim.API()))
	mux.Handle("/display/api/", http.StripPrefix("/display/api", displayAPI))
	mux.Handle("/assets/", http.StripPrefix("/assets", pages.Assets()))
	mux.Handle("/{$}", pages.HomePage())
	mux.Handle("/manager", managerPage)
	mux.Handle("/display", displayPage)
	mux.Handle("/status", pages.StatusPage())
	return mux, nil
}

// wrap adds context to a non-nil error.
func wrap(context string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", context, err)
}
