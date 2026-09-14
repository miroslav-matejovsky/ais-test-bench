package display

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

type api struct {
	logger *slog.Logger
	client *Client
}

// NewAPI returns the display API backed by client:
//
//	GET /display/api/vessels  Fleet decoded from the simulator API
//
// Mount it at /display/api/. Every request reads the simulator; nothing is
// cached. Upstream failures a retry can resolve return 503, simulator responses
// that violate the API return 502 with context.
func NewAPI(logger *slog.Logger, client *Client) http.Handler {
	a := &api{logger: logger, client: client}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /display/api/vessels", a.vessels)
	return mux
}

func (a *api) vessels(w http.ResponseWriter, r *http.Request) {
	fleet, err := a.client.Fleet(r.Context())
	if err != nil {
		status, message := http.StatusInternalServerError, "could not read simulator"
		switch {
		case errors.Is(err, errInvalid):
			status, message = http.StatusBadGateway, err.Error()
		case errors.Is(err, errUnavailable):
			status, message = http.StatusServiceUnavailable, "simulator unavailable, retrying"
		}
		// A browser that went away needs no log entry.
		if r.Context().Err() == nil {
			a.logger.Warn("read simulator fleet", "status", status, "error", err)
		}
		http.Error(w, message, status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(fleet); err != nil {
		a.logger.Error("write JSON response", "path", r.URL.Path, "error", err)
	}
}
