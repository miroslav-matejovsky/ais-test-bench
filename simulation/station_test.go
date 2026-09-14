package simulation_test

import (
	"context"
	"encoding/json"
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-test-bench/simulation"
)

func site(name string) simulation.StationDefinition {
	return simulation.StationDefinition{
		Name: name, Latitude: 52, Longitude: 4, Enabled: true,
		AntennaHeightMeters: 25, ReceiveGainDBi: 3, FeederLossDB: 2,
		ChannelA: simulation.ReceiverChannel{Enabled: true, SensitivityDBm: -110},
		ChannelB: simulation.ReceiverChannel{Enabled: true, SensitivityDBm: -110},
	}
}

func sectors(bounds ...[2]float64) []simulation.ShadowSector {
	result := make([]simulation.ShadowSector, 0, len(bounds))
	for _, b := range bounds {
		result = append(result, simulation.ShadowSector{StartDegrees: b[0], EndDegrees: b[1], LossDB: 10})
	}
	return result
}

func TestStationValidationRejectsInvalidDefinitions(t *testing.T) {
	nine := make([][2]float64, 0, 9)
	for i := range 9 {
		nine = append(nine, [2]float64{float64(i * 10), float64(i*10 + 10)})
	}
	for name, change := range map[string]func(*simulation.StationDefinition){
		"empty name":                 func(d *simulation.StationDefinition) { d.Name = "" },
		"blank name":                 func(d *simulation.StationDefinition) { d.Name = " \t" },
		"invalid UTF-8 name":         func(d *simulation.StationDefinition) { d.Name = "\xff" },
		"81 character name":          func(d *simulation.StationDefinition) { d.Name = strings.Repeat("é", 81) },
		"latitude above 90":          func(d *simulation.StationDefinition) { d.Latitude = math.Nextafter(90, 91) },
		"latitude below -90":         func(d *simulation.StationDefinition) { d.Latitude = -91 },
		"latitude not a number":      func(d *simulation.StationDefinition) { d.Latitude = math.NaN() },
		"longitude above 180":        func(d *simulation.StationDefinition) { d.Longitude = math.Nextafter(180, 181) },
		"longitude below -180":       func(d *simulation.StationDefinition) { d.Longitude = math.Nextafter(-180, -181) },
		"longitude infinity":         func(d *simulation.StationDefinition) { d.Longitude = math.Inf(1) },
		"zero height":                func(d *simulation.StationDefinition) { d.AntennaHeightMeters = 0 },
		"negative height":            func(d *simulation.StationDefinition) { d.AntennaHeightMeters = -1 },
		"height above 500":           func(d *simulation.StationDefinition) { d.AntennaHeightMeters = math.Nextafter(500, 501) },
		"height infinity":            func(d *simulation.StationDefinition) { d.AntennaHeightMeters = math.Inf(1) },
		"gain below -10":             func(d *simulation.StationDefinition) { d.ReceiveGainDBi = -10.01 },
		"gain above 20":              func(d *simulation.StationDefinition) { d.ReceiveGainDBi = 20.01 },
		"negative feeder loss":       func(d *simulation.StationDefinition) { d.FeederLossDB = -0.01 },
		"feeder loss above 30":       func(d *simulation.StationDefinition) { d.FeederLossDB = 30.01 },
		"absent channel A":           func(d *simulation.StationDefinition) { d.ChannelA = simulation.ReceiverChannel{} },
		"absent channel B":           func(d *simulation.StationDefinition) { d.ChannelB = simulation.ReceiverChannel{} },
		"sensitivity below -125":     func(d *simulation.StationDefinition) { d.ChannelA.SensitivityDBm = -125.01 },
		"sensitivity above -80":      func(d *simulation.StationDefinition) { d.ChannelB.SensitivityDBm = -79.99 },
		"negative noise penalty":     func(d *simulation.StationDefinition) { d.ChannelA.NoisePenaltyDB = -0.1 },
		"noise penalty above 40":     func(d *simulation.StationDefinition) { d.ChannelB.NoisePenaltyDB = 40.1 },
		"negative drop probability":  func(d *simulation.StationDefinition) { d.ChannelA.DropProbability = -0.1 },
		"drop probability above 1":   func(d *simulation.StationDefinition) { d.ChannelB.DropProbability = 1.1 },
		"drop probability NaN":       func(d *simulation.StationDefinition) { d.ChannelB.DropProbability = math.NaN() },
		"disabled channel invalid":   func(d *simulation.StationDefinition) { d.ChannelA = simulation.ReceiverChannel{SensitivityDBm: -130} },
		"nine sectors":               func(d *simulation.StationDefinition) { d.ShadowSectors = sectors(nine...) },
		"sector start 360":           func(d *simulation.StationDefinition) { d.ShadowSectors = sectors([2]float64{360, 10}) },
		"sector end 360":             func(d *simulation.StationDefinition) { d.ShadowSectors = sectors([2]float64{10, 360}) },
		"negative sector start":      func(d *simulation.StationDefinition) { d.ShadowSectors = sectors([2]float64{-1, 10}) },
		"sector start NaN":           func(d *simulation.StationDefinition) { d.ShadowSectors = sectors([2]float64{math.NaN(), 10}) },
		"equal sector start and end": func(d *simulation.StationDefinition) { d.ShadowSectors = sectors([2]float64{10, 10}) },
		"sector loss above 60": func(d *simulation.StationDefinition) {
			d.ShadowSectors = []simulation.ShadowSector{{StartDegrees: 10, EndDegrees: 20, LossDB: 60.01}}
		},
		"negative sector loss": func(d *simulation.StationDefinition) {
			d.ShadowSectors = []simulation.ShadowSector{{StartDegrees: 10, EndDegrees: 20, LossDB: -1}}
		},
		"overlapping sectors": func(d *simulation.StationDefinition) {
			d.ShadowSectors = sectors([2]float64{10, 50}, [2]float64{40, 60})
		},
		"duplicate sectors": func(d *simulation.StationDefinition) {
			d.ShadowSectors = sectors([2]float64{10, 20}, [2]float64{10, 20})
		},
		"nested sector": func(d *simulation.StationDefinition) {
			d.ShadowSectors = sectors([2]float64{10, 50}, [2]float64{20, 30})
		},
		"sector overlaps wrap": func(d *simulation.StationDefinition) {
			d.ShadowSectors = sectors([2]float64{350, 10}, [2]float64{5, 20})
		},
		"sector inside wrap": func(d *simulation.StationDefinition) {
			d.ShadowSectors = sectors([2]float64{300, 100}, [2]float64{0, 10})
		},
		"two sectors wrap north": func(d *simulation.StationDefinition) {
			d.ShadowSectors = sectors([2]float64{350, 10}, [2]float64{355, 5})
		},
		"wrap overlaps start side": func(d *simulation.StationDefinition) {
			d.ShadowSectors = sectors([2]float64{300, 10}, [2]float64{290, 301})
		},
	} {
		t.Run(name, func(t *testing.T) {
			definition := site("Site")
			change(&definition)
			require.ErrorIs(t, simulation.ValidateStation(definition), simulation.ErrInvalid)

			config := testConfig(1, 1)
			config.Stations = []simulation.StationDefinition{site("Valid"), definition}
			s, err := simulation.New(config)
			require.ErrorIs(t, err, simulation.ErrInvalid)
			require.Nil(t, s)

			s = newSimulator(t, testConfig(1, 1))
			before := observe(s)
			_, _, err = s.AddStation(1, definition)
			require.ErrorIs(t, err, simulation.ErrInvalid)
			_, err = s.UpdateStation(1, "station-1", definition)
			require.ErrorIs(t, err, simulation.ErrInvalid)
			require.Equal(t, before, observe(s))
		})
	}
}

