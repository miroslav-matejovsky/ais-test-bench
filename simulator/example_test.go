package simulator_test

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/miroslav-matejovsky/ais-testbench/display"
	"github.com/miroslav-matejovsky/ais-testbench/simulation"
	"github.com/miroslav-matejovsky/ais-testbench/simulator"
)

func Example() {
	sim, err := simulator.New(simulator.Config{Simulation: simulation.Config{
		ID: "embedded", StartTime: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
		Seed: 42, InitialVesselCount: 1, Speed: 0,
		Transmitter: simulation.TransmitterProfile{PowerWatts: 12.5, HeightMeters: 10, GainDBi: 2, FeederLossDB: 1},
	}})
	if err != nil {
		panic(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/api/", sim.API())
	// A host may serve mux with its own server and middleware. This local
	// display client needs no HTTP listener and never reads the truth fleet.
	client, err := display.New(sim)
	if err != nil {
		panic(err)
	}
	observations, err := client.Observations(context.Background(), nil)
	if err != nil {
		panic(err)
	}
	fmt.Println(observations.SimulationID, len(observations.Targets))
	// In a running application supervise Run alongside the host server; drain
	// requests before canceling and joining Run. Here it is canceled immediately.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sim.Run(ctx); err != nil {
		panic(err)
	}
	// Output: embedded 0
}
