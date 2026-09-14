package simulation

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var internalConfig = Config{
	ID: "run-1", StartTime: time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC), Seed: 1, InitialVesselCount: 2, Speed: 0.33,
}

// TestFailedMutationKeepsState forces encoding failures through internal state,
// because valid public inputs always encode. A failed call must not change any
// future result, so the engine is compared with an untouched control engine.
func TestFailedMutationKeepsState(t *testing.T) {
	s, err := New(internalConfig)
	require.NoError(t, err)
	control, err := New(internalConfig)
	require.NoError(t, err)
	requireSame := func(msgAndArgs ...any) {
		t.Helper()
		require.Equal(t, control.state, s.state, msgAndArgs...)
		require.Equal(t, control.messages, s.messages, msgAndArgs...)
	}

	// Creation draws random values for the first added vessel, which encodes;
	// the second MMSI exceeds nine digits. The random source must not advance.
	s.state.nextMMSI = 999999999
	reports, err := s.SetCount(4)
	require.Error(t, err)
	require.Nil(t, reports)
	require.Equal(t, uint32(999999999), s.state.nextMMSI)
	s.state.nextMMSI = control.state.nextMMSI
	requireSame()
	want, err := control.SetCount(4)
	require.NoError(t, err)
	got, err := s.SetCount(4)
	require.NoError(t, err)
	require.Equal(t, want, got, "a retry creates the vessels of an untouched engine")
	requireSame()

	// A multi-tick elapse with a carried remainder fails on the last vessel of
	// its first tick. Clock, remainder, navigation, and sequence stay unchanged.
	for _, sim := range []*Simulator{s, control} {
		_, err := sim.Elapse(t.Context(), 3)
		require.NoError(t, err)
	}
	speed := s.state.vessels[3].Speed
	s.state.vessels[3].Speed = math.NaN()
	reports, err = s.Elapse(t.Context(), 10*time.Second)
	require.Error(t, err)
	require.Nil(t, reports)
	s.state.vessels[3].Speed = speed
	requireSame()
	want, err = control.Elapse(t.Context(), 10*time.Second)
	require.NoError(t, err)
	got, err = s.Elapse(t.Context(), 10*time.Second)
	require.NoError(t, err)
	require.Equal(t, want, got)
	requireSame()
}

func TestCapacityLimitsFailBeforeGeneration(t *testing.T) {
	s, err := New(internalConfig)
	require.NoError(t, err)

	// Two vessels leave room for one more tick and one more creation.
	s.state.sequence = math.MaxUint64 - 3
	_, err = s.Advance(t.Context(), 2*time.Second)
	require.ErrorIs(t, err, ErrLimit)
	reports, err := s.Advance(t.Context(), time.Second)
	require.NoError(t, err)
	require.Equal(t, uint64(math.MaxUint64-1), reports[1].Sequence)
	_, err = s.SetCount(4)
	require.ErrorIs(t, err, ErrLimit)
	reports, err = s.SetCount(3)
	require.NoError(t, err)
	require.Equal(t, uint64(math.MaxUint64), reports[0].Sequence)
	_, err = s.Advance(t.Context(), time.Second)
	require.ErrorIs(t, err, ErrLimit)

	s.state.elapsed = math.MaxInt64 - time.Second/2
	_, err = s.SetCount(0)
	require.NoError(t, err)
	_, err = s.Advance(t.Context(), time.Second)
	require.ErrorIs(t, err, ErrLimit)
}
