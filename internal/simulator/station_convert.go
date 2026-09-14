package simulator

import (
	"cmp"
	"slices"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/simulatorapi"
	"github.com/miroslav-matejovsky/ais-test-bench/simulation"
)

func stationSetResponse(set simulation.StationConfiguration) simulatorapi.StationSet {
	result := simulatorapi.StationSet{
		Metadata: metadataResponse(set.Metadata), StateRevision: set.StateRevision,
		StationSetRevision: set.Revision, SnapshotAt: set.Metadata.Time.Now,
		Stations: make([]simulatorapi.Station, 0, len(set.Stations)),
	}
	for _, station := range set.Stations {
		result.Stations = append(result.Stations, stationResponse(station))
	}
	slices.SortFunc(result.Stations, func(a, b simulatorapi.Station) int { return cmp.Compare(a.ID, b.ID) })
	return result
}

func stationResponse(station simulation.Station) simulatorapi.Station {
	d := station.Definition
	sectors := make([]simulatorapi.ShadowSector, 0, len(d.ShadowSectors))
	for _, sector := range d.ShadowSectors {
		sectors = append(sectors, simulatorapi.ShadowSector(sector))
	}
	coverage := make([]simulatorapi.Coverage, 0, len(station.Coverage))
	for _, contour := range station.Coverage {
		coverage = append(coverage, simulatorapi.Coverage{
			Channel: string(contour.Channel), Threshold: contour.Threshold,
			MinRadiusMeters: contour.MinRadiusMeters, MaxRadiusMeters: contour.MaxRadiusMeters,
			Geometry: coverageGeometry(contour.Ring),
		})
	}
	return simulatorapi.Station{
		ID: station.ID, ConfigRevision: station.ConfigRevision, RFRevision: station.RFRevision,
		CreatedAt: station.CreatedAt, RFUpdatedAt: station.RFUpdatedAt, Coverage: coverage,
		Definition: simulatorapi.StationDefinition{
			Name: d.Name, Latitude: d.Latitude, Longitude: d.Longitude, Enabled: d.Enabled,
			AntennaHeightMeters: d.AntennaHeightMeters, ReceiveGainDBi: d.ReceiveGainDBi,
			FeederLossDB: d.FeederLossDB, ChannelA: simulatorapi.ReceiverChannel(d.ChannelA),
			ChannelB: simulatorapi.ReceiverChannel(d.ChannelB), ShadowSectors: sectors,
		},
	}
}

func receptionResponse(r simulation.Reception) simulatorapi.Reception {
	return simulatorapi.Reception{
		StationID: r.StationID, Sequence: r.Sequence, TransmissionSequence: r.TransmissionSequence,
		MMSI: r.MMSI, Channel: string(r.Channel), Timestamp: r.Timestamp, Sentence: r.Sentence,
		VesselName: r.VesselName, VesselTypeID: r.VesselTypeID,
		ConfigRevision: r.ConfigRevision, RFRevision: r.RFRevision,
		Receiver: simulatorapi.ReceiverSnapshot{
			Name:     r.Receiver.Name,
			Latitude: r.Receiver.Latitude, Longitude: r.Receiver.Longitude,
			AntennaHeightMeters: r.Receiver.AntennaHeightMeters, ReceiveGainDBi: r.Receiver.ReceiveGainDBi,
			FeederLossDB: r.Receiver.FeederLossDB, Channel: simulatorapi.ReceiverChannel(r.Receiver.Channel),
		}, Link: simulatorapi.ReceptionLink(r.Link),
	}
}

