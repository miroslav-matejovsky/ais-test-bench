package simulation

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

var referenceTransmitter = TransmitterProfile{PowerWatts: 12.5, HeightMeters: 10, GainDBi: 2, FeederLossDB: 1}

// rotterdamCoast is the application's first demonstration station.
func rotterdamCoast() StationDefinition {
	channel := ReceiverChannel{Enabled: true, SensitivityDBm: -110}
	return StationDefinition{
		Name: "Rotterdam coast", Latitude: 51.98, Longitude: 4.05, Enabled: true,
		AntennaHeightMeters: 25, ReceiveGainDBi: 3, FeederLossDB: 2, ChannelA: channel, ChannelB: channel,
	}
}

func bearing(degrees float64) *float64 { return new(degrees) }

func TestLinkWorkedExample(t *testing.T) {
	station := rotterdamCoast()

	near := linkAt(referenceTransmitter, station, ChannelA, 20000, nil)
	require.InDelta(t, 33647.2, near.horizonMeters, 1)
	require.InDelta(t, -94.156, near.receivedPowerDBm, 0.01)
	require.InDelta(t, -110, near.effectiveSensitivityDBm, 0)
	require.InDelta(t, 15.844, near.marginDB, 0.01)
	require.InDelta(t, 1, near.probability, 0)

	// Positive margin, but the horizon taper limits probability.
	far := linkAt(referenceTransmitter, station, ChannelA, 30000, nil)
	require.InDelta(t, -100.319, far.receivedPowerDBm, 0.01)
	require.InDelta(t, 0.542, far.probability, 0.001)

	channelB := linkAt(referenceTransmitter, station, ChannelB, 30000, nil)
	require.InDelta(t, 20*math.Log10(162.025/161.975), far.receivedPowerDBm-channelB.receivedPowerDBm, 1e-9)
}

func TestHorizon(t *testing.T) {
	for _, tt := range []struct{ rx, want float64 }{{rx: 15, want: 29001.3}, {rx: 25, want: 33647.2}, {rx: 40, want: 39107.4}} {
		require.InDelta(t, tt.want, horizonMeters(10, tt.rx), 1)
	}
	horizon := 10000.0
	for _, tt := range []struct{ distance, want float64 }{{0, 1}, {8000, 1}, {9000, 0.5}, {10000, 0}, {20000, 0}} {
		require.InDelta(t, tt.want, horizonProbability(tt.distance, horizon), 1e-12, "distance %v", tt.distance)
	}
}

func TestMarginProbabilityKnots(t *testing.T) {
	for _, tt := range []struct{ margin, want float64 }{
		{margin: -40, want: 0}, {margin: -12, want: 0}, {margin: -6, want: 0.4}, {margin: 0, want: 0.8},
		{margin: 3, want: 0.9}, {margin: 6, want: 1}, {margin: 10, want: 1},
	} {
		require.InDelta(t, tt.want, marginProbability(tt.margin), 1e-12, "margin %v", tt.margin)
	}
}

func TestGeodesic(t *testing.T) {
	degree := 2 * math.Pi * earthRadiusMeters / 360
	for _, tt := range []struct {
		from, to [2]float64
		distance float64
		bearing  float64
	}{
		{from: [2]float64{0, 0}, to: [2]float64{0, 1}, distance: degree, bearing: 90},
		{from: [2]float64{0, 0}, to: [2]float64{1, 0}, distance: degree, bearing: 0},
		{from: [2]float64{1, 0}, to: [2]float64{0, 0}, distance: degree, bearing: 180},
		{from: [2]float64{0, 0}, to: [2]float64{0, -1}, distance: degree, bearing: 270},
		{from: [2]float64{0, 179.5}, to: [2]float64{0, -179.5}, distance: degree, bearing: 90},
		{from: [2]float64{52, 4}, to: [2]float64{52, 4}, distance: 0, bearing: 0},
	} {
		distance, bearing := geodesic(tt.from[0], tt.from[1], tt.to[0], tt.to[1])
		require.InDelta(t, tt.distance, distance, 1e-6)
		require.InDelta(t, tt.bearing, bearing, 1e-9)
	}

	lat, lon := destination(51.98, 4.05, 123, 25000)
	distance, bearing := geodesic(51.98, 4.05, lat, lon)
	require.InDelta(t, 25000, distance, 1e-6)
	require.InDelta(t, 123, bearing, 1e-9)
	_, lon = destination(0, 179.9, 90, 50000)
	require.Greater(t, lon, 180.0, "destination keeps longitude continuous")
}

