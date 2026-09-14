package simulation_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/miroslav-matejovsky/ais-test-bench/simulation"
)

// These examples use only the public package and the standard library. They
// need no server, network, or sleep.

// New creates the initial fleet deterministically: the same Config always
// yields the same vessels and sentences.
func ExampleNew() {
	sim, err := simulation.New(simulation.Config{
		ID:                 "test-run",
		StartTime:          time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC),
		Seed:               42,
		InitialVesselCount: 2,
		Speed:              1,
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, message := range sim.History().Messages {
		// Sentences end with CRLF.
		fmt.Println(message.Sequence, message.MMSI, message.Timestamp.Format(time.RFC3339), strings.TrimSpace(message.Sentence))
	}

	_, err = simulation.New(simulation.Config{StartTime: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)})
	fmt.Println("missing ID rejected:", errors.Is(err, simulation.ErrInvalid))
	// Output:
	// 1 200000000 2030-01-02T03:04:05Z !AIVDM,1,1,,A,12vg200P0w0B9jjMiLDPe0T:0000,0*1D
	// 2 200000001 2030-01-02T03:04:05Z !AIVDM,1,1,,A,12vg20@P1n0B@wjMhDVo9Uf:0000,0*3E
	// missing ID rejected: true
}

// Advance steps virtual time and returns every report, including reports that
// no longer fit in History. Sequence numbers reveal what History lost.
func ExampleSimulator_Advance() {
	ctx := context.Background()
	sim, err := simulation.New(simulation.Config{
		ID:                 "test-run",
		StartTime:          time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC),
		Seed:               42,
		InitialVesselCount: simulation.MaxVessels,
		Speed:              1,
	})
	if err != nil {
		fmt.Println(err)
		return
	}

	reports, err := sim.Advance(ctx, simulation.MaxAdvance)
	if err != nil {
		fmt.Println(err)
		return
	}
	first, last := reports[0], reports[len(reports)-1]
	fmt.Printf("returned %d reports, sequences %d-%d, %s to %s\n", len(reports),
		first.Sequence, last.Sequence, first.Timestamp.Format(time.RFC3339), last.Timestamp.Format(time.RFC3339))
	history := sim.History()
	fmt.Printf("retained %d reports from sequence %d\n", len(history.Messages), *history.OldestSequence)

	// Split steps process the same ticks; a partial second emits nothing extra.
	for _, step := range []time.Duration{1500 * time.Millisecond, 500 * time.Millisecond} {
		reports, err := sim.Advance(ctx, step)
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Printf("advance %v: %d reports, now %s\n", step, len(reports), sim.Metadata().Time.Now.Format(time.RFC3339Nano))
	}

	// Failed calls change nothing.
	_, err = sim.Advance(ctx, simulation.MaxAdvance+time.Second)
	fmt.Println("over limit:", errors.Is(err, simulation.ErrLimit))
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = sim.Advance(cancelled, time.Second)
	fmt.Println("cancelled:", errors.Is(err, context.Canceled), "now", sim.Metadata().Time.Now.Format(time.RFC3339))
	// Output:
	// returned 6000 reports, sequences 101-6100, 2030-01-02T03:04:06Z to 2030-01-02T03:05:05Z
	// retained 1000 reports from sequence 5101
	// advance 1.5s: 100 reports, now 2030-01-02T03:05:06.5Z
	// advance 500ms: 100 reports, now 2030-01-02T03:05:07Z
	// over limit: true
	// cancelled: true now 2030-01-02T03:05:07Z
}