func observationsResponse(o simulation.Observations) simulatorapi.Observations {
	result := simulatorapi.Observations{
		Metadata: metadataResponse(o.Metadata), StateRevision: o.StateRevision,
		StationSetRevision: o.StationSetRevision, SnapshotAt: o.Metadata.Time.Now,
		Selection: append([]string{}, o.Selection...), CurrentTargets: o.CurrentTargets, LostTargets: o.LostTargets,
		Stations:         make([]simulatorapi.StationObservation, 0, len(o.Stations)),
		Targets:          make([]simulatorapi.ObservedTarget, 0, len(o.Targets)),
		RecentReceptions: make([]simulatorapi.Reception, 0, len(o.RecentReceptions)), RecentIsSample: true,
		Transmissions: o.Transmissions, ReceivedTransmissions: o.ReceivedTransmissions,
		Receptions: o.Receptions, TargetEvictions: o.TargetEvictions,
	}
	slices.Sort(result.Selection)
	for _, s := range o.Stations {
		c, r := s.Counters, s.Recent
		result.Stations = append(result.Stations, simulatorapi.StationObservation{
			Station: stationResponse(s.Station), ReceiveRatio: s.ReceiveRatio,
			OldestReception: s.OldestReception, LatestReception: s.LatestReception,
			CurrentTargets: s.CurrentTargets, LostTargets: s.LostTargets,
			Counters: simulatorapi.ReceptionCounters{
				Opportunities: c.Opportunities, Received: c.Received, StationDisabled: c.StationDisabled,
				ChannelDisabled: c.ChannelDisabled, OutsideHorizon: c.OutsideHorizon,
				InsufficientMargin: c.InsufficientMargin, ProbabilisticLoss: c.ProbabilisticLoss,
				ChannelA: simulatorapi.ChannelCounters(c.ChannelA), ChannelB: simulatorapi.ChannelCounters(c.ChannelB),
			},
			Recent: simulatorapi.RecentCounters{
				DurationMs: r.Duration.Milliseconds(), Opportunities: r.Opportunities, Received: r.Received,
				OpportunityRate: r.OpportunityRate, ReceptionRate: r.ReceptionRate, ReceiveRatio: r.ReceiveRatio,
			},
		})
	}
	slices.SortFunc(result.Stations, func(a, b simulatorapi.StationObservation) int { return cmp.Compare(a.ID, b.ID) })
	for _, t := range o.Targets {
		stations := make([]simulatorapi.TargetStation, 0, len(t.Stations))
		for _, s := range t.Stations {
			stations = append(stations, simulatorapi.TargetStation{
				StationID: s.StationID, StationEnabled: s.StationEnabled, Sequence: s.Sequence,
				TransmissionSequence: s.TransmissionSequence, Timestamp: s.Timestamp, Channel: string(s.Channel),
				ReceivedPowerDBm: s.ReceivedPowerDBm, RFRevision: s.RFRevision,
				AgeMs: s.Age.Milliseconds(), Status: string(s.Status), Chosen: s.Chosen,
			})
		}
		slices.SortFunc(stations, func(a, b simulatorapi.TargetStation) int { return cmp.Compare(a.StationID, b.StationID) })
		result.Targets = append(result.Targets, simulatorapi.ObservedTarget{
			MMSI: t.MMSI, Report: receptionResponse(t.Report), AgeMs: t.Age.Milliseconds(), Status: string(t.Status), Stations: stations,
		})
	}
	for _, r := range o.RecentReceptions {
		result.RecentReceptions = append(result.RecentReceptions, receptionResponse(r))
	}
	slices.SortFunc(result.RecentReceptions, func(a, b simulatorapi.Reception) int {
		return cmp.Or(a.Timestamp.Compare(b.Timestamp), cmp.Compare(a.TransmissionSequence, b.TransmissionSequence), cmp.Compare(a.StationID, b.StationID))
	})
	return result
}

func receptionPageResponse(page simulation.ReceptionPage) simulatorapi.ReceptionPage {
	result := simulatorapi.ReceptionPage{
		SimulationID: page.SimulationID, StationID: page.StationID, Tail: page.Tail,
		OldestAvailable: page.OldestSequence, LatestAvailable: page.LatestSequence,
		NextAfter: page.NextAfter, HasMore: page.HasMore, Gap: page.Gap,
		Receptions: make([]simulatorapi.Reception, 0, len(page.Receptions)),
	}
	if page.OldestSequence != nil && *page.OldestSequence > 1 {
		result.TruncatedBefore = page.OldestSequence
	}
	for _, r := range page.Receptions {
		result.Receptions = append(result.Receptions, receptionResponse(r))
	}
	return result
}
