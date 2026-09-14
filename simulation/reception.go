package simulation

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"slices"
)

// Reception model parameters, published in Metadata.Settings.Reception. They
// are empirical scenario choices, not calibrated coverage predictions.
const (
	siteLossDB                 = 15.0 // Excess loss applied to every path.
	pathExponent               = 3.5  // Distance exponent of excess loss beyond 1 km.
	effectiveEarthRadiusFactor = 4.0 / 3.0
	channelAFrequencyMHz       = 161.975
	channelBFrequencyMHz       = 162.025
	horizonTaperStart          = 0.8   // Fraction of the horizon where probability starts to fall.
	zeroProbabilityMarginDB    = -12.0 // Margin at and below which decoding never succeeds.
	referenceProbability       = 0.8   // Decode probability at zero margin.
	fullProbabilityMarginDB    = 6.0   // Margin at and above which decoding always succeeds.
)

// Geometry settings.
const (
	earthRadiusMeters   = 6371000.0
	minPathKilometers   = 0.01 // Attenuation distance for nearly coincident positions.
	coverageBearingStep = 5    // Degrees between sampled coverage bearings.
	// boundaryOffsetDegrees samples just outside a shadow sector start and just
	// inside its exclusive end.
	boundaryOffsetDegrees = 1e-6
	// coverageIterations bisects the horizon to about 1/65536 of its length,
	// under 2.1 m at the largest valid horizon.
	coverageIterations = 16
)

// coverageThresholds are the probabilities of the coverage contours.
var coverageThresholds = []float64{0.9, 0.5}

// outcome is the simulator's reception decision for one station and one
// transmission. When several apply, the first in declaration order wins.
type outcome string

const (
	outcomeStationDisabled    outcome = "station_disabled"
	outcomeChannelDisabled    outcome = "channel_disabled"
	outcomeOutsideHorizon     outcome = "outside_horizon"
	outcomeInsufficientMargin outcome = "insufficient_margin"
	outcomeProbabilisticLoss  outcome = "probabilistic_loss"
	outcomeReceived           outcome = "received"
)

// link is the model evaluation of one transmitter position at one station
// channel. All values are simulation diagnostics, not measurements.
type link struct {
	distanceMeters float64
	// bearingDegrees is the true bearing from station to transmitter in [0, 360),
	// nil at coincident positions where it is undefined.
	bearingDegrees          *float64
	horizonMeters           float64
	receivedPowerDBm        float64
	effectiveSensitivityDBm float64
	marginDB                float64
	probability             float64
}

// evaluateLink evaluates a transmitter at latitude and longitude against one
// channel of station.
//
//	d_km      = max(distance_km, 0.01)
//	P_rx_dBm  = 10 log10(power_W * 1000) + tx_gain + rx_gain - tx_feeder - rx_feeder
//	            - (32.4 + 20 log10(f_MHz) + 20 log10(d_km))
//	            - (site_loss + 10 (path_exponent - 2) log10(max(d_km, 1)))
//	            - sector_loss
//	margin_dB = P_rx_dBm - (sensitivity + noise_penalty)
//	p         = p_margin * p_horizon * (1 - drop_probability)
//
// p is 0 when the station or channel is disabled.
func evaluateLink(tx TransmitterProfile, latitude, longitude float64, station StationDefinition, channel Channel) link {
	distance, bearing := geodesic(station.Latitude, station.Longitude, latitude, longitude)
	if distance == 0 {
		return linkAt(tx, station, channel, 0, nil)
	}
	return linkAt(tx, station, channel, distance, &bearing)
}

// linkAt evaluates the model at a distance and optional bearing from the
// station. Without a bearing no shadow sector applies.
func linkAt(tx TransmitterProfile, station StationDefinition, channel Channel, distanceMeters float64, bearing *float64) link {
	receiver, frequency := station.ChannelA, channelAFrequencyMHz
	if channel == ChannelB {
		receiver, frequency = station.ChannelB, channelBFrequencyMHz
	}
	kilometers := max(distanceMeters/1000, minPathKilometers)
	freeSpaceLoss := 32.4 + 20*math.Log10(frequency) + 20*math.Log10(kilometers)
	excessLoss := siteLossDB + 10*(pathExponent-2)*math.Log10(max(kilometers, 1))
	sectorLoss := 0.0
	if bearing != nil {
		sectorLoss = shadowLoss(station.ShadowSectors, *bearing)
	}
	power := 10*math.Log10(tx.PowerWatts*1000) + tx.GainDBi + station.ReceiveGainDBi -
		tx.FeederLossDB - station.FeederLossDB - freeSpaceLoss - excessLoss - sectorLoss
	sensitivity := receiver.SensitivityDBm + receiver.NoisePenaltyDB
	horizon := horizonMeters(tx.HeightMeters, station.AntennaHeightMeters)
	result := link{
		distanceMeters: distanceMeters, bearingDegrees: bearing, horizonMeters: horizon,
		receivedPowerDBm: power, effectiveSensitivityDBm: sensitivity, marginDB: power - sensitivity,
	}
	if station.Enabled && receiver.Enabled {
		result.probability = marginProbability(result.marginDB) * horizonProbability(distanceMeters, horizon) * (1 - receiver.DropProbability)
	}
	return result
}

