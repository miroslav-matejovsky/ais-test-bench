package simulation_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-testbench/simulation"
)

// receivers returns stations with distinct outcomes for vessels in the spawn
// area: one always receives, one is marginal near its horizon, one drops every
// channel B report, and one is disabled.
func receivers() []simulation.StationDefinition {
	marginal := site("Marginal")
	marginal.Latitude = 52.28
	dropB := site("Drop B")
	dropB.ChannelB.DropProbability = 1
	disabled := site("Disabled")
	disabled.Enabled = false
	return []simulation.StationDefinition{site("Near"), marginal, dropB, disabled}
}

func observations(t *testing.T, s *simulation.Simulator, stationIDs ...string) simulation.Observations {
	t.Helper()
	result, err := s.Observations(stationIDs)
	require.NoError(t, err)
	return result
}

func receptions(t *testing.T, s *simulation.Simulator, stationID string, after *uint64, limit int) simulation.ReceptionPage {
	t.Helper()
	page, err := s.ReceptionHistory(stationID, after, limit)
	require.NoError(t, err)
	return page
}

func sequences(page simulation.ReceptionPage) []uint64 {
	result := make([]uint64, 0, len(page.Receptions))
	for _, r := range page.Receptions {
		result = append(result, r.Sequence)
	}
	return result
}

// advanceTo advances s in steps of at most MaxAdvance to virtual elapsed time d.
func advanceTo(t *testing.T, s *simulation.Simulator, d time.Duration) {
	t.Helper()
	for elapsed := s.Metadata().Time.Elapsed; elapsed < d; elapsed = s.Metadata().Time.Elapsed {
		runSteps(t, s, advance(min(simulation.MaxAdvance, d-elapsed)))
	}
	require.Equal(t, d, s.Metadata().Time.Elapsed)
}

func TestZeroStationsReceiveNothing(t *testing.T) {
	s := newSimulator(t, testConfig(1, 3))
	runSteps(t, s, advance(2*time.Second))

	got := observations(t, s)
	require.Equal(t, s.Metadata(), got.Metadata)
	require.Equal(t, uint64(9), got.Transmissions)
	require.Zero(t, got.ReceivedTransmissions)
	require.Zero(t, got.Receptions)
	for _, empty := range []any{got.Selection, got.Stations, got.Targets, got.RecentReceptions} {
		require.NotNil(t, empty)
		require.Empty(t, empty)
	}
	_, err := s.Observations([]string{"station-1"})
	require.ErrorIs(t, err, simulation.ErrNotFound)
}

