package simulation

import (
	"cmp"
	"fmt"
	"maps"
	"math"
	"slices"
	"time"
)

// Reception state limits and target ages, published in
// Metadata.Settings.Observation. Ages are virtual time.
const (
	// ReceptionHistoryLimit is the number of newest receptions kept per station.
	ReceptionHistoryLimit = 1000
	// TargetLimit is the number of distinct MMSIs in the observation store.
	TargetLimit = 1000
	// RecentReceptionLimit is the size of Observations.RecentReceptions.
	RecentReceptionLimit = 50
	// FreshAge is the largest age of a fresh observation.
	FreshAge = 10 * time.Second
	// StaleAge is the largest age of a stale observation; older ones are lost.
	StaleAge = 60 * time.Second
	// ExpiryAge is the age at which an observation leaves the store.
	ExpiryAge = 600 * time.Second
	// RateWindow is the longest span of RecentCounters.
	RateWindow = 60 * time.Second
)

// rateBuckets is the number of one-second buckets in RateWindow.
const rateBuckets = int64(RateWindow / time.Second)

// receptionStore is all reception state: per-station counters and history,
// latest observations per MMSI, and run totals. It changes only when a
// mutation commits, and applying a staged batch cannot fail. Reception records
// are immutable once staged; history and targets share them.
type receptionStore struct {
	stations              map[string]*stationReceptions // Exactly the configured stations, by ID.
	targets               map[uint32]*target
	receptions            uint64 // Successful receptions at all stations, including removed ones.
	receivedTransmissions uint64 // Transmissions received by at least one station.
	evictions             uint64 // Targets removed at TargetLimit.
}

// stationReceptions is the reception state of one station.
type stationReceptions struct {
	stats   stationStats
	history []*Reception // Ring of at most ReceptionHistoryLimit records with contiguous sequences.
	oldest  int          // Index of the oldest record; 0 until the ring is full.
}

// stationStats are the station counters, staged by value.
type stationStats struct {
	lastReception uint64 // Last allocated reception sequence; 0 before the first.
	counters      ReceptionCounters
	buckets       [rateBuckets]rateBucket // Indexed by virtual second modulo rateBuckets.
}

// rateBucket counts the events of one virtual second after the start instant.
// A bucket whose second is outside the rate window is stale.
type rateBucket struct {
	second        int64
	opportunities uint64
	received      uint64
}

// target holds the latest observation of one MMSI at each station that has one.
type target struct {
	observations []*Reception // At most one per station.
	receivedAt   time.Time    // Newest observation timestamp.
}

// transmission is one emitted report with the data reception evaluation needs.
type transmission struct {
	Message
	channel             Channel
	latitude, longitude float64
	name, typeID        string
}

// receptionBatch stages the reception results of one mutation without changing
// the store: counters by value, new records, and run totals. Every limit is
// checked while staging.
type receptionBatch struct {
	stats                 []stationStats // Parallel to the staged stations.
	records               []*Reception   // In transmission sequence, then station order.
	receptions            uint64
	receivedTransmissions uint64
}

// newReceptionStore returns an empty store for stations.
func newReceptionStore(stations []stationState) receptionStore {
	store := receptionStore{stations: make(map[string]*stationReceptions, len(stations)), targets: make(map[uint32]*target)}
	for _, station := range stations {
		store.stations[station.id] = &stationReceptions{}
	}
	return store
}

// stageReceptions starts a batch from the committed store. Called with the
// lock held.
func (s *Simulator) stageReceptions(stations []stationState) receptionBatch {
	stats := make([]stationStats, len(stations))
	for i, station := range stations {
		stats[i] = s.store.stations[station.id].stats
	}
	return receptionBatch{stats: stats, receptions: s.store.receptions, receivedTransmissions: s.store.receivedTransmissions}
}

