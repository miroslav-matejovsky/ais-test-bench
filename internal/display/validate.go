package display

import (
	"errors"
	"fmt"
	"math"
	"math/bits"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/miroslav-matejovsky/ais-testbench/internal/simulatorapi"
)

// Contract checks for simulator metadata, station configuration, counters, and
// coverage. They check published ranges and internal consistency only; no RF
// value is recalculated.

const (
	// speedStepTolerance accepts binary float representations of valid speed steps.
	speedStepTolerance = 1e-6
	// maxAgeMs bounds published age limits so they convert to time.Duration.
	maxAgeMs = int64(math.MaxInt64 / int64(time.Millisecond))
	// maxCoveragePolygons and maxCoverageVertices bound one coverage contour.
	// The simulator samples at most 104 bearings and splits a ring into at most
	// three antimeridian pieces.
	maxCoveragePolygons = 8
	maxCoverageVertices = 2048
)

func validateMetadata(metadata simulatorapi.Metadata) error {
	if metadata.SimulationID == "" || metadata.StartedAt.IsZero() {
		return errors.New("simulationId and startedAt are required")
	}
	if !slices.Contains(metadata.SupportedMessageTypes, 1) {
		return errors.New("supportedMessageTypes must include AIS message type 1")
	}
	if len(metadata.VesselTypes) == 0 {
		return errors.New("vesselTypes is required")
	}
	seen := make(map[string]bool, len(metadata.VesselTypes))
	for _, vesselType := range metadata.VesselTypes {
		if vesselType.ID == "" || vesselType.Name == "" || seen[vesselType.ID] {
			return fmt.Errorf("vessel type %q: unique id and name are required", vesselType.ID)
		}
		seen[vesselType.ID] = true
	}
	b := metadata.Settings.SpawnBounds
	if !inRange(b.South, -90, 90) || !inRange(b.North, -90, 90) || b.South >= b.North ||
		!inRange(b.West, -180, 180) || !inRange(b.East, -180, 180) || b.West >= b.East {
		return fmt.Errorf("invalid spawnBounds %+v", b)
	}
	return validateSettings(metadata.Settings)
}

// validateSettings checks the reference transmitter, reception model, and
// observation limits against their documented ranges.
func validateSettings(s simulatorapi.Settings) error {
	t, r, o := s.Transmitter, s.Reception, s.Observation
	switch {
	case s.MaxStations < 1:
		return fmt.Errorf("maxStations %d must be positive", s.MaxStations)
	case !inRange(t.PowerWatts, 0, 25) || t.PowerWatts == 0 || !inRange(t.HeightMeters, 0, 100) || t.HeightMeters == 0 ||
		!inRange(t.GainDBi, -10, 20) || !inRange(t.FeederLossDB, 0, 30):
		return fmt.Errorf("invalid reference transmitter %+v", t)
	case !finite(r.SiteLossDB) || !finite(r.PathExponent) || !(r.EffectiveEarthRadiusFactor > 0) || math.IsInf(r.EffectiveEarthRadiusFactor, 0) ||
		!(r.ChannelAFrequencyMHz > 0) || math.IsInf(r.ChannelAFrequencyMHz, 0) || !(r.ChannelBFrequencyMHz > 0) || math.IsInf(r.ChannelBFrequencyMHz, 0) ||
		!inRange(r.HorizonTaperStart, 0, 1) || !inRange(r.ReferenceProbability, 0, 1) ||
		!finite(r.ZeroProbabilityMarginDB) || !finite(r.FullProbabilityMarginDB) || r.ZeroProbabilityMarginDB >= r.FullProbabilityMarginDB:
		return fmt.Errorf("invalid reception model %+v", r)
	case len(r.CoverageThresholds) == 0:
		return errors.New("reception coverageThresholds is required")
	case o.ReceptionHistoryLimit < 1 || o.TargetLimit < 1 || o.RecentReceptionLimit < 1 ||
		o.FreshAgeMs < 1 || o.FreshAgeMs >= o.StaleAgeMs || o.StaleAgeMs >= o.ExpiryAgeMs || o.ExpiryAgeMs > maxAgeMs ||
		o.RateWindowMs < 1 || o.RateWindowMs > maxAgeMs:
		return fmt.Errorf("invalid observation settings %+v", o)
	}
	for i, threshold := range r.CoverageThresholds {
		if !inRange(threshold, 0, 1) || threshold == 0 || slices.Contains(r.CoverageThresholds[:i], threshold) {
			return fmt.Errorf("invalid coverage thresholds %v", r.CoverageThresholds)
		}
	}
	return nil
}