func TestReceptionIdentities(t *testing.T) {
	far, disabled := site("Far"), site("Disabled")
	far.Latitude = 60
	disabled.Enabled = false
	config := testConfig(3, 2)
	config.Stations = []simulation.StationDefinition{site("First"), site("Second"), far, disabled}
	s := newSimulator(t, config)
	runSteps(t, s, advance(time.Second))
	messages := s.History().Messages
	require.Len(t, messages, 4)
	got := observations(t, s)

	// Four transmissions, each received at two stations.
	require.Equal(t, []string{"station-1", "station-2", "station-3", "station-4"}, got.Selection)
	require.Equal(t, uint64(4), got.Transmissions)
	require.Equal(t, uint64(4), got.ReceivedTransmissions)
	require.Equal(t, uint64(8), got.Receptions)
	half := simulation.ChannelCounters{Opportunities: 2, Received: 2}
	missed := simulation.ChannelCounters{Opportunities: 2}
	for i, want := range []simulation.ReceptionCounters{
		{Opportunities: 4, Received: 4, ChannelA: half, ChannelB: half},
		{Opportunities: 4, Received: 4, ChannelA: half, ChannelB: half},
		{Opportunities: 4, OutsideHorizon: 4, ChannelA: missed, ChannelB: missed},
		{Opportunities: 4, StationDisabled: 4, ChannelA: missed, ChannelB: missed},
	} {
		station := got.Stations[i]
		require.Equal(t, want, station.Counters, station.Station.ID)
		require.InDelta(t, float64(want.Received)/4, *station.ReceiveRatio, 0)
		require.Equal(t, time.Second, station.Recent.Duration)
		require.Equal(t, uint64(4), station.Recent.Opportunities)
		require.InDelta(t, 4, *station.Recent.OpportunityRate, 0)
	}
	require.Equal(t, uint64(1), *got.Stations[0].OldestReception)
	require.Equal(t, uint64(4), *got.Stations[0].LatestReception)
	require.Nil(t, got.Stations[2].OldestReception)
	require.Nil(t, got.Stations[2].LatestReception)

	names := map[uint32]string{}
	for _, vessel := range s.Fleet().Vessels {
		names[vessel.MMSI] = vessel.Name
	}
	page := receptions(t, s, "station-2", nil, simulation.ReceptionHistoryLimit)
	require.Len(t, page.Receptions, 4)
	for i, r := range page.Receptions {
		message := messages[i]
		require.Equal(t, "station-2", r.StationID)
		require.Equal(t, uint64(i+1), r.Sequence)
		require.Equal(t, message.Sequence, r.TransmissionSequence)
		require.Equal(t, message.MMSI, r.MMSI)
		require.Equal(t, message.Timestamp, r.Timestamp)
		require.Equal(t, message.Sentence, r.Sentence, "every receiver keeps the transmitted bytes")
		require.Equal(t, simulation.Channel(decode(t, message.Sentence).Channel), r.Channel)
		require.Equal(t, names[r.MMSI], r.VesselName)
		require.Equal(t, "cargo", r.VesselTypeID)
		require.Equal(t, uint64(1), r.ConfigRevision)
		require.Equal(t, uint64(1), r.RFRevision)
		require.Equal(t, simulation.ReceiverSnapshot{
			Name:     "Second",
			Latitude: 52, Longitude: 4, AntennaHeightMeters: 25, ReceiveGainDBi: 3, FeederLossDB: 2, Channel: site("").ChannelA,
		}, r.Receiver)
		require.InDelta(t, 1, r.Link.Probability, 0)
		require.Positive(t, r.Link.MarginDB)
		require.NotNil(t, r.Link.BearingDegrees)
		require.Less(t, r.Link.DistanceMeters, 6000.0)
	}

	// One marker per MMSI, from the first station among equal receptions.
	require.Len(t, got.Targets, 2)
	require.Equal(t, 2, got.CurrentTargets)
	require.Zero(t, got.LostTargets)
	for i, target := range got.Targets {
		latest := messages[2+i]
		require.Equal(t, latest.MMSI, target.MMSI)
		require.Equal(t, latest.Sequence, target.Report.TransmissionSequence)
		require.Equal(t, "station-1", target.Report.StationID)
		require.Equal(t, simulation.TargetFresh, target.Status)
		require.Zero(t, target.Age)
		require.Len(t, target.Stations, 2)
		for j, observed := range target.Stations {
			require.Equal(t, fmt.Sprintf("station-%d", j+1), observed.StationID)
			require.Equal(t, latest.Sequence, observed.TransmissionSequence)
			require.True(t, observed.Chosen)
			require.True(t, observed.StationEnabled)
		}
	}
	require.Equal(t, 2, got.Stations[1].CurrentTargets)
	require.Zero(t, got.Stations[2].CurrentTargets)

	require.Len(t, got.RecentReceptions, 8)
	for i, r := range got.RecentReceptions {
		require.Equal(t, messages[i/2].Sequence, r.TransmissionSequence)
		require.Equal(t, fmt.Sprintf("station-%d", i%2+1), r.StationID)
	}
}