// receive evaluates t at every station in creation order, counts each outcome,
// and stages a record for each successful reception. An exhausted reception
// sequence or total wraps ErrLimit. Called with the lock held.
func (s *Simulator) receive(batch *receptionBatch, stations []stationState, t transmission) error {
	second := int64(t.Timestamp.Sub(s.start) / time.Second)
	received := false
	for i := range stations {
		station := &stations[i]
		l := evaluateLink(s.transmitter, t.latitude, t.longitude, station.definition, t.channel)
		// A draw only matters for a positive probability; skip the hash otherwise.
		draw := 1.0
		if l.probability > 0 {
			draw = receptionDraw(s.seed, station.id, station.rfRevision, t.Sequence)
		}
		result := decide(station.definition, t.channel, l, draw)
		stats := &batch.stats[i]
		stats.count(result, t.channel, second)
		if result != outcomeReceived {
			continue
		}
		if stats.lastReception == math.MaxUint64 || batch.receptions == math.MaxUint64 {
			return fmt.Errorf("%w: reception sequence of station %s or reception total exhausted", ErrLimit, station.id)
		}
		stats.lastReception++
		batch.receptions++
		received = true
		batch.records = append(batch.records, newReception(station, stats.lastReception, t, l))
	}
	if received {
		batch.receivedTransmissions++
	}
	return nil
}

// count adds one opportunity with its outcome at the virtual second. Counters
// cannot overflow: opportunities never exceed transmission sequences.
func (st *stationStats) count(result outcome, channel Channel, second int64) {
	c := &st.counters
	perChannel := &c.ChannelA
	if channel == ChannelB {
		perChannel = &c.ChannelB
	}
	c.Opportunities++
	perChannel.Opportunities++
	switch result {
	case outcomeStationDisabled:
		c.StationDisabled++
	case outcomeChannelDisabled:
		c.ChannelDisabled++
	case outcomeOutsideHorizon:
		c.OutsideHorizon++
	case outcomeInsufficientMargin:
		c.InsufficientMargin++
	case outcomeProbabilisticLoss:
		c.ProbabilisticLoss++
	case outcomeReceived:
		c.Received++
		perChannel.Received++
	}
	bucket := &st.buckets[second%rateBuckets]
	if bucket.second != second {
		*bucket = rateBucket{second: second}
	}
	bucket.opportunities++
	if result == outcomeReceived {
		bucket.received++
	}
}

// newReception records a successful reception of t at station.
func newReception(station *stationState, sequence uint64, t transmission, l link) *Reception {
	d := station.definition
	return &Reception{
		StationID: station.id, Sequence: sequence, TransmissionSequence: t.Sequence,
		MMSI: t.MMSI, Channel: t.channel, Timestamp: t.Timestamp, Sentence: t.Sentence,
		VesselName: t.name, VesselTypeID: t.typeID,
		ConfigRevision: station.configRevision, RFRevision: station.rfRevision,
		Receiver: ReceiverSnapshot{
			Name:     d.Name,
			Latitude: d.Latitude, Longitude: d.Longitude, AntennaHeightMeters: d.AntennaHeightMeters,
			ReceiveGainDBi: d.ReceiveGainDBi, FeederLossDB: d.FeederLossDB, Channel: d.receiver(t.channel),
		},
		Link: ReceptionLink{
			DistanceMeters: l.distanceMeters, BearingDegrees: l.bearingDegrees, HorizonMeters: l.horizonMeters,
			ShadowLossDB: l.shadowLossDB, ReceivedPowerDBm: l.receivedPowerDBm,
			EffectiveSensitivityDBm: l.effectiveSensitivityDBm, MarginDB: l.marginDB, Probability: l.probability,
		},
	}
}

// applyReceptions publishes a staged batch for stations at the committed
// instant now. Before each new record time it expires observations at that
// time, as a separate call ending there would have, so eviction and expiry do
// not depend on how calls split virtual time. Called with the lock held.
func (s *Simulator) applyReceptions(stations []stationState, batch receptionBatch, now time.Time) {
	for i, station := range stations {
		s.store.stations[station.id].stats = batch.stats[i]
	}
	var expired time.Time
	for _, r := range batch.records {
		if r.Timestamp.After(expired) {
			s.store.expire(r.Timestamp)
			expired = r.Timestamp
		}
		s.store.stations[r.StationID].push(r)
		s.store.record(r)
	}
	s.store.expire(now)
	s.store.receptions = batch.receptions
	s.store.receivedTransmissions = batch.receivedTransmissions
}

