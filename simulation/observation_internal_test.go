package simulation

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestReceptionLimitsFailBeforeCommit drives reception and revision counters to
// their maximum, which public calls cannot reach in a test. A failed call must
// leave the engine equal to an untouched control engine.
func TestReceptionLimitsFailBeforeCommit(t *testing.T) {
	channel := ReceiverChannel{Enabled: true, SensitivityDBm: -110}
	near := StationDefinition{Name: "Near", Latitude: 52, Longitude: 4, Enabled: true, AntennaHeightMeters: 25, ChannelA: channel, ChannelB: channel}
	renamed := near
	renamed.Name = "Renamed"
	config := internalConfig
	config.Stations = []StationDefinition{near}
	advance := func(s *Simulator) error {
		_, err := s.Advance(t.Context(), time.Second)
		return err
	}
	for name, tt := range map[string]struct {
		exhaust func(*Simulator)
		call    func(*Simulator) error
	}{
		"reception sequence": {
			exhaust: func(s *Simulator) { s.store.stations["station-1"].stats.lastReception = math.MaxUint64 },
			call:    advance,
		},
		"reception total": {
			exhaust: func(s *Simulator) { s.store.receptions = math.MaxUint64 },
			call: func(s *Simulator) error {
				_, err := s.SetCount(3)
				return err
			},
		},
		"state revision on advance": {call: advance},
		"state revision on count": {call: func(s *Simulator) error {
			_, err := s.SetCount(0)
			return err
		}},
		"state revision on speed": {call: func(s *Simulator) error { return s.SetSpeed(2) }},
		"state revision on add": {call: func(s *Simulator) error {
			_, _, err := s.AddStation(1, near)
			return err
		}},
		"state revision on update": {call: func(s *Simulator) error {
			_, err := s.UpdateStation(1, "station-1", renamed)
			return err
		}},
		"state revision on removal": {call: func(s *Simulator) error {
			_, err := s.RemoveStation(1, "station-1")
			return err
		}},
	} {
		t.Run(name, func(t *testing.T) {
			if tt.exhaust == nil {
				tt.exhaust = func(s *Simulator) { s.state.revision = math.MaxUint64 }
			}
			s, err := New(config)
			require.NoError(t, err)
			control, err := New(config)
			require.NoError(t, err)
			tt.exhaust(s)
			tt.exhaust(control)

			require.ErrorIs(t, tt.call(s), ErrLimit)
			require.Equal(t, control.state, s.state)
			require.Equal(t, control.messages, s.messages)
			require.Equal(t, control.store, s.store)
		})
	}
}