func TestMissedReportKeepsLastReceivedPosition(t *testing.T) {
	dropB := site("Drop B")
	dropB.ChannelB.DropProbability = 1
	config := testConfig(5, 1)
	config.Stations = []simulation.StationDefinition{dropB, site("Full")}
	s := newSimulator(t, config)
	// MMSI 200000000 reports on channel A at creation, then on B.
	runSteps(t, s, advance(time.Second), setCount(0))
	messages := s.History().Messages
	require.Equal(t, simulation.ChannelA, simulation.Channel(decode(t, messages[0].Sentence).Channel))
	require.Empty(t, s.Fleet().Vessels)

	only := observations(t, s, "station-1")
	require.Equal(t, []string{"station-1"}, only.Selection)
	require.Len(t, only.Targets, 1, "the target outlives its vessel")
	target := only.Targets[0]
	require.Equal(t, messages[0].Sentence, target.Report.Sentence, "the missed report does not move the target")
	require.Equal(t, time.Second, target.Age)
	require.Equal(t, uint64(1), only.Stations[0].Counters.ProbabilisticLoss)

	all := observations(t, s)
	target = all.Targets[0]
	require.Equal(t, messages[1].Sentence, target.Report.Sentence)
	require.Equal(t, "station-2", target.Report.StationID)
	require.Len(t, target.Stations, 2)
	first, second := target.Stations[0], target.Stations[1]
	require.Equal(t, uint64(1), first.TransmissionSequence)
	require.Equal(t, start, first.Timestamp)
	require.Equal(t, time.Second, first.Age)
	require.False(t, first.Chosen, "station-1 did not receive the chosen report")
	require.Equal(t, uint64(2), second.TransmissionSequence)
	require.True(t, second.Chosen)

	require.Equal(t, []string{"station-1", "station-2"}, observations(t, s, "station-2", "station-1", "station-2").Selection)
	_, err := s.Observations([]string{"station-1", "station-3"})
	require.ErrorIs(t, err, simulation.ErrNotFound)
}

func TestObservationAging(t *testing.T) {
	config := testConfig(2, 1)
	config.Stations = []simulation.StationDefinition{site("Site")}
	s := newSimulator(t, config)
	runSteps(t, s, setCount(0)) // One reception at the start instant, then no traffic.

	for _, tt := range []struct {
		age    time.Duration
		status simulation.TargetStatus
	}{
		{age: 0, status: simulation.TargetFresh},
		{age: simulation.FreshAge, status: simulation.TargetFresh},
		{age: simulation.FreshAge + 1, status: simulation.TargetStale},
		{age: simulation.StaleAge, status: simulation.TargetStale},
		{age: simulation.StaleAge + 1, status: simulation.TargetLost},
		{age: simulation.ExpiryAge - 1, status: simulation.TargetLost},
	} {
		advanceTo(t, s, tt.age)
		got := observations(t, s)
		require.Equal(t, got, observations(t, s), "reads change nothing")
		require.Len(t, got.Targets, 1, tt.age)
		require.Equal(t, tt.age, got.Targets[0].Age)
		require.Equal(t, tt.status, got.Targets[0].Status)
		require.Equal(t, tt.status, got.Targets[0].Stations[0].Status)
		lost := tt.status == simulation.TargetLost
		require.Equal(t, map[bool]int{false: 1, true: 0}[lost], got.CurrentTargets)
		require.Equal(t, map[bool]int{false: 0, true: 1}[lost], got.LostTargets)
		require.Equal(t, got.CurrentTargets, got.Stations[0].CurrentTargets)
		require.Equal(t, got.LostTargets, got.Stations[0].LostTargets)
	}

	// Pause freezes age.
	runSteps(t, s, setSpeed(0), elapse(time.Hour))
	require.Equal(t, simulation.ExpiryAge-1, observations(t, s).Targets[0].Age)

	runSteps(t, s, advance(1))
	got := observations(t, s)
	require.Empty(t, got.Targets)
	require.Zero(t, got.CurrentTargets+got.LostTargets+got.Stations[0].CurrentTargets+got.Stations[0].LostTargets)
	require.Zero(t, got.TargetEvictions, "expiry is not eviction")
	require.Equal(t, []uint64{1}, sequences(receptions(t, s, "station-1", nil, 10)), "expiry keeps history")
}

