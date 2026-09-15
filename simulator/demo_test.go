package simulator_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-testbench/simulation"
	"github.com/miroslav-matejovsky/ais-testbench/simulator"
)

func TestDemoConfigStartsFreshRealTimeRun(t *testing.T) {
	before := time.Now()
	a, b := simulator.DemoConfig(), simulator.DemoConfig()

	require.NotEmpty(t, a.ID)
	require.NotEqual(t, a.ID, b.ID)
	require.False(t, a.StartTime.Before(before))
	require.Equal(t, 1, a.InitialVesselCount)
	require.InDelta(t, 1.0, a.Speed, 0)
	require.Equal(t, simulation.TransmitterProfile{PowerWatts: 12.5, HeightMeters: 10, GainDBi: 2, FeederLossDB: 1}, a.Transmitter)
	a.Stations[0].Name = "changed"
	require.Equal(t, "Rotterdam coast", b.Stations[0].Name, "every call returns an independent value")
	sim, err := simulation.New(b)
	require.NoError(t, err)

	stations := sim.Stations().Stations
	require.Len(t, stations, 3)
	for i, want := range []struct {
		name        string
		lat, lon    float64
		height      float64
		sensitivity float64
		sectors     int
	}{
		{name: "Rotterdam coast", lat: 51.98, lon: 4.05, height: 25, sensitivity: -110},
		{name: "Northern coast", lat: 52.12, lon: 4.24, height: 40, sensitivity: -112},
		{name: "Harbour receiver", lat: 51.95, lon: 4.14, height: 15, sensitivity: -108, sectors: 1},
	} {
		got := stations[i].Definition
		require.Equal(t, want.name, got.Name)
		require.InDelta(t, want.lat, got.Latitude, 0)
		require.InDelta(t, want.lon, got.Longitude, 0)
		require.InDelta(t, want.height, got.AntennaHeightMeters, 0)
		require.True(t, got.Enabled)
		for _, channel := range []simulation.ReceiverChannel{got.ChannelA, got.ChannelB} {
			require.Equal(t, simulation.ReceiverChannel{Enabled: true, SensitivityDBm: want.sensitivity}, channel)
		}
		require.Len(t, got.ShadowSectors, want.sectors)
	}
	require.Equal(t, []simulation.ShadowSector{{StartDegrees: 270, EndDegrees: 330, LossDB: 15}}, stations[2].Definition.ShadowSectors)
}
