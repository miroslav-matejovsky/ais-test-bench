package display

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/miroslav-matejovsky/ais-testbench/internal/ais"
	"github.com/miroslav-matejovsky/ais-testbench/internal/simulatorapi"
)

// defaultHistoryLimit is the simulator page size when a request has no limit.
const defaultHistoryLimit = 100

// upstreamObservations decodes simulatorapi.Observations with a presence-aware
// clock, so a missing or null time field differs from a valid zero. The outer
// Time field shadows the embedded Metadata.Time when decoding.
type upstreamObservations struct {
	simulatorapi.Observations
	Time *upstreamTime `json:"time"`
}

type upstreamTime struct {
	Now       *time.Time `json:"now"`
	ElapsedMs *int64     `json:"elapsedMs"`
	Speed     *float64   `json:"speed"`
	Paused    *bool      `json:"paused"`
}

// projector validates and decodes the receptions of one upstream response. It
// lives for one request: every distinct sentence is decoded once, and repeated
// references to one transmission or reception must agree.
type projector struct {
	// Snapshot context; stations is nil for a history page, which has no clock
	// or station configuration to check references against.
	settings      simulatorapi.Settings
	startedAt     time.Time
	now           time.Time
	transmissions uint64
	stations      map[string]*simulatorapi.StationObservation
	selected      map[string]bool

	decoded    map[string]ais.Report
	sent       map[uint64]transmission
	received   map[receptionKey]uint64
	categories map[string]bool
	// current and lost count provenance statuses per station.
	current, lost map[string]int
}

// transmission is what every reference to one transmission sequence must share.
// sentence is empty for compact provenance references.
type transmission struct {
	mmsi      uint32
	channel   string
	timestamp time.Time
	sentence  string
}

// receptionKey identifies one station reception within a run.
type receptionKey struct {
	stationID string
	sequence  uint64
}

func newProjector() *projector {
	return &projector{
		decoded: make(map[string]ais.Report), sent: make(map[uint64]transmission),
		received: make(map[receptionKey]uint64), categories: make(map[string]bool),
		current: make(map[string]int), lost: make(map[string]int),
	}
}

