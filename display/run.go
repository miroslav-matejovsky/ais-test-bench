package display

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/miroslav-matejovsky/ais-testbench/internal/httpserver"
	"github.com/miroslav-matejovsky/ais-testbench/internal/urlpath"
	"github.com/miroslav-matejovsky/ais-testbench/ui"
)

// StandaloneConfig configures the standalone display.
type StandaloneConfig struct {
	// Client is required and borrowed. Serve closes only its owned idle connections.
	Client *Client
	// Logger receives display records with component=display and page records
	// with component=ui. Nil means slog.Default().
	Logger *slog.Logger
	// ManagerURL is an optional simulator manager link: an absolute path or an
	// absolute http(s) URL. It is never inferred from the source.
	ManagerURL string
	// BasePath is the public prefix of every route, such as "/tools/ais", without
	// a trailing "/". Empty serves at the root.
	BasePath string
	// Tiles configures the map tile source; the zero value uses OpenStreetMap.
	Tiles ui.MapTiles
}

// NewStandaloneHandler returns the standalone display routes below BasePath:
//
//	GET {base}/              redirect to {base}/display, keeping the query
//	GET {base}/display       display page
//	GET {base}/assets/       embedded assets
//	    {base}/display/api/  display API, see NewHandler
//
// The handler matches full request paths; mount it without http.StripPrefix.
func NewStandaloneHandler(config StandaloneConfig) (http.Handler, error) {
	if err := urlpath.CheckPrefix(config.BasePath); err != nil {
		return nil, fmt.Errorf("base path: %w", err)
	}
	api, err := NewHandler(Config{Client: config.Client, Logger: config.Logger})
	if err != nil {
		return nil, err
	}
	base := config.BasePath
	pages, err := ui.New(ui.Config{
		DisplayAPIBase: base + "/display/api/", AssetsBase: base + "/assets/",
		HomeURL: base + "/", DisplayURL: base + "/display", ManagerURL: config.ManagerURL,
		Tiles: config.Tiles, Logger: config.Logger,
	})
	if err != nil {
		return nil, fmt.Errorf("create pages: %w", err)
	}
	page, err := pages.DisplayPage()
	if err != nil {
		return nil, fmt.Errorf("create display page: %w", err)
	}
	mux := http.NewServeMux()
	mux.Handle(base+"/display/api/", http.StripPrefix(base+"/display/api", api))
	mux.Handle(base+"/assets/", http.StripPrefix(base+"/assets", pages.Assets()))
	mux.HandleFunc("GET "+base+"/{$}", func(w http.ResponseWriter, r *http.Request) {
		target := base + "/display"
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, http.StatusFound)
	})
	mux.Handle(base+"/display", page)
	return mux, nil
}

// Serve serves the standalone display (see NewStandaloneHandler) on ln until ctx
// is cancelled or serving fails. Serve takes ownership of ln and closes it, also
// when construction fails. Shutdown waits up to five seconds of real time for
// in-flight requests, which cancels their source reads when it expires, and then
// closes idle connections owned by config.Client; a borrowed source or HTTP client
// stays open. An unreachable simulator does not stop Serve; each request reports
// it. Serve returns nil after a clean shutdown. Lifecycle and HTTP server records
// use component=display.
func Serve(ctx context.Context, ln net.Listener, config StandaloneConfig) error {
	handler, err := NewStandaloneHandler(config)
	if err != nil {
		return errors.Join(fmt.Errorf("create display handler: %w", err), ln.Close())
	}
	defer config.Client.CloseIdleConnections()

	logger := componentLogger(config.Logger)
	logger.Info("display started", "url", "http://"+ln.Addr().String()+config.BasePath+"/display")
	if err := httpserver.Run(ctx, logger, ln, handler, nil); err != nil {
		return err
	}
	logger.Info("display stopped")
	return nil
}
