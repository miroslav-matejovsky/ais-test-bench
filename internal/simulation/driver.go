package simulation

import (
	"context"
	"crypto/rand"
	"errors"
	mathrand "math/rand/v2"
	"sync/atomic"
	"time"

	"github.com/miroslav-matejovsky/ais-test-bench/simulation"
)

// elapseChunk bounds one Elapse call so its scaled duration stays within
// simulation.MaxAdvance even at simulation.MaxSpeed.
const elapseChunk = simulation.MaxAdvance / time.Duration(simulation.MaxSpeed)

// Driver paces one public simulation engine with elapsed wall-clock time. It
// holds no simulation state; all generation, history, and metadata come from the
// engine.
type Driver struct {
	sim     *simulation.Simulator
	started atomic.Bool
}

// NewConfig returns the application's engine configuration: a fresh random
// identity and seed, so runs are distinguishable, the current real instant as
// the virtual start, one vessel, and real-time speed.
func NewConfig() simulation.Config {
	return simulation.Config{
		ID: rand.Text(), StartTime: time.Now(), Seed: mathrand.Uint64(),
		InitialVesselCount: 1, Speed: 1,
	}
}

// NewDriver returns a driver for sim. The driver must be the only caller of
// sim's mutating methods.
func NewDriver(sim *simulation.Simulator) *Driver {
	return &Driver{sim: sim}
}

// Run wakes every simulation.TickInterval, measures the real time elapsed since
// the previous wake, and passes all of it to the engine, so late or coalesced
// wakes lose no virtual time. It stops on cancellation or an engine error. It
// may be called once per driver; a second call returns an error.
func (d *Driver) Run(ctx context.Context) error {
	if !d.started.CompareAndSwap(false, true) {
		return errors.New("simulation driver already started")
	}
	ticker := time.NewTicker(simulation.TickInterval)
	defer ticker.Stop()
	last := time.Now()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			now := time.Now()
			elapsed := now.Sub(last)
			last = now
			for elapsed > 0 {
				chunk := min(elapsed, elapseChunk)
				if _, err := d.sim.Elapse(ctx, chunk); err != nil {
					if ctx.Err() != nil {
						return nil
					}
					return err
				}
				elapsed -= chunk
			}
		}
	}
}

// SetCount sets the active fleet size at the engine's committed virtual time,
// which trails real time by at most one tick. See simulation.Simulator.SetCount.
func (d *Driver) SetCount(count int) error {
	_, err := d.sim.SetCount(count)
	return err
}

// Fleet returns a copy of the active fleet.
func (d *Driver) Fleet() simulation.Fleet {
	return d.sim.Fleet()
}

// History returns a copy of the retained reports.
func (d *Driver) History() simulation.History {
	return d.sim.History()
}

// Metadata returns the run identity, clock, catalogs, and effective settings.
func (d *Driver) Metadata() simulation.Metadata {
	return d.sim.Metadata()
}