// projectObservations validates one complete snapshot for the requested
// selection, nil meaning all stations, and decodes every received report. Any
// violation fails the whole projection, so a corrupt report never looks like a
// missing target.
func projectObservations(requested []string, upstream upstreamObservations) (Observations, error) {
	o := upstream.Observations
	metadata := o.Metadata
	if err := validateMetadata(metadata); err != nil {
		return Observations{}, err
	}
	clock, err := validateTime(upstream.Time, metadata.StartedAt, metadata.Settings.Speed)
	if err != nil {
		return Observations{}, err
	}
	settings := metadata.Settings
	switch {
	case o.StateRevision == 0 || o.StationSetRevision == 0:
		return Observations{}, errors.New("stateRevision and stationSetRevision are required")
	case !o.SnapshotAt.Equal(clock.Now):
		return Observations{}, fmt.Errorf("snapshotAt %s differs from time.now %s", o.SnapshotAt.Format(time.RFC3339Nano), clock.Now.Format(time.RFC3339Nano))
	case o.Selection == nil || o.Stations == nil || o.Targets == nil || o.RecentReceptions == nil:
		return Observations{}, errors.New("selection, stations, targets, and recentReceptions are required")
	case !o.RecentIsSample:
		return Observations{}, errors.New("recentIsSample must be true")
	case len(o.Stations) > settings.MaxStations:
		return Observations{}, fmt.Errorf("%d stations exceed maxStations %d", len(o.Stations), settings.MaxStations)
	case len(o.Targets) > settings.Observation.TargetLimit:
		return Observations{}, fmt.Errorf("%d targets exceed targetLimit %d", len(o.Targets), settings.Observation.TargetLimit)
	case len(o.RecentReceptions) > settings.Observation.RecentReceptionLimit:
		return Observations{}, fmt.Errorf("%d recent receptions exceed recentReceptionLimit %d", len(o.RecentReceptions), settings.Observation.RecentReceptionLimit)
	case o.ReceivedTransmissions > o.Transmissions || o.ReceivedTransmissions > o.Receptions:
		return Observations{}, fmt.Errorf("receivedTransmissions %d exceeds transmissions %d or receptions %d", o.ReceivedTransmissions, o.Transmissions, o.Receptions)
	}

	p := newProjector()
	p.settings, p.startedAt, p.now, p.transmissions = settings, metadata.StartedAt, clock.Now, o.Transmissions
	p.stations = make(map[string]*simulatorapi.StationObservation, len(o.Stations))
	ids := make([]string, 0, len(o.Stations))
	for i := range o.Stations {
		s := &o.Stations[i]
		if err := validateStation(*s, settings, metadata.StartedAt, clock.Now); err != nil {
			return Observations{}, fmt.Errorf("station %q: %w", s.ID, err)
		}
		if i > 0 && s.ID <= ids[i-1] {
			return Observations{}, fmt.Errorf("station %q is duplicate or out of ID order", s.ID)
		}
		p.stations[s.ID] = s
		ids = append(ids, s.ID)
	}
	want := requested
	if want == nil {
		want = ids
	}
	if want = slices.Sorted(slices.Values(want)); !slices.Equal(o.Selection, want) {
		return Observations{}, fmt.Errorf("selection %q does not match requested stations %q", o.Selection, want)
	}
	p.selected = make(map[string]bool, len(o.Selection))
	for _, id := range o.Selection {
		p.selected[id] = true
	}

	result := Observations{
		SimulationID: metadata.SimulationID, StartedAt: metadata.StartedAt, Time: clock,
		StateRevision: o.StateRevision, StationSetRevision: o.StationSetRevision,
		Settings: Settings{
			MaxStations: settings.MaxStations, Transmitter: settings.Transmitter, Reception: settings.Reception,
			Observation: settings.Observation, SpawnBounds: settings.SpawnBounds,
		},
		Selection: o.Selection, Stations: make([]Station, 0, len(o.Stations)),
		Targets: make([]Target, 0, len(o.Targets)), CurrentTargets: o.CurrentTargets, LostTargets: o.LostTargets,
		RecentReceptions: make([]Reception, 0, len(o.RecentReceptions)), RecentIsSample: true,
		Transmissions: o.Transmissions, ReceivedTransmissions: o.ReceivedTransmissions,
		Receptions: o.Receptions, TargetEvictions: o.TargetEvictions,
	}
	current, lost := 0, 0
	seenMMSI := make(map[uint32]bool, len(o.Targets))
	for _, t := range o.Targets {
		if seenMMSI[t.MMSI] {
			return Observations{}, fmt.Errorf("duplicate target MMSI %d", t.MMSI)
		}
		seenMMSI[t.MMSI] = true
		target, err := p.target(t)
		if err != nil {
			return Observations{}, fmt.Errorf("target %d: %w", t.MMSI, err)
		}
		if target.Status == "lost" {
			lost++
		} else {
			current++
		}
		result.Targets = append(result.Targets, target)
	}
	if current != o.CurrentTargets || lost != o.LostTargets {
		return Observations{}, fmt.Errorf("currentTargets %d and lostTargets %d disagree with target statuses %d and %d", o.CurrentTargets, o.LostTargets, current, lost)
	}
	for i, s := range o.Stations {
		selected := p.selected[s.ID]
		// A selected station's every observation is provenance of some target.
		if selected && (s.CurrentTargets != p.current[s.ID] || s.LostTargets != p.lost[s.ID]) {
			return Observations{}, fmt.Errorf("station %q target counts %d and %d disagree with provenance %d and %d", s.ID, s.CurrentTargets, s.LostTargets, p.current[s.ID], p.lost[s.ID])
		}
		result.Stations = append(result.Stations, Station{StationObservation: o.Stations[i], Selected: selected})
	}

	recent := make(map[receptionKey]bool, len(o.RecentReceptions))
	for i, r := range o.RecentReceptions {
		key := receptionKey{stationID: r.StationID, sequence: r.Sequence}
		if recent[key] {
			return Observations{}, fmt.Errorf("duplicate recent reception %s/%d", r.StationID, r.Sequence)
		}
		recent[key] = true
		if i > 0 {
			prev := o.RecentReceptions[i-1]
			if cmp.Or(r.Timestamp.Compare(prev.Timestamp), cmp.Compare(r.TransmissionSequence, prev.TransmissionSequence), cmp.Compare(r.StationID, prev.StationID)) < 0 {
				return Observations{}, fmt.Errorf("recent reception %s/%d is out of order", r.StationID, r.Sequence)
			}
		}
		reception, err := p.reception(r)
		if err != nil {
			return Observations{}, fmt.Errorf("recent reception %s/%d: %w", r.StationID, r.Sequence, err)
		}
		result.RecentReceptions = append(result.RecentReceptions, reception)
	}

	result.ScenarioCategories = append([]simulatorapi.VesselType{}, metadata.VesselTypes...)
	for _, id := range slices.Sorted(maps.Keys(p.categories)) {
		if !slices.ContainsFunc(metadata.VesselTypes, func(v simulatorapi.VesselType) bool { return v.ID == id }) {
			result.ScenarioCategories = append(result.ScenarioCategories, simulatorapi.VesselType{ID: id, Name: id})
		}
	}
	return result, nil
}