// validateTime checks the clock: all fields present, now not before startedAt,
// nonnegative elapsed time, speed 0 or within limits on a step, and paused
// exactly when speed is 0.
func validateTime(clock *upstreamTime, startedAt time.Time, limits simulatorapi.SpeedLimits) (simulatorapi.TimeState, error) {
	if clock == nil {
		return simulatorapi.TimeState{}, errors.New("time is required")
	}
	if clock.Now == nil || clock.ElapsedMs == nil || clock.Speed == nil || clock.Paused == nil {
		return simulatorapi.TimeState{}, errors.New("time.now, time.elapsedMs, time.speed, and time.paused are required")
	}
	now, elapsed, speed, paused := *clock.Now, *clock.ElapsedMs, *clock.Speed, *clock.Paused
	if now.Before(startedAt) {
		return simulatorapi.TimeState{}, fmt.Errorf("time.now %s is before startedAt", now.Format(time.RFC3339Nano))
	}
	if elapsed < 0 {
		return simulatorapi.TimeState{}, fmt.Errorf("time.elapsedMs %d is negative", elapsed)
	}
	if limits.Min <= 0 || limits.Max < limits.Min || limits.Step <= 0 {
		return simulatorapi.TimeState{}, fmt.Errorf("invalid settings.speed %+v", limits)
	}
	steps := speed / limits.Step
	if speed != 0 && (speed < limits.Min || speed > limits.Max || math.Abs(steps-math.Round(steps)) > speedStepTolerance) {
		return simulatorapi.TimeState{}, fmt.Errorf("time.speed %v is not 0 or %v-%v in steps of %v", speed, limits.Min, limits.Max, limits.Step)
	}
	if paused != (speed == 0) {
		return simulatorapi.TimeState{}, fmt.Errorf("time.paused %t disagrees with time.speed %v", paused, speed)
	}
	return simulatorapi.TimeState{Now: now, ElapsedMs: elapsed, Speed: speed, Paused: paused}, nil
}

// validateStation checks one station's configuration, lifecycle times,
// counters, history bounds, and coverage within a snapshot at now.
func validateStation(s simulatorapi.StationObservation, settings simulatorapi.Settings, startedAt, now time.Time) error {
	switch {
	case s.ID == "":
		return errors.New("id is required")
	case s.ConfigRevision == 0 || s.RFRevision == 0:
		return errors.New("configRevision and rfRevision are required")
	case s.CreatedAt.Before(startedAt) || s.CreatedAt.After(now):
		return fmt.Errorf("createdAt %s is outside the run", s.CreatedAt.Format(time.RFC3339Nano))
	case s.RFUpdatedAt.Before(s.CreatedAt) || s.RFUpdatedAt.After(now):
		return fmt.Errorf("rfUpdatedAt %s is outside createdAt and time.now", s.RFUpdatedAt.Format(time.RFC3339Nano))
	}
	if err := validateDefinition(s.Definition); err != nil {
		return fmt.Errorf("definition: %w", err)
	}
	if err := validateCounters(s, settings.Observation); err != nil {
		return err
	}
	return validateCoverage(s.Coverage, settings.Reception.CoverageThresholds)
}

func validateDefinition(d simulatorapi.StationDefinition) error {
	switch {
	case strings.TrimSpace(d.Name) == "" || !utf8.ValidString(d.Name) || utf8.RuneCountInString(d.Name) > simulatorapi.MaxStationNameRunes:
		return fmt.Errorf("name %q is blank, invalid, or too long", d.Name)
	case !inRange(d.Latitude, -85, 85) || !inRange(d.Longitude, -180, 180) || d.Longitude == 180:
		return fmt.Errorf("invalid location %v, %v", d.Latitude, d.Longitude)
	case !inRange(d.AntennaHeightMeters, 0, 500) || d.AntennaHeightMeters == 0 ||
		!inRange(d.ReceiveGainDBi, -10, 20) || !inRange(d.FeederLossDB, 0, 30):
		return errors.New("invalid antenna height, receive gain, or feeder loss")
	case d.ShadowSectors == nil || len(d.ShadowSectors) > simulatorapi.MaxShadowSectors:
		return errors.New("shadowSectors is required and bounded")
	}
	if err := validateChannel(d.ChannelA); err != nil {
		return fmt.Errorf("channelA: %w", err)
	}
	if err := validateChannel(d.ChannelB); err != nil {
		return fmt.Errorf("channelB: %w", err)
	}
	for _, sector := range d.ShadowSectors {
		if !inRange(sector.StartDegrees, 0, 360) || sector.StartDegrees == 360 ||
			!inRange(sector.EndDegrees, 0, 360) || sector.EndDegrees == 360 || !inRange(sector.LossDB, 0, 60) {
			return fmt.Errorf("invalid shadow sector %+v", sector)
		}
	}
	return nil
}