// push appends r to the history ring, replacing the oldest record when full.
func (sr *stationReceptions) push(r *Reception) {
	if len(sr.history) < ReceptionHistoryLimit {
		sr.history = append(sr.history, r)
		return
	}
	sr.history[sr.oldest] = r
	sr.oldest = (sr.oldest + 1) % ReceptionHistoryLimit
}

// at returns the i-th oldest retained record.
func (sr *stationReceptions) at(i int) *Reception {
	return sr.history[(sr.oldest+i)%len(sr.history)]
}

// record makes r the latest observation of its station and MMSI. A new MMSI in
// a full store first evicts another target.
func (st *receptionStore) record(r *Reception) {
	t := st.targets[r.MMSI]
	if t == nil {
		if len(st.targets) >= TargetLimit {
			st.evict()
		}
		t = &target{}
		st.targets[r.MMSI] = t
	}
	t.receivedAt = r.Timestamp
	if i := slices.IndexFunc(t.observations, func(o *Reception) bool { return o.StationID == r.StationID }); i >= 0 {
		t.observations[i] = r
		return
	}
	t.observations = append(t.observations, r)
}

// evict removes the target with the oldest newest observation, the lowest MMSI
// among equal times, with all its station observations.
func (st *receptionStore) evict() {
	var victim uint32
	var oldest time.Time
	first := true
	for mmsi, t := range st.targets {
		if first || t.receivedAt.Before(oldest) || t.receivedAt.Equal(oldest) && mmsi < victim {
			victim, oldest, first = mmsi, t.receivedAt, false
		}
	}
	delete(st.targets, victim)
	st.evictions++
}

// expire removes observations aged ExpiryAge or more at at.
func (st *receptionStore) expire(at time.Time) {
	st.removeObservations(func(r *Reception) bool { return at.Sub(r.Timestamp) >= ExpiryAge })
}

// removeStation deletes the state of station id and its observations. Run
// totals keep its receptions.
func (st *receptionStore) removeStation(id string) {
	delete(st.stations, id)
	st.removeObservations(func(r *Reception) bool { return r.StationID == id })
}

// removeObservations deletes matching observations and targets left empty.
func (st *receptionStore) removeObservations(match func(*Reception) bool) {
	for mmsi, t := range st.targets {
		before := len(t.observations)
		t.observations = slices.DeleteFunc(t.observations, match)
		switch {
		case len(t.observations) == 0:
			delete(st.targets, mmsi)
		case len(t.observations) < before:
			t.receivedAt = time.Time{}
			for _, r := range t.observations {
				if r.Timestamp.After(t.receivedAt) {
					t.receivedAt = r.Timestamp
				}
			}
		}
	}
}

// Observations returns the received traffic of the selected stations from one
// committed state. An empty stationIDs selects all stations, including
// disabled ones; duplicates are ignored. An unknown ID wraps ErrNotFound.
//
// Each target's navigation is the newest report a selected station actually
// received; later missed reports never change it, and targets stay after their
// vessel leaves the fleet. Observations age with committed virtual time,
// including with an empty fleet, and freeze while paused. Reads compute ages
// without changing state; clock mutations remove observations aged ExpiryAge or
// more. The result is detached from the engine.
func (s *Simulator) Observations(stationIDs []string) (Observations, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	selected, err := s.selectStations(stationIDs)
	if err != nil {
		return Observations{}, err
	}
	now := s.start.Add(s.state.elapsed)
	stations := s.stationSet().Stations
	result := Observations{
		Metadata: s.metadata(), StateRevision: s.state.revision, StationSetRevision: s.state.stationRevision,
		Selection: make([]string, 0, len(stations)), Stations: make([]StationObservation, 0, len(stations)),
		Targets: make([]ObservedTarget, 0, len(s.store.targets)), RecentReceptions: s.recentReceptions(selected),
		Transmissions: s.state.sequence, ReceivedTransmissions: s.store.receivedTransmissions,
		Receptions: s.store.receptions, TargetEvictions: s.store.evictions,
	}
	for i, station := range stations {
		if selected[i] {
			result.Selection = append(result.Selection, station.ID)
		}
		result.Stations = append(result.Stations, s.store.stations[station.ID].observation(station, s.state.elapsed, now))
	}
	for _, mmsi := range slices.Sorted(maps.Keys(s.store.targets)) {
		observed, ok := s.observedTarget(mmsi, selected, now, result.Stations)
		if !ok {
			continue
		}
		result.Targets = append(result.Targets, observed)
		if observed.Status == TargetLost {
			result.LostTargets++
		} else {
			result.CurrentTargets++
		}
	}
	return result, nil
}