func TestShadowLoss(t *testing.T) {
	sectors := []ShadowSector{{StartDegrees: 270, EndDegrees: 330, LossDB: 15}, {StartDegrees: 350, EndDegrees: 10, LossDB: 20}}
	for _, tt := range []struct{ bearing, want float64 }{
		{270, 15}, {300, 15}, {math.Nextafter(330, 0), 15}, {330, 0}, {math.Nextafter(270, 0), 0},
		{350, 20}, {359.9, 20}, {0, 20}, {9.99, 20}, {10, 0}, {180, 0},
	} {
		require.InDelta(t, tt.want, shadowLoss(sectors, tt.bearing), 0, "bearing %v", tt.bearing)
	}
}

func TestCoincidentPositionHasNoBearing(t *testing.T) {
	station := rotterdamCoast()
	station.ShadowSectors = []ShadowSector{{StartDegrees: 0, EndDegrees: 359, LossDB: 60}}

	at := evaluateLink(referenceTransmitter, station.Latitude, station.Longitude, station, ChannelA)
	require.Zero(t, at.distanceMeters)
	require.Nil(t, at.bearingDegrees)
	require.Equal(t, linkAt(referenceTransmitter, station, ChannelA, 0, nil), at, "no sector and a 10 m path")
	require.Equal(t, at.receivedPowerDBm, linkAt(referenceTransmitter, station, ChannelA, 5, nil).receivedPowerDBm)

	// About 5.6 m north: still the 10 m path, now inside the sector.
	north := evaluateLink(referenceTransmitter, station.Latitude+0.00005, station.Longitude, station, ChannelA)
	require.NotNil(t, north.bearingDegrees)
	require.InDelta(t, at.receivedPowerDBm-60, north.receivedPowerDBm, 0.01)
}

func TestLinkEffectsAreCorrectlyDirected(t *testing.T) {
	evaluate := func(station StationDefinition, tx TransmitterProfile) link {
		return linkAt(tx, station, ChannelA, 25000, bearing(300))
	}
	reference := evaluate(rotterdamCoast(), referenceTransmitter)
	for name, tt := range map[string]struct {
		change      func(*StationDefinition, *TransmitterProfile)
		power       float64
		sensitivity float64
	}{
		"receive gain":         {change: func(s *StationDefinition, _ *TransmitterProfile) { s.ReceiveGainDBi += 3 }, power: 3},
		"receive feeder loss":  {change: func(s *StationDefinition, _ *TransmitterProfile) { s.FeederLossDB += 3 }, power: -3},
		"transmit power":       {change: func(_ *StationDefinition, tx *TransmitterProfile) { tx.PowerWatts *= 2 }, power: 10 * math.Log10(2)},
		"transmit gain":        {change: func(_ *StationDefinition, tx *TransmitterProfile) { tx.GainDBi++ }, power: 1},
		"transmit feeder loss": {change: func(_ *StationDefinition, tx *TransmitterProfile) { tx.FeederLossDB++ }, power: -1},
		"shadow sector": {change: func(s *StationDefinition, _ *TransmitterProfile) {
			s.ShadowSectors = []ShadowSector{{StartDegrees: 270, EndDegrees: 330, LossDB: 10}}
		}, power: -10},
		"sector elsewhere": {change: func(s *StationDefinition, _ *TransmitterProfile) {
			s.ShadowSectors = []ShadowSector{{StartDegrees: 0, EndDegrees: 90, LossDB: 10}}
		}},
		"sensitivity":   {change: func(s *StationDefinition, _ *TransmitterProfile) { s.ChannelA.SensitivityDBm = -120 }, sensitivity: -10},
		"noise penalty": {change: func(s *StationDefinition, _ *TransmitterProfile) { s.ChannelA.NoisePenaltyDB = 5 }, sensitivity: 5},
		"other channel": {change: func(s *StationDefinition, _ *TransmitterProfile) { s.ChannelB.NoisePenaltyDB = 40 }},
	} {
		t.Run(name, func(t *testing.T) {
			station, tx := rotterdamCoast(), referenceTransmitter
			tt.change(&station, &tx)
			got := evaluate(station, tx)
			require.InDelta(t, tt.power, got.receivedPowerDBm-reference.receivedPowerDBm, 1e-9)
			require.InDelta(t, tt.sensitivity, got.effectiveSensitivityDBm-reference.effectiveSensitivityDBm, 1e-9)
			require.InDelta(t, tt.power-tt.sensitivity, got.marginDB-reference.marginDB, 1e-9)
		})
	}
}

