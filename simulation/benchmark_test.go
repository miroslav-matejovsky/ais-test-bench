package simulation_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-testbench/simulation"
)

// Benchmarks measure the engine at MaxVessels. One Advance operation is one
// virtual second, so ns/op must stay below 10 ms for the real-time driver to
// sustain MaxSpeed (100 virtual seconds per real second). Run them with:
//
//	go test ./simulation -run '^$' -bench . -benchmem

// benchmarkEngine returns an engine with MaxVessels and n stations. Stations at
// the spawn area receive every report; far stations are outside the horizon.
// A full engine has a full reception history and TargetLimit observed MMSIs.
func benchmarkEngine(b *testing.B, n int, far, full bool) *simulation.Simulator {
	b.Helper()
	config := testConfig(42, simulation.MaxVessels)
	for i := range n {
		station := site(fmt.Sprintf("Station %d", i+1))
		if far {
			station.Latitude = 40
		}
		config.Stations = append(config.Stations, station)
	}
	s, err := simulation.New(config)
	require.NoError(b, err)
	if full {
		for range simulation.TargetLimit / simulation.MaxVessels {
			churn(b, s)
		}
	}
	return s
}

// churn replaces the fleet with MaxVessels new MMSIs.
func churn(b *testing.B, s *simulation.Simulator) {
	b.Helper()
	_, err := s.SetCount(0)
	require.NoError(b, err)
	_, err = s.SetCount(simulation.MaxVessels)
	require.NoError(b, err)
}

func BenchmarkAdvance(b *testing.B) {
	for _, n := range []int{1, 3, simulation.MaxStations} {
		for _, tt := range []struct {
			name  string
			far   bool
			churn bool
		}{
			{name: "no receptions", far: true},
			{name: "all received full stores"},
			{name: "all received new MMSIs", churn: true},
		} {
			b.Run(fmt.Sprintf("stations=%d/%s", n, tt.name), func(b *testing.B) {
				s := benchmarkEngine(b, n, tt.far, !tt.far)
				for b.Loop() {
					if tt.churn {
						churn(b, s)
					}
					if _, err := s.Advance(b.Context(), time.Second); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

// BenchmarkObservations measures one snapshot at maximum occupancy: 16
// stations, TargetLimit targets each observed by every station, and full
// reception histories.
func BenchmarkObservations(b *testing.B) {
	s := benchmarkEngine(b, simulation.MaxStations, false, true)
	for b.Loop() {
		if _, err := s.Observations(nil); err != nil {
			b.Fatal(err)
		}
	}
}