// target validates one observed target: its chosen report, virtual age, and
// provenance that marks exactly the stations that received the chosen transmission.
func (p *projector) target(t simulatorapi.ObservedTarget) (Target, error) {
	report, err := p.reception(t.Report)
	if err != nil {
		return Target{}, fmt.Errorf("report: %w", err)
	}
	if t.MMSI != t.Report.MMSI {
		return Target{}, fmt.Errorf("report MMSI %d differs", t.Report.MMSI)
	}
	if err := p.age(t.AgeMs, t.Status, t.Report.Timestamp); err != nil {
		return Target{}, err
	}
	if len(t.Stations) == 0 {
		return Target{}, errors.New("stations provenance is required")
	}
	result := Target{MMSI: t.MMSI, AgeMs: t.AgeMs, Status: t.Status, Report: report, Stations: make([]TargetStation, 0, len(t.Stations))}
	seen := make(map[string]bool, len(t.Stations))
	reporter := false
	for _, s := range t.Stations {
		if seen[s.StationID] {
			return Target{}, fmt.Errorf("duplicate provenance station %q", s.StationID)
		}
		seen[s.StationID] = true
		provenance, err := p.provenance(t, s)
		if err != nil {
			return Target{}, fmt.Errorf("provenance %q: %w", s.StationID, err)
		}
		if s.StationID == t.Report.StationID {
			if s.Sequence != t.Report.Sequence {
				return Target{}, fmt.Errorf("provenance %q sequence %d differs from report sequence %d", s.StationID, s.Sequence, t.Report.Sequence)
			}
			reporter = true
		}
		result.Stations = append(result.Stations, provenance)
	}
	if !reporter {
		return Target{}, fmt.Errorf("report station %q is missing from provenance", t.Report.StationID)
	}
	return result, nil
}

// provenance validates one station's last reception of target t.
func (p *projector) provenance(t simulatorapi.ObservedTarget, s simulatorapi.TargetStation) (TargetStation, error) {
	if err := p.identity(s.StationID, s.Sequence, s.TransmissionSequence); err != nil {
		return TargetStation{}, err
	}
	station := p.stations[s.StationID]
	switch {
	case !validChannel(s.Channel):
		return TargetStation{}, fmt.Errorf("channel %q is not A or B", s.Channel)
	case s.TransmissionSequence > t.Report.TransmissionSequence:
		return TargetStation{}, fmt.Errorf("transmission %d is newer than chosen transmission %d", s.TransmissionSequence, t.Report.TransmissionSequence)
	case s.Chosen != (s.TransmissionSequence == t.Report.TransmissionSequence):
		return TargetStation{}, fmt.Errorf("chosen %t disagrees with transmission %d", s.Chosen, s.TransmissionSequence)
	case s.StationEnabled != station.Definition.Enabled:
		return TargetStation{}, fmt.Errorf("stationEnabled %t disagrees with station configuration", s.StationEnabled)
	case s.RFRevision == 0 || s.RFRevision > station.RFRevision:
		return TargetStation{}, fmt.Errorf("rfRevision %d is not 1-%d", s.RFRevision, station.RFRevision)
	case !finite(s.ReceivedPowerDBm):
		return TargetStation{}, errors.New("receivedPowerDbm must be finite")
	}
	if err := p.inSnapshot(s.Timestamp); err != nil {
		return TargetStation{}, err
	}
	if err := p.age(s.AgeMs, s.Status, s.Timestamp); err != nil {
		return TargetStation{}, err
	}
	if err := p.transmission(s.TransmissionSequence, transmission{mmsi: t.MMSI, channel: s.Channel, timestamp: s.Timestamp}); err != nil {
		return TargetStation{}, err
	}
	if s.Status == "lost" {
		p.lost[s.StationID]++
	} else {
		p.current[s.StationID]++
	}
	return TargetStation{
		StationID: s.StationID, StationEnabled: s.StationEnabled, Sequence: s.Sequence,
		TransmissionSequence: s.TransmissionSequence, ReceivedAt: s.Timestamp, Channel: s.Channel,
		EstimatedPowerDBm: s.ReceivedPowerDBm, RFRevision: s.RFRevision, CurrentRFRevision: s.RFRevision == station.RFRevision,
		AgeMs: s.AgeMs, Status: s.Status, Chosen: s.Chosen,
	}, nil
}