func TestStationEditsBetweenTicks(t *testing.T) {
	ms := time.Millisecond
	config := testConfig(4, 1)
	config.Stations = []simulation.StationDefinition{site("Edited"), site("Other")}
	s := newSimulator(t, config)
	runSteps(t, s, advance(1500*ms))

	disabled := site("Edited")
	disabled.Enabled = false
	_, err := s.UpdateStation(1, "station-1", disabled)
	require.NoError(t, err)
	runSteps(t, s, advance(2*time.Second))
	got := observations(t, s, "station-1")
	counters := got.Stations[0].Counters
	require.Equal(t, uint64(4), counters.Opportunities, "disabled stations count opportunities")
	require.Equal(t, uint64(2), counters.Received)
	require.Equal(t, uint64(2), counters.StationDisabled)
	require.Equal(t, start.Add(1500*ms), got.Stations[0].Station.RFUpdatedAt)
	target := got.Targets[0]
	require.Equal(t, uint64(2), target.Report.TransmissionSequence, "disabled stations keep observations")
	require.Equal(t, 2500*ms, target.Age)
	require.False(t, target.Stations[0].StationEnabled)

	_, err = s.UpdateStation(2, "station-1", site("Edited"))
	require.NoError(t, err)
	runSteps(t, s, advance(time.Second))
	page := receptions(t, s, "station-1", nil, 10)
	require.Equal(t, []uint64{1, 2, 3}, sequences(page), "reception sequences stay contiguous")
	for i, want := range []struct{ transmission, rf uint64 }{{1, 1}, {2, 1}, {5, 3}} {
		require.Equal(t, want.transmission, page.Receptions[i].TransmissionSequence)
		require.Equal(t, want.rf, page.Receptions[i].RFRevision)
	}

	// Removal drops the station's state but not the run totals.
	_, err = s.RemoveStation(3, "station-1")
	require.NoError(t, err)
	got = observations(t, s)
	require.Equal(t, []string{"station-2"}, got.Selection)
	require.Len(t, got.Targets[0].Stations, 1)
	require.Equal(t, uint64(8), got.Receptions)
	_, err = s.ReceptionHistory("station-1", nil, 10)
	require.ErrorIs(t, err, simulation.ErrNotFound)
	_, err = s.Observations([]string{"station-1"})
	require.ErrorIs(t, err, simulation.ErrNotFound)

	// A new station starts empty and receives no earlier report.
	id, _, err := s.AddStation(4, site("Edited"))
	require.NoError(t, err)
	require.Equal(t, "station-3", id)
	_, err = s.RemoveStation(5, "station-2")
	require.NoError(t, err)
	got = observations(t, s)
	require.Empty(t, got.Targets, "a target without observations is removed")
	added := got.Stations[0]
	require.Equal(t, simulation.ReceptionCounters{}, added.Counters)
	require.Nil(t, added.ReceiveRatio)
	require.Equal(t, simulation.RecentCounters{}, added.Recent, "rates are undefined at the creation instant")
	require.Nil(t, added.LatestReception)

	runSteps(t, s, advance(time.Second))
	got = observations(t, s)
	require.Equal(t, uint64(1), got.Stations[0].Counters.Received)
	require.Equal(t, time.Second, got.Stations[0].Recent.Duration)
	require.Equal(t, uint64(6), got.Targets[0].Report.TransmissionSequence)
	require.Equal(t, uint64(1), got.Targets[0].Report.Sequence)
}