// selectStations resolves stationIDs to flags parallel to the configured
// stations. Called with the lock held.
func (s *Simulator) selectStations(stationIDs []string) ([]bool, error) {
	selected := make([]bool, len(s.state.stations))
	for _, id := range stationIDs {
		i, err := s.findStation(id)
		if err != nil {
			return nil, err
		}
		selected[i] = true
	}
	if len(stationIDs) == 0 {
		for i := range selected {
			selected[i] = true
		}
	}
	return selected, nil
}

// observation copies the reception state of station at elapsed virtual time now.
func (sr *stationReceptions) observation(station Station, elapsed time.Duration, now time.Time) StationObservation {
	counters := sr.stats.counters
	result := StationObservation{
		Station: station, Counters: counters, ReceiveRatio: ratio(counters.Received, counters.Opportunities),
		Recent: RecentCounters{Duration: min(RateWindow, now.Sub(station.CreatedAt))},
	}
	current := int64(elapsed / time.Second)
	for _, bucket := range sr.stats.buckets {
		if bucket.second > current-rateBuckets && bucket.second <= current {
			result.Recent.Opportunities += bucket.opportunities
			result.Recent.Received += bucket.received
		}
	}
	if seconds := result.Recent.Duration.Seconds(); seconds > 0 {
		result.Recent.OpportunityRate = new(float64(result.Recent.Opportunities) / seconds)
		result.Recent.ReceptionRate = new(float64(result.Recent.Received) / seconds)
	}
	result.Recent.ReceiveRatio = ratio(result.Recent.Received, result.Recent.Opportunities)
	if len(sr.history) > 0 {
		result.OldestReception = new(sr.at(0).Sequence)
		result.LatestReception = new(sr.stats.lastReception)
	}
	return result
}

// observedTarget builds target mmsi for the selection and adds its observation
// statuses to the station counts. It reports false when no selected station
// observes the target. Called with the lock held.
func (s *Simulator) observedTarget(mmsi uint32, selected []bool, now time.Time, stations []StationObservation) (ObservedTarget, bool) {
	t := s.store.targets[mmsi]
	var chosen *Reception
	provenance := make([]TargetStation, 0, len(t.observations))
	for i, station := range s.state.stations {
		j := slices.IndexFunc(t.observations, func(r *Reception) bool { return r.StationID == station.id })
		if j < 0 {
			continue
		}
		r := t.observations[j]
		age := now.Sub(r.Timestamp)
		status := targetStatus(age)
		if status == TargetLost {
			stations[i].LostTargets++
		} else {
			stations[i].CurrentTargets++
		}
		if !selected[i] {
			continue
		}
		if chosen == nil || r.TransmissionSequence > chosen.TransmissionSequence {
			chosen = r
		}
		provenance = append(provenance, TargetStation{
			StationID: station.id, StationEnabled: station.definition.Enabled,
			Sequence: r.Sequence, TransmissionSequence: r.TransmissionSequence, Timestamp: r.Timestamp,
			Channel: r.Channel, ReceivedPowerDBm: r.Link.ReceivedPowerDBm, RFRevision: r.RFRevision,
			Age: age, Status: status,
		})
	}
	if chosen == nil {
		return ObservedTarget{}, false
	}
	for i := range provenance {
		provenance[i].Chosen = provenance[i].TransmissionSequence == chosen.TransmissionSequence
	}
	age := now.Sub(chosen.Timestamp)
	return ObservedTarget{MMSI: mmsi, Report: chosen.detach(), Age: age, Status: targetStatus(age), Stations: provenance}, true
}