func TestStationValidationAcceptsBoundaries(t *testing.T) {
	adjacent := make([][2]float64, 0, simulation.MaxShadowSectors)
	for i := range simulation.MaxShadowSectors {
		adjacent = append(adjacent, [2]float64{float64(i * 45), float64((i + 1) * 45 % 360)})
	}
	for name, change := range map[string]func(*simulation.StationDefinition){
		"north pole":      func(d *simulation.StationDefinition) { d.Latitude = 90 },
		"south pole":      func(d *simulation.StationDefinition) { d.Latitude = -90 },
		"longitude -180":  func(d *simulation.StationDefinition) { d.Longitude = -180 },
		"longitude 180":   func(d *simulation.StationDefinition) { d.Longitude = 180 },
		"smallest height": func(d *simulation.StationDefinition) { d.AntennaHeightMeters = math.SmallestNonzeroFloat64 },
		"height 500":      func(d *simulation.StationDefinition) { d.AntennaHeightMeters = 500 },
		"gain -10":        func(d *simulation.StationDefinition) { d.ReceiveGainDBi = -10 },
		"gain 20":         func(d *simulation.StationDefinition) { d.ReceiveGainDBi = 20 },
		"feeder loss 30":  func(d *simulation.StationDefinition) { d.FeederLossDB = 30 },
		"channel limits": func(d *simulation.StationDefinition) {
			d.ChannelA = simulation.ReceiverChannel{SensitivityDBm: -125, NoisePenaltyDB: 40, DropProbability: 1}
		},
		"channel other limits":  func(d *simulation.StationDefinition) { d.ChannelB = simulation.ReceiverChannel{SensitivityDBm: -80} },
		"station disabled":      func(d *simulation.StationDefinition) { d.Enabled = false },
		"all channels disabled": func(d *simulation.StationDefinition) { d.ChannelA.Enabled, d.ChannelB.Enabled = false, false },
		"80 four-byte runes": func(d *simulation.StationDefinition) {
			d.Name = strings.Repeat("\U0001F6A2", simulation.MaxStationNameRunes)
		},
		"eight adjacent sectors": func(d *simulation.StationDefinition) { d.ShadowSectors = sectors(adjacent...) },
		"north wrap":             func(d *simulation.StationDefinition) { d.ShadowSectors = sectors([2]float64{350, 10}) },
		"adjacent to north wrap": func(d *simulation.StationDefinition) {
			d.ShadowSectors = sectors([2]float64{350, 10}, [2]float64{10, 20}, [2]float64{340, 350})
		},
		"sector limits": func(d *simulation.StationDefinition) {
			d.ShadowSectors = []simulation.ShadowSector{
				{StartDegrees: 0, EndDegrees: math.Nextafter(360, 0), LossDB: 60},
			}
		},
		"zero loss sector": func(d *simulation.StationDefinition) {
			d.ShadowSectors = []simulation.ShadowSector{{StartDegrees: 90, EndDegrees: 180, LossDB: 0}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			definition := site("Site")
			change(&definition)
			require.NoError(t, simulation.ValidateStation(definition))
			s := newSimulator(t, testConfig(1, 1))
			_, _, err := s.AddStation(1, definition)
			require.NoError(t, err)
		})
	}
}

