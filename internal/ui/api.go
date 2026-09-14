package ui

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/simulation"
)

func (s *server) vessels(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, r, s.simulator.Snapshot())
}

func (s *server) messages(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, r, s.simulator.History().Messages)
}

func (s *server) setVesselCount(w http.ResponseWriter, r *http.Request) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	var input struct {
		Count *int `json:"count"`
	}
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
		http.Error(w, "count must be between 0 and 100", http.StatusBadRequest)
		return
	}
	if err := s.simulator.SetCount(*input.Count, time.Now()); err != nil {
		s.logger.Error("set vessel count", "error", err)
		http.Error(w, "could not update vessel count", http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, r, s.simulator.Snapshot())
}

func (s *server) writeJSON(w http.ResponseWriter, r *http.Request, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		s.logger.Error("write JSON response", "path", r.URL.Path, "error", err)
	}
}