// recentReceptions returns the newest RecentReceptionLimit receptions of the
// selected stations. Called with the lock held.
func (s *Simulator) recentReceptions(selected []bool) []Reception {
	type ranked struct {
		reception *Reception
		station   int
	}
	candidates := make([]ranked, 0)
	for i, station := range s.state.stations {
		if !selected[i] {
			continue
		}
		sr := s.store.stations[station.id]
		for j := max(len(sr.history)-RecentReceptionLimit, 0); j < len(sr.history); j++ {
			candidates = append(candidates, ranked{reception: sr.at(j), station: i})
		}
	}
	slices.SortFunc(candidates, func(a, b ranked) int {
		return cmp.Or(
			a.reception.Timestamp.Compare(b.reception.Timestamp),
			cmp.Compare(a.reception.TransmissionSequence, b.reception.TransmissionSequence),
			cmp.Compare(a.station, b.station),
		)
	})
	result := make([]Reception, 0, min(len(candidates), RecentReceptionLimit))
	for _, c := range candidates[max(len(candidates)-RecentReceptionLimit, 0):] {
		result = append(result, c.reception.detach())
	}
	return result
}

// ReceptionHistory returns up to limit retained receptions of station
// stationID in ascending sequence order. A nil after returns the newest
// receptions; otherwise it returns the oldest retained receptions with a
// sequence above *after. History keeps the newest ReceptionHistoryLimit
// receptions per station, so a cursor can fall behind it; Gap then reports the
// loss. Expiry and target eviction never remove history. Reception sequences
// of different stations are unrelated, and a transmission sequence gap within
// one station's history is a missed reception, not lost history.
//
// Errors: limit outside 1 through ReceptionHistoryLimit or a cursor above the
// latest reception wraps ErrInvalid; an unknown or removed station wraps
// ErrNotFound. The result is detached from the engine.
func (s *Simulator) ReceptionHistory(stationID string, after *uint64, limit int) (ReceptionPage, error) {
	if limit < 1 || limit > ReceptionHistoryLimit {
		return ReceptionPage{}, fmt.Errorf("%w: reception history limit must be between 1 and %d: %d", ErrInvalid, ReceptionHistoryLimit, limit)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.findStation(stationID); err != nil {
		return ReceptionPage{}, err
	}
	sr := s.store.stations[stationID]
	latest, retained := sr.stats.lastReception, len(sr.history)
	page := ReceptionPage{SimulationID: s.id, StationID: stationID, Tail: after == nil, Receptions: make([]Reception, 0)}
	from := max(retained-limit, 0)
	if after != nil {
		if *after > latest {
			return ReceptionPage{}, fmt.Errorf("%w: reception cursor %d is after the latest reception %d", ErrInvalid, *after, latest)
		}
		page.NextAfter = *after
		from = retained
		if retained > 0 && *after < latest {
			oldest := sr.at(0).Sequence
			page.Gap = *after < oldest-1
			from = int(max(*after+1, oldest) - oldest)
		}
	}
	if retained > 0 {
		page.OldestSequence, page.LatestSequence = new(sr.at(0).Sequence), new(latest)
	}
	to := min(from+limit, retained)
	for i := from; i < to; i++ {
		page.Receptions = append(page.Receptions, sr.at(i).detach())
	}
	if to > from {
		page.NextAfter = page.Receptions[len(page.Receptions)-1].Sequence
	}
	page.HasMore = to < retained
	return page, nil
}

// detach returns a copy of r that shares no memory a caller can change.
func (r *Reception) detach() Reception {
	c := *r
	if r.Link.BearingDegrees != nil {
		c.Link.BearingDegrees = new(*r.Link.BearingDegrees)
	}
	return c
}

// targetStatus classifies an observation age below ExpiryAge.
func targetStatus(age time.Duration) TargetStatus {
	switch {
	case age <= FreshAge:
		return TargetFresh
	case age <= StaleAge:
		return TargetStale
	default:
		return TargetLost
	}
}

// ratio returns part/whole, or nil when whole is 0.
func ratio(part, whole uint64) *float64 {
	if whole == 0 {
		return nil
	}
	return new(float64(part) / float64(whole))
}
