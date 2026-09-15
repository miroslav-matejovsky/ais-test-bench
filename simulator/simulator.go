package simulator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/miroslav-matejovsky/ais-testbench/internal/simdriver"
	"github.com/miroslav-matejovsky/ais-testbench/simulation"
	"github.com/miroslav-matejovsky/ais-testbench/simulatorapi"
)

// Config configures an owned engine and its HTTP error logger. Simulation fields
// are explicit; zero count, seed, and speed retain their engine meanings.
type Config struct {
	Simulation simulation.Config
	// Logger defaults to slog.Default() at construction.
	Logger *slog.Logger
}

// Simulator owns one engine and its sole mutator, a serialized real-time driver.
// Methods are concurrency-safe. Construct with New; the zero value is not usable.
// Construction starts no goroutines or listeners. Run must be supervised by the
// caller; it never owns the caller's HTTP server or context.
type Simulator struct {
	driver *simdriver.Driver
	logger *slog.Logger
}

// New validates configuration and creates an engine on the system clock. Elapsed
// real time is measured from construction, including time before Run starts.
func New(config Config) (*Simulator, error) {
	return newSimulator(config, simdriver.SystemClock{})
}

func newSimulator(config Config, clock simdriver.Clock) (*Simulator, error) {
	engine, err := simulation.New(config.Simulation)
	if err != nil {
		return nil, fmt.Errorf("create simulation: %w", err)
	}
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Simulator{driver: simdriver.NewDriver(engine, clock), logger: logger}, nil
}

// API returns the /api/* handler over this simulator. Requests retain their
// original paths; mount at /api/ without stripping that prefix. It owns no server.
func (s *Simulator) API() http.Handler { return NewAPI(s.logger, s) }

// Run paces the engine until cancellation or settlement failure. It may be called
// once, returns nil on cancellation, and returns operational errors to its caller.
// After it returns, writes fail with simulatorapi.ErrUnavailable; snapshots remain
// readable. Commands before Run are permitted and settle time from construction.
func (s *Simulator) Run(ctx context.Context) error { return s.driver.Run(ctx) }

// SetCount settles elapsed time and changes fleet size. Invalid counts wrap
// simulation.ErrInvalid; failed settlement preserves any already delivered chunks.
func (s *Simulator) SetCount(ctx context.Context, count int) error {
	return commandError(s.driver.SetCount(ctx, count))
}

// SetSpeed settles time at the previous speed and applies speed to later time.
// Invalid speeds wrap simulation.ErrInvalid. Zero pauses real-time delivery.
func (s *Simulator) SetSpeed(ctx context.Context, speed float64) error {
	return commandError(s.driver.SetSpeed(ctx, speed))
}

// AddStation validates the definition and expected run/revision before settling.
// It returns the new ID and exact post-command configuration. Engine errors retain
// their simulation.ErrInvalid, ErrConflict, ErrNotFound, or ErrLimit identity.
func (s *Simulator) AddStation(ctx context.Context, run string, revision uint64, definition simulation.StationDefinition) (string, simulation.StationConfiguration, error) {
	id, config, err := s.driver.AddStation(ctx, run, revision, definition)
	return id, config, commandError(err)
}

// UpdateStation replaces a station after checking run/revision and station ID.
// It preserves AddStation's validation, settlement, and error guarantees.
func (s *Simulator) UpdateStation(ctx context.Context, run string, revision uint64, id string, definition simulation.StationDefinition) (simulation.StationConfiguration, error) {
	config, err := s.driver.UpdateStation(ctx, run, revision, id, definition)
	return config, commandError(err)
}

// RemoveStation removes a station after checking run/revision and station ID.
// It returns the exact post-command configuration, as UpdateStation does.
func (s *Simulator) RemoveStation(ctx context.Context, run string, revision uint64, id string) (simulation.StationConfiguration, error) {
	config, err := s.driver.RemoveStation(ctx, run, revision, id)
	return config, commandError(err)
}

// Fleet copies committed fleet state without advancing time.
func (s *Simulator) Fleet() simulation.Fleet { return s.driver.Fleet() }

// History copies retained transmissions without advancing time.
func (s *Simulator) History() simulation.History { return s.driver.History() }

// Metadata copies committed clock, identity, catalogs, and settings.
func (s *Simulator) Metadata() simulation.Metadata { return s.driver.Metadata() }

// Stations copies committed station configuration, clock, and settings atomically.
func (s *Simulator) Stations() simulation.StationConfiguration { return s.driver.Stations() }

// Observations returns an owned wire snapshot of received traffic for selected
// stations, nil meaning all. It satisfies display.Source without a network hop.
// It never advances time. Errors wrap simulatorapi source categories and causes.
func (s *Simulator) Observations(ctx context.Context, stations []string) (simulatorapi.Observations, error) {
	if err := ctx.Err(); err != nil {
		return simulatorapi.Observations{}, sourceError(err)
	}
	if err := simulatorapi.ValidateSelection(stations); err != nil {
		return simulatorapi.Observations{}, err
	}
	o, err := s.driver.Observations(stations)
	if err != nil {
		return simulatorapi.Observations{}, sourceError(err)
	}
	result := observationsResponse(o)
	if err := ctx.Err(); err != nil {
		return simulatorapi.Observations{}, sourceError(err)
	}
	return result, nil
}

// ReceptionHistory returns an owned received-NMEA page. A stale run is rejected
// before station lookup. Nil After selects the newest page; zero Limit means 100.
// Cancellation and source errors preserve their causes via wrapping.
func (s *Simulator) ReceptionHistory(ctx context.Context, stationID string, req simulatorapi.HistoryRequest) (simulatorapi.ReceptionPage, error) {
	if err := ctx.Err(); err != nil {
		return simulatorapi.ReceptionPage{}, sourceError(err)
	}
	if err := req.Validate(); err != nil {
		return simulatorapi.ReceptionPage{}, err
	}
	if err := simulatorapi.ValidateSelection([]string{stationID}); err != nil {
		return simulatorapi.ReceptionPage{}, err
	}
	limit := req.Limit
	if limit == 0 {
		limit = 100
	}
	page, err := s.driver.ReceptionHistory(req.SimulationID, stationID, req.After, limit)
	if err != nil {
		return simulatorapi.ReceptionPage{}, sourceError(err)
	}
	result := receptionPageResponse(page)
	if err := ctx.Err(); err != nil {
		return simulatorapi.ReceptionPage{}, sourceError(err)
	}
	return result, nil
}

// sourceError translates engine errors without discarding their identities.
func sourceError(err error) error {
	kind := simulatorapi.ErrUnavailable
	switch {
	case errors.Is(err, simulation.ErrInvalid):
		kind = simulatorapi.ErrInvalidRequest
	case errors.Is(err, simulation.ErrNotFound):
		kind = simulatorapi.ErrNotFound
	case errors.Is(err, simulation.ErrConflict):
		kind = simulatorapi.ErrConflict
	}
	return fmt.Errorf("%w: %w", kind, err)
}

func commandError(err error) error {
	if errors.Is(err, simdriver.ErrStopped) {
		return fmt.Errorf("%w: %w", simulatorapi.ErrUnavailable, err)
	}
	return err
}
