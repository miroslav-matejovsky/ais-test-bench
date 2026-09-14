package simulation_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/miroslav-matejovsky/ais-test-bench/simulation"
)

// This example uses only the public package and the standard library. It runs a
// reproducible fleet through explicit virtual steps, scaled real time, and pause.
func ExampleSimulator() {
	ctx := context.Background()
	sim, err := simulation.New(simulation.Config{
		ID:                 "test-run",
		StartTime:          time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC),
		Seed:               42,
		InitialVesselCount: 2,
		Speed:              1,
	})
	if err != nil {
		panic(err)
	}
	show := func(step string, reports []simulation.Message, err error) {
		if err != nil {
			panic(err)
		}
		clock := sim.Metadata().Time
		fmt.Printf("%s: %d reports, now %s, speed %v\n", step, len(reports), clock.Now.Format(time.RFC3339), clock.Speed)
	}
	show("start", sim.History().Messages, nil)

	reports, err := sim.Advance(ctx, 5*time.Second)
	show("advance 5s", reports, err)
	last := reports[len(reports)-1]
	// Sentences end with CRLF.
	fmt.Println(last.Sequence, last.MMSI, last.Timestamp.Format(time.RFC3339), strings.TrimSpace(last.Sentence))

	if err := sim.SetSpeed(0.5); err != nil {
		panic(err)
	}
	reports, err = sim.Elapse(ctx, 2*time.Second)
	show("elapse 2s at 0.5x", reports, err)

	if err := sim.SetSpeed(0); err != nil {
		panic(err)
	}
	reports, err = sim.Elapse(ctx, time.Minute)
	show("elapse 1m paused", reports, err)
	reports, err = sim.Advance(ctx, time.Second)
	show("advance 1s paused", reports, err)
	// Output:
	// start: 2 reports, now 2030-01-02T03:04:05Z, speed 1
	// advance 5s: 10 reports, now 2030-01-02T03:04:10Z, speed 1
	// 12 200000001 2030-01-02T03:04:10Z !AIVDM,1,1,,A,12vg20@P1n0B@wFMhCv79UfD0000,0*13
	// elapse 2s at 0.5x: 2 reports, now 2030-01-02T03:04:11Z, speed 0.5
	// elapse 1m paused: 0 reports, now 2030-01-02T03:04:11Z, speed 0
	// advance 1s paused: 2 reports, now 2030-01-02T03:04:12Z, speed 0
}
