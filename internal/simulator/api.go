package simulator

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"time"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/simulation"
	"github.com/miroslav-matejovsky/ais-test-bench/internal/simulatorapi"
)

// maxCountRequestBytes bounds the PUT /api/vessels request body.
const maxCountRequestBytes = 1024

type api struct {
	logger *slog.Logger
	sim    *simulation.Simulator
}

// NewAPI returns the simulator HTTP API for sim:
//
//	GET /api/vessels   simulatorapi.Fleet
//	PUT /api/vessels   simulatorapi.CountRequest in, simulatorapi.Fleet out
//	GET /api/messages  simulatorapi.History
//	GET /api/metadata  simulatorapi.Metadata
//
// Mount it at /api/. The handler is stateless apart from sim, so mounting it on
// several listeners serves one engine consistently.
func NewAPI(logger *slog.Logger, sim *simulation.Simulator) http.Handler {
	a := &api{logger: logger, sim: sim}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/vessels", a.fleet)
	mux.HandleFunc("PUT /api/vessels", a.setCount)
	mux.HandleFunc("GET /api/messages", a.history)
	mux.HandleFunc("GET /api/metadata", a.metadata)
	return mux
}

func (a *api) fleet(w http.ResponseWriter, r *http.Request) {
	a.writeJSON(w, r, a.sim.Fleet())
}

func (a *api) history(w http.ResponseWriter, r *http.Request) {
	a.writeJSON(w, r, a.sim.History())
}

func (a *api) metadata(w http.ResponseWriter, r *http.Request) {
	a.writeJSON(w, r, a.sim.Metadata())
}

// setCount validates one JSON CountRequest before touching the engine, so an
// invalid request never changes state.
func (a *api) setCount(w http.ResponseWriter, r *http.Request) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxCountRequestBytes)
	var input simulatorapi.CountRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		http.Error(w, "request must contain one JSON object", http.StatusBadRequest)
		return
	}
	if input.Count == nil {
		http.Error(w, "count is required", http.StatusBadRequest)
		return
	}
	if *input.Count < 0 || *input.Count > simulation.MaxVessels {
		http.Error(w, fmt.Sprintf("count must be between 0 and %d", simulation.MaxVessels), http.StatusBadRequest)
		return
	}
	if err := a.sim.SetCount(*input.Count, time.Now()); err != nil {
		a.logger.Error("set vessel count", "count", *input.Count, "error", err)
		http.Error(w, "could not update vessel count", http.StatusInternalServerError)
		return
	}
	a.writeJSON(w, r, a.sim.Fleet())
}

func (a *api) writeJSON(w http.ResponseWriter, r *http.Request, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		a.logger.Error("write JSON response", "path", r.URL.Path, "error", err)
	}
}