// horizonMeters is the radio horizon for two antenna heights in metres above
// sea level, with an effective earth radius of 4/3 of the real one.
func horizonMeters(txHeight, rxHeight float64) float64 {
	return 3570 * math.Sqrt(effectiveEarthRadiusFactor) * (math.Sqrt(txHeight) + math.Sqrt(rxHeight))
}

// marginProbability interpolates linearly through (-12 dB, 0), (0 dB, 0.8),
// and (6 dB, 1), clamped outside.
func marginProbability(margin float64) float64 {
	switch {
	case margin <= zeroProbabilityMarginDB:
		return 0
	case margin < 0:
		return referenceProbability * (margin - zeroProbabilityMarginDB) / -zeroProbabilityMarginDB
	case margin < fullProbabilityMarginDB:
		return referenceProbability + (1-referenceProbability)*margin/fullProbabilityMarginDB
	default:
		return 1
	}
}

// horizonProbability is 1 up to 80% of the horizon, falls linearly to 0 at the
// horizon, and is 0 beyond.
func horizonProbability(distance, horizon float64) float64 {
	switch start := horizonTaperStart * horizon; {
	case distance <= start:
		return 1
	case distance < horizon:
		return (horizon - distance) / (horizon - start)
	default:
		return 0
	}
}

// shadowLoss returns the loss of the sector containing bearing, or 0. Sectors
// do not overlap, so at most one contains it.
func shadowLoss(sectors []ShadowSector, bearing float64) float64 {
	for _, s := range sectors {
		inside := bearing >= s.StartDegrees && bearing < s.EndDegrees
		if s.StartDegrees > s.EndDegrees {
			inside = bearing >= s.StartDegrees || bearing < s.EndDegrees
		}
		if inside {
			return s.LossDB
		}
	}
	return 0
}

// decide returns the outcome of l for a draw uniform in [0, 1). A transmission
// is received iff draw < l.probability and no deterministic cause excludes it.
func decide(station StationDefinition, channel Channel, l link, draw float64) outcome {
	receiver := station.ChannelA
	if channel == ChannelB {
		receiver = station.ChannelB
	}
	switch {
	case !station.Enabled:
		return outcomeStationDisabled
	case !receiver.Enabled:
		return outcomeChannelDisabled
	case l.distanceMeters >= l.horizonMeters:
		return outcomeOutsideHorizon
	case l.marginDB <= zeroProbabilityMarginDB:
		return outcomeInsufficientMargin
	case draw < l.probability:
		return outcomeReceived
	default:
		return outcomeProbabilisticLoss
	}
}

// receptionDraw returns a value uniform in [0, 1) for one reception opportunity.
// It hashes with SHA-256 a domain tag, the run seed, the length-prefixed station
// ID, the station RF revision, and the transmission sequence, all big-endian,
// and divides the first 53 bits by 2^53. It consumes no random source, so
// outcomes do not depend on batching, station order, or other stations.
func receptionDraw(seed uint64, stationID string, rfRevision, sequence uint64) float64 {
	const tag = "ais-test-bench reception\x00"
	buf := make([]byte, 0, len(tag)+32+len(stationID))
	buf = append(buf, tag...)
	buf = binary.BigEndian.AppendUint64(buf, seed)
	buf = binary.BigEndian.AppendUint64(buf, uint64(len(stationID)))
	buf = append(buf, stationID...)
	buf = binary.BigEndian.AppendUint64(buf, rfRevision)
	buf = binary.BigEndian.AppendUint64(buf, sequence)
	sum := sha256.Sum256(buf)
	return float64(binary.BigEndian.Uint64(sum[:8])>>11) / (1 << 53)
}