func validateChannel(c simulatorapi.ReceiverChannel) error {
	if !inRange(c.SensitivityDBm, -125, -80) || !inRange(c.NoisePenaltyDB, 0, 40) || !inRange(c.DropProbability, 0, 1) {
		return fmt.Errorf("invalid receiver channel %+v", c)
	}
	return nil
}

// validateReceiver checks the reception-time station configuration.
func validateReceiver(r simulatorapi.ReceiverSnapshot) error {
	return validateDefinition(simulatorapi.StationDefinition{
		Name: r.Name, Latitude: r.Latitude, Longitude: r.Longitude, AntennaHeightMeters: r.AntennaHeightMeters,
		ReceiveGainDBi: r.ReceiveGainDBi, FeederLossDB: r.FeederLossDB,
		ChannelA: r.Channel, ChannelB: r.Channel, ShadowSectors: []simulatorapi.ShadowSector{},
	})
}

// validateLink checks that estimated link values are finite and in range.
func validateLink(l simulatorapi.ReceptionLink) error {
	switch {
	case !(l.DistanceMeters >= 0) || math.IsInf(l.DistanceMeters, 0) || !(l.HorizonMeters >= 0) || math.IsInf(l.HorizonMeters, 0):
		return errors.New("distanceMeters and horizonMeters must be finite and nonnegative")
	case l.BearingDegrees != nil && (!inRange(*l.BearingDegrees, 0, 360) || *l.BearingDegrees == 360):
		return fmt.Errorf("bearingDegrees %v is outside [0, 360)", *l.BearingDegrees)
	case !inRange(l.ShadowLossDB, 0, 60) || !finite(l.ReceivedPowerDBm) || !finite(l.EffectiveSensitivityDBm) || !finite(l.MarginDB):
		return errors.New("shadow loss, power, sensitivity, and margin must be finite and in range")
	case !inRange(l.Probability, 0, 1):
		return fmt.Errorf("probability %v is outside [0, 1]", l.Probability)
	}
	return nil
}

// validateCounters checks that loss reasons and channels partition the
// lifetime opportunities, recent windows fit within them, ratios exist exactly
// with opportunities, and history bounds match the contiguous reception sequence.
func validateCounters(s simulatorapi.StationObservation, limits simulatorapi.ObservationSettings) error {
	c, recent := s.Counters, s.Recent
	outcomes, overflow := sum(c.Received, c.StationDisabled, c.ChannelDisabled, c.OutsideHorizon, c.InsufficientMargin, c.ProbabilisticLoss)
	opportunities, overflowA := sum(c.ChannelA.Opportunities, c.ChannelB.Opportunities)
	received, overflowB := sum(c.ChannelA.Received, c.ChannelB.Received)
	switch {
	case overflow || outcomes != c.Opportunities:
		return errors.New("received and loss counters do not sum to opportunities")
	case overflowA || overflowB || opportunities != c.Opportunities || received != c.Received ||
		c.ChannelA.Received > c.ChannelA.Opportunities || c.ChannelB.Received > c.ChannelB.Opportunities:
		return errors.New("channel counters disagree with totals")
	case recent.DurationMs < 0 || recent.DurationMs > limits.RateWindowMs:
		return fmt.Errorf("recent durationMs %d is outside 0-%d", recent.DurationMs, limits.RateWindowMs)
	case recent.Received > recent.Opportunities || recent.Opportunities > c.Opportunities || recent.Received > c.Received:
		return errors.New("recent counters exceed their totals")
	case (recent.OpportunityRate == nil) != (recent.ReceptionRate == nil) || recent.DurationMs > 0 && recent.OpportunityRate == nil:
		return errors.New("recent rates must both be set when durationMs is positive")
	case recent.OpportunityRate != nil && (!(*recent.OpportunityRate >= 0) || math.IsInf(*recent.OpportunityRate, 0) ||
		!(*recent.ReceptionRate >= 0) || math.IsInf(*recent.ReceptionRate, 0)):
		return errors.New("recent rates must be finite and nonnegative")
	case (s.OldestReception == nil) != (s.LatestReception == nil) || (s.LatestReception == nil) != (c.Received == 0):
		return errors.New("reception bounds must be set exactly when receptions exist")
	case s.LatestReception != nil && (*s.OldestReception == 0 || *s.OldestReception > *s.LatestReception ||
		*s.LatestReception != c.Received || *s.LatestReception-*s.OldestReception >= uint64(limits.ReceptionHistoryLimit)):
		return fmt.Errorf("impossible reception bounds %d-%d for %d receptions", *s.OldestReception, *s.LatestReception, c.Received)
	case s.CurrentTargets < 0 || s.LostTargets < 0 || s.CurrentTargets+s.LostTargets > limits.TargetLimit:
		return errors.New("target counts are outside 0-targetLimit")
	}
	if err := validateRatio(s.ReceiveRatio, c.Opportunities); err != nil {
		return fmt.Errorf("receiveRatio: %w", err)
	}
	if err := validateRatio(recent.ReceiveRatio, recent.Opportunities); err != nil {
		return fmt.Errorf("recent receiveRatio: %w", err)
	}
	return nil
}

