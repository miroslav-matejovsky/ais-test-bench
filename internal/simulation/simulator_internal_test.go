package simulation

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestFailedMutationKeepsState forces encoding failures through internal state,
// because valid public inputs always encode.
func TestFailedMutationKeepsState(t *testing.T) {
	start := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	s, err := New("run-1", start, 1)
	require.NoError(t, err)
	require.NoError(t, s.SetCount(2, start))

	// The first added vessel encodes; the second MMSI exceeds nine digits.
	// Fleet reports encode positions, so fleet equality covers navigation state.
	fleet, history := s.Fleet(), s.History()
	s.nextMMSI = 999999999
	require.Error(t, s.SetCount(4, start.Add(time.Second)))
	require.Equal(t, fleet, s.Fleet())
	require.Equal(t, history, s.History())
	require.Equal(t, uint32(999999999), s.nextMMSI)

	// The first vessel moves and encodes; the second cannot encode.
	speed := s.vessels[1].Speed
	s.vessels[1].Speed = math.NaN()
	require.Error(t, s.Advance(start.Add(time.Second)))
	require.Equal(t, fleet, s.Fleet())
	require.Equal(t, history, s.History())

	// No sequence was consumed by the failed mutations.
	s.vessels[1].Speed = speed
	require.NoError(t, s.Advance(start.Add(time.Second)))
	latest := s.History().LatestSequence
	require.NotNil(t, latest)
	require.Equal(t, uint64(4), *latest)
}