func TestProbabilityIsMonotonic(t *testing.T) {
	station := rotterdamCoast()
	for loss := 0.0; loss <= 60; loss += 5 {
		station.ShadowSectors = []ShadowSector{{StartDegrees: 270, EndDegrees: 330, LossDB: loss}}
		previous := 1.0
		for distance := 0.0; distance <= 40000; distance += 250 {
			p := linkAt(referenceTransmitter, station, ChannelA, distance, bearing(300)).probability
			require.LessOrEqual(t, p, previous, "loss %v, distance %v", loss, distance)
			if loss > 0 {
				station.ShadowSectors[0].LossDB = loss - 5
				require.LessOrEqual(t, p, linkAt(referenceTransmitter, station, ChannelA, distance, bearing(300)).probability)
				station.ShadowSectors[0].LossDB = loss
			}
			previous = p
		}
	}
}

func TestSensitivityAndHeightMatter(t *testing.T) {
	// Sensitivity matters on an impaired path, not on a strong one.
	impaired := rotterdamCoast()
	impaired.ShadowSectors = []ShadowSector{{StartDegrees: 270, EndDegrees: 330, LossDB: 20}}
	sensitive := impaired
	sensitive.ChannelA.SensitivityDBm = -120
	p := func(s StationDefinition, distance float64) float64 {
		return linkAt(referenceTransmitter, s, ChannelA, distance, bearing(300)).probability
	}
	require.InDelta(t, 0.523, p(impaired, 20000), 0.01)
	require.Greater(t, p(sensitive, 20000), p(impaired, 20000)+0.3)
	require.InDelta(t, 1, p(impaired, 5000), 0)
	require.InDelta(t, 1, p(sensitive, 5000), 0)

	// Height matters near the horizon.
	low, high := rotterdamCoast(), rotterdamCoast()
	high.AntennaHeightMeters = 40
	require.Zero(t, p(low, 36000))
	require.Greater(t, p(high, 36000), 0.3)
}

func TestDisabledAndDroppedProbability(t *testing.T) {
	full := linkAt(referenceTransmitter, rotterdamCoast(), ChannelA, 30000, nil).probability
	for name, tt := range map[string]struct {
		change func(*StationDefinition)
		want   float64
	}{
		"station disabled":  {change: func(s *StationDefinition) { s.Enabled = false }},
		"channel disabled":  {change: func(s *StationDefinition) { s.ChannelA.Enabled = false }},
		"drop probability":  {change: func(s *StationDefinition) { s.ChannelA.DropProbability = 0.25 }, want: 0.75 * full},
		"always dropped":    {change: func(s *StationDefinition) { s.ChannelA.DropProbability = 1 }},
		"other channel off": {change: func(s *StationDefinition) { s.ChannelB.Enabled = false }, want: full},
	} {
		station := rotterdamCoast()
		tt.change(&station)
		l := linkAt(referenceTransmitter, station, ChannelA, 30000, nil)
		require.InDelta(t, tt.want, l.probability, 1e-12, name)
		require.InDelta(t, -100.319, l.receivedPowerDBm, 0.01, "%s keeps power diagnostics", name)
	}
}

