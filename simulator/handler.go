package simulator

import (
	"fmt"
	"net/http"

	"github.com/miroslav-matejovsky/ais-testbench/ui"
)

// NewStandaloneHandler returns the standalone simulator routes for sim, served
// at the root path and logging to sim's logger:
//
//	GET /           redirect to /manager, keeping the query
//	GET /manager    manager page
//	GET /status     server status, a fragment for htmx partial requests
//	GET /assets/    embedded assets
//	    /api/       simulator API, see Simulator.API
func NewStandaloneHandler(sim *Simulator) (http.Handler, error) {
	pages, err := ui.New(ui.Config{
		ManagerAPIBase: "/api/", AssetsBase: "/assets/",
		HomeURL: "/", ManagerURL: "/manager", StatusURL: "/status", Logger: sim.baseLogger,
	})
	if err != nil {
		return nil, fmt.Errorf("create pages: %w", err)
	}
	manager, err := pages.ManagerPage()
	if err != nil {
		return nil, fmt.Errorf("create manager page: %w", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/api/", http.StripPrefix("/api", sim.API()))
	mux.Handle("/assets/", http.StripPrefix("/assets", pages.Assets()))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		target := "/manager"
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, http.StatusFound)
	})
	mux.Handle("/manager", manager)
	mux.Handle("/status", pages.StatusPage())
	return mux, nil
}