func TestNewRejectsInvalidTransmitterAndStationCount(t *testing.T) {
	for name, change := range map[string]func(*simulation.Config){
		"zero transmitter":     func(c *simulation.Config) { c.Transmitter = simulation.TransmitterProfile{} },
		"zero power":           func(c *simulation.Config) { c.Transmitter.PowerWatts = 0 },
		"power above 25":       func(c *simulation.Config) { c.Transmitter.PowerWatts = 25.01 },
		"power NaN":            func(c *simulation.Config) { c.Transmitter.PowerWatts = math.NaN() },
		"zero height":          func(c *simulation.Config) { c.Transmitter.HeightMeters = 0 },
		"height above 100":     func(c *simulation.Config) { c.Transmitter.HeightMeters = 100.01 },
		"gain below -10":       func(c *simulation.Config) { c.Transmitter.GainDBi = -10.01 },
		"gain infinity":        func(c *simulation.Config) { c.Transmitter.GainDBi = math.Inf(-1) },
		"negative feeder loss": func(c *simulation.Config) { c.Transmitter.FeederLossDB = -0.01 },
		"feeder loss above 30": func(c *simulation.Config) { c.Transmitter.FeederLossDB = 30.01 },
		"stations over MaxStations": func(c *simulation.Config) {
			c.Stations = slices.Repeat([]simulation.StationDefinition{site("Site")}, 17)
		},
	} {
		t.Run(name, func(t *testing.T) {
			config := testConfig(1, 1)
			change(&config)
			s, err := simulation.New(config)
			require.ErrorIs(t, err, simulation.ErrInvalid)
			require.Nil(t, s)
		})
	}

	config := testConfig(1, 1)
	config.Transmitter = simulation.TransmitterProfile{PowerWatts: 25, HeightMeters: 100, GainDBi: 20, FeederLossDB: 30}
	_, err := simulation.New(config)
	require.NoError(t, err)
	config.Transmitter = simulation.TransmitterProfile{PowerWatts: math.SmallestNonzeroFloat64, HeightMeters: math.SmallestNonzeroFloat64, GainDBi: -10}
	_, err = simulation.New(config)
	require.NoError(t, err)
}

