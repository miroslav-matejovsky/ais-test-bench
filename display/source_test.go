package display_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-testbench/display"
	"github.com/miroslav-matejovsky/ais-testbench/simulatorapi"
)

type fixtureSource struct {
	observations func(context.Context, []string) (simulatorapi.Observations, error)
	history      func(context.Context, string, simulatorapi.HistoryRequest) (simulatorapi.ReceptionPage, error)
}

func (s fixtureSource) Observations(ctx context.Context, ids []string) (simulatorapi.Observations, error) {
	return s.observations(ctx, ids)
}

func (s fixtureSource) ReceptionHistory(ctx context.Context, id string, req simulatorapi.HistoryRequest) (simulatorapi.ReceptionPage, error) {
	return s.history(ctx, id, req)
}

func TestSourceProjectionRejectsMalformedSnapshots(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*simulatorapi.Observations)
	}{
		{"missing identity", func(o *simulatorapi.Observations) { o.SimulationID = "" }},
		{"bad clock", func(o *simulatorapi.Observations) { o.Time.Paused = true }},
		{"bad NMEA", func(o *simulatorapi.Observations) { o.Targets[0].Report.Sentence = "broken" }},
		{"wrong selection", func(o *simulatorapi.Observations) { o.Selection = []string{"unknown"} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newScenario(t)
			client, err := display.New(fixtureSource{observations: func(ctx context.Context, ids []string) (simulatorapi.Observations, error) {
				_, hasDeadline := ctx.Deadline()
				require.True(t, hasDeadline)
				o := fixture.observations(ids)
				test.change(&o)
				return o, nil
			}})
			require.NoError(t, err)
			got, err := client.Observations(t.Context(), nil)
			require.ErrorIs(t, err, simulatorapi.ErrInvalidResponse)
			require.Empty(t, got)
		})
	}
	fixture := newScenario(t)
	client, err := display.New(fixtureSource{history: func(_ context.Context, id string, _ simulatorapi.HistoryRequest) (simulatorapi.ReceptionPage, error) {
		page := fixture.page(id, nil, 100)
		page.Receptions[0].Sentence = "broken"
		return page, nil
	}})
	require.NoError(t, err)
	page, err := client.ReceptionHistory(t.Context(), "s1", simulatorapi.HistoryRequest{SimulationID: "run-1"})
	require.ErrorIs(t, err, simulatorapi.ErrInvalidResponse)
	require.Empty(t, page)
}

func TestSourceErrorsAndCancellation(t *testing.T) {
	_, err := display.New(nil)
	require.ErrorIs(t, err, simulatorapi.ErrInvalidRequest)
	cause := errors.New("source failed")
	for _, category := range []error{simulatorapi.ErrInvalidRequest, simulatorapi.ErrNotFound, simulatorapi.ErrConflict, simulatorapi.ErrUnavailable, simulatorapi.ErrInvalidResponse} {
		failure := fmt.Errorf("%w: %w", category, cause)
		client, err := display.New(fixtureSource{
			observations: func(context.Context, []string) (simulatorapi.Observations, error) {
				return simulatorapi.Observations{}, failure
			},
			history: func(context.Context, string, simulatorapi.HistoryRequest) (simulatorapi.ReceptionPage, error) {
				return simulatorapi.ReceptionPage{}, failure
			},
		})
		require.NoError(t, err)
		_, err = client.Observations(t.Context(), nil)
		require.ErrorIs(t, err, category)
		require.ErrorIs(t, err, cause)
		_, err = client.ReceptionHistory(t.Context(), "s1", simulatorapi.HistoryRequest{SimulationID: "run-1"})
		require.ErrorIs(t, err, category)
		require.ErrorIs(t, err, cause)
	}
	ctx, cancel := context.WithCancel(t.Context())
	client, err := display.New(fixtureSource{observations: func(context.Context, []string) (simulatorapi.Observations, error) {
		cancel()
		return simulatorapi.Observations{}, nil
	}})
	require.NoError(t, err)
	_, err = client.Observations(ctx, nil)
	require.ErrorIs(t, err, context.Canceled, "cancellation after a read takes precedence over invalid data")
	require.ErrorIs(t, err, simulatorapi.ErrUnavailable)
	// A canceled request is rejected before invoking a source method.
	client, err = display.New(fixtureSource{})
	require.NoError(t, err)
	_, err = client.Observations(ctx, nil)
	require.ErrorIs(t, err, context.Canceled)
	_, err = client.ReceptionHistory(ctx, "s1", simulatorapi.HistoryRequest{SimulationID: "run-1"})
	require.ErrorIs(t, err, context.Canceled)
}

type countingTransport struct {
	reads  atomic.Int32
	closes atomic.Int32
}

func (t *countingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	t.reads.Add(1)
	return nil, errors.New("test connection failure")
}

func (t *countingTransport) CloseIdleConnections() { t.closes.Add(1) }

func TestSourceConstructionAndBorrowedTransportOwnership(t *testing.T) {
	transport := &countingTransport{}
	httpClient := &http.Client{Transport: transport}
	source, err := display.NewHTTPSource(display.HTTPConfig{Origin: "http://example.test", Client: httpClient})
	require.NoError(t, err)
	client, err := display.New(source)
	require.NoError(t, err)
	require.Zero(t, transport.reads.Load(), "constructors must not connect")
	_, err = client.Observations(t.Context(), nil)
	require.ErrorIs(t, err, simulatorapi.ErrUnavailable)
	require.Equal(t, int32(1), transport.reads.Load())
	client.CloseIdleConnections()
	source.CloseIdleConnections()
	require.Zero(t, transport.closes.Load(), "borrowed transports must remain caller-owned")
	require.Same(t, transport, httpClient.Transport)
	require.Zero(t, httpClient.Timeout)
}

func TestHTTPSourceCancellationDuringRead(t *testing.T) {
	reading := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(reading)
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	source, err := display.NewHTTPSource(display.HTTPConfig{Origin: server.URL})
	require.NoError(t, err)
	t.Cleanup(source.CloseIdleConnections)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := source.Observations(ctx, nil); done <- err }()
	<-reading
	cancel()
	err = <-done
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, err, simulatorapi.ErrUnavailable)
}
