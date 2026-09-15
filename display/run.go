package display

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/miroslav-matejovsky/ais-testbench/internal/httpserver"
	"github.com/miroslav-matejovsky/ais-testbench/ui"
)

// StandaloneConfig configures the standalone display served at the root path.
type StandaloneConfig struct {
	// Client is required and borrowed. Run closes only its owned idle connections.
	Client *Client
	// Logger receives display records with component=display and page records
	// with component=ui. Nil means slog.Default().
	Logger *slog.Logger
	// ManagerURL is an optional simulator manager link: an absolute path or an
	// absolute http(s) URL. It is never inferred from the source.
	ManagerURL string
}

// NewStandaloneHandler returns the standalone display routes:
//
//	GET /              redirect to /display, keeping the query
//	GET /display       display page
//	GET /assets/       embedded assets
//	    /display/api/  display API, see NewHandler
func NewStandaloneHandler(config StandaloneConfig) (http.Handler, error) {
	api, err := NewHandler(Config{Client: config.Client, Logger: config.Logger})
	if err != nil {
		return nil, err
	}
	pages, err := ui.New(ui.Config{
		DisplayAPIBase: "/display/api/", AssetsBase: "/assets/",
		HomeURL: "/", DisplayURL: "/display", ManagerURL: config.ManagerURL, Logger: config.Logger,
	})
	if err != nil {
		return nil, fmt.Errorf("create pages: %w", err)
	}
	page, err := pages.DisplayPage()
	if err != nil {
		return nil, fmt.Errorf("create display page: %w", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/display/api/", http.StripPrefix("/display/api", api))
	mux.Handle("/assets/", http.StripPrefix("/assets", pages.Assets()))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		target := "/display"
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, http.StatusFound)
	})
	mux.Handle("/display", page)
	return mux, nil
}

// Run serves the standalone display (see NewStandaloneHandler) on ln until ctx
// is cancelled or serving fails. Run takes ownership of ln and closes it, also
// when construction fails. Shutdown waits up to httpserver.ShutdownTimeout for
// in-flight requests, which cancels their simulator reads when it expires, and
// then closes idle simulator connections. An unreachable simulator does not
// stop Run; each request reports it. Run returns nil after a clean shutdown.
// Lifecycle and HTTP server records use component=display.
func Run(ctx context.Context, ln net.Listener, config StandaloneConfig) error {
	handler, err := NewStandaloneHandler(config)
	if err != nil {
		return errors.Join(fmt.Errorf("create display handler: %w", err), ln.Close())
	}
	defer config.Client.CloseIdleConnections()

	logger := componentLogger(config.Logger)
	server := httpserver.Serve(logger, ln, handler)
	logger.Info("display started", "url", "http://"+ln.Addr().String())
	select {
	case <-server.Done():
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), httpserver.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}
	logger.Info("display stopped")
	return nil
}