func TestReceptionHistoryPaging(t *testing.T) {
	far := site("Far")
	far.Latitude = 60
	config := testConfig(6, simulation.MaxVessels)
	config.Stations = []simulation.StationDefinition{site("Site"), far}
	s := newSimulator(t, config)
	// 100 creation reports and 11 ticks: 1,200 receptions, the oldest 200 evicted.
	runSteps(t, s, advance(11*time.Second))

	tail := receptions(t, s, "station-1", nil, 10)
	require.True(t, tail.Tail)
	require.Equal(t, runID, tail.SimulationID)
	require.Equal(t, "station-1", tail.StationID)
	require.Equal(t, uint64(201), *tail.OldestSequence)
	require.Equal(t, uint64(1200), *tail.LatestSequence)
	require.Equal(t, []uint64{1191, 1192, 1193, 1194, 1195, 1196, 1197, 1198, 1199, 1200}, sequences(tail))
	require.Equal(t, uint64(1200), tail.NextAfter)
	require.False(t, tail.HasMore)
	require.False(t, tail.Gap)

	for _, tt := range []struct {
		name      string
		after     uint64
		limit     int
		want      []uint64
		next      uint64
		more, gap bool
	}{
		{name: "evicted cursor", after: 0, limit: 3, want: []uint64{201, 202, 203}, next: 203, more: true, gap: true},
		{name: "one evicted", after: 199, limit: 1, want: []uint64{201}, next: 201, more: true, gap: true},
		{name: "oldest next", after: 200, limit: 1, want: []uint64{201}, next: 201, more: true},
		{name: "inside", after: 1000, limit: 2, want: []uint64{1001, 1002}, next: 1002, more: true},
		{name: "last page", after: 1198, limit: 5, want: []uint64{1199, 1200}, next: 1200},
		{name: "up to date", after: 1200, limit: 5, want: []uint64{}, next: 1200},
	} {
		t.Run(tt.name, func(t *testing.T) {
			page := receptions(t, s, "station-1", new(tt.after), tt.limit)
			require.False(t, page.Tail)
			require.Equal(t, tt.want, sequences(page))
			require.Equal(t, tt.next, page.NextAfter)
			require.Equal(t, tt.more, page.HasMore)
			require.Equal(t, tt.gap, page.Gap)
		})
	}
	require.Len(t, receptions(t, s, "station-1", new(uint64(200)), simulation.ReceptionHistoryLimit).Receptions, simulation.ReceptionHistoryLimit)

	for _, tt := range []struct {
		station string
		after   *uint64
		limit   int
		want    error
	}{
		{station: "station-1", after: new(uint64(1201)), limit: 1, want: simulation.ErrInvalid},
		{station: "station-1", limit: 0, want: simulation.ErrInvalid},
		{station: "station-1", limit: simulation.ReceptionHistoryLimit + 1, want: simulation.ErrInvalid},
		{station: "station-2", after: new(uint64(1)), limit: 1, want: simulation.ErrInvalid},
		{station: "station-9", limit: 1, want: simulation.ErrNotFound},
	} {
		_, err := s.ReceptionHistory(tt.station, tt.after, tt.limit)
		require.ErrorIs(t, err, tt.want)
	}

	for _, after := range []*uint64{nil, new(uint64(0))} {
		never := receptions(t, s, "station-2", after, 10)
		require.Nil(t, never.OldestSequence)
		require.Nil(t, never.LatestSequence)
		require.NotNil(t, never.Receptions)
		require.Empty(t, never.Receptions)
		require.Zero(t, never.NextAfter)
		require.False(t, never.Gap)
	}

	sentence, bearing := tail.Receptions[0].Sentence, *tail.Receptions[0].Link.BearingDegrees
	tail.Receptions[0].Sentence = "changed"
	*tail.Receptions[0].Link.BearingDegrees = 999
	*tail.OldestSequence = 1
	again := receptions(t, s, "station-1", nil, 10)
	require.Equal(t, sentence, again.Receptions[0].Sentence)
	require.InDelta(t, bearing, *again.Receptions[0].Link.BearingDegrees, 0)
	require.Equal(t, uint64(201), *again.OldestSequence)
}

func TestTargetCapacityEviction(t *testing.T) {
	config := testConfig(7, 0)
	config.Stations = []simulation.StationDefinition{site("First"), site("Second")}
	s := newSimulator(t, config)
	// Ten groups of 100 new MMSIs, received one virtual second apart.
	for range 10 {
		runSteps(t, s, setCount(simulation.MaxVessels), setCount(0), advance(time.Second))
	}
	got := observations(t, s)
	require.Len(t, got.Targets, simulation.TargetLimit)
	require.Zero(t, got.TargetEvictions)

	// A new MMSI evicts the lowest MMSI among the oldest targets at both stations.
	runSteps(t, s, setCount(1), setCount(0))
	got = observations(t, s)
	require.Len(t, got.Targets, simulation.TargetLimit)
	require.Equal(t, uint64(1), got.TargetEvictions)
	require.Equal(t, uint32(200000001), got.Targets[0].MMSI)
	require.Equal(t, uint32(200001000), got.Targets[len(got.Targets)-1].MMSI)
	for _, station := range got.Stations {
		require.Equal(t, simulation.TargetLimit, station.CurrentTargets+station.LostTargets)
	}

	// The rest of the oldest group goes first, then the lowest MMSI of the next.
	runSteps(t, s, advance(time.Second), setCount(simulation.MaxVessels), setCount(0))
	got = observations(t, s)
	require.Len(t, got.Targets, simulation.TargetLimit)
	require.Equal(t, uint64(101), got.TargetEvictions)
	require.Equal(t, uint32(200000101), got.Targets[0].MMSI)

	page := receptions(t, s, "station-1", nil, 1)
	require.Equal(t, uint64(1101), *page.LatestSequence, "eviction keeps history")
	require.Equal(t, uint64(102), *page.OldestSequence)
}

