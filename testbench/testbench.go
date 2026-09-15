package testbench

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/miroslav-matejovsky/ais-testbench/display"
	"github.com/miroslav-matejovsky/ais-testbench/internal/httpserver"
	"github.com/miroslav-matejovsky/ais-testbench/internal/urlpath"
	"github.com/miroslav-matejovsky/ais-testbench/simulation"
	"github.com/miroslav-matejovsky/ais-testbench/simulator"
	"github.com/miroslav-matejovsky/ais-testbench/ui"
)

// Config configures one bench.
type Config struct {
	// Simulation is the owned engine configuration, for example
	// simulator.DemoConfig(). Zero values keep their engine meanings.
	Simulation simulation.Config
	// Logger is shared by every child: engine, simulator, display, and UI. A
	// non-nil Logger replaces Simulation.Logger. Nil falls back to
	// Simulation.Logger, then slog.Default(), resolved once in New.
	Logger *slog.Logger
	// BasePath is the public prefix of every route, such as "/tools/ais", without
	// a trailing "/". Empty serves at the root. Handler matches request paths that
	// start with it, so a proxy that strips a prefix needs the lower-level packages.
	BasePath string
	// Tiles configures the display map tile source; the zero value uses
	// OpenStreetMap.
	Tiles ui.MapTiles
}

// Bench is one simulator with its API, manager, display, status page, and assets
// below one base path. Methods are safe for concurrent use. Construct it with New.
type Bench struct {
	sim     *simulator.Simulator
	ui      *ui.UI
	handler http.Handler
	logger  *slog.Logger // component=testbench, for Serve.
}

// New validates config and composes a bench through the public simulator,
// display, and ui packages. The display reads the simulator in process through
// display.New, so no listener or HTTP client is created. New starts no
// goroutines and binds nothing.
func New(config Config) (*Bench, error) {
	if err := urlpath.CheckPrefix(config.BasePath); err != nil {
		return nil, fmt.Errorf("BasePath: %w", err)
	}
	logger := cmp.Or(config.Logger, config.Simulation.Logger, slog.Default())
	sim, err := simulator.New(simulator.Config{Simulation: config.Simulation, Logger: logger})
	if err != nil {
		return nil, err
	}
	client, err := display.New(sim)
	if err != nil {
		return nil, fmt.Errorf("create display client: %w", err)
	}
	displayAPI, err := display.NewHandler(display.Config{Client: client, Logger: logger})
	if err != nil {
		return nil, fmt.Errorf("create display API: %w", err)
	}
	base := config.BasePath
	pages, err := ui.New(ui.Config{
		ManagerAPIBase: base + "/api/", DisplayAPIBase: base + "/display/api/", AssetsBase: base + "/assets/",
		HomeURL: base + "/", ManagerURL: base + "/manager", DisplayURL: base + "/display", StatusURL: base + "/status",
		Tiles: config.Tiles, Logger: logger,
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
	mux.Handle(base+"/api/", http.StripPrefix(base+"/api", sim.API()))
	mux.Handle(base+"/display/api/", http.StripPrefix(base+"/display/api", displayAPI))
	mux.Handle(base+"/assets/", http.StripPrefix(base+"/assets", pages.Assets()))
	mux.Handle(base+"/{$}", pages.HomePage())
	mux.Handle(base+"/manager", managerPage)
	mux.Handle(base+"/display", displayPage)
	mux.Handle(base+"/status", pages.StatusPage())
	return &Bench{sim: sim, ui: pages, handler: mux, logger: logger.With("component", "testbench")}, nil
}

// Handler returns every bench route, matching full request paths:
//
//	{base}/              home page
//	{base}/manager       manager page
//	{base}/display       display page
//	{base}/status        status page
//	{base}/api/          simulator API, see simulator.Simulator.API
//	{base}/display/api/  display API, see display.NewHandler
//	{base}/assets/       embedded assets, see ui.UI.Assets
//
// Mount it at the base path without http.StripPrefix, for example
// mux.Handle("/tools/ais/", bench.Handler()). Other paths return 404. The handler
// owns no server or client and can be wrapped in host middleware.
func (b *Bench) Handler() http.Handler { return b.handler }

// Run paces the simulation until ctx is cancelled or pacing fails; see
// simulator.Simulator.Run. It may be called once. Supervise it next to the host
// server: drain requests, then cancel and join Run. After Run returns, writes
// answer 503 while pages and reads keep working.
func (b *Bench) Run(ctx context.Context) error { return b.sim.Run(ctx) }

// UI returns the bench's UI, configured with its public URLs, for rendering the
// manager and display components into host templates.
func (b *Bench) UI() *ui.UI { return b.ui }

// Serve creates a bench and serves its Handler on ln next to Run until ctx is
// cancelled, serving fails, or pacing fails. Serve takes ownership of ln and
// closes it, also when construction fails. On any stop, requests drain within a
// five-second budget of real time while pacing still runs; then pacing is
// cancelled and joined. Serve logs start and stop at Info and HTTP server errors
// at Error with component=testbench, and returns failures with their context. It
// returns nil after a clean shutdown.
func Serve(ctx context.Context, ln net.Listener, config Config) error {
	bench, err := New(config)
	if err != nil {
		return errors.Join(fmt.Errorf("create test bench: %w", err), ln.Close())
	}
	bench.logger.Info("ais-testbench started", "url", "http://"+ln.Addr().String()+config.BasePath+"/")
	if err := httpserver.Run(ctx, bench.logger, ln, bench.Handler(), bench.pace); err != nil {
		return err
	}
	bench.logger.Info("ais-testbench stopped")
	return nil
}

// pace runs Run and adds context to its failure.
func (b *Bench) pace(ctx context.Context) error {
	if err := b.Run(ctx); err != nil {
		return fmt.Errorf("run simulation: %w", err)
	}
	return nil
}