func TestDecisionPrecedence(t *testing.T) {
	for name, tt := range map[string]struct {
		change   func(*StationDefinition)
		distance float64
		draw     float64
		want     outcome
	}{
		"station before channel": {change: func(s *StationDefinition) { s.Enabled, s.ChannelA.Enabled = false, false }, want: outcomeStationDisabled},
		"channel disabled":       {change: func(s *StationDefinition) { s.ChannelA.Enabled = false }, want: outcomeChannelDisabled},
		"horizon before drop": {change: func(s *StationDefinition) { s.ChannelA.DropProbability = 1 }, distance: 40000,
			want: outcomeOutsideHorizon},
		"at horizon": {distance: horizonMeters(10, 25), want: outcomeOutsideHorizon},
		"insufficient margin": {change: func(s *StationDefinition) {
			s.ShadowSectors = []ShadowSector{{StartDegrees: 270, EndDegrees: 330, LossDB: 60}}
		}, distance: 20000, want: outcomeInsufficientMargin},
		"drop is probabilistic": {change: func(s *StationDefinition) { s.ChannelA.DropProbability = 1 }, distance: 10000,
			want: outcomeProbabilisticLoss},
		"certain reception":    {distance: 10000, want: outcomeReceived},
		"draw below taper":     {distance: 30000, draw: 0.5, want: outcomeReceived},
		"draw above taper":     {distance: 30000, draw: 0.6, want: outcomeProbabilisticLoss},
		"draw equals estimate": {distance: 10000, draw: math.Nextafter(1, 0), want: outcomeReceived},
	} {
		t.Run(name, func(t *testing.T) {
			station := rotterdamCoast()
			if tt.change != nil {
				tt.change(&station)
			}
			l := linkAt(referenceTransmitter, station, ChannelA, tt.distance, bearing(300))
			require.Equal(t, tt.want, decide(station, ChannelA, l, tt.draw))
		})
	}
}

func TestReceptionDraw(t *testing.T) {
	// Fixed vectors pin the documented hash encoding.
	for _, tt := range []struct {
		seed       uint64
		station    string
		rfRevision uint64
		sequence   uint64
		want       float64
	}{
		{seed: 0, station: "station-1", rfRevision: 1, sequence: 1, want: 0.8709159596269299},
		{seed: 42, station: "station-2", rfRevision: 3, sequence: 12345, want: 0.10350254410121684},
	} {
		require.InDelta(t, tt.want, receptionDraw(tt.seed, tt.station, tt.rfRevision, tt.sequence), 0)
	}

	base := receptionDraw(42, "station-1", 1, 7)
	require.InDelta(t, base, receptionDraw(42, "station-1", 1, 7), 0)
	for name, other := range map[string]float64{
		"seed":        receptionDraw(43, "station-1", 1, 7),
		"station":     receptionDraw(42, "station-10", 1, 7),
		"rf revision": receptionDraw(42, "station-1", 2, 7),
		"sequence":    receptionDraw(42, "station-1", 1, 8),
	} {
		require.NotEqual(t, base, other, name)
	}

	// A fixed population of marginal opportunities matches its probability
	// within a broad predeclared tolerance (about 4.4 standard deviations).
	received := 0
	for sequence := range uint64(10000) {
		draw := receptionDraw(7, "station-1", 1, sequence)
		require.GreaterOrEqual(t, draw, 0.0)
		require.Less(t, draw, 1.0)
		if draw < 0.3 {
			received++
		}
	}
	require.InDelta(t, 3000, received, 200)
}

func TestCoverageContours(t *testing.T) {
	station := rotterdamCoast()
	coverage := computeCoverage(referenceTransmitter, station)
	require.Len(t, coverage, 4)
	horizon := horizonMeters(10, 25)
	for _, c := range coverage {
		require.Len(t, c.Ring, 73)
		require.Equal(t, c.Ring[0], c.Ring[72])
		requireRingAtThreshold(t, station, c)
		require.InDelta(t, c.MinRadiusMeters, c.MaxRadiusMeters, 1e-6, "no sector gives a circle")
		require.Less(t, c.MaxRadiusMeters, horizon)
	}
	// With at least 6 dB margin the horizon taper alone sets the radius, so the
	// channel frequency does not matter.
	require.InDelta(t, coverage[0].MaxRadiusMeters, coverage[2].MaxRadiusMeters, 0)
	require.Greater(t, coverage[1].MaxRadiusMeters, coverage[0].MaxRadiusMeters)
}

func requireRingAtThreshold(t *testing.T, station StationDefinition, c Coverage) {
	t.Helper()
	for i, point := range c.Ring[:len(c.Ring)-1] {
		l := evaluateLink(referenceTransmitter, point.Latitude, point.Longitude, station, c.Channel)
		require.InDelta(t, c.Threshold, l.probability, 0.002, "vertex %d", i)
		require.GreaterOrEqual(t, l.distanceMeters, c.MinRadiusMeters-0.01)
		require.LessOrEqual(t, l.distanceMeters, c.MaxRadiusMeters+0.01)
	}
}

