package display

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/miroslav-matejovsky/ais-testbench/simulatorapi"
)

type api struct {
	logger *slog.Logger
	client *Client
}

// NewAPI returns the display API backed by client:
//
//	GET /display/api/observations?stations=all          Observations
//	GET /display/api/stations/{id}/receptions?simulationId=...&after=...&limit=...  ReceptionPage
//
// Mount it at /display/api/. Every request performs one simulator read;
// nothing is cached. Invalid display queries return 400. A simulator 400, 404,
// or 409 keeps its status and message. Retryable upstream failures return 503;
// simulator responses that violate the API return 502 with context.
func NewAPI(logger *slog.Logger, client *Client) http.Handler {
	a := &api{logger: logger, client: client}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /display/api/observations", a.observations)
	mux.HandleFunc("GET /display/api/stations/{id}/receptions", a.receptions)
	return mux
}

func (a *api) observations(w http.ResponseWriter, r *http.Request) {
	values, err := queryValues(r, "stations")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var stations []string
	if values.Has("stations") && values.Get("stations") != "all" {
		stations = strings.Split(values.Get("stations"), ",")
		for i, id := range stations {
			if id == "" || id == "all" || strings.TrimSpace(id) != id || slices.Contains(stations[:i], id) {
				http.Error(w, "stations must be all or distinct, nonempty station IDs", http.StatusBadRequest)
				return
			}
		}
	}
	o, err := a.client.Observations(r.Context(), stations)
	if err != nil {
		a.writeError(w, r, "read simulator observations", err)
		return
	}
	a.writeJSON(w, r, o)
}

func (a *api) receptions(w http.ResponseWriter, r *http.Request) {
	values, err := queryValues(r, "simulationId", "after", "limit")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req := simulatorapi.HistoryRequest{SimulationID: values.Get("simulationId")}
	if req.SimulationID == "" {
		http.Error(w, "simulationId is required", http.StatusBadRequest)
		return
	}
	if values.Has("after") {
		after, err := parseDecimal(values.Get("after"))
		if err != nil {
			http.Error(w, "after: "+err.Error(), http.StatusBadRequest)
			return
		}
		req.After = &after
	}
	if values.Has("limit") {
		limit, err := parseDecimal(values.Get("limit"))
		if err != nil || limit < 1 || limit > simulatorapi.ReceptionPageLimit {
			http.Error(w, fmt.Sprintf("limit must be an integer from 1 through %d", simulatorapi.ReceptionPageLimit), http.StatusBadRequest)
			return
		}
		req.Limit = int(limit)
	}
	page, err := a.client.ReceptionHistory(r.Context(), r.PathValue("id"), req)
	if err != nil {
		a.writeError(w, r, "read simulator reception history", err)
		return
	}
	a.writeJSON(w, r, page)
}

// queryValues rejects malformed, unknown, and repeated query parameters.
func queryValues(r *http.Request, allowed ...string) (url.Values, error) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, fmt.Errorf("invalid query: %w", err)
	}
	for key, entries := range values {
		if !slices.Contains(allowed, key) || len(entries) != 1 {
			return nil, fmt.Errorf("unknown or repeated query parameter %q", key)
		}
	}
	return values, nil
}

// writeError maps a client error to a plain text response. A simulator 400,
// 404, or 409 is user-visible; other failures never return partial data.
func (a *api) writeError(w http.ResponseWriter, r *http.Request, operation string, err error) {
	status, message := simulatorapi.ErrorStatus(err), "could not read simulator"
	switch status {
	case http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusBadGateway:
		message = err.Error()
	case http.StatusServiceUnavailable:
		message = "simulator unavailable, retrying"
	}
	// A browser that went away and user-visible request errors need no log entry.
	if status >= http.StatusInternalServerError && r.Context().Err() == nil {
		a.logger.Warn(operation, "path", r.URL.Path, "status", status, "error", err)
	}
	http.Error(w, message, status)
}

func (a *api) writeJSON(w http.ResponseWriter, r *http.Request, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		a.logger.Error("write JSON response", "path", r.URL.Path, "error", err)
	}
}
