package simulation_test

import (
	"fmt"
	"strings"
	"time"

	"github.com/miroslav-matejovsky/ais-test-bench/simulation"
)

// This example uses only the public package and the standard library. It grows
// a reproducible fleet to two vessels, advances one tick, and prints every
// retained report.
func ExampleSimulator() {
	start := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	sim, err := simulation.New("test-run", start, 42)
	if err != nil {
		fmt.Println(err)
		return
	}
	if err := sim.SetCount(2, start); err != nil {
		fmt.Println(err)
		return
	}
	if err := sim.Advance(start.Add(simulation.TickInterval)); err != nil {
		fmt.Println(err)
		return
	}
	for _, message := range sim.History().Messages {
		// Sentences end with CRLF.
		fmt.Println(message.Sequence, message.MMSI, message.Timestamp.Format(time.RFC3339), strings.TrimSpace(message.Sentence))
	}
	// Output:
	// 1 200000000 2030-01-02T03:04:05Z !AIVDM,1,1,,A,12vg200P0w0B9jjMiLDPe0T:0000,0*1D
	// 2 200000001 2030-01-02T03:04:05Z !AIVDM,1,1,,A,12vg20@P1n0B@wjMhDVo9Uf:0000,0*3E
	// 3 200000000 2030-01-02T03:04:06Z !AIVDM,1,1,,A,12vg200P0w0B9k4MiLHhe0T<0000,0*70
	// 4 200000001 2030-01-02T03:04:06Z !AIVDM,1,1,,A,12vg20@P1n0B@wdMhDNo9Uf<0000,0*2E
}