func TestCoverageSectorsAndEmptyContours(t *testing.T) {
	station := rotterdamCoast()
	station.ShadowSectors = []ShadowSector{{StartDegrees: 270, EndDegrees: 330, LossDB: 40}}
	station.ChannelA.DropProbability = 0.2
	station.ChannelB.Enabled = false
	coverage := computeCoverage(referenceTransmitter, station)

	require.Empty(t, coverage[0].Ring, "0.9 is above the dropped maximum of 0.8")
	require.NotNil(t, coverage[0].Ring)
	require.Zero(t, coverage[0].MaxRadiusMeters)
	require.Empty(t, coverage[2].Ring)
	require.Empty(t, coverage[3].Ring)

	c := coverage[1]
	bearings := coverageBearings(station.ShadowSectors)
	require.Len(t, c.Ring, len(bearings)+1)
	requireRingAtThreshold(t, station, c)
	radius := func(b float64) float64 {
		i := indexOf(t, bearings, b)
		distance, _ := geodesic(station.Latitude, station.Longitude, c.Ring[i].Latitude, c.Ring[i].Longitude)
		return distance
	}
	outside, inside := radius(270-boundaryOffsetDegrees), radius(270)
	require.Greater(t, outside, 2*inside, "start is inclusive")
	inside, outside = radius(330-boundaryOffsetDegrees), radius(330)
	require.Greater(t, outside, 2*inside, "end is exclusive")

	disabled := rotterdamCoast()
	disabled.Enabled = false
	for _, c := range computeCoverage(referenceTransmitter, disabled) {
		require.Equal(t, Coverage{Channel: c.Channel, Threshold: c.Threshold, Ring: []GeoPoint{}}, c)
	}
}

func indexOf(t *testing.T, values []float64, value float64) int {
	t.Helper()
	for i, v := range values {
		if math.Abs(v-value) < 1e-9 {
			return i
		}
	}
	require.Fail(t, fmt.Sprintf("bearing %v not sampled", value))
	return 0
}

func TestCoverageNestsThresholds(t *testing.T) {
	station := rotterdamCoast()
	station.ShadowSectors = []ShadowSector{{StartDegrees: 100, EndDegrees: 200, LossDB: 12}}
	coverage := computeCoverage(referenceTransmitter, station)
	for i := range coverage[0].Ring {
		inner, _ := geodesic(station.Latitude, station.Longitude, coverage[0].Ring[i].Latitude, coverage[0].Ring[i].Longitude)
		outer, _ := geodesic(station.Latitude, station.Longitude, coverage[1].Ring[i].Latitude, coverage[1].Ring[i].Longitude)
		require.LessOrEqual(t, inner, outer+3, "vertex %d", i)
	}
}

func TestCoverageGlobalGeometry(t *testing.T) {
	for _, position := range [][2]float64{{0, 179.95}, {10, -179.95}, {85, 0}, {-85, 120}} {
		t.Run(fmt.Sprint(position), func(t *testing.T) {
			station := rotterdamCoast()
			station.Latitude, station.Longitude = position[0], position[1]
			c := computeCoverage(referenceTransmitter, station)[1]
			require.Len(t, c.Ring, 73)
			requireRingAtThreshold(t, station, c)
			for i := 1; i < len(c.Ring); i++ {
				require.Less(t, math.Abs(c.Ring[i].Longitude-c.Ring[i-1].Longitude), 20.0, "longitudes stay continuous")
				require.LessOrEqual(t, math.Abs(c.Ring[i].Latitude), 90.0)
			}
		})
	}
}

func TestCoverageBearingLimit(t *testing.T) {
	sectors := make([]ShadowSector, 0, MaxShadowSectors)
	for i := range MaxShadowSectors {
		start := float64(i*45) + 1.5
		sectors = append(sectors, ShadowSector{StartDegrees: start, EndDegrees: start + 20, LossDB: 10})
	}
	require.Len(t, coverageBearings(sectors), 104)
	require.Len(t, coverageBearings(nil), 72)
}
