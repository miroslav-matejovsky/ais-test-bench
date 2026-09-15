package simulator

import (
	"crypto/rand"
	mathrand "math/rand/v2"
	"time"

	"github.com/miroslav-matejovsky/ais-testbench/simulation"
)

// DemoConfig returns the demonstration engine configuration of the commands: a
// fresh random identity and seed, so runs are distinguishable, the current real
// instant as the virtual start, one vessel, real-time speed, the reference
// transmitter, and three synthetic receiving stations. Every call returns a new
// value that callers may change before New. The engine gives no field a default;
// DemoConfig is the only place these values are chosen.
func DemoConfig() simulation.Config {
	return simulation.Config{
		ID: rand.Text(), StartTime: time.Now(), Seed: mathrand.Uint64(),
		InitialVesselCount: 1, Speed: 1,
		Transmitter: simulation.TransmitterProfile{PowerWatts: 12.5, HeightMeters: 10, GainDBi: 2, FeederLossDB: 1},
		Stations:    demoStations(),
	}
}

// demoStations returns three explicitly synthetic receiving sites around the
// Rotterdam scenario. They are chosen scenario values, not real installations
// or field measurements. All channels start enabled without impairment.
func demoStations() []simulation.StationDefinition {
	channel := func(sensitivity float64) simulation.ReceiverChannel {
		return simulation.ReceiverChannel{Enabled: true, SensitivityDBm: sensitivity}
	}
	return []simulation.StationDefinition{
		{
			Name: "Rotterdam coast", Latitude: 51.98, Longitude: 4.05, Enabled: true,
			AntennaHeightMeters: 25, ReceiveGainDBi: 3, FeederLossDB: 2,
			ChannelA: channel(-110), ChannelB: channel(-110),
		},
		{
			Name: "Northern coast", Latitude: 52.12, Longitude: 4.24, Enabled: true,
			AntennaHeightMeters: 40, ReceiveGainDBi: 3, FeederLossDB: 2,
			ChannelA: channel(-112), ChannelB: channel(-112),
		},
		{
			Name: "Harbour receiver", Latitude: 51.95, Longitude: 4.14, Enabled: true,
			AntennaHeightMeters: 15, ReceiveGainDBi: 2, FeederLossDB: 3,
			ChannelA: channel(-108), ChannelB: channel(-108),
			ShadowSectors: []simulation.ShadowSector{{StartDegrees: 270, EndDegrees: 330, LossDB: 15}},
		},
	}
}
