package simulator

import (
	simdriver "github.com/miroslav-matejovsky/ais-test-bench/internal/simulation"
	"github.com/miroslav-matejovsky/ais-test-bench/internal/simulatorapi"
	"github.com/miroslav-matejovsky/ais-test-bench/simulation"
)

// The functions below map engine copies to wire types explicitly, so the public
// engine types and the HTTP contract can evolve separately. Leaf structs with
// identical fields use Go struct conversion, which ignores JSON tags and fails
// to compile when the fields diverge. Slices are always non-nil so empty lists
// encode as [], and nil history bounds encode as null.

func fleetResponse(fleet simulation.Fleet) simulatorapi.Fleet {
	vessels := make([]simulatorapi.Vessel, 0, len(fleet.Vessels))
	for _, vessel := range fleet.Vessels {
		vessels = append(vessels, simulatorapi.Vessel{
			MMSI: vessel.MMSI, Name: vessel.Name, TypeID: vessel.TypeID,
			Report: simulatorapi.Report(vessel.Report),
		})
	}
	return simulatorapi.Fleet{
		SimulationID: fleet.SimulationID, UpdatedAt: fleet.UpdatedAt,
		MessageCount: fleet.MessageCount, MessageLimit: fleet.MessageLimit, Vessels: vessels,
	}
}

func historyResponse(history simulation.History) simulatorapi.History {
	messages := make([]simulatorapi.Message, 0, len(history.Messages))
	for _, message := range history.Messages {
		messages = append(messages, simulatorapi.Message(message))
	}
	return simulatorapi.History{
		SimulationID: history.SimulationID, MessageLimit: history.MessageLimit,
		OldestSequence: history.OldestSequence, LatestSequence: history.LatestSequence,
		Messages: messages,
	}
}

// metadataResponse adds the driver's real pacing interval, which the engine does
// not know, and truncates elapsed virtual time to milliseconds.
func metadataResponse(metadata simulation.Metadata) simulatorapi.Metadata {
	types := make([]simulatorapi.VesselType, 0, len(metadata.VesselTypes))
	for _, vesselType := range metadata.VesselTypes {
		types = append(types, simulatorapi.VesselType(vesselType))
	}
	clock, settings := metadata.Time, metadata.Settings
	return simulatorapi.Metadata{
		SimulationID: metadata.SimulationID,
		StartedAt:    metadata.StartedAt,
		Time: simulatorapi.TimeState{
			Now: clock.Now, ElapsedMs: clock.Elapsed.Milliseconds(), Speed: clock.Speed, Paused: clock.Paused,
		},
		VesselTypes:           types,
		SupportedMessageTypes: append([]int{}, metadata.SupportedMessageTypes...),
		Settings: simulatorapi.Settings{
			InitialVesselCount:  settings.InitialVesselCount,
			MaxVessels:          settings.MaxVessels,
			TickIntervalMs:      settings.TickIntervalMs,
			MessageIntervalMs:   settings.MessageIntervalMs,
			PacingIntervalMs:    simdriver.Heartbeat.Milliseconds(),
			MessageHistoryLimit: settings.MessageHistoryLimit,
			SpeedKnots:          simulatorapi.SpeedRange(settings.SpeedKnots),
			Speed:               simulatorapi.SpeedLimits(settings.Speed),
			SpawnBounds:         simulatorapi.SpawnBounds(settings.SpawnBounds),
		},
	}
}
