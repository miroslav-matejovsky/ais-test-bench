package display

import (
	"context"
	"fmt"

	"github.com/miroslav-matejovsky/ais-testbench/simulatorapi"
)

// Source supplies received-traffic snapshots and station history, never the truth
// fleet. Methods must be concurrency-safe, honor cancellation, and return owned
// data the caller may retain and mutate. Nil station selection means all stations.
// Errors wrap simulatorapi.ErrInvalidRequest, ErrNotFound, ErrConflict,
// ErrUnavailable, or ErrInvalidResponse, preserving underlying causes. A failed
// read returns no partial result. Source implementations own their resources;
// passing one to New does not transfer ownership.
type Source interface {
	Observations(context.Context, []string) (simulatorapi.Observations, error)
	ReceptionHistory(context.Context, string, simulatorapi.HistoryRequest) (simulatorapi.ReceptionPage, error)
}

// Client validates and projects every Source response through the same semantic
// and AIS checks. It has no cache, background poller, engine, or server. Construct
// it with New for a borrowed source or NewClient for an owned HTTP source.
type Client struct {
	source Source
	owned  *HTTPSource
}

// New constructs a client borrowing source. Source must be non-nil, including its
// concrete value. Construction performs no reads and starts no background work.
// The caller remains responsible for source cleanup after all requests finish.
func New(source Source) (*Client, error) {
	if source == nil {
		return nil, fmt.Errorf("%w: source is required", simulatorapi.ErrInvalidRequest)
	}
	return &Client{source: source}, nil
}

// NewClient constructs a client with its own HTTPSource for an HTTP(S) origin.
// It validates the origin without connecting. Call CloseIdleConnections after
// requests finish to release idle connections owned by this convenience client.
func NewClient(simulatorURL string) (*Client, error) {
	source, err := NewHTTPSource(HTTPConfig{Origin: simulatorURL})
	if err != nil {
		return nil, err
	}
	client, err := New(source)
	if err != nil {
		return nil, err
	}
	client.owned = source
	return client, nil
}

// Origin returns the HTTP source's scheme://host[:port], or empty for other
// sources. A local source does not imply any browser navigation destination.
func (c *Client) Origin() string {
	if source, ok := c.source.(*HTTPSource); ok {
		return source.Origin()
	}
	return ""
}

// CloseIdleConnections releases only the source created by NewClient. Sources
// supplied to New remain caller-owned and are never closed by this method.
func (c *Client) CloseIdleConnections() {
	if c.owned != nil {
		c.owned.CloseIdleConnections()
	}
}

// Observations reads and validates one complete received-traffic snapshot. Nil
// stations selects all stations. Any semantic or AIS violation wraps
// simulatorapi.ErrInvalidResponse and returns no partial projection.
func (c *Client) Observations(ctx context.Context, stations []string) (Observations, error) {
	ctx, cancel := context.WithTimeout(ctx, upstreamTimeout)
	defer cancel()
	if err := sourceContext(ctx); err != nil {
		return Observations{}, err
	}
	if err := simulatorapi.ValidateSelection(stations); err != nil {
		return Observations{}, err
	}
	upstream, err := c.source.Observations(ctx, stations)
	if err != nil {
		return Observations{}, err
	}
	if err := sourceContext(ctx); err != nil {
		return Observations{}, err
	}
	result, err := projectObservations(stations, upstream)
	if err != nil {
		return Observations{}, fmt.Errorf("%w: observations: %w", simulatorapi.ErrInvalidResponse, err)
	}
	if err := sourceContext(ctx); err != nil {
		return Observations{}, err
	}
	return result, nil
}

// ReceptionHistory reads and validates a complete station history page. A stale
// simulation identity wraps simulatorapi.ErrConflict. Invalid responses wrap
// simulatorapi.ErrInvalidResponse. The request context bounds the entire read.
func (c *Client) ReceptionHistory(ctx context.Context, stationID string, req simulatorapi.HistoryRequest) (ReceptionPage, error) {
	ctx, cancel := context.WithTimeout(ctx, upstreamTimeout)
	defer cancel()
	if err := sourceContext(ctx); err != nil {
		return ReceptionPage{}, err
	}
	if err := req.Validate(); err != nil {
		return ReceptionPage{}, err
	}
	if err := simulatorapi.ValidateSelection([]string{stationID}); err != nil {
		return ReceptionPage{}, err
	}
	upstream, err := c.source.ReceptionHistory(ctx, stationID, req)
	if err != nil {
		return ReceptionPage{}, err
	}
	if err := sourceContext(ctx); err != nil {
		return ReceptionPage{}, err
	}
	result, err := projectReceptionPage(stationID, req, upstream)
	if err != nil {
		return ReceptionPage{}, fmt.Errorf("%w: receptions: %w", simulatorapi.ErrInvalidResponse, err)
	}
	if err := sourceContext(ctx); err != nil {
		return ReceptionPage{}, err
	}
	return result, nil
}

func sourceContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w: %w", simulatorapi.ErrUnavailable, err)
	}
	return nil
}