// reception validates one reception and decodes its sentence. In a snapshot it
// also checks the station, selection, revisions, and clock it refers to.
func (p *projector) reception(r simulatorapi.Reception) (Reception, error) {
	if err := p.identity(r.StationID, r.Sequence, r.TransmissionSequence); err != nil {
		return Reception{}, err
	}
	switch {
	case !validChannel(r.Channel):
		return Reception{}, fmt.Errorf("channel %q is not A or B", r.Channel)
	case r.Timestamp.IsZero():
		return Reception{}, errors.New("timestamp is required")
	case r.ConfigRevision == 0 || r.RFRevision == 0:
		return Reception{}, errors.New("configRevision and rfRevision are required")
	case r.VesselTypeID == "":
		return Reception{}, errors.New("vesselTypeId is required")
	case !r.Receiver.Channel.Enabled:
		return Reception{}, errors.New("receiver channel was disabled at reception")
	}
	if err := validateReceiver(r.Receiver); err != nil {
		return Reception{}, fmt.Errorf("receiver: %w", err)
	}
	if err := validateLink(r.Link); err != nil {
		return Reception{}, fmt.Errorf("link: %w", err)
	}
	if p.stations != nil {
		station := p.stations[r.StationID]
		if r.ConfigRevision > station.ConfigRevision || r.RFRevision > station.RFRevision {
			return Reception{}, fmt.Errorf("revisions %d/%d are newer than station revisions %d/%d", r.ConfigRevision, r.RFRevision, station.ConfigRevision, station.RFRevision)
		}
		if err := p.inSnapshot(r.Timestamp); err != nil {
			return Reception{}, err
		}
	}
	report, err := p.decode(r.Sentence)
	if err != nil {
		return Reception{}, err
	}
	switch {
	case report.MMSI != r.MMSI:
		return Reception{}, fmt.Errorf("payload MMSI %d differs from MMSI %d", report.MMSI, r.MMSI)
	case string(report.Channel) != r.Channel:
		return Reception{}, fmt.Errorf("sentence channel %s differs from channel %s", report.Channel, r.Channel)
	case report.Second != nil && *report.Second != r.Timestamp.UTC().Second():
		return Reception{}, fmt.Errorf("payload UTC second %d differs from timestamp %s", *report.Second, r.Timestamp.Format(time.RFC3339Nano))
	}
	if err := p.transmission(r.TransmissionSequence, transmission{mmsi: r.MMSI, channel: r.Channel, timestamp: r.Timestamp, sentence: r.Sentence}); err != nil {
		return Reception{}, err
	}
	p.categories[r.VesselTypeID] = true
	l := r.Link
	return Reception{
		StationID: r.StationID, Sequence: r.Sequence, TransmissionSequence: r.TransmissionSequence,
		MMSI: r.MMSI, MessageType: 1, Channel: r.Channel, ReceivedAt: r.Timestamp, Sentence: r.Sentence,
		Navigation: Navigation{
			Latitude: report.Latitude, Longitude: report.Longitude, Speed: report.Speed,
			Course: report.Course, Heading: report.Heading, UTCSecond: report.Second,
		},
		Scenario:       Scenario{Name: r.VesselName, CategoryID: r.VesselTypeID},
		ConfigRevision: r.ConfigRevision, RFRevision: r.RFRevision, Receiver: r.Receiver,
		Signal: Signal{
			EstimatedPowerDBm: l.ReceivedPowerDBm, EffectiveSensitivityDBm: l.EffectiveSensitivityDBm,
			MarginDB: l.MarginDB, Probability: l.Probability, DistanceMeters: l.DistanceMeters,
			BearingDegrees: l.BearingDegrees, HorizonMeters: l.HorizonMeters, ShadowLossDB: l.ShadowLossDB,
		},
	}, nil
}