func TestNewAssignsStationIdentity(t *testing.T) {
	for _, stations := range [][]simulation.StationDefinition{nil, {}} {
		config := testConfig(1, 1)
		config.Stations = stations
		s := newSimulator(t, config)
		require.Equal(t, simulation.StationSet{SimulationID: runID, Revision: 1, Stations: []simulation.Station{}}, s.Stations())
	}

	config := testConfig(1, 1)
	for i := range simulation.MaxStations {
		config.Stations = append(config.Stations, site("Site "+strconv.Itoa(i)))
	}
	config.Stations[0].Name = config.Stations[1].Name // Duplicate names are allowed.
	s := newSimulator(t, config)
	stations := s.Stations()
	require.Len(t, stations.Stations, simulation.MaxStations)
	for i, station := range stations.Stations {
		require.Equal(t, "station-"+strconv.Itoa(i+1), station.ID)
		require.Equal(t, config.Stations[i], station.Definition)
		require.Equal(t, uint64(1), station.ConfigRevision)
		require.Equal(t, uint64(1), station.RFRevision)
		require.Equal(t, start, station.CreatedAt)
	}
	_, _, err := s.AddStation(1, site("Extra"))
	require.ErrorIs(t, err, simulation.ErrLimit)
	require.Equal(t, stations, s.Stations())
}

