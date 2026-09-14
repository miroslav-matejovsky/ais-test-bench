package display

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/ais"
	"github.com/miroslav-matejovsky/ais-test-bench/internal/simulatorapi"
)

// speedStepTolerance accepts binary float representations of valid speed steps.
const speedStepTolerance = 1e-6

// upstreamMetadata decodes simulatorapi.Metadata with a presence-aware time, so
// a missing or null time field differs from a valid zero. The outer Time field
// shadows the embedded one when decoding.
type upstreamMetadata struct {
	simulatorapi.Metadata
	Time *upstreamTime `json:"time"`
}

type upstreamTime struct {
	Now       *time.Time `json:"now"`
	ElapsedMs *int64     `json:"elapsedMs"`
	Speed     *float64   `json:"speed"`
	Paused    *bool      `json:"paused"`
}

// project validates a fleet against metadata from the same run and decodes
// every report. Navigation comes only from the NMEA payloads; names and type
// IDs come from the fleet, type names and the clock from metadata. Any invalid
// vessel fails the whole projection, so a corrupt report never looks like a
// removed vessel. Reads from different runs return errUnavailable; other
// violations errInvalid. Fleet and clock may come from adjacent ticks of one run.
func project(fleet simulatorapi.Fleet, upstream upstreamMetadata) (Fleet, error) {
	metadata := upstream.Metadata
	if err := validateMetadata(metadata); err != nil {
		return Fleet{}, fmt.Errorf("%w: metadata: %w", errInvalid, err)
	}
	clock, err := validateTime(upstream.Time, metadata.StartedAt, metadata.Settings.Speed)
	if err != nil {
		return Fleet{}, fmt.Errorf("%w: metadata: %w", errInvalid, err)
	}
	switch {
	case fleet.SimulationID == "":
		return Fleet{}, fmt.Errorf("%w: fleet: simulationId is required", errInvalid)
	case fleet.SimulationID != metadata.SimulationID:
		return Fleet{}, fmt.Errorf("%w: simulator restarted between reads: fleet run %q, metadata run %q", errUnavailable, fleet.SimulationID, metadata.SimulationID)
	case fleet.UpdatedAt.IsZero():
		return Fleet{}, fmt.Errorf("%w: fleet: updatedAt is required", errInvalid)
	case fleet.Vessels == nil:
		return Fleet{}, fmt.Errorf("%w: fleet: vessels is required", errInvalid)
	}

	typeNames := make(map[string]string, len(metadata.VesselTypes))
	for _, vesselType := range metadata.VesselTypes {
		typeNames[vesselType.ID] = vesselType.Name
	}
	seen := make(map[uint32]bool, len(fleet.Vessels))
	vessels := make([]Vessel, 0, len(fleet.Vessels))
	for _, vessel := range fleet.Vessels {
		projected, err := projectVessel(vessel, typeNames)
		if err != nil {
			return Fleet{}, fmt.Errorf("%w: fleet: vessel %d: %w", errInvalid, vessel.MMSI, err)
		}
		if seen[vessel.MMSI] {
			return Fleet{}, fmt.Errorf("%w: fleet: duplicate MMSI %d", errInvalid, vessel.MMSI)
		}
		seen[vessel.MMSI] = true
		vessels = append(vessels, projected)
	}
	return Fleet{
		SimulationID: fleet.SimulationID,
		UpdatedAt:    fleet.UpdatedAt,
		Time:         clock,
		SpawnBounds:  metadata.Settings.SpawnBounds,
		Vessels:      vessels,
	}, nil
}

func validateMetadata(metadata simulatorapi.Metadata) error {
	if metadata.SimulationID == "" || metadata.StartedAt.IsZero() {
		return fmt.Errorf("simulationId and startedAt are required")
	}
	if len(metadata.VesselTypes) == 0 {
		return fmt.Errorf("vesselTypes is required")
	}
	for _, vesselType := range metadata.VesselTypes {
		if vesselType.ID == "" || vesselType.Name == "" {
			return fmt.Errorf("vessel type %q: id and name are required", vesselType.ID)
		}
	}
	b := metadata.Settings.SpawnBounds
	if b.South < -90 || b.North > 90 || b.South >= b.North || b.West < -180 || b.East > 180 || b.West >= b.East {
		return fmt.Errorf("invalid spawnBounds %+v", b)
	}
	return nil
}

// validateTime checks the metadata clock: all fields present, now not before
// startedAt, nonnegative elapsed time, speed 0 or within limits on a step, and
// paused exactly when speed is 0.
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

// projectVessel decodes the report of one vessel. An unknown type ID is shown
// as the type name.
func projectVessel(vessel simulatorapi.Vessel, typeNames map[string]string) (Vessel, error) {
	if vessel.TypeID == "" {
		return Vessel{}, fmt.Errorf("typeId is required")
	}
	if vessel.Report.Timestamp.IsZero() {
		return Vessel{}, fmt.Errorf("report timestamp is required")
	}
	report, err := ais.DecodePosition(vessel.Report.Sentence)
	if err != nil {
		return Vessel{}, fmt.Errorf("decode report %d: %w", vessel.Report.Sequence, err)
	}
	if report.MMSI != vessel.MMSI {
		return Vessel{}, fmt.Errorf("report %d has MMSI %d", vessel.Report.Sequence, report.MMSI)
	}
	typeName, ok := typeNames[vessel.TypeID]
	if !ok {
		typeName = vessel.TypeID
	}
	return Vessel{
		MMSI: vessel.MMSI, Name: vessel.Name, TypeID: vessel.TypeID, TypeName: typeName,
		Latitude: report.Latitude, Longitude: report.Longitude,
		Speed: report.Speed, Course: report.Course, Heading: report.Heading,
		UpdatedAt: vessel.Report.Timestamp,
	}, nil
}