// identity checks a reception identity and that every reference to it names
// the same transmission. In a snapshot the station must be known and selected,
// and the sequences within the station and run bounds.
func (p *projector) identity(stationID string, sequence, transmissionSequence uint64) error {
	if stationID == "" || sequence == 0 || transmissionSequence == 0 {
		return errors.New("stationId, sequence, and transmissionSequence are required")
	}
	if p.stations != nil {
		station, ok := p.stations[stationID]
		switch {
		case !ok:
			return fmt.Errorf("unknown station %q", stationID)
		case !p.selected[stationID]:
			return fmt.Errorf("station %q is not selected", stationID)
		case station.LatestReception == nil || sequence > *station.LatestReception:
			return fmt.Errorf("sequence %d is beyond the station's latest reception", sequence)
		case transmissionSequence > p.transmissions:
			return fmt.Errorf("transmission %d is beyond %d run transmissions", transmissionSequence, p.transmissions)
		}
	}
	key := receptionKey{stationID: stationID, sequence: sequence}
	if known, ok := p.received[key]; ok && known != transmissionSequence {
		return fmt.Errorf("reception %s/%d refers to transmissions %d and %d", stationID, sequence, known, transmissionSequence)
	}
	p.received[key] = transmissionSequence
	return nil
}

// transmission requires every reference to one transmission sequence to carry
// the same MMSI, channel, timestamp, and, where present, sentence.
func (p *projector) transmission(sequence uint64, t transmission) error {
	known, ok := p.sent[sequence]
	if !ok {
		p.sent[sequence] = t
		return nil
	}
	if known.mmsi != t.mmsi || known.channel != t.channel || !known.timestamp.Equal(t.timestamp) ||
		known.sentence != "" && t.sentence != "" && known.sentence != t.sentence {
		return fmt.Errorf("references to transmission %d disagree", sequence)
	}
	if known.sentence == "" {
		known.sentence = t.sentence
		p.sent[sequence] = known
	}
	return nil
}

// decode decodes each distinct sentence once per projection.
func (p *projector) decode(sentence string) (ais.Report, error) {
	if report, ok := p.decoded[sentence]; ok {
		return report, nil
	}
	report, err := ais.DecodePosition(sentence)
	if err != nil {
		return ais.Report{}, fmt.Errorf("decode NMEA: %w", err)
	}
	p.decoded[sentence] = report
	return report, nil
}

// inSnapshot requires a virtual instant within the run and not after the
// snapshot clock. Wall time is never consulted.
func (p *projector) inSnapshot(at time.Time) error {
	if at.Before(p.startedAt) || at.After(p.now) {
		return fmt.Errorf("timestamp %s is outside startedAt %s and time.now %s", at.Format(time.RFC3339Nano), p.startedAt.Format(time.RFC3339Nano), p.now.Format(time.RFC3339Nano))
	}
	return nil
}

// age requires ageMs and status to match the virtual age of at: fresh through
// FreshAgeMs, stale through StaleAgeMs, lost below ExpiryAgeMs. Expired
// observations are removed before a snapshot.
func (p *projector) age(ageMs int64, status string, at time.Time) error {
	limits := p.settings.Observation
	age := p.now.Sub(at)
	want := "lost"
	switch {
	case age <= time.Duration(limits.FreshAgeMs)*time.Millisecond:
		want = "fresh"
	case age <= time.Duration(limits.StaleAgeMs)*time.Millisecond:
		want = "stale"
	}
	switch {
	case age >= time.Duration(limits.ExpiryAgeMs)*time.Millisecond:
		return fmt.Errorf("observation age %s reached expiry", age)
	case ageMs != age.Milliseconds():
		return fmt.Errorf("ageMs %d differs from virtual age %d ms", ageMs, age.Milliseconds())
	case status != want:
		return fmt.Errorf("status %q differs from %q for age %s", status, want, age)
	}
	return nil
}

