package simulation_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-testbench/simulation"
)

func TestStationCoverage(t *testing.T) {
	harbour := site("Harbour")
	harbour.ChannelB.Enabled = false
	harbour.ShadowSectors = []simulation.ShadowSector{{StartDegrees: 270, EndDegrees: 330, LossDB: 30}}
	config := testConfig(1, 0)
	config.Stations = []simulation.StationDefinition{harbour}
	s := newSimulator(t, config)

	coverage := s.Stations().Stations[0].Coverage
	require.Len(t, coverage, 4)
	for i, want := range []struct {
		channel   simulation.Channel
		threshold float64
		empty     bool
	}{
		{channel: simulation.ChannelA, threshold: 0.9},
		{channel: simulation.ChannelA, threshold: 0.5},
		{channel: simulation.ChannelB, threshold: 0.9, empty: true},
		{channel: simulation.ChannelB, threshold: 0.5, empty: true},
	} {
		c := coverage[i]
		require.Equal(t, want.channel, c.Channel)
		require.InDelta(t, want.threshold, c.Threshold, 0)
		require.NotNil(t, c.Ring)
		if want.empty {
			require.Equal(t, simulation.Coverage{Channel: want.channel, Threshold: want.threshold, Ring: []simulation.GeoPoint{}}, c)
			continue
		}
		// 72 five-degree bearings, the two points just outside 270 and inside 330,
		// and the closing point.
		require.Len(t, c.Ring, 75)
		require.Equal(t, c.Ring[0], c.Ring[len(c.Ring)-1])
		require.Less(t, c.MinRadiusMeters, c.MaxRadiusMeters/2, "the shadow sector shortens coverage")
	}
	require.Greater(t, coverage[1].MaxRadiusMeters, coverage[0].MaxRadiusMeters)

	coverage[0].Ring[0].Latitude = 99
	require.NotEqual(t, coverage, s.Stations().Stations[0].Coverage, "coverage is detached")

	// A name-only edit keeps coverage; an RF edit replaces it.
	renamed := harbour
	renamed.Name = "Renamed"
	stations, err := s.UpdateStation(1, "station-1", renamed)
	require.NoError(t, err)
	require.Equal(t, s.Stations().Stations[0].Coverage, stations.Stations[0].Coverage)
	before := stations.Stations[0].Coverage
	renamed.ChannelB.Enabled = true
	stations, err = s.UpdateStation(2, "station-1", renamed)
	require.NoError(t, err)
	require.Equal(t, before[:2], stations.Stations[0].Coverage[:2])
	require.NotEmpty(t, stations.Stations[0].Coverage[3].Ring)
}