// Elapse scales real durations by the speed. Remainders carry to the next call,
// so the caller can pass whatever real time it measured.
func ExampleSimulator_Elapse() {
	ctx := context.Background()
	sim, err := simulation.New(simulation.Config{
		ID:                 "test-run",
		StartTime:          time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC),
		Seed:               42,
		InitialVesselCount: 1,
		Speed:              0.5,
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, step := range []struct {
		speed    float64
		realTime time.Duration
	}{
		{speed: 0.5, realTime: 2 * time.Second},
		{speed: 0.5, realTime: 300 * time.Millisecond},
		{speed: 0.5, realTime: 700 * time.Millisecond},
		{speed: 2, realTime: 250 * time.Millisecond},
	} {
		if err := sim.SetSpeed(step.speed); err != nil {
			fmt.Println(err)
			return
		}
		reports, err := sim.Elapse(ctx, step.realTime)
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Printf("elapse %v at %vx: %d reports, virtual elapsed %v\n", step.realTime, step.speed, len(reports), sim.Metadata().Time.Elapsed)
	}
	// Output:
	// elapse 2s at 0.5x: 1 reports, virtual elapsed 1s
	// elapse 300ms at 0.5x: 0 reports, virtual elapsed 1.15s
	// elapse 700ms at 0.5x: 0 reports, virtual elapsed 1.5s
	// elapse 250ms at 2x: 1 reports, virtual elapsed 2s
}

// Speed 0 pauses Elapse without catch-up after resuming, while Advance still
// steps explicitly.
func ExampleSimulator_SetSpeed() {
	ctx := context.Background()
	sim, err := simulation.New(simulation.Config{
		ID:                 "test-run",
		StartTime:          time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC),
		Seed:               42,
		InitialVesselCount: 1,
		Speed:              1,
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	show := func(step string, reports []simulation.Message, err error) bool {
		if err != nil {
			fmt.Println(err)
			return false
		}
		clock := sim.Metadata().Time
		fmt.Printf("%s: %d reports, now %s, paused %t\n", step, len(reports), clock.Now.Format(time.RFC3339), clock.Paused)
		return true
	}

	if err := sim.SetSpeed(0); err != nil {
		fmt.Println(err)
		return
	}
	reports, err := sim.Elapse(ctx, time.Hour)
	if !show("paused, elapse 1h", reports, err) {
		return
	}
	reports, err = sim.Advance(ctx, time.Second)
	if !show("paused, advance 1s", reports, err) {
		return
	}
	if err := sim.SetSpeed(1); err != nil {
		fmt.Println(err)
		return
	}
	reports, err = sim.Elapse(ctx, time.Second)
	show("resumed, elapse 1s", reports, err)
	// Output:
	// paused, elapse 1h: 0 reports, now 2030-01-02T03:04:05Z, paused true
	// paused, advance 1s: 1 reports, now 2030-01-02T03:04:06Z, paused true
	// resumed, elapse 1s: 1 reports, now 2030-01-02T03:04:07Z, paused false
}

// A complete run through explicit virtual steps, scaled real time, and pause.
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
		fmt.Println(err)
		return
	}
	show := func(step string, reports []simulation.Message, err error) bool {
		if err != nil {
			fmt.Println(err)
			return false
		}
		clock := sim.Metadata().Time
		fmt.Printf("%s: %d reports, now %s, speed %v\n", step, len(reports), clock.Now.Format(time.RFC3339), clock.Speed)
		return true
	}
	show("start", sim.History().Messages, nil)

	reports, err := sim.Advance(ctx, 5*time.Second)
	if !show("advance 5s", reports, err) {
		return
	}
	last := reports[len(reports)-1]
	fmt.Println(last.Sequence, last.MMSI, last.Timestamp.Format(time.RFC3339), strings.TrimSpace(last.Sentence))

	if err := sim.SetSpeed(0.5); err != nil {
		fmt.Println(err)
		return
	}
	reports, err = sim.Elapse(ctx, 2*time.Second)
	if !show("elapse 2s at 0.5x", reports, err) {
		return
	}

	if err := sim.SetSpeed(0); err != nil {
		fmt.Println(err)
		return
	}
	reports, err = sim.Elapse(ctx, time.Minute)
	if !show("elapse 1m paused", reports, err) {
		return
	}
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