func TestStationCanonicalForm(t *testing.T) {
	definition := site("Site")
	definition.Longitude = 180
	definition.ShadowSectors = sectors([2]float64{300, 310}, [2]float64{350, 10}, [2]float64{10, 20})
	s := newSimulator(t, testConfig(1, 1))

	id, stations, err := s.AddStation(1, definition)
	require.NoError(t, err)
	stored := stations.Stations[0].Definition
	stored.ShadowSectors = slices.Clone(stored.ShadowSectors)
	require.InDelta(t, -180, stored.Longitude, 0)
	require.Equal(t, sectors([2]float64{10, 20}, [2]float64{300, 310}, [2]float64{350, 10}), stored.ShadowSectors)

	// Caller and result slices are detached from engine state.
	definition.ShadowSectors[0].LossDB = 99
	stations.Stations[0].Definition.ShadowSectors[0].LossDB = 99
	stations.Stations[0].Definition.Name = "changed"
	require.Equal(t, stored, s.Stations().Stations[0].Definition)

	// Equivalent input is a no-op.
	definition.ShadowSectors = sectors([2]float64{350, 10}, [2]float64{10, 20}, [2]float64{300, 310})
	before := s.Stations()
	after, err := s.UpdateStation(2, id, definition)
	require.NoError(t, err)
	require.Equal(t, before, after)
	definition.Longitude = -180
	after, err = s.UpdateStation(2, id, definition)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestStationRevisions(t *testing.T) {
	for name, tt := range map[string]struct {
		change func(*simulation.StationDefinition)
		rf     bool
	}{
		"name":             {change: func(d *simulation.StationDefinition) { d.Name = "Renamed" }},
		"latitude":         {change: func(d *simulation.StationDefinition) { d.Latitude = 51 }, rf: true},
		"longitude":        {change: func(d *simulation.StationDefinition) { d.Longitude = 5 }, rf: true},
		"enabled":          {change: func(d *simulation.StationDefinition) { d.Enabled = false }, rf: true},
		"antenna height":   {change: func(d *simulation.StationDefinition) { d.AntennaHeightMeters = 30 }, rf: true},
		"receive gain":     {change: func(d *simulation.StationDefinition) { d.ReceiveGainDBi = 4 }, rf: true},
		"feeder loss":      {change: func(d *simulation.StationDefinition) { d.FeederLossDB = 1 }, rf: true},
		"channel A":        {change: func(d *simulation.StationDefinition) { d.ChannelA.Enabled = false }, rf: true},
		"channel B":        {change: func(d *simulation.StationDefinition) { d.ChannelB.SensitivityDBm = -100 }, rf: true},
		"drop probability": {change: func(d *simulation.StationDefinition) { d.ChannelB.DropProbability = 0.5 }, rf: true},
		"noise penalty":    {change: func(d *simulation.StationDefinition) { d.ChannelA.NoisePenaltyDB = 3 }, rf: true},
		"shadow sector":    {change: func(d *simulation.StationDefinition) { d.ShadowSectors = sectors([2]float64{0, 90}) }, rf: true},
		"name and RF": {change: func(d *simulation.StationDefinition) {
			d.Name, d.Latitude = "Renamed", 51
		}, rf: true},
	} {
		t.Run(name, func(t *testing.T) {
			config := testConfig(1, 1)
			config.Stations = []simulation.StationDefinition{site("Other"), site("Site")}
			s := newSimulator(t, config)
			definition := site("Site")
			tt.change(&definition)

			stations, err := s.UpdateStation(1, "station-2", definition)
			require.NoError(t, err)
			wantRF := uint64(1)
			if tt.rf {
				wantRF = 2
			}
			require.Equal(t, simulation.StationSet{SimulationID: runID, Revision: 2, Stations: []simulation.Station{
				{ID: "station-1", Definition: site("Other"), ConfigRevision: 1, RFRevision: 1, CreatedAt: start},
				{ID: "station-2", Definition: definition, ConfigRevision: 2, RFRevision: wantRF, CreatedAt: start},
			}}, stations)

			// Repeating the edit is a no-op, including for the set revision.
			again, err := s.UpdateStation(2, "station-2", definition)
			require.NoError(t, err)
			require.Equal(t, stations, again)
		})
	}
}

func TestStationLifecycle(t *testing.T) {
	config := testConfig(1, 1)
	config.Stations = []simulation.StationDefinition{site("First"), site("Second")}
	s := newSimulator(t, config)
	runSteps(t, s, advance(1500*time.Millisecond))

	id, stations, err := s.AddStation(1, site("Third"))
	require.NoError(t, err)
	require.Equal(t, "station-3", id)
	require.Equal(t, uint64(2), stations.Revision)
	require.Equal(t, start.Add(1500*time.Millisecond), stations.Stations[2].CreatedAt, "created at the current virtual instant")

	stations, err = s.RemoveStation(2, id)
	require.NoError(t, err)
	require.Equal(t, uint64(3), stations.Revision)
	require.Len(t, stations.Stations, 2)
	id, _, err = s.AddStation(3, site("Third"))
	require.NoError(t, err)
	require.Equal(t, "station-4", id, "IDs are never reused")

	before := observe(s)
	_, err = s.RemoveStation(3, "station-1")
	require.ErrorIs(t, err, simulation.ErrConflict)
	_, _, err = s.AddStation(5, site("Stale"))
	require.ErrorIs(t, err, simulation.ErrConflict)
	_, err = s.UpdateStation(3, "station-1", site("Stale"))
	require.ErrorIs(t, err, simulation.ErrConflict)
	_, err = s.RemoveStation(4, "station-3")
	require.ErrorIs(t, err, simulation.ErrNotFound)
	_, err = s.UpdateStation(4, "station-3", site("Missing"))
	require.ErrorIs(t, err, simulation.ErrNotFound)
	require.Equal(t, before, observe(s))

	// Editing keeps ID and creation time; the station keeps its list position.
	stations, err = s.UpdateStation(4, "station-1", site("Renamed"))
	require.NoError(t, err)
	require.Equal(t, []string{"station-1", "station-2", "station-4"}, stationIDs(stations))
	require.Equal(t, start, stations.Stations[0].CreatedAt)

	for _, id := range []string{"station-2", "station-1", "station-4"} {
		stations, err = s.RemoveStation(stations.Revision, id)
		require.NoError(t, err)
	}
	require.NotNil(t, stations.Stations)
	require.Empty(t, stations.Stations)
	require.Len(t, runSteps(t, s, advance(time.Second)), 1, "vessel generation continues without stations")
}

func stationIDs(stations simulation.StationSet) []string {
	ids := make([]string, 0, len(stations.Stations))
	for _, station := range stations.Stations {
		ids = append(ids, station.ID)
	}
	return ids
}

// TestStationsDoNotChangeVesselReports checks that station configuration,
// including rejected edits, draws no vessel randomness.
func TestStationsDoNotChangeVesselReports(t *testing.T) {
	steps := []step{advance(2 * time.Second), setCount(4), advance(3 * time.Second)}
	reference := newSimulator(t, testConfig(11, 2))
	want := runSteps(t, reference, steps...)

	sites := []simulation.StationDefinition{site("A"), site("B"), site("C")}
	sites[1].Latitude = 53
	for name, stations := range map[string][]simulation.StationDefinition{
		"three stations": sites,
		"reordered":      {sites[2], sites[0], sites[1]},
	} {
		t.Run(name, func(t *testing.T) {
			config := testConfig(11, 2)
			config.Stations = stations
			s := newSimulator(t, config)
			require.Equal(t, want, runSteps(t, s, steps...))
			require.Equal(t, reference.History(), s.History())
		})
	}

	s := newSimulator(t, testConfig(11, 2))
	edit := func(_ context.Context, s *simulation.Simulator) ([]simulation.Message, error) {
		stations := s.Stations()
		if _, _, err := s.AddStation(stations.Revision, site("")); err == nil {
			t.Error("blank name accepted")
		}
		id, stations, err := s.AddStation(stations.Revision, site("Added"))
		if err != nil {
			return nil, err
		}
		if _, err := s.UpdateStation(stations.Revision, id, site("Renamed")); err != nil {
			return nil, err
		}
		return []simulation.Message{}, nil
	}
	got := runSteps(t, s, steps[0], edit, steps[1], edit, steps[2])
	require.Equal(t, want, got)
	require.Equal(t, reference.Fleet(), s.Fleet())
	require.Equal(t, reference.History(), s.History())
}

// TestMaximalStationFitsEncodedLimit shows that the structured limits keep one
// station configuration below 4 KiB even with worst-case JSON escaping.
func TestMaximalStationFitsEncodedLimit(t *testing.T) {
	long := -0.12345678901234568
	channel := simulation.ReceiverChannel{Enabled: true, SensitivityDBm: -124.12345678901234, NoisePenaltyDB: 39.123456789012345, DropProbability: long + 1}
	definition := simulation.StationDefinition{
		Name:     strings.Repeat("<", simulation.MaxStationNameRunes),
		Latitude: -89.12345678901234, Longitude: -179.12345678901234, Enabled: true,
		AntennaHeightMeters: 499.12345678901234, ReceiveGainDBi: -9.123456789012345, FeederLossDB: 29.123456789012345,
		ChannelA: channel, ChannelB: channel,
	}
	for i := range simulation.MaxShadowSectors {
		definition.ShadowSectors = append(definition.ShadowSectors, simulation.ShadowSector{
			StartDegrees: float64(i*45) + 0.12345678901234, EndDegrees: float64(i*45) + 44.12345678901234, LossDB: 59.123456789012345,
		})
	}
	require.NoError(t, simulation.ValidateStation(definition))
	s := newSimulator(t, testConfig(1, 0))
	_, stations, err := s.AddStation(1, definition)
	require.NoError(t, err)
	encoded, err := json.Marshal(stations.Stations[0])
	require.NoError(t, err)
	require.LessOrEqual(t, len(encoded), 4096)
}