// TestExpiryBeforeCapacityIsSplitIndependent checks that a call expires targets
// before recording a later reception, as a split call would have.
func TestExpiryBeforeCapacityIsSplitIndependent(t *testing.T) {
	ms := time.Millisecond
	disabled := site("Site")
	disabled.Enabled = false
	prepare := func() *simulation.Simulator {
		config := testConfig(8, 0)
		config.Stations = []simulation.StationDefinition{site("Site")}
		s := newSimulator(t, config)
		runSteps(t, s, advance(250*ms))
		for range 10 {
			runSteps(t, s, setCount(simulation.MaxVessels), setCount(0))
		}
		_, err := s.UpdateStation(1, "station-1", disabled)
		require.NoError(t, err)
		// This vessel is new to the full store when the station is enabled again.
		runSteps(t, s, setCount(1))
		advanceTo(t, s, 600100*ms)
		_, err = s.UpdateStation(2, "station-1", site("Site"))
		require.NoError(t, err)
		require.Len(t, observations(t, s).Targets, simulation.TargetLimit)
		return s
	}

	combined, split := prepare(), prepare()
	want := runSteps(t, combined, advance(900*ms))
	require.Equal(t, want, runSteps(t, split, advance(400*ms), advance(500*ms)))
	require.Equal(t, observeOutcome(t, combined), observeOutcome(t, split))
	got := observations(t, combined)
	require.Zero(t, got.TargetEvictions, "the old targets expired before the new reception")
	require.Len(t, got.Targets, 1)
}

func TestCancellationBeforeCommitRollsBack(t *testing.T) {
	config := testConfig(6, 3)
	config.Stations = receivers()
	s, control := newSimulator(t, config), newSimulator(t, config)
	before := observe(t, s)

	// Five ticks pass the check before the first tick and the four between ticks;
	// the check after the last station evaluation cancels.
	reports, err := s.Advance(&cancelAfter{Context: t.Context(), checks: 5}, 5*time.Second)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, reports)
	require.Equal(t, before, observe(t, s))

	steps := []step{advance(5 * time.Second)}
	require.Equal(t, runSteps(t, control, steps...), runSteps(t, s, steps...))
	require.Equal(t, observe(t, control), observe(t, s))
}

func TestStateRevisions(t *testing.T) {
	config := testConfig(1, 1)
	config.Stations = []simulation.StationDefinition{site("Site")}
	s := newSimulator(t, config)
	requireRevisions := func(state, stations uint64) {
		t.Helper()
		got := observations(t, s)
		require.Equal(t, state, got.StateRevision)
		require.Equal(t, stations, got.StationSetRevision)
	}
	requireRevisions(1, 1)

	s.Fleet()
	s.History()
	runSteps(t, s, setCount(1), setSpeed(1), advance(0), elapse(0))
	requireRevisions(1, 1)
	runSteps(t, s, setSpeed(0.33))
	requireRevisions(2, 1)
	runSteps(t, s, elapse(1))
	requireRevisions(2, 1)
	runSteps(t, s, advance(500*time.Millisecond))
	requireRevisions(3, 1)
	runSteps(t, s, setCount(2))
	requireRevisions(4, 1)

	_, err := s.UpdateStation(1, "station-1", site("Renamed"))
	require.NoError(t, err)
	requireRevisions(5, 2)
	_, err = s.UpdateStation(2, "station-1", site("Renamed"))
	require.NoError(t, err)
	_, err = s.UpdateStation(1, "station-1", site("Stale"))
	require.ErrorIs(t, err, simulation.ErrConflict)
	requireRevisions(5, 2)
}