func validateRatio(ratio *float64, whole uint64) error {
	if (ratio == nil) != (whole == 0) {
		return errors.New("must be null exactly without opportunities")
	}
	if ratio != nil && !inRange(*ratio, 0, 1) {
		return fmt.Errorf("%v is outside [0, 1]", *ratio)
	}
	return nil
}

// validateCoverage requires one contour per channel and threshold with
// consistent radii and valid GeoJSON MultiPolygon geometry.
func validateCoverage(coverage []simulatorapi.Coverage, thresholds []float64) error {
	if len(coverage) != 2*len(thresholds) {
		return fmt.Errorf("%d coverage contours, want %d", len(coverage), 2*len(thresholds))
	}
	type contour struct {
		channel   string
		threshold float64
	}
	seen := make(map[contour]bool, len(coverage))
	for _, c := range coverage {
		key := contour{channel: c.Channel, threshold: c.Threshold}
		switch {
		case !validChannel(c.Channel) || !slices.Contains(thresholds, c.Threshold) || seen[key]:
			return fmt.Errorf("unexpected or duplicate coverage contour %s/%v", c.Channel, c.Threshold)
		case !(c.MinRadiusMeters >= 0) || math.IsInf(c.MaxRadiusMeters, 0) || !(c.MaxRadiusMeters >= c.MinRadiusMeters):
			return fmt.Errorf("coverage %s/%v radii %v-%v are invalid", c.Channel, c.Threshold, c.MinRadiusMeters, c.MaxRadiusMeters)
		}
		seen[key] = true
		if err := validateGeometry(c.Geometry); err != nil {
			return fmt.Errorf("coverage %s/%v: %w", c.Channel, c.Threshold, err)
		}
	}
	return nil
}

// validateGeometry checks a bounded MultiPolygon of closed rings with at least
// four [longitude, latitude] positions in WGS84 range.
func validateGeometry(g simulatorapi.Geometry) error {
	if g.Type != "MultiPolygon" || g.Coordinates == nil {
		return fmt.Errorf("geometry type %q with coordinates is required, want MultiPolygon", g.Type)
	}
	if len(g.Coordinates) > maxCoveragePolygons {
		return fmt.Errorf("%d polygons exceed %d", len(g.Coordinates), maxCoveragePolygons)
	}
	vertices := 0
	for _, polygon := range g.Coordinates {
		if len(polygon) == 0 {
			return errors.New("polygon without rings")
		}
		for _, ring := range polygon {
			if vertices += len(ring); vertices > maxCoverageVertices {
				return fmt.Errorf("more than %d vertices", maxCoverageVertices)
			}
			if len(ring) < 4 || ring[0] != ring[len(ring)-1] {
				return errors.New("ring must be closed with at least four positions")
			}
			for _, position := range ring {
				if !inRange(position[0], -180, 180) || !inRange(position[1], -90, 90) {
					return fmt.Errorf("position %v is outside WGS84 range", position)
				}
			}
		}
	}
	return nil
}

func validChannel(channel string) bool { return channel == "A" || channel == "B" }

// finite reports whether v is neither NaN nor infinite.
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// inRange reports whether v is a number within [lo, hi].
func inRange(v, lo, hi float64) bool { return v >= lo && v <= hi }

// sum adds counters and reports uint64 overflow.
func sum(values ...uint64) (uint64, bool) {
	var total, carry uint64
	for _, v := range values {
		var c uint64
		total, c = bits.Add64(total, v, 0)
		carry |= c
	}
	return total, carry != 0
}
