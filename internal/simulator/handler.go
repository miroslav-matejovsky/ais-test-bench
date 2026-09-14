package simulator

import (
	"fmt"
	"log/slog"
	"net/http"

	simdriver "github.com/miroslav-matejovsky/ais-test-bench/internal/simulation"
	"github.com/miroslav-matejovsky/ais-test-bench/internal/ui"
)

// NewHandler returns the standalone simulator routes for sim:
//
//	GET /           redirect to /manager
//	GET /manager    manager page
//	GET /status     server status, a fragment for htmx partial requests
//	GET /static/    embedded static files
//	    /api/       simulator API, see NewAPI
func NewHandler(logger *slog.Logger, sim *simdriver.Driver) (http.Handler, error) {
	pages, err := ui.NewPages(logger, []ui.Link{{Href: "/manager", Label: "Manager"}})
	if err != nil {
		return nil, fmt.Errorf("create pages: %w", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/api/", NewAPI(logger, sim))
	mux.Handle("GET /static/", ui.Static())
	mux.Handle("GET /{$}", http.RedirectHandler("/manager", http.StatusFound))
	mux.HandleFunc("GET /manager", pages.Manager)
	mux.HandleFunc("GET /status", pages.Status)
	return mux, nil
}
