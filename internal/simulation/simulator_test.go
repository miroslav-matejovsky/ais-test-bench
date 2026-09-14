package simulation_test

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/simulation"
	"github.com/stretchr/testify/require"
)

func TestFleetLifecycle(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	s, err := simulation.New(now, 42)
	require.NoError(t, err)
	initial := s.Snapshot()
	require.Len(t, initial.Vessels, 1)
	require.Equal(t, 1, initial.MessageCount)
	vessel := initial.Vessels[0]
	require.NoError(t, s.SetCount(3, now))
	fleet := s.Snapshot().Vessels
	require.Len(t, fleet, 3)
	require.Equal(t, vessel, fleet[0])
	require.NotEqual(t, fleet[0].MMSI, fleet[1].MMSI)
	require.NotEqual(t, fleet[1].MMSI, fleet[2].MMSI)
	require.NoError(t, s.Advance(now.Add(time.Minute)))
	moved := s.Snapshot().Vessels[0]
	require.NotEqual(t, vessel.Latitude, moved.Latitude)
	// The great-circle distance must agree with one minute at reported speed.
	rad := math.Pi / 180
	dlat := (moved.Latitude - vessel.Latitude) * rad
	dlon := (moved.Longitude - vessel.Longitude) * rad
	a := math.Pow(math.Sin(dlat/2), 2) + math.Cos(vessel.Latitude*rad)*math.Cos(moved.Latitude*rad)*math.Pow(math.Sin(dlon/2), 2)
	meters := 6371000 * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	require.InDelta(t, vessel.Speed*1852/60, meters, 0.001)
	require.Equal(t, now.Add(time.Minute), moved.UpdatedAt)
	require.Equal(t, 6, s.Snapshot().MessageCount)
	require.NoError(t, s.Advance(now))
	require.Equal(t, 6, s.Snapshot().MessageCount)
	require.NoError(t, s.SetCount(0, now.Add(time.Minute)))
	require.Empty(t, s.Snapshot().Vessels)
	require.NoError(t, s.Advance(now.Add(2*time.Minute)))
	require.Len(t, s.Messages(), 6)
	require.NoError(t, s.SetCount(1, now.Add(2*time.Minute)))
	require.NotEqual(t, vessel.MMSI, s.Snapshot().Vessels[0].MMSI)
	before := s.Snapshot()
	for _, count := range []int{-1, simulation.MaxVessels + 1} {
		require.Error(t, s.SetCount(count, now))
		require.Equal(t, before, s.Snapshot())
	}
}

func TestHistoryRetentionAndSnapshotIsolation(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	s, err := simulation.New(now, 1)
	require.NoError(t, err)
	for i := 1; i <= simulation.MessageLimit+5; i++ {
		require.NoError(t, s.Advance(now.Add(time.Duration(i)*time.Second)))
	}
	messages := s.Messages()
	require.Len(t, messages, simulation.MessageLimit)
	require.Equal(t, now.Add(6*time.Second), messages[0].Timestamp)
	require.Equal(t, now.Add(1005*time.Second), messages[len(messages)-1].Timestamp)
	messages[0].Sentence = "changed"
	require.NotEqual(t, messages[0], s.Messages()[0])
	snapshot := s.Snapshot()
	snapshot.Vessels[0].Name = "changed"
	require.NotEqual(t, snapshot.Vessels[0], s.Snapshot().Vessels[0])
}

func TestSeedReproducesFleet(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	a, err := simulation.New(now, 42)
	require.NoError(t, err)
	b, err := simulation.New(now, 42)
	require.NoError(t, err)
	require.Equal(t, a.Snapshot(), b.Snapshot())
	require.Equal(t, a.Messages(), b.Messages())
}

func TestConcurrentAccessAndCancellation(t *testing.T) {
	now := time.Now()
	s, err := simulation.New(now, 1)
	require.NoError(t, err)
	var group sync.WaitGroup
	for range 4 {
		group.Go(func() {
			for i := range 25 {
				if err := s.SetCount(i, now); err != nil {
					t.Error(err)
				}
				if err := s.Advance(now.Add(time.Duration(i) * time.Second)); err != nil {
					t.Error(err)
				}
				s.Snapshot()
				s.Messages()
			}
		})
	}
	group.Wait()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.NoError(t, s.Run(ctx))
}