// computeCoverage returns the contours of station for the reference
// transmitter: channel A then B, each at every coverage threshold.
func computeCoverage(tx TransmitterProfile, station StationDefinition) []Coverage {
	bearings := coverageBearings(station.ShadowSectors)
	result := make([]Coverage, 0, 2*len(coverageThresholds))
	for _, channel := range []Channel{ChannelA, ChannelB} {
		for _, threshold := range coverageThresholds {
			result = append(result, contour(tx, station, channel, threshold, bearings))
		}
	}
	return result
}

// contour samples the threshold radius at every bearing. Probability does not
// increase with distance along a bearing, so bisection over [0, horizon] finds
// the radius. A threshold unreachable even at the station gives an empty ring.
func contour(tx TransmitterProfile, station StationDefinition, channel Channel, threshold float64, bearings []float64) Coverage {
	result := Coverage{Channel: channel, Threshold: threshold, Ring: []GeoPoint{}}
	if linkAt(tx, station, channel, 0, nil).probability < threshold {
		return result
	}
	horizon := horizonMeters(tx.HeightMeters, station.AntennaHeightMeters)
	result.MinRadiusMeters = horizon
	for _, bearing := range bearings {
		low, high := 0.0, horizon
		for range coverageIterations {
			mid := (low + high) / 2
			if linkAt(tx, station, channel, mid, &bearing).probability >= threshold {
				low = mid
			} else {
				high = mid
			}
		}
		result.MinRadiusMeters = min(result.MinRadiusMeters, low)
		result.MaxRadiusMeters = max(result.MaxRadiusMeters, low)
		latitude, longitude := destination(station.Latitude, station.Longitude, bearing, low)
		result.Ring = append(result.Ring, GeoPoint{Latitude: latitude, Longitude: longitude})
	}
	result.Ring = append(result.Ring, result.Ring[0])
	return result
}

// coverageBearings returns sorted unique bearings: every coverageBearingStep
// degrees plus both sides of every sector boundary. At most 72 + 4 *
// MaxShadowSectors = 104 bearings.
func coverageBearings(sectors []ShadowSector) []float64 {
	bearings := make([]float64, 0, 360/coverageBearingStep+4*len(sectors))
	for b := 0; b < 360; b += coverageBearingStep {
		bearings = append(bearings, float64(b))
	}
	for _, s := range sectors {
		for _, b := range []float64{s.StartDegrees, s.StartDegrees - boundaryOffsetDegrees, s.EndDegrees, s.EndDegrees - boundaryOffsetDegrees} {
			bearings = append(bearings, math.Mod(b+360, 360))
		}
	}
	slices.Sort(bearings)
	return slices.Compact(bearings)
}

// geodesic returns the great-circle distance in metres and the initial true
// bearing in [0, 360) from the first to the second position.
func geodesic(lat1, lon1, lat2, lon2 float64) (float64, float64) {
	phi1, phi2 := lat1*math.Pi/180, lat2*math.Pi/180
	dPhi, dLambda := phi2-phi1, (lon2-lon1)*math.Pi/180
	h := math.Pow(math.Sin(dPhi/2), 2) + math.Cos(phi1)*math.Cos(phi2)*math.Pow(math.Sin(dLambda/2), 2)
	distance := 2 * earthRadiusMeters * math.Atan2(math.Sqrt(h), math.Sqrt(1-h))
	y := math.Sin(dLambda) * math.Cos(phi2)
	x := math.Cos(phi1)*math.Sin(phi2) - math.Sin(phi1)*math.Cos(phi2)*math.Cos(dLambda)
	bearing := math.Mod(math.Atan2(y, x)*180/math.Pi+360, 360)
	return distance, bearing
}

// destination moves distance metres from a position along a great circle with
// the initial true bearing. The longitude is not normalized: it stays within
// 180 degrees of the start, so points near the antimeridian stay continuous.
func destination(latitude, longitude, bearing, distance float64) (float64, float64) {
	lat := latitude * math.Pi / 180
	lon := longitude * math.Pi / 180
	theta := bearing * math.Pi / 180
	angle := distance / earthRadiusMeters
	nextLat := math.Asin(math.Sin(lat)*math.Cos(angle) + math.Cos(lat)*math.Sin(angle)*math.Cos(theta))
	nextLon := lon + math.Atan2(math.Sin(theta)*math.Sin(angle)*math.Cos(lat), math.Cos(angle)-math.Sin(lat)*math.Sin(nextLat))
	return nextLat * 180 / math.Pi, nextLon * 180 / math.Pi
}