// projectReceptionPage validates one history page for stationID and req: run
// and station identity, bounds, contiguous ascending sequences, cursor, gap,
// and every reception. It has no snapshot clock or station configuration.
func projectReceptionPage(stationID string, req HistoryRequest, page simulatorapi.ReceptionPage) (ReceptionPage, error) {
	oldest, latest, after := page.OldestAvailable, page.LatestAvailable, req.After
	limit := cmp.Or(req.Limit, defaultHistoryLimit)
	n := len(page.Receptions)
	switch {
	case page.SimulationID != req.SimulationID || page.StationID != stationID:
		return ReceptionPage{}, fmt.Errorf("page identity %q/%q differs from request %q/%q", page.SimulationID, page.StationID, req.SimulationID, stationID)
	case page.Tail != (after == nil):
		return ReceptionPage{}, fmt.Errorf("tail %t disagrees with the request cursor", page.Tail)
	case page.Receptions == nil:
		return ReceptionPage{}, errors.New("receptions is required")
	case n > limit:
		return ReceptionPage{}, fmt.Errorf("%d receptions exceed limit %d", n, limit)
	case (oldest == nil) != (latest == nil):
		return ReceptionPage{}, errors.New("oldestAvailable and latestAvailable must both be null or set")
	case oldest != nil && (*oldest == 0 || *oldest > *latest):
		return ReceptionPage{}, fmt.Errorf("impossible bounds %d-%d", *oldest, *latest)
	case (page.TruncatedBefore != nil) != (oldest != nil && *oldest > 1) || page.TruncatedBefore != nil && *page.TruncatedBefore != *oldest:
		return ReceptionPage{}, errors.New("truncatedBefore disagrees with oldestAvailable")
	case n > 0 && oldest == nil:
		return ReceptionPage{}, errors.New("receptions without retained bounds")
	case after != nil && latest != nil && *after > *latest:
		return ReceptionPage{}, fmt.Errorf("cursor %d is after latestAvailable %d", *after, *latest)
	case after != nil && latest == nil && *after != 0:
		return ReceptionPage{}, fmt.Errorf("cursor %d without receptions", *after)
	}

	p := newProjector()
	result := ReceptionPage{
		SimulationID: page.SimulationID, StationID: page.StationID, Tail: page.Tail,
		OldestAvailable: oldest, LatestAvailable: latest, TruncatedBefore: page.TruncatedBefore,
		Receptions: make([]Reception, 0, n), NextAfter: page.NextAfter, HasMore: page.HasMore, Gap: page.Gap,
	}
	for i, r := range page.Receptions {
		switch {
		case r.StationID != stationID:
			return ReceptionPage{}, fmt.Errorf("reception %d belongs to station %q", r.Sequence, r.StationID)
		case r.Sequence < *oldest || r.Sequence > *latest:
			return ReceptionPage{}, fmt.Errorf("reception %d is outside retained bounds", r.Sequence)
		case i > 0 && (r.Sequence != page.Receptions[i-1].Sequence+1 ||
			r.TransmissionSequence <= page.Receptions[i-1].TransmissionSequence || r.Timestamp.Before(page.Receptions[i-1].Timestamp)):
			return ReceptionPage{}, fmt.Errorf("reception %d does not follow reception %d", r.Sequence, page.Receptions[i-1].Sequence)
		}
		reception, err := p.reception(r)
		if err != nil {
			return ReceptionPage{}, fmt.Errorf("reception %d: %w", r.Sequence, err)
		}
		result.Receptions = append(result.Receptions, reception)
	}

	var wantNext uint64
	if after != nil {
		wantNext = *after
	}
	if n > 0 {
		wantNext = page.Receptions[n-1].Sequence
	}
	switch {
	case page.NextAfter != wantNext:
		return ReceptionPage{}, fmt.Errorf("nextAfter %d, want %d", page.NextAfter, wantNext)
	case n > 0 && after == nil && page.Receptions[n-1].Sequence != *latest:
		return ReceptionPage{}, errors.New("tail page does not end at latestAvailable")
	case n > 0 && after != nil && page.Receptions[0].Sequence != max(*after+1, *oldest):
		return ReceptionPage{}, fmt.Errorf("page starts at %d, not after cursor %d", page.Receptions[0].Sequence, *after)
	case n == 0 && latest != nil && (after == nil || *after < *latest):
		return ReceptionPage{}, errors.New("empty page although later receptions are retained")
	case page.HasMore != (latest != nil && page.NextAfter < *latest):
		return ReceptionPage{}, fmt.Errorf("hasMore %t disagrees with nextAfter and latestAvailable", page.HasMore)
	case page.Gap != (after != nil && oldest != nil && *after < *oldest-1):
		return ReceptionPage{}, fmt.Errorf("gap %t disagrees with cursor and oldestAvailable", page.Gap)
	}
	return result, nil
}