func TestReceptionCountersAndRates(t *testing.T) {
	config := testConfig(8, 2)
	config.Stations = receivers()
	s := newSimulator(t, config)
	runSteps(t, s, advance(30*time.Second))
	near := observations(t, s).Stations[0]
	require.Equal(t, 30*time.Second, near.Recent.Duration)
	require.Equal(t, uint64(62), near.Recent.Opportunities, "creation reports and 30 ticks")
	require.InDelta(t, 62.0/30, *near.Recent.OpportunityRate, 1e-12)

	advanceTo(t, s, 330500*time.Millisecond)
	got := observations(t, s)
	for _, station := range got.Stations {
		c := station.Counters
		require.Equal(t, uint64(662), c.Opportunities)
		require.Equal(t, c.Opportunities, c.Received+c.StationDisabled+c.ChannelDisabled+c.OutsideHorizon+c.InsufficientMargin+c.ProbabilisticLoss)
		require.Equal(t, c.Opportunities, c.ChannelA.Opportunities+c.ChannelB.Opportunities)
		require.Equal(t, c.Received, c.ChannelA.Received+c.ChannelB.Received)
		require.Equal(t, simulation.RateWindow, station.Recent.Duration)
		require.Equal(t, uint64(120), station.Recent.Opportunities, "exactly the ticks of the last minute")
		require.InDelta(t, 2, *station.Recent.OpportunityRate, 1e-12)
	}
	near, dropB, disabled := got.Stations[0], got.Stations[2], got.Stations[3]
	require.Equal(t, uint64(120), near.Recent.Received)
	require.InDelta(t, 1, *near.Recent.ReceiveRatio, 0)
	require.InDelta(t, 2, *near.Recent.ReceptionRate, 1e-12)
	require.Zero(t, dropB.Counters.ChannelB.Received)
	require.Equal(t, dropB.Counters.ChannelA.Opportunities, dropB.Counters.ChannelA.Received)
	require.Equal(t, dropB.Counters.ChannelB.Opportunities, dropB.Counters.ProbabilisticLoss)
	require.Equal(t, disabled.Counters.Opportunities, disabled.Counters.StationDisabled)
	require.InDelta(t, 0, *disabled.ReceiveRatio, 0)
	require.Equal(t, uint64(662), got.ReceivedTransmissions)
	require.Equal(t, uint64(662), got.Transmissions)
}

func TestRecentReceptionsSampleSelection(t *testing.T) {
	config := testConfig(9, 30)
	config.Stations = receivers()
	s := newSimulator(t, config)
	runSteps(t, s, advance(3*time.Second))

	got := observations(t, s, "station-3", "station-1")
	require.Len(t, got.RecentReceptions, simulation.RecentReceptionLimit)
	last := got.RecentReceptions[len(got.RecentReceptions)-1]
	require.Equal(t, uint64(120), last.TransmissionSequence)
	for i, r := range got.RecentReceptions {
		require.Contains(t, []string{"station-1", "station-3"}, r.StationID)
		if i > 0 {
			previous := got.RecentReceptions[i-1]
			require.True(t, previous.TransmissionSequence < r.TransmissionSequence ||
				previous.TransmissionSequence == r.TransmissionSequence && previous.StationID < r.StationID)
		}
	}
}

func TestObservationsAreDetached(t *testing.T) {
	config := testConfig(3, 2)
	config.Stations = receivers()
	s := newSimulator(t, config)
	runSteps(t, s, advance(time.Second))
	got := observations(t, s)
	want := struct {
		selection, sentence, provenance string
		bearing, latitude               float64
		latest                          uint64
	}{
		got.Selection[0], got.RecentReceptions[0].Sentence, got.Targets[0].Stations[0].StationID,
		*got.Targets[0].Report.Link.BearingDegrees, got.Stations[0].Station.Coverage[0].Ring[0].Latitude,
		*got.Stations[0].LatestReception,
	}

	got.Selection[0] = "changed"
	got.RecentReceptions[0].Sentence = "changed"
	got.Targets[0].Stations[0].StationID = "changed"
	*got.Targets[0].Report.Link.BearingDegrees = 999
	got.Stations[0].Station.Coverage[0].Ring[0].Latitude = 99
	*got.Stations[0].LatestReception = 99
	got.Metadata.VesselTypes[0].Name = "changed"

	again := observations(t, s)
	require.Equal(t, want.selection, again.Selection[0])
	require.Equal(t, want.sentence, again.RecentReceptions[0].Sentence)
	require.Equal(t, want.provenance, again.Targets[0].Stations[0].StationID)
	require.InDelta(t, want.bearing, *again.Targets[0].Report.Link.BearingDegrees, 0)
	require.InDelta(t, want.latitude, again.Stations[0].Station.Coverage[0].Ring[0].Latitude, 0)
	require.Equal(t, want.latest, *again.Stations[0].LatestReception)
	require.Equal(t, "Cargo vessel", again.Metadata.VesselTypes[0].Name)
}
