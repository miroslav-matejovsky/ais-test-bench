package simulatorapi

import (
	"errors"
	"net/http"
	"strings"
)

// Source error categories preserve their underlying cause through wrapping.
// Sources return no partial snapshot or page with an error.
var (
	ErrInvalidRequest  = errors.New("invalid source request")
	ErrNotFound        = errors.New("station not found")
	ErrConflict        = errors.New("simulation identity changed")
	ErrUnavailable     = errors.New("simulator unavailable")
	ErrInvalidResponse = errors.New("invalid simulator response")
)

// HistoryRequest selects a station's reception page within one simulation run.
type HistoryRequest struct {
	// SimulationID is required and must identify the current run.
	SimulationID string
	// After is the last reception sequence read; nil selects the newest page.
	After *uint64
	// Limit is 1 through ReceptionPageLimit; zero selects 100 receptions.
	Limit int
}

// Validate rejects invalid paging options before reading any source.
func (r HistoryRequest) Validate() error {
	if strings.TrimSpace(r.SimulationID) == "" || r.Limit < 0 || r.Limit > ReceptionPageLimit {
		return ErrInvalidRequest
	}
	return nil
}

// ValidateSelection checks station IDs. Nil selects all stations. A non-nil
// selection must contain distinct, nonempty IDs without commas or surrounding whitespace.
// The reserved selector "all" is represented by nil, never as an ID.
func ValidateSelection(ids []string) error {
	if ids != nil && len(ids) == 0 {
		return ErrInvalidRequest
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id == "" || id == "all" || strings.Contains(id, ",") || strings.TrimSpace(id) != id || seen[id] {
			return ErrInvalidRequest
		}
		seen[id] = true
	}
	return nil
}

// ErrorStatus maps a source error to an HTTP status. Unclassified failures are
// internal errors. The original wrapped error remains available to the caller.
func ErrorStatus(err error) int {
	switch {
	case errors.Is(err, ErrInvalidRequest):
		return http.StatusBadRequest
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrConflict):
		return http.StatusConflict
	case errors.Is(err, ErrUnavailable):
		return http.StatusServiceUnavailable
	case errors.Is(err, ErrInvalidResponse):
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}
