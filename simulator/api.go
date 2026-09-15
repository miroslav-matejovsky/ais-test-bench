package simulator

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"

	"github.com/miroslav-matejovsky/ais-testbench/simulation"
	"github.com/miroslav-matejovsky/ais-testbench/simulatorapi"
)

// maxRequestBytes bounds PUT request bodies.
const maxRequestBytes = 1024

type api struct {
	logger *slog.Logger
	sim    *Simulator
}

// newAPI returns the simulator HTTP API for the engine paced by sim, with local
// routes:
//
//	GET    /vessels                  simulatorapi.Fleet
//	PUT    /vessels                  simulatorapi.CountRequest in, simulatorapi.Fleet out
//	GET    /messages                 simulatorapi.History
//	GET    /metadata                 simulatorapi.Metadata
//	PUT    /time                     simulatorapi.TimeRequest in, simulatorapi.Metadata out
//	GET    /stations                 simulatorapi.StationSet
//	POST   /stations                 simulatorapi.StationRequest in, simulatorapi.StationCreated out
//	PUT    /stations/{id}            simulatorapi.StationRequest in, simulatorapi.StationSet out
//	DELETE /stations/{id}            simulatorapi.StationSet
//	GET    /observations             simulatorapi.Observations
//	GET    /stations/{id}/receptions simulatorapi.ReceptionPage
//
// The handler is stateless apart from sim, so mounting it on several listeners
// serves one engine consistently. Logger must be non-nil.
func newAPI(logger *slog.Logger, sim *Simulator) http.Handler {
	a := &api{logger: logger, sim: sim}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /vessels", a.fleet)
	mux.HandleFunc("PUT /vessels", a.setCount)
	mux.HandleFunc("GET /messages", a.history)
	mux.HandleFunc("GET /metadata", a.metadata)
	mux.HandleFunc("PUT /time", a.setTime)
	mux.HandleFunc("GET /stations", a.stations)
	mux.HandleFunc("POST /stations", a.addStation)
	mux.HandleFunc("PUT /stations/{id}", a.updateStation)
	mux.HandleFunc("DELETE /stations/{id}", a.removeStation)
	mux.HandleFunc("GET /observations", a.observations)
	mux.HandleFunc("GET /stations/{id}/receptions", a.receptions)
	return mux
}

func (a *api) fleet(w http.ResponseWriter, r *http.Request) {
	a.writeJSON(w, r, fleetResponse(a.sim.Fleet()))
}

func (a *api) history(w http.ResponseWriter, r *http.Request) {
	a.writeJSON(w, r, historyResponse(a.sim.History()))
}

func (a *api) metadata(w http.ResponseWriter, r *http.Request) {
	a.writeJSON(w, r, metadataResponse(a.sim.Metadata()))
}

// setCount validates one CountRequest before calling the driver, so an invalid
// request neither settles time nor changes state.
func (a *api) setCount(w http.ResponseWriter, r *http.Request) {
	var input simulatorapi.CountRequest
	if !readJSON(w, r, &input) {
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
	if err := a.sim.SetCount(r.Context(), *input.Count); err != nil {
		a.logger.ErrorContext(r.Context(), "set vessel count", "path", r.URL.Path, "count", *input.Count, "error", err)
		http.Error(w, "could not update vessel count", simulatorapi.ErrorStatus(err))
		return
	}
	a.writeJSON(w, r, fleetResponse(a.sim.Fleet()))
}

// setTime validates one TimeRequest before calling the driver and returns the
// resulting metadata.
func (a *api) setTime(w http.ResponseWriter, r *http.Request) {
	var input simulatorapi.TimeRequest
	if !readJSON(w, r, &input) {
		return
	}
	if input.Speed == nil {
		http.Error(w, "speed is required", http.StatusBadRequest)
		return
	}
	if err := simulation.ValidateSpeed(*input.Speed); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.sim.SetSpeed(r.Context(), *input.Speed); err != nil {
		a.logger.ErrorContext(r.Context(), "set simulation speed", "path", r.URL.Path, "speed", *input.Speed, "error", err)
		http.Error(w, "could not update simulation speed", simulatorapi.ErrorStatus(err))
		return
	}
	a.writeJSON(w, r, metadataResponse(a.sim.Metadata()))
}

// readJSON decodes exactly one application/json object of at most
// maxRequestBytes without unknown fields into value. Otherwise it writes 415 or
// 400 and returns false.
func readJSON(w http.ResponseWriter, r *http.Request, value any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return false
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		http.Error(w, "request must contain one JSON object", http.StatusBadRequest)
		return false
	}
	return true
}

func (a *api) writeJSON(w http.ResponseWriter, r *http.Request, value any) {
	a.writeStatusJSON(w, r, http.StatusOK, value)
}

func (a *api) writeStatusJSON(w http.ResponseWriter, r *http.Request, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		a.logger.ErrorContext(r.Context(), "write JSON response", "path", r.URL.Path, "error", err)
	}
}
